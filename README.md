# BleRiot Sense

BleRiot Sense is a compact single-sensor BleRiot node built around a Puya
PY32 microcontroller and a PAN2110 long-range 2.4 GHz radio. Its sensor input is
configured at assembly time for one of three uses:

![BleRiot Sense board-003 front render](board-003.png)
![BleRiot Sense board-030 front render](board-030.png)
![BleRiot Sense board back with sensor-mode assembly table](board-back.png)

- A 10 kΩ B3950 NTC thermistor.
- A pulse-output flow sensor such as the YF-B10.
- A pressure sensor with a driven 0.5-5 V analog output.

The assembly population and the firmware mode must agree. The hardware does not
switch sensor modes electronically at runtime.

## Repository layout

| Path | Description |
|---|---|
| [`board-003/bleriot-sense.kicad_pro`](board-003/bleriot-sense.kicad_pro) | Eight-pin `PY32F003L16S6TU` hardware supported by `fw-003` |
| [`board-030/bleriot-sense.kicad_pro`](board-030/bleriot-sense.kicad_pro) | Larger `PY32F030F1xPx` hardware variant |
| [`fw-003/`](fw-003/README.md) | BleRiot node firmware and hub inventory for `board-003` |
| [`ANALOG-INPUT.md`](ANALOG-INPUT.md) | Analog input circuit, populations, calculations, protection, and layout |
| [`sub/hw-kicad/`](sub/hw-kicad) | Shared KiCad symbols and footprints, included as a Git submodule |

`board-030` does not currently have a matching firmware module in this
repository.

## Sensor modes

| Mode | Node measurement | Registry value |
|---|---|---|
| NTC | Averaged 12-bit ADC code | Temperature in °C |
| Flow | Rising-edge pulse count | Frequency in Hz and cumulative pulse count |
| Pressure | Averaged 12-bit ADC code | Reconstructed sensor-output voltage in V |

See [`ANALOG-INPUT.md`](ANALOG-INPUT.md) before assembling a board. It defines
the required values for `AR1`, `AR2`, `AR3`, and `AC1`, including the two
alternative flow-sensor output circuits.

The pressure transfer function and flow K-factor depend on the exact sensor.
The hub currently exposes pressure input voltage and flow frequency without
assuming undocumented sensor calibration constants.

## Firmware status

[`fw-003`](fw-003/README.md) targets the `PY32F003L16S6TU` and uses `PA0` for
the universal sensor input. The NTC path has been verified end to end on
hardware through the ADC, PAN2110 radio, MCP2210 hub dongle, BleRiot hub,
conversion layer, and Registry. Flow and pressure support build and have
host-side conversion coverage but still require mode-specific hardware tests.

The F003 firmware uses a board-specific TinyGo target with a 1 KiB system stack
and no task scheduler. See the firmware README for pin assignments, first-boot
option-byte handling, configuration, and memory constraints.

## Getting started

Initialize the shared KiCad library after cloning:

```sh
git submodule update --init --recursive
```

Open one of the hardware projects in KiCad:

```sh
kicad board-003/bleriot-sense.kicad_pro
kicad board-030/bleriot-sense.kicad_pro
```

Configure `sensorConfig.Mode` in
[`fw-003/test-hub.go`](fw-003/test-hub.go), then test and build the F003
firmware:

```sh
go -C fw-003 test ./...
go -C fw-003 run . make sense build
```

Flash with a supported SWD probe:

```sh
go -C fw-003 run . make sense flash
```

With a Registry server running, start the hub in a separate terminal:

```sh
go -C fw-003 run . hub --registry http://localhost:8080 --diagnostics rf
```

For an NTC-configured node, read live temperatures:

```bash
export REGISTRY=http://localhost:8080
reg get sense.temperature --stay
```

## Toolchain

- KiCad 10 for the hardware projects.
- Go 1.25.2 or newer for the firmware inventory, generator, tests, and hub.
- TinyGo with the Puya `py32f003x6` target.
- pyOCD with the Puya CMSIS pack and a supported SWD probe.
- An MCP2210/PAN2110 BleRiot hub dongle for RF operation.
- The [`reg`](https://github.com/burgrp/reg) Registry service and CLI.
