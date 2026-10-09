package spec

type Mode uint8

const (
	ModeNTC Mode = iota + 1
	ModeFlow
	ModePressure
	ModeDS18B20
)

type Config struct {
	Mode                       Mode
	SampleIntervalMilliseconds uint32
	ADCSamples                 uint8
	DS18B20Sensors             []DS18B20Sensor
}

// DS18B20Sensor maps one temperature register to a physical sensor ID. ID may
// be zero only when exactly one DS18B20 register is configured.
type DS18B20Sensor struct {
	ID [8]byte
}

const (
	RegSample                = 1
	RegPulseCount            = 2
	RegDS18B20Base    uint16 = 0x0100
	MaxDS18B20Sensors        = 20
)

// DS18B20RegisterTag returns the permanent register tag for a sensor index.
func DS18B20RegisterTag(index int) uint16 {
	if index < 0 || index >= MaxDS18B20Sensors {
		panic("bleriot-sense: DS18B20 sensor index out of range")
	}
	return RegDS18B20Base + uint16(index)
}
