package spec

import (
	"fmt"
	"math"

	"github.com/burgrp/bleriot/lib/shared/conversion"
	"github.com/burgrp/bleriot/lib/shared/conversion/ntc"
	"github.com/burgrp/bleriot/lib/shared/inventory"
	"github.com/burgrp/bleriot/lib/shared/puya"
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
	ntcFaultLowThreshold     = int32(64)
	ntcFaultHighThreshold    = int32(adcMax) - ntcFaultLowThreshold
	adcReferenceVoltage      = 3.3
	pressureDividerTopKOhm   = 4.02
	pressureDividerLowerKOhm = 6.49
)

var Chip = puya.PY32F003x6

func Type(mode Mode) inventory.DeviceType {
	deviceType := inventory.DeviceType{
		Name: "bleriot-sense-" + mode.String(),
		Chip: Chip,
	}

	switch mode {
	case ModeNTC:
		deviceType.Registers = []inventory.Register{
			{
				Tag:        RegSample,
				Name:       "temperature",
				Type:       inventory.TypeFloat,
				ReadOnly:   true,
				Conversion: ntcTemperatureConversion(),
				Metadata:   map[string]string{"mode": "ntc", "unit": "degC"},
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
		panic(fmt.Sprintf("bleriot-sense: unsupported sensor mode %d", mode))
	}

	return deviceType
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
