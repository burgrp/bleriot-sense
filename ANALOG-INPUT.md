# Universal analog sensor input

## Purpose

The board uses one configurable input circuit for three mutually exclusive sensor modes:

1. A pressure sensor with a driven 0.5–5 V analog output.
2. A YF-B10 flow sensor with a pulse output.
3. An external 10 kΩ, B3950 NTC thermistor.

The operating mode is selected by component population when the board is assembled. It is not switched electronically at runtime.

## Common circuit

```text
                        AR1
                 +3V3 ─/\/\/─+
                              |
                              +──── INPUT ─── AR2 ───+──── SENSOR_OUT ─── MCU
                              |                     |
                       external sensor              +── AR3 ── GND
                              |                     |
                             GND                    +── AC1 ── GND
```

`AR3` and `AC1` are separate parallel branches from `SENSOR_OUT` to ground. They are not connected in series.

The sensor connector should provide:

| Pin | Signal | Description |
|---:|---|---|
| 1 | `+5V_SENSOR` | Supply for a pressure or flow sensor |
| 2 | `INPUT` | Driven sensor output, flow pulse, or NTC terminal |
| 3 | `GND` | Sensor ground |

In NTC mode, the thermistor connects between `INPUT` and ground. The `+5V_SENSOR` pin is unused. `AR1` connects to regulated `+3V3`, making the measurement ratiometric with the MCU ADC supply.

## Recommended component values

| Component | Nominal value | Function |
|---|---:|---|
| AR1 | 10.0 kΩ, 0.1% | NTC or open-collector flow pull-up |
| AR2 | 4.02 kΩ, 0.1% | Input scaling, isolation, and fault-current limiting |
| AR3 | 6.49 kΩ, 0.1% | Lower resistor of the 5 V input divider |
| AC1 | 100 nF | Analog low-pass filter |

Use low-temperature-coefficient resistors for pressure and temperature measurement. One-percent parts may be sufficient if the assembled board is calibrated.

## Population table

| Mode | AR1 | AR2 | AR3 | AC1 | MCU input mode |
|---|---:|---:|---:|---:|---|
| Pressure, 0.5–5 V | DNP | 4.02 kΩ | 6.49 kΩ | 100 nF | ADC |
| Flow, driven 5 V output | DNP | 4.02 kΩ | 6.49 kΩ | DNP | GPIO/timer |
| Flow, open collector | 10.0 kΩ | 4.02 kΩ | DNP | DNP | GPIO/timer |
| NTC 10k B3950 | 10.0 kΩ | 4.02 kΩ | DNP | 100 nF | ADC |

The two flow populations are alternatives. Select one only after confirming the electrical output of the exact YF-B10 variant.

## Pressure mode

### Population and connection

- Do not populate `AR1`.
- Populate `AR2` with 4.02 kΩ.
- Populate `AR3` with 6.49 kΩ.
- Populate `AC1` with 100 nF.
- Configure the MCU pin as an ADC input.
- Power the pressure sensor from `+5V_SENSOR` and connect its driven output to `INPUT`.

### Voltage scaling

`AR2` and `AR3` form a divider:

$$
V_{\mathrm{OUT}}=V_{\mathrm{INPUT}}\frac{R_{\mathrm{AR3}}}{R_{\mathrm{AR2}}+R_{\mathrm{AR3}}}
$$

For the recommended values:

$$
\frac{R_{\mathrm{AR3}}}{R_{\mathrm{AR2}}+R_{\mathrm{AR3}}}=\frac{6.49}{4.02+6.49}\approx0.6175
$$

| Sensor voltage | MCU voltage | Approximate 12-bit ADC code at 3.3 V |
|---:|---:|---:|
| 0.5 V | 0.309 V | 383 |
| 2.5 V | 1.544 V | 1916 |
| 4.5 V | 2.779 V | 3448 |
| 5.0 V | 3.087 V | 3831 |
| 5.25 V | 3.242 V | 4023 |

The original sensor voltage is recovered with:

$$
V_{\mathrm{INPUT}}=V_{\mathrm{OUT}}\frac{R_{\mathrm{AR2}}+R_{\mathrm{AR3}}}{R_{\mathrm{AR3}}}\approx1.6194V_{\mathrm{OUT}}
$$

The divider provides headroom for a nominal 5 V signal but does not make arbitrary overvoltage safe.

### Filtering

For a low-impedance sensor output, the resistance driving `AC1` is approximately:

