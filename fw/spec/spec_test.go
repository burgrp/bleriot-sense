//go:build !tinygo

package spec

import (
	"fmt"
	"math"
	"testing"

	"github.com/burgrp/bleriot/lib/shared/firmware"
	"github.com/burgrp/tinygo-drivers/onewire"
)

func TestTypesValidate(t *testing.T) {
	for _, mode := range []Mode{ModeNTC, ModeFlow, ModePressure, ModeDS18B20} {
		t.Run(mode.String(), func(t *testing.T) {
			if err := Type(mode).Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFirmwareProfile(t *testing.T) {
	for _, mode := range []Mode{ModeNTC, ModeFlow, ModePressure, ModeDS18B20} {
		profile := Type(mode).Firmware
		if profile.Package != "github.com/burgrp/bleriot-sense/fw/v2" {
			t.Fatalf("%s firmware package = %q", mode, profile.Package)
		}
		if profile.TinyGo.Scheduler != firmware.SchedulerNone || profile.TinyGo.StackSizeBytes != 1024 {
			t.Fatalf("%s TinyGo profile = %+v", mode, profile.TinyGo)
		}
		if !profile.PyOCD.Reclaim || profile.PyOCD.ReclaimDelayMilliseconds != 1000 {
			t.Fatalf("%s pyOCD profile = %+v", mode, profile.PyOCD)
		}
	}
}

func TestPressureConversion(t *testing.T) {
	value, err := Type(ModePressure).Registers[0].Conversion.Decode(3831)
	if err != nil {
		t.Fatal(err)
	}
	voltage := value.(float64)
	if voltage < 4.99 || voltage > 5.01 {
		t.Fatalf("pressure conversion = %v V, want about 5 V", voltage)
	}
}

func TestFlowConversion(t *testing.T) {
	value, err := Type(ModeFlow).Registers[0].Conversion.Decode(1000)
	if err != nil {
		t.Fatal(err)
	}
	if frequency := value.(float64); frequency != 1 {
		t.Fatalf("flow conversion = %v Hz, want 1 Hz", frequency)
	}
}

func TestNTCConversion(t *testing.T) {
	value, err := Type(ModeNTC).Registers[0].Conversion.Decode(2048)
	if err != nil {
		t.Fatal(err)
	}
	temperature := value.(float64)
	if temperature != 24.99 {
		t.Fatalf("NTC conversion = %v degC, want 24.99 degC", temperature)
	}
}

func TestNTCFaultConversion(t *testing.T) {
	decode := Type(ModeNTC).Registers[0].Conversion.Decode
	for _, raw := range []int32{0, ntcFaultLowThreshold, ntcFaultHighThreshold, adcMax} {
		value, err := decode(raw)
		if err != nil {
			t.Fatalf("Decode(%d): %v", raw, err)
		}
		if value != nil {
			t.Errorf("Decode(%d) = %v, want nil", raw, value)
		}
	}
	for _, raw := range []int32{ntcFaultLowThreshold + 1, ntcFaultHighThreshold - 1} {
		value, err := decode(raw)
		if err != nil {
			t.Fatalf("Decode(%d): %v", raw, err)
		}
		if value == nil {
			t.Errorf("Decode(%d) = nil, want temperature", raw)
		}
	}
}

func TestDS18B20Conversion(t *testing.T) {
	decode := Type(ModeDS18B20).Registers[0].Conversion.Decode
	for _, test := range []struct {
		raw  int32
		want float64
	}{
		{raw: 0x0191, want: 25.0625},
		{raw: -162, want: -10.125},
	} {
		value, err := decode(test.raw)
		if err != nil {
			t.Fatalf("Decode(%d): %v", test.raw, err)
		}
		if temperature := value.(float64); temperature != test.want {
			t.Errorf("Decode(%d) = %v degC, want %v degC", test.raw, temperature, test.want)
		}
	}
}

func TestDS18B20OffsetConversion(t *testing.T) {
	config := Config{Mode: ModeDS18B20, DS18B20Sensors: []DS18B20Sensor{{ID: validDS18B20ID(1)}}}
	deviceType := TypeForConfig(config, Calibration{DS18B20: map[int]float64{1: 1}})
	value, err := deviceType.Registers[0].Conversion.Decode(0x0140)
	if err != nil {
		t.Fatal(err)
	}
	if temperature := value.(float64); temperature != 21 {
		t.Fatalf("offset temperature = %v °C, want 21 °C", temperature)
	}
	if got := deviceType.Registers[0].Metadata["offsetCelsius"]; got != "1" {
		t.Fatalf("offset metadata = %q, want 1", got)
	}
}

func TestDS18B20FaultConversion(t *testing.T) {
	decode := Type(ModeDS18B20).Registers[0].Conversion.Decode
	for _, raw := range []int32{ds18b20MinimumRaw - 1, ds18b20MaximumRaw + 1} {
		value, err := decode(raw)
		if err != nil {
			t.Fatalf("Decode(%d): %v", raw, err)
		}
		if value != nil {
			t.Errorf("Decode(%d) = %v, want nil", raw, value)
		}
	}
}

func TestDS18B20ConfiguredType(t *testing.T) {
	config := Config{
		Mode: ModeDS18B20,
		DS18B20Sensors: []DS18B20Sensor{
			{ID: validDS18B20ID(1)},
			{ID: validDS18B20ID(2)},
			{ID: validDS18B20ID(3)},
		},
	}
	deviceType := TypeForConfig(config, Calibration{})
	if err := deviceType.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(deviceType.Registers) != 3 {
		t.Fatalf("register count = %d, want 3", len(deviceType.Registers))
	}
	for index, register := range deviceType.Registers {
		wantName := fmt.Sprintf("temperature.%d", index+1)
		if register.Tag != DS18B20RegisterTag(index) || register.Name != wantName {
			t.Errorf("register %d = tag %d name %q, want tag %d name %q", index, register.Tag, register.Name, DS18B20RegisterTag(index), wantName)
		}
		if register.Metadata["id"] == "" {
			t.Errorf("register %d has no sensor ID metadata", index)
		}
	}
}

func TestDS18B20SingleSensorIDOptional(t *testing.T) {
	for _, sensors := range [][]DS18B20Sensor{nil, {{}}} {
		config := Config{Mode: ModeDS18B20, DS18B20Sensors: sensors}
		if err := config.Validate(); err != nil {
			t.Fatalf("Validate(): %v", err)
		}
		if registers := TypeForConfig(config, Calibration{}).Registers; len(registers) != 1 || registers[0].Tag != RegDS18B20Base || registers[0].Name != "temperature" {
			t.Fatalf("registers = %+v, want one tag-%d register named temperature", registers, RegDS18B20Base)
		}
	}
}

func TestDS18B20ConfigValidation(t *testing.T) {
	valid := validDS18B20ID(1)
	badCRC := valid
	badCRC[7] ^= 1
	wrongFamily := valid
	wrongFamily[0] = 0x10
	wrongFamily[7] = onewire.CRC8(wrongFamily[:7])

	tests := []struct {
		name   string
		config Config
	}{
		{name: "too many", config: Config{Mode: ModeDS18B20, DS18B20Sensors: make([]DS18B20Sensor, MaxDS18B20Sensors+1)}},
		{name: "missing multidrop ID", config: Config{Mode: ModeDS18B20, DS18B20Sensors: []DS18B20Sensor{{ID: valid}, {}}}},
		{name: "bad CRC", config: Config{Mode: ModeDS18B20, DS18B20Sensors: []DS18B20Sensor{{ID: badCRC}}}},
		{name: "wrong family", config: Config{Mode: ModeDS18B20, DS18B20Sensors: []DS18B20Sensor{{ID: wrongFamily}}}},
		{name: "duplicate", config: Config{Mode: ModeDS18B20, DS18B20Sensors: []DS18B20Sensor{{ID: valid}, {ID: valid}}}},
		{name: "wrong mode", config: Config{Mode: ModeNTC, DS18B20Sensors: []DS18B20Sensor{{ID: valid}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.config.Validate(); err == nil {
				t.Fatal("Validate() succeeded, want error")
			}
		})
	}
}

func TestDS18B20CalibrationValidation(t *testing.T) {
	config := Config{
		Mode: ModeDS18B20,
		DS18B20Sensors: []DS18B20Sensor{
			{ID: validDS18B20ID(1)},
			{ID: validDS18B20ID(2)},
		},
	}
	for _, calibration := range []Calibration{
		{DS18B20: map[int]float64{0: 1}},
		{DS18B20: map[int]float64{3: 1}},
		{DS18B20: map[int]float64{2: math.NaN()}},
		{DS18B20: map[int]float64{2: math.Inf(1)}},
	} {
		if err := validateCalibration(config, calibration); err == nil {
			t.Errorf("validateCalibration(%+v) succeeded, want error", calibration)
		}
	}
	if err := validateCalibration(Config{Mode: ModeNTC}, Calibration{DS18B20: map[int]float64{1: 1}}); err == nil {
		t.Error("NTC calibration validation succeeded, want error")
	}
}

func TestDS18B20RegisterTags(t *testing.T) {
	seen := map[uint16]bool{RegSample: true, RegPulseCount: true}
	for index := range MaxDS18B20Sensors {
		tag := DS18B20RegisterTag(index)
		want := RegDS18B20Base + uint16(index)
		if tag != want {
			t.Errorf("DS18B20RegisterTag(%d) = %d, want %d", index, tag, want)
		}
		if seen[tag] {
			t.Fatalf("duplicate register tag %d", tag)
		}
		seen[tag] = true
	}
}

func validDS18B20ID(serial byte) [8]byte {
	id := [8]byte{0x28, serial, 2, 3, 4, 5, 6}
	id[7] = onewire.CRC8(id[:7])
	return id
}
