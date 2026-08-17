package spec

import (
	"fmt"

	"github.com/burgrp/bleriot/lib/shared/conversion"
	"github.com/burgrp/bleriot/lib/shared/conversion/ntc"
	"github.com/burgrp/bleriot/lib/shared/inventory"
)

type Mode uint8

const (
	ModeNTC Mode = iota + 1
	ModeFlow
	ModePressure
)

func (mode Mode) String() string {
	switch mode {
	case ModeNTC:
		return "ntc"
	case ModeFlow:
		return "flow"
	case ModePressure:
		return "pressure"
	default:
		return fmt.Sprintf("unknown-%d", mode)
	}
}

type Config struct {
	Mode                       Mode
	SampleIntervalMilliseconds uint32
	ADCSamples                 uint8
}

const (
	RegSample     = 1
	RegPulseCount = 2

	adcMax                   = 4095
	adcReferenceVoltage      = 3.3
	pressureDividerTopKOhm   = 4.02
	pressureDividerLowerKOhm = 6.49
)

var Chip = inventory.Chip{
	Name:         "py32f003x6-sense",
	TinygoTarget: "./py32f003x6-sense.json",
	PyocdTarget:  "py32f003x6",
	CmsisPack:    "PY32F003",
}

func Type(config Config) inventory.DeviceType {
	deviceType := inventory.DeviceType{
		Name: "bleriot-sense-" + config.Mode.String(),
		Chip: Chip,
	}

	switch config.Mode {
	case ModeNTC:
		deviceType.Registers = []inventory.Register{
			{
				Tag:      RegSample,
				Name:     "temperature",
				Type:     inventory.TypeFloat,
				ReadOnly: true,
				Conversion: ntc.Beta(ntc.BetaParams{
					ADCMax:              adcMax,
					FixedResistance:     10000,
					NominalResistance:   10000,
					NominalTemperatureC: 25,
					Beta:                3950,
					Position:            ntc.ThermistorLowSide,
				}),
				Metadata: map[string]string{"mode": "ntc", "unit": "degC"},
			},
		}
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
		deviceType.Registers = []inventory.Register{
			{
				Tag:        RegSample,
				Name:       "voltage",
				Type:       inventory.TypeFloat,
				ReadOnly:   true,
				Conversion: readOnlyScale(inputVoltagePerCode),
				Metadata:   map[string]string{"mode": "pressure", "unit": "V"},
			},
		}
	default:
		panic(fmt.Sprintf("bleriot-sense: unsupported sensor mode %d", config.Mode))
	}

	return deviceType
}

func readOnlyScale(factor float64) inventory.Conversion {
	result := conversion.Scale(factor)
	result.Encode = nil
	return result
}