$$
R_{\mathrm{TH}}=R_{\mathrm{AR2}}\parallel R_{\mathrm{AR3}}\approx2.48\ \mathrm{k\Omega}
$$

With 100 nF:

$$
f_c=\frac{1}{2\pi R_{\mathrm{TH}}C_{\mathrm{AC1}}}\approx642\ \mathrm{Hz}
$$

$$
	au=R_{\mathrm{TH}}C_{\mathrm{AC1}}\approx248\ \mathrm{\mu s}
$$

Pressure normally changes slowly, so firmware can additionally average several ADC samples.

## Flow mode

The YF-B10 produces pulses whose frequency represents flow. The MCU pin must be configured as a digital GPIO interrupt, timer capture, or timer counter input—not as an ADC input. `AC1 = 100 nF` is not populated because it would slow pulse edges and could distort the duty cycle.

### Driven or internally pulled-up output

Use this population only if the exact sensor produces a driven pulse reaching approximately 5 V:

- Do not populate `AR1`.
- Populate `AR2` with 4.02 kΩ.
- Populate `AR3` with 6.49 kΩ.
- Do not populate `AC1`.

The divider applies the same scale factor as pressure mode:

$$
V_{\mathrm{HIGH,OUT}}\approx0.6175V_{\mathrm{HIGH,INPUT}}
$$

A 5 V pulse becomes approximately 3.09 V; a 4.5 V pulse becomes approximately 2.78 V.

The sensor drives a total divider resistance of:

$$
R_{\mathrm{AR2}}+R_{\mathrm{AR3}}=10.51\ \mathrm{k\Omega}
$$

At 5 V, its high-state load current is approximately:

$$
I=\frac{5}{10.51\ \mathrm{k\Omega}}\approx0.476\ \mathrm{mA}
$$

Confirm that the sensor can source this current and that 2.78 V is safely above the MCU's guaranteed digital-high threshold under all operating conditions.

### Open-collector output

Some YF-B10 variants are described as NPN/open-collector outputs. Such an output needs a pull-up:

- Populate `AR1` with 10 kΩ to `+3V3`.
- Populate `AR2` with 4.02 kΩ.
- Do not populate `AR3`.
- Do not populate `AC1`.

`AR1` pulls `INPUT` to 3.3 V while the sensor transistor pulls it to ground. Since the MCU input draws negligible DC current, there is almost no DC drop across `AR2`, and the MCU receives a nominal 0–3.3 V pulse.

Before choosing the production population, verify the exact sensor's:

- Supply-voltage range.
- Push-pull/open-collector output type.
- Internal pull-up, if any.
- Output-high voltage and source current.
- Maximum pulse frequency.
- Required external pull-up resistance.

An optional 100 pF–1 nF capacitor footprint can be added for high-frequency EMI suppression. Its effect on pulse shape and maximum measurable frequency must be verified before population.

Firmware calculates flow by counting pulses in a fixed interval or measuring the time between edges. Use the calibration constant for the exact sensor, preferably verified by system calibration.

## NTC mode

### Population and connection

- Populate `AR1` with 10.0 kΩ, preferably 0.1% and low temperature coefficient.
- Populate `AR2` with 4.02 kΩ.
- Do not populate `AR3`.
- Populate `AC1` with 100 nF.
- Configure the MCU pin as an ADC input.
- Connect the external 10 kΩ B3950 NTC from `INPUT` to ground.

The temperature divider is:

```text
+3V3 ── AR1 10k ── INPUT ── external NTC 10k B3950 ── GND
```

`AR2` isolates the cable-side divider from the ADC and forms a low-pass filter with `AC1`. Since the ADC draws negligible DC current, `AR2` causes almost no steady-state voltage drop.

### Resistance calculation

The nominal ADC voltage is:

$$
V_{\mathrm{OUT}}\approx V_{\mathrm{CC}}\frac{R_{\mathrm{NTC}}}{R_{\mathrm{AR1}}+R_{\mathrm{NTC}}}
$$

At 25 °C, both `AR1` and the NTC are nominally 10 kΩ, so:

$$
V_{\mathrm{OUT}}=3.3\frac{10\ \mathrm{k\Omega}}{10\ \mathrm{k\Omega}+10\ \mathrm{k\Omega}}=1.65\ \mathrm{V}
$$

Thermistor resistance can be recovered from voltage:

