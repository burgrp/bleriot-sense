package spec

import "testing"

func TestTypesValidate(t *testing.T) {
	for _, mode := range []Mode{ModeNTC, ModeFlow, ModePressure} {
		t.Run(mode.String(), func(t *testing.T) {
			if err := Type(mode).Validate(); err != nil {
				t.Fatal(err)
			}
		})
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
	if temperature < 24.9 || temperature > 25.1 {
		t.Fatalf("NTC conversion = %v degC, want about 25 degC", temperature)
	}
}
