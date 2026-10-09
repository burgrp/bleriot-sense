package sense

import (
	"testing"

	"github.com/burgrp/bleriot-sense/fw/v2/spec"
)

func TestAnalogSampleValid(t *testing.T) {
	for _, mode := range []spec.Mode{spec.ModeNTC, spec.ModePressure} {
		for _, sample := range []int32{0, analogFaultLowThreshold, analogFaultHighThreshold, 4095} {
			if analogSampleValid(mode, sample) {
				t.Errorf("analogSampleValid(%s, %d) = true, want false", mode, sample)
			}
		}
		for _, sample := range []int32{analogFaultLowThreshold + 1, 2048, analogFaultHighThreshold - 1} {
			if !analogSampleValid(mode, sample) {
				t.Errorf("analogSampleValid(%s, %d) = false, want true", mode, sample)
			}
		}
	}
}

func TestAnalogSampleValidIgnoresDigitalModes(t *testing.T) {
	for _, mode := range []spec.Mode{spec.ModeFlow, spec.ModeDS18B20} {
		if !analogSampleValid(mode, 0) {
			t.Errorf("analogSampleValid(%s, 0) = false, want true", mode)
		}
	}
}

func TestPressureDocumentedRangeIsValid(t *testing.T) {
	for _, sample := range []int32{383, 3831, 4023} {
		if !analogSampleValid(spec.ModePressure, sample) {
			t.Errorf("analogSampleValid(pressure, %d) = false, want true", sample)
		}
	}
}
