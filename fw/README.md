# BleRiot Sense firmware

This module is the importable BleRiot Sense firmware and local inventory for the
`PY32F003L16S6TU` hardware in `board`.

External hubs import the versioned device specification as:

```go
import sense "github.com/burgrp/bleriot-sense/fw/v2/spec"
```

- MCU: `PY32F003L16S6TU` (32 KiB flash, 4 KiB RAM).
- TinyGo and pyOCD target: `py32f003x6`.
- Sensor input: `PA0`.
- PAN2110 SCK: `PA1`.
- PAN2110 DATA: `PA2`.
- PAN2110 CSN: `PB0`.

## Sensor mode

Set `senseConfig` in `cmd/dev/main.go` to match the assembled input network.
Build the device type from the same value so its register table matches the
baked firmware configuration:

```go
var senseConfig = spec.Config{
	Mode:                       spec.ModeNTC,
	SampleIntervalMilliseconds: 1000,
	ADCSamples:                 16,
}

Type:   spec.TypeForConfig(senseConfig, spec.Calibration{}),
Config: senseConfig,
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

DS18B20 mode supports up to 20 externally powered devices. Connect all sensors
in parallel: J2 pin 1 to GND, pin 2 to DQ, and pin 3 to VDD. DQ is pulled up to
3.3 V; do not connect a 5 V DQ pull-up. Parasite power is not supported.

At every boot, firmware enumerates the bus and prints copy-pasteable IDs over
RTT before starting the radio:

```text
DS18B20 ID: [8]byte{40, 255, 100, 30, 147, 22, 4, 128}
```

One temperature register needs no configured ID. It binds automatically only
when exactly one DS18B20 is discovered. For multiple registers, configure every
ID in register order:

```go
var senseConfig = spec.Config{
	Mode:                       spec.ModeDS18B20,
	SampleIntervalMilliseconds: 1000,
	DS18B20Sensors: []spec.DS18B20Sensor{
		{ID: [8]byte{40, 255, 100, 30, 147, 22, 4, 128}},
		{ID: [8]byte{40, 2, 2, 3, 4, 5, 6, 199}},
	},
}
```

Optional per-register calibration is supplied only when constructing the host
device type. It is not part of `spec.Config`, is not baked into firmware, and
does not alter the signed Q12.4 value returned by the node:

```go
var senseCalibration = spec.Calibration{
	DS18B20: map[int]float64{
		3: 1,
	},
}

Type:   spec.TypeForConfig(senseConfig, senseCalibration),
Config: senseConfig,
```

The hub publishes $raw/16 + OffsetCelsius$. Supply either no calibration entries
or sparse offsets keyed by one-based DS18B20 sensor index, matching register names
such as `temperature.3`. Indexes must identify configured registers, offsets must
be finite, and nonzero offsets are included in Registry metadata as
`offsetCelsius`.

DS18B20 registers use the dedicated contiguous tag range 256–275. A one-register
inventory uses `temperature`; a multidrop inventory uses `temperature.1` through
`temperature.20`. Generic sample tag 1 and flow pulse-count tag 2 remain separate.
Per-instance `RegistryNames` may assign meaningful deployment names without
changing these wire identities.

The node validates ROM and scratchpad CRCs and transmits signed Q12.4 values.
It broadcasts one conversion, waits 800 ms while polling the radio, then reads
one addressed sensor per main-loop pass. One 12-bit addressed read takes about
11.7 ms; `bleNode.Poll()` runs between reads, so 20 sensors do not create one
continuous roughly 234 ms radio blackout. With all 20 sensors, the 800 ms
conversion plus addressed reads stretches a nominal 1 s cycle to about 1.04 s.
A failed sensor becomes null without invalidating successful readings from the
other sensors.

The protocol implementation comes from `github.com/burgrp/tinygo-drivers/onewire`.
This board supplies only the PA0 open-drain, TIM3 1 MHz counter, and interrupt
adapter in `onewire_py32.go`.

The node retains the latest full-resolution sample for scheduled GET responses.
In flow mode, the pulse interrupt only increments an in-memory counter; each
sample interval updates both the frequency and cumulative `pulses` register.

NTC and pressure modes reply with protocol NULL for averaged ADC codes at or
below 64 or at or above 4031. An open NTC is pulled high; an open pressure input
is pulled low. The pressure sensor's documented 0.5–5.25 V range remains inside
these thresholds. Hub-side NTC checks remain as a defensive fallback.

DS18B20 scratchpad/address failures null only the affected register. No-presence,
stuck-low, parasite-power, or broadcast-conversion failures null the whole bus.
Flow is the hardware exception: no pulses is electrically identical to an
unplugged sensor, so firmware preserves a valid `0 Hz` reading. True flow-sensor
presence detection requires an additional hardware signal or heartbeat.

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