# BleRiot Sense firmware

This module is the BleRiot node and hub inventory for the SSOP board variant:

- MCU: `PY32F003L16S6TU` (32 KiB flash, 4 KiB RAM).
- TinyGo target: `./py32f003x6-sense.json`; pyOCD target: `py32f003x6`.
- Sensor input: `PA0`.
- PAN2110 SCK: `PA1`.
- PAN2110 DATA: `PA2`.
- PAN2110 CSN: `PB0`.

## Sensor mode

Set `sensorConfig.Mode` in `test-hub.go` to match the assembled input network:

```go
sensorConfig = spec.Config{
	Mode:                       spec.ModeNTC,
	SampleIntervalMilliseconds: 1000,
	ADCSamples:                 16,
}
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
identity and `sensorConfig`.

The board-specific TinyGo target inherits `py32f003x6` but reserves a 1 KiB
system stack. The stock 512-byte linker stack overflows while handling radio
traffic. Firmware uses `--scheduler none`; sensor acquisition shares the
nonblocking node loop so no heap-backed goroutine stacks are required.

## First boot

On the eight-pin package, the PAN2110 CSN pad is shared with `PF2-NRST`. On first
boot the firmware preserves all option bytes except `NRST_MODE`, changes that
one option to GPIO, and requests one option-byte reload reset. Later boots do
not write the option bytes. SWD remains available on the separate SWDIO and
SWCLK pads.