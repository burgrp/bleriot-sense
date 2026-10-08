# BleRiot Sense firmware

This module is the importable BleRiot Sense firmware and local inventory for the
`PY32F003L16S6TU` hardware in `board`.

External hubs import the versioned device specification as:

```go
import sense "github.com/burgrp/bleriot-sense/fw/spec"
```

- MCU: `PY32F003L16S6TU` (32 KiB flash, 4 KiB RAM).
- TinyGo and pyOCD target: `py32f003x6`.
- Sensor input: `PA0`.
- PAN2110 SCK: `PA1`.
- PAN2110 DATA: `PA2`.
- PAN2110 CSN: `PB0`.

## Sensor mode

Set `sensorMode` in `cmd/dev/main.go` to match the assembled input network. The
device type needs only that mode; timing and averaging remain inline in the
firmware configuration:

```go
const sensorMode = spec.ModeNTC

Type: spec.Type(sensorMode),
Config: spec.Config{
	Mode:                       sensorMode,
	SampleIntervalMilliseconds: 1000,
	ADCSamples:                 16,
},
```

Supported values are:

| Mode | Assembly population | Registry values |
|---|---|---|
| `spec.ModeNTC` | AR1 10 kΩ, AR2 4.02 kΩ, AR3 DNP, AC1 100 nF | `temperature` in °C |
| `spec.ModeFlow` | Driven: AR1 DNP, AR2 4.02 kΩ, AR3 6.49 kΩ, AC1 DNP; open collector: AR1 10 kΩ, AR2 4.02 kΩ, AR3/AC1 DNP | `frequency` in Hz and cumulative `pulses` |
| `spec.ModePressure` | AR1 DNP, AR2 4.02 kΩ, AR3 6.49 kΩ, AC1 100 nF | Sensor-output `voltage` in V |
| `spec.ModeDS18B20` | AR1 4.7 kΩ, AR2 100 Ω, AR3/AC1 DNP | `temperature` in °C |

The mode is inventory-as-code and is baked into the image by `node build`. It
does not electronically change the assembly population at runtime. Rebuild and
flash the node after changing it.

NTC and pressure calculations run in the hub; the node transmits averaged raw
12-bit ADC codes. Flow frequency is transmitted in millihertz and converted to
hertz by the hub. A sensor-specific pressure transfer function or flow K-factor
can be added to the host conversion once the exact sensor calibration is known.

DS18B20 mode supports one externally powered device. Connect J2 pin 1 to GND,
pin 2 to DQ, and pin 3 to VDD. DQ is pulled up to 3.3 V; do not connect a 5 V DQ
pull-up. Parasite power and multiple devices on the bus are not supported. The
node validates ROM and scratchpad CRCs, transmits signed Q12.4 temperature, and
continues polling the radio during the sensor's conversion interval.

The protocol implementation comes from `github.com/burgrp/tinygo-drivers/onewire`.
This board supplies only the PA0 open-drain, TIM3 1 MHz counter, and interrupt
adapter in `onewire_py32.go`.

The node retains the latest full-resolution sample for scheduled GET responses.
In flow mode, the pulse interrupt only increments an in-memory counter; each
sample interval updates both the frequency and cumulative `pulses` register.

In NTC mode, the hub conversion publishes `temperature = null` for raw ADC
codes at or below 64 (shorted probe or cable) and at or above 4031
(disconnected probe or cable). The node continues to report averaged raw ADC
codes. These thresholds leave ample margin around the documented −20…100 °C
range. Pressure mode does not apply the NTC wiring-fault thresholds.

## Build and run

From this directory:

```sh
go test ./...
go run ./cmd/dev node gen --name sense
go run ./cmd/dev node build --name sense --disassembly
go run ./cmd/dev node build --name sense --flash --rtt
```

Run the hub against a Registry server with:

```sh
go run ./cmd/dev hub --registry http://localhost:8080 --diagnostics rf
```

The build command creates a private module under `.bleriot/firmware/sense`,
generates the node entry point, and calls `sense.Run` with the RF identity and
baked `spec.Config`.

The upstream TinyGo `py32f003x6` target reserves a 1 KiB system stack. Firmware
uses `--scheduler none`; sensor acquisition shares the nonblocking node loop so
no heap-backed goroutine stacks are required.

## First boot

On the eight-pin package, the PAN2110 CSN pad is shared with `PF2-NRST`. On first
boot the firmware preserves all option bytes except `NRST_MODE`, changes that
one option to GPIO, and requests one option-byte reload reset. Later boots do
not write the option bytes. SWD remains available on the separate SWDIO and
SWCLK pads.