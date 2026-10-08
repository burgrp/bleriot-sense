package spec

import (
	"testing"

	"github.com/burgrp/bleriot/lib/shared/firmware"
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
		if profile.Package != "github.com/burgrp/bleriot-sense/fw" {
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
