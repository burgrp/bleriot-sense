package main

import (
	"math"
	"testing"
)

func TestSampleChanged(t *testing.T) {
	tests := []struct {
		name         string
		previous     int32
		previousNull bool
		current      int32
		currentNull  bool
		hysteresis   uint32
		want         bool
	}{
		{name: "same value", previous: 100, current: 100, hysteresis: 4},
		{name: "zero preserves any change", previous: 100, current: 101, want: true},
		{name: "one preserves any change", previous: 100, current: 101, hysteresis: 1, want: true},
		{name: "positive below threshold", previous: 100, current: 103, hysteresis: 4},
		{name: "positive at threshold", previous: 100, current: 104, hysteresis: 4, want: true},
		{name: "negative below threshold", previous: 100, current: 97, hysteresis: 4},
		{name: "negative at threshold", previous: 100, current: 96, hysteresis: 4, want: true},
		{name: "full int32 range", previous: math.MinInt32, current: math.MaxInt32, hysteresis: math.MaxUint32, want: true},
		{name: "null remains null", previousNull: true, currentNull: true, hysteresis: 4},
		{name: "null becomes value", previousNull: true, current: 100, hysteresis: 4, want: true},
		{name: "value becomes null", previous: 100, currentNull: true, hysteresis: 4, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := sampleChanged(test.previous, test.previousNull, test.current, test.currentNull, test.hysteresis)
			if got != test.want {
				t.Fatalf("sampleChanged() = %v, want %v", got, test.want)
			}
		})
	}
}
