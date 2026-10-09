package sense

import "github.com/burgrp/bleriot-sense/fw/v2/spec"

const (
	analogFaultLowThreshold  = int32(64)
	analogFaultHighThreshold = int32(4095) - analogFaultLowThreshold
)

func analogSampleValid(mode spec.Mode, sample int32) bool {
	switch mode {
	case spec.ModeNTC, spec.ModePressure:
		return sample > analogFaultLowThreshold && sample < analogFaultHighThreshold
	default:
		return true
	}
}
