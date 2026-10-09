//go:build !tinygo

package spec

import (
	"fmt"
	"math"
	"strconv"

	"github.com/burgrp/bleriot/lib/shared/conversion"
	"github.com/burgrp/bleriot/lib/shared/conversion/ntc"
	"github.com/burgrp/bleriot/lib/shared/firmware"
	"github.com/burgrp/bleriot/lib/shared/inventory"
	"github.com/burgrp/bleriot/lib/shared/puya"
	"github.com/burgrp/tinygo-drivers/onewire"
)

const (
	adcMax                   = 4095
	ntcFaultLowThreshold     = int32(64)
	ntcFaultHighThreshold    = int32(adcMax) - ntcFaultLowThreshold
	adcReferenceVoltage      = 3.3
	pressureDividerTopKOhm   = 4.02
	pressureDividerLowerKOhm = 6.49
	ds18b20MinimumRaw        = int32(-55 * 16)
	ds18b20MaximumRaw        = int32(125 * 16)
)

var Chip = puya.PY32F003x6

func (mode Mode) String() string {
	switch mode {
	case ModeNTC:
		return "ntc"
	case ModeFlow:
		return "flow"
	case ModePressure:
		return "pressure"
	case ModeDS18B20:
		return "ds18b20"
	default:
		return fmt.Sprintf("unknown-%d", mode)
	}
}

func Type(mode Mode) inventory.DeviceType {
	return TypeForConfig(Config{Mode: mode})
}

// TypeForConfig returns the register table selected by config. It panics when
// config is invalid because inventories must be valid at construction time.
func TypeForConfig(config Config) inventory.DeviceType {
	if err := config.Validate(); err != nil {
		panic("bleriot-sense: " + err.Error())
	}
	mode := config.Mode
	deviceType := inventory.DeviceType{
		Name: "bleriot-sense-" + mode.String(),
		Chip: Chip,
		Firmware: firmware.Manifest{
			Package: "github.com/burgrp/bleriot-sense/fw/v2",
			TinyGo: firmware.TinyGoProfile{
				Scheduler:        firmware.SchedulerNone,
				StackSizeBytes:   1024,
				GarbageCollector: firmware.GCLeaking,
				Serial:           firmware.SerialRTT,
				SizeReport:       firmware.SizeReportHTML,
				PrintAllocs:      true,
			},
			PyOCD: firmware.PyOCDProfile{
				Reclaim:                  true,
				ReclaimDelayMilliseconds: 1000,
			},
		},
	}

	switch mode {
	case ModeNTC:
		deviceType.Registers = []inventory.Register{{
			Tag:        RegSample,
			Name:       "temperature",
			Type:       inventory.TypeFloat,
			ReadOnly:   true,
			Conversion: ntcTemperatureConversion(),
			Metadata:   map[string]string{"mode": "ntc", "unit": "°C"},
		}}
	case ModeFlow:
		deviceType.Registers = []inventory.Register{
			{
				Tag:        RegSample,
				Name:       "frequency",
				Type:       inventory.TypeFloat,
				ReadOnly:   true,
				Conversion: readOnlyScale(0.001),
				Metadata:   map[string]string{"mode": "flow", "unit": "Hz"},
			},
			{
				Tag:      RegPulseCount,
				Name:     "pulses",
				Type:     inventory.TypeInt,
				ReadOnly: true,
				Metadata: map[string]string{"mode": "flow", "unit": "count"},
			},
		}
	case ModePressure:
		inputVoltagePerCode := adcReferenceVoltage / adcMax *
			(pressureDividerTopKOhm + pressureDividerLowerKOhm) / pressureDividerLowerKOhm
		deviceType.Registers = []inventory.Register{{
			Tag:        RegSample,
			Name:       "voltage",
			Type:       inventory.TypeFloat,
			ReadOnly:   true,
			Conversion: readOnlyScale(inputVoltagePerCode),
			Metadata:   map[string]string{"mode": "pressure", "unit": "V"},
		}}
	case ModeDS18B20:
		count := len(config.DS18B20Sensors)
		if count == 0 {
			count = 1
		}
		deviceType.Registers = make([]inventory.Register, count)
		for index := range count {
			name := "temperature"
			if count > 1 {
				name += "." + strconv.Itoa(index+1)
			}
			metadata := map[string]string{"mode": "ds18b20", "unit": "°C"}
			if index < len(config.DS18B20Sensors) && !zeroDS18B20ID(config.DS18B20Sensors[index].ID) {
				metadata["id"] = fmt.Sprintf("%x", config.DS18B20Sensors[index].ID)
			}
			deviceType.Registers[index] = inventory.Register{
				Tag:        DS18B20RegisterTag(index),
				Name:       name,
				Type:       inventory.TypeFloat,
				ReadOnly:   true,
				Conversion: ds18b20TemperatureConversion(),
				Metadata:   metadata,
			}
		}
	default:
		panic(fmt.Sprintf("bleriot-sense: unsupported sensor mode %d", mode))
	}

	return deviceType
}

// Validate checks mode-specific configuration constraints.
func (config Config) Validate() error {
	if config.Mode != ModeDS18B20 {
		if len(config.DS18B20Sensors) != 0 {
			return fmt.Errorf("DS18B20 sensors require DS18B20 mode")
		}
		return nil
	}
	if len(config.DS18B20Sensors) > MaxDS18B20Sensors {
		return fmt.Errorf("at most %d DS18B20 sensors are supported", MaxDS18B20Sensors)
	}
	for index, sensor := range config.DS18B20Sensors {
		if zeroDS18B20ID(sensor.ID) {
			if len(config.DS18B20Sensors) > 1 {
				return fmt.Errorf("DS18B20 sensor %d requires an ID in multidrop mode", index+1)
			}
			continue
		}
		if sensor.ID[0] != 0x28 {
			return fmt.Errorf("DS18B20 sensor %d has family %#02x, want 0x28", index+1, sensor.ID[0])
		}
		if onewire.CRC8(sensor.ID[:7]) != sensor.ID[7] {
			return fmt.Errorf("DS18B20 sensor %d has invalid ID CRC", index+1)
		}
		for previous := range index {
			if config.DS18B20Sensors[previous].ID == sensor.ID {
				return fmt.Errorf("DS18B20 sensors %d and %d have duplicate IDs", previous+1, index+1)
			}
		}
	}
	return nil
}

func zeroDS18B20ID(id [8]byte) bool {
	return id == [8]byte{}
}

func readOnlyScale(factor float64) inventory.Conversion {
	result := conversion.Scale(factor)
	result.Encode = nil
	return result
}

func ntcTemperatureConversion() inventory.Conversion {
	result := ntc.Beta(ntc.BetaParams{
		ADCMax:              adcMax,
		FixedResistance:     10000,
		NominalResistance:   10000,
		NominalTemperatureC: 25,
		Beta:                3950,
		Position:            ntc.ThermistorLowSide,
	})
	decode := result.Decode
	result.Decode = func(raw int32) (any, error) {
		if raw <= ntcFaultLowThreshold || raw >= ntcFaultHighThreshold {
			return nil, nil
		}
		value, err := decode(raw)
		if err != nil {
			return nil, err
		}
		temperature := value.(float64)
		return math.Round(temperature*100) / 100, nil
	}
	return result
}

func ds18b20TemperatureConversion() inventory.Conversion {
	result := readOnlyScale(1.0 / 16)
	decode := result.Decode
	result.Decode = func(raw int32) (any, error) {
		if raw < ds18b20MinimumRaw || raw > ds18b20MaximumRaw {
			return nil, nil
		}
		return decode(raw)
	}
	return result
}
