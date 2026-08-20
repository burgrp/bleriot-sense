# BleRiot Sense firmware

This module is the BleRiot node and hub inventory for the `board-003` hardware
variant. The sibling `board-030` project uses a PY32F030 and requires separate
firmware.

External hubs import the versioned device specification as:

```go
import sense "github.com/burgrp/bleriot-sense/fw-003/spec"
```

- MCU: `PY32F003L16S6TU` (32 KiB flash, 4 KiB RAM).
- TinyGo and pyOCD target: `py32f003x6`.
- Sensor input: `PA0`.
- PAN2110 SCK: `PA1`.
- PAN2110 DATA: `PA2`.
- PAN2110 CSN: `PB0`.

## Sensor mode

Set `sensorMode` in `test-hub.go` to match the assembled input network. The
device type needs only that mode; timing and averaging remain inline in the
firmware configuration:

```go
const sensorMode = spec.ModeNTC

Type: spec.Type(sensorMode),
Config: spec.Config{
	Mode:                       sensorMode,
	SampleIntervalMilliseconds: 1000,
	ADCSamples:                 16,
	SampleHysteresis:           4,
},
```

Supported values are:

| Mode | Assembly population | Registry values |
|---|---|---|
| `spec.ModeNTC` | AR1 10 kΩ, AR2 4.02 kΩ, AR3 DNP, AC1 100 nF | `temperature` in °C |
| `spec.ModeFlow` | Driven: AR1 DNP, AR2 4.02 kΩ, AR3 6.49 kΩ, AC1 DNP; open collector: AR1 10 kΩ, AR2 4.02 kΩ, AR3/AC1 DNP | `frequency` in Hz and cumulative `pulses` |
| `spec.ModePressure` | AR1 DNP, AR2 4.02 kΩ, AR3 6.49 kΩ, AC1 100 nF | Sensor-output `voltage` in V |

The mode is inventory-as-code and is baked into the image by `bleriot make`. It
does not electronically change the assembly population at runtime. Rebuild and
flash the node after changing it.

NTC and pressure calculations run in the hub; the node transmits averaged raw
12-bit ADC codes. Flow frequency is transmitted in millihertz and converted to
hertz by the hub. A sensor-specific pressure transfer function or flow K-factor
can be added to the host conversion once the exact sensor calibration is known.

`SampleHysteresis` reduces RF traffic by comparing each sample with the last
notified sample. A notification is sent only when the absolute difference is at
least the configured threshold. `0` and `1` preserve notification on every raw
change. The threshold uses node wire units: ADC counts in NTC and pressure
modes, and millihertz in flow mode. The example value of four ADC counts is
approximately 0.09 °C near 25 °C for the specified thermistor. GET responses
retain the latest full-resolution sample.

In flow mode, the pulse interrupt only increments an in-memory counter. The
cumulative `pulses` register is sampled and can notify at most once per sample
interval while pulses are arriving; it does not send one notification per
physical pulse. `SampleHysteresis` applies to `frequency`, not to `pulses`.

## Build and run

From this directory:

```sh
go test ./...
go run . make sense build
go run . make sense flash
```

Run the hub against a Registry server with:

```sh
go run . hub --registry http://localhost:8080 --diagnostics rf
```

The build command generates the ignored `main_gen.go` containing the node's RF
identity and baked `spec.Config`.

The upstream TinyGo `py32f003x6` target reserves a 1 KiB system stack. Firmware
uses `--scheduler none`; sensor acquisition shares the nonblocking node loop so
no heap-backed goroutine stacks are required.

## First boot

On the eight-pin package, the PAN2110 CSN pad is shared with `PF2-NRST`. On first
boot the firmware preserves all option bytes except `NRST_MODE`, changes that
one option to GPIO, and requests one option-byte reload reset. Later boots do
not write the option bytes. SWD remains available on the separate SWDIO and
SWCLK pads.