$$
R_{\mathrm{NTC}}=R_{\mathrm{AR1}}\frac{V_{\mathrm{OUT}}}{V_{\mathrm{CC}}-V_{\mathrm{OUT}}}
$$

For a 12-bit ADC using the same supply as its reference, use the ratiometric form:

$$
R_{\mathrm{NTC}}=R_{\mathrm{AR1}}\frac{N_{\mathrm{ADC}}}{4095-N_{\mathrm{ADC}}}
$$

This result is largely independent of the exact 3.3 V supply voltage.

### Temperature calculation

For a nominal 10 kΩ B3950 thermistor:

$$
T_K=\left[\frac{1}{298.15}+\frac{1}{3950}\ln\left(\frac{R_{\mathrm{NTC}}}{10000}\right)\right]^{-1}
$$

Convert kelvin to degrees Celsius with:

$$
T_C=T_K-273.15
$$

Approximate nominal values are:

| Temperature | NTC resistance | ADC voltage at 3.3 V |
|---:|---:|---:|
| −20 °C | 105 kΩ | 3.01 V |
| 0 °C | 33.6 kΩ | 2.54 V |
| 25 °C | 10.0 kΩ | 1.65 V |
| 50 °C | 3.59 kΩ | 0.87 V |
| 80 °C | 1.27 kΩ | 0.37 V |
| 100 °C | 0.70 kΩ | 0.21 V |

Use the thermistor manufacturer's resistance table or Steinhart–Hart coefficients when better accuracy is required.

### Filtering

At 25 °C, the approximate resistance driving `AC1` is:

$$
R_{\mathrm{TH}}=R_{\mathrm{AR2}}+(R_{\mathrm{AR1}}\parallel R_{\mathrm{NTC}})=4.02\ \mathrm{k\Omega}+5.00\ \mathrm{k\Omega}=9.02\ \mathrm{k\Omega}
$$

With 100 nF:

$$
f_c\approx\frac{1}{2\pi(9.02\ \mathrm{k\Omega})(100\ \mathrm{nF})}\approx176\ \mathrm{Hz}
$$

This is much faster than the thermal response of a typical probe. Firmware averaging can provide additional noise rejection.

### Fault detection

Firmware should treat readings near either ADC rail as possible faults:

- Near 0 V: shorted thermistor/cable or temperature above the intended range.
- Near 3.3 V: open thermistor/cable or temperature below the intended range.

Set thresholds with sufficient margin for the supported operating-temperature range and all component tolerances.

## Input protection

The common four-component network does not by itself provide complete protection. Recommended additions are:

```text
connector INPUT ── cable ESD device ── AR2 ── SENSOR_OUT ── MCU
                                               |
                                      low-leakage clamps
                                        to +3V3 and GND
```

- Put a cable-rated, low-capacitance ESD device next to the connector.
- Its working voltage must not load a valid 5–5.25 V signal at `INPUT`.
- Put low-leakage rail clamps close to `SENSOR_OUT` and the MCU.
- Orient the lower clamp from ground to `SENSOR_OUT` and the upper clamp from `SENSOR_OUT` to `+3V3`.
- Keep `AR2` before the MCU-side clamps so it limits fault current.
- Check the clamp leakage over temperature because it contributes analog measurement error.
- Check unpowered behavior: an energized sensor must not back-power the 3.3 V rail through the upper clamp.

The circuit is intended for normal 0–5 V signals and transient protection. It is not automatically safe against a sustained 12 V or 24 V wiring error.

## PCB layout

- Place the cable ESD device immediately behind the sensor connector.
- Give the ESD device a short, direct connection to the ground plane.
- Place `AC1`, `AR3`, and MCU-side clamps close to the MCU input.
- Keep `SENSOR_OUT` away from the RF antenna, crystal, clocks, LED data, and noisy supply-current paths.
- Avoid sharing the sensor ground-return path with high-current LED, radio, or regulator currents.
- Provide test points for `INPUT` and `SENSOR_OUT`.
- Clearly document the pressure, flow, and NTC assembly populations on the schematic and BOM.

## Firmware summary

| Mode | MCU peripheral | Processing |
|---|---|---|
| Pressure | ADC | Average samples and reverse the divider ratio |
| Flow | GPIO/timer | Count edges or measure pulse period |
| NTC | ADC | Convert ADC ratio to resistance and then temperature |

The selected MCU pin must support both ADC operation and a suitable timer/GPIO function if all three populations are to use exactly the same physical input.
