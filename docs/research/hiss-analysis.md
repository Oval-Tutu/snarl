# Hiss and high-RPM loss of authenticity

Measured at HEAD `a553aef`, tier 1, f32, 48 kHz. No model code or profile changed.

## Answer

The constant hiss is the noise AM layer (`[noise] am`). When `noise.am=0`, the 4 to 12 kHz level falls by 12 to 20 dB at 2000 RPM, and the noise floor between engine orders falls by 9 to 13 dB. No other layer lowers these values by more than 1 dB. The output stage, the jet noise, the intake, the valve tick, the fan, the steepening and the idle character are not the source.

The loss of authenticity at high RPM has two causes. The main cause is that the noise AM has a fixed depth over RPM. In snarl, the harmonic-to-noise ratio below 500 Hz falls by 7 to 15 dB from idle to the redline. In engine-sim, it rises by 10 to 20 dB for flat6, v8 and flat12, and stays near 31 dB for inline4. The second cause is smaller: the pulse harmonics get brighter with RPM, because the pulse width is fixed in crank degrees.

The harness load change (load is at least the throttle, `4b61331`) adds 1.4 to 2.1 dB of hiss at full throttle compared with load 0.7. It makes the hiss more audible but does not cause it. The noise AM is unchanged since the first model commit `ca36d33`.

Recommendation: band-limit the AM noise and taper its depth with RPM (option 1 below). A prototype of this change lowers the floor by 5 to 10 dB and the 4 to 12 kHz level by 9 to 19 dB at 2000 RPM, with no change of loudness.

## Method

- Renders: `holds-t100`, `holds-t050` and `sweep-wot` for flat12, flat6, inline4 and v8, and `holds-t100` and `holds-t050` for `es-v12-60-aero`, `es-v12-75-race`, `el-v12-60-aero`, `es-v10-72`, `es-i4-dohc-cam-switch` and `el-i4-dohc-cam-switch`. Files are in `out/tmp/hiss/head/`.
- References: `out/reference/engine-sim/<profile>/`, and `just ref-set catalogue/<id>` for `es-v12-60-aero`, `es-v12-75-race` and `es-v10-72`.
- Analyser: a scratch Go program (`/tmp/hiss/an/main.go`). For each 3 s hold, it takes the window from 0.8 s to 2.6 s and computes a Welch power spectrum (65536-point Hann, 0.73 Hz bins). It finds the half-order spacing from the hold RPM, refined within 2 % on the 20 to 500 Hz harmonics.
- Metrics:
  - HNR: the mean power density within 0.15 of a half order, divided by the mean density from 0.3 to 0.7 between half orders, in dB, per band.
  - Floor: the density between orders, times the band width, relative to the total power from 20 Hz to 16 kHz, in dB. -10 dB means that the noise is 10 % of the power.
  - Tilt: the slope of the third-octave levels from 500 Hz to 12 kHz, in dB per octave.
  - 4 to 12 kHz: the power from 4 to 12 kHz relative to 20 Hz to 20 kHz, in dB.
- Above 2 kHz, the HNR is near 0 dB for snarl and engine-sim. Both are noise-like there, so the 4 to 12 kHz level is the measure of the hiss.

## Snarl against engine-sim at full throttle

Each cell is snarl / engine-sim. The flat6 and inline4 references run at a simulation rate of 20 kHz, so they are valid to 10 kHz. The v8 and flat12 references run at 10 kHz, so their content above 5 kHz is less reliable.

### flat6

| RPM | HNR 20-500 Hz | HNR 0.5-2 kHz | HNR 2-6 kHz | HNR 6-16 kHz | Floor | Tilt | 4-12 kHz |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 800 | 30.3 / 23.3 | 1.2 / 10.5 | 0.1 / -0.7 | 0.1 / 0.1 | -16.4 / -16.4 | 0.7 / -3.7 | -22.0 / -28.9 |
| 2000 | 29.4 / 33.2 | 6.9 / 20.0 | 0.6 / 1.1 | -0.0 / 0.1 | -14.7 / -25.7 | -0.7 / -5.9 | -20.2 / -36.7 |
| 3000 | 29.6 / 38.8 | 12.4 / 15.5 | 1.4 / 1.7 | 0.1 / -0.3 | -14.7 / -29.2 | -2.5 / -6.1 | -19.5 / -38.4 |
| 4000 | 25.3 / 38.8 | 16.3 / 18.7 | 3.6 / 1.9 | 0.1 / 0.1 | -11.5 / -29.5 | -2.9 / -5.9 | -16.4 / -39.2 |
| 5000 | 21.7 / 41.6 | 18.8 / 23.2 | 5.5 / 2.5 | 0.2 / 1.3 | -10.3 / -32.6 | -2.9 / -5.6 | -14.5 / -42.0 |
| 6000 | 19.4 / 38.8 | 16.6 / 19.2 | 7.8 / 2.3 | 0.7 / 0.5 | -8.7 / -30.3 | -2.0 / -5.4 | -12.2 / -40.6 |
| 6600 | 19.8 / 35.5 | 19.7 / 16.7 | 9.4 / 1.8 | 0.6 / -0.6 | -10.5 / -28.0 | -1.9 / -5.2 | -13.9 / -39.6 |

### inline4

| RPM | HNR 20-500 Hz | HNR 0.5-2 kHz | HNR 2-6 kHz | HNR 6-16 kHz | Floor | Tilt | 4-12 kHz |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 800 | 31.1 / 30.2 | 0.9 / 13.2 | -0.4 / 0.7 | 0.3 / 0.2 | -17.4 / -24.9 | -1.1 / -4.9 | -24.8 / -47.9 |
| 2000 | 28.9 / 47.2 | 8.9 / 22.9 | 0.2 / 1.2 | 0.5 / -0.2 | -16.6 / -40.9 | -3.2 / -5.3 | -24.4 / -52.1 |
| 3000 | 27.4 / 34.7 | 13.4 / 28.1 | 1.7 / 1.0 | 0.2 / -0.1 | -15.6 / -29.1 | -4.2 / -6.7 | -23.1 / -42.6 |
| 4000 | 30.6 / 48.7 | 15.3 / 30.8 | 3.9 / 1.2 | -0.1 / 0.2 | -16.2 / -42.4 | -4.5 / -6.7 | -22.9 / -52.1 |
| 5000 | 28.3 / 49.4 | 17.7 / 30.9 | 5.8 / 2.1 | 0.6 / -0.1 | -14.9 / -42.9 | -5.1 / -7.2 | -21.6 / -52.5 |
| 6000 | 21.4 / 39.2 | 18.1 / 33.5 | 10.5 / 6.4 | 1.6 / 0.7 | -12.3 / -32.7 | -4.4 / -8.2 | -18.6 / -42.4 |
| 7000 | 21.3 / 34.7 | 19.2 / 28.9 | 11.7 / 3.4 | 4.6 / 0.6 | -12.3 / -28.0 | -5.0 / -7.8 | -16.3 / -37.2 |
| 8000 | 17.4 / 31.2 | 19.8 / 35.5 | 12.1 / 1.8 | 4.7 / 0.7 | -12.1 / -26.1 | -4.0 / -8.3 | -15.2 / -35.5 |
| 8400 | 16.1 / 31.3 | 17.9 / 36.5 | 13.1 / 4.5 | 5.2 / 0.2 | -10.9 / -26.8 | -3.6 / -8.7 | -14.6 / -35.0 |

### v8

| RPM | HNR 20-500 Hz | HNR 0.5-2 kHz | HNR 2-6 kHz | HNR 6-16 kHz | Floor | Tilt | 4-12 kHz |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 675 | 27.9 / 22.7 | 0.4 / 9.9 | -0.4 / 0.0 | -1.2 / 1.1 | -12.8 / -16.5 | -0.9 / -3.6 | -20.4 / -29.9 |
| 2000 | 26.9 / 36.2 | 7.9 / 20.8 | 0.3 / 0.1 | -0.0 / 0.2 | -12.4 / -25.3 | -2.7 / -6.6 | -20.0 / -33.2 |
| 3000 | 22.8 / 38.1 | 13.7 / 19.0 | 1.2 / 0.5 | 0.1 / 0.0 | -8.9 / -26.6 | -4.1 / -6.5 | -16.0 / -34.5 |
| 4000 | 23.0 / 41.7 | 17.3 / 23.0 | 3.5 / 0.8 | -0.1 / 0.0 | -9.7 / -29.8 | -4.5 / -6.7 | -16.4 / -38.0 |
| 5000 | 19.8 / 37.7 | 16.4 / 18.2 | 2.9 / 0.2 | 0.4 / -0.1 | -8.0 / -26.0 | -5.1 / -6.9 | -15.2 / -34.3 |
| 6000 | 20.6 / 42.4 | 22.2 / 19.8 | 6.3 / 0.6 | 0.9 / 0.1 | -12.6 / -30.5 | -5.8 / -6.9 | -19.3 / -38.3 |

### flat12

| RPM | HNR 20-500 Hz | HNR 0.5-2 kHz | HNR 2-6 kHz | HNR 6-16 kHz | Floor | Tilt | 4-12 kHz |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1000 | 24.4 / 9.2 | 4.6 / 3.3 | -0.4 / 0.1 | 0.1 / -0.2 | -14.1 / -4.9 | -1.5 / -2.3 | -20.6 / -28.0 |
| 2000 | 19.6 / 10.9 | 11.2 / 5.8 | 0.4 / 0.2 | 0.2 / 0.1 | -9.7 / -6.2 | -3.1 / -1.8 | -16.5 / -26.8 |
| 3000 | 18.5 / 13.5 | 15.6 / 7.2 | 1.9 / -0.0 | 0.1 / 0.4 | -9.2 / -8.6 | -4.2 / -2.0 | -15.6 / -29.2 |
| 4000 | 17.4 / 14.5 | 16.2 / 4.2 | 3.7 / 0.4 | 0.2 / 0.0 | -8.7 / -9.5 | -4.1 / -2.9 | -14.6 / -31.2 |
| 5000 | 17.1 / 10.5 | 20.1 / 17.1 | 6.2 / 0.7 | 1.0 / 0.8 | -10.8 / -6.0 | -4.9 / -3.1 | -16.4 / -26.6 |
| 6000 | 18.1 / 15.0 | 18.1 / 12.1 | 8.6 / -0.2 | 1.6 / 0.9 | -9.9 / -9.8 | -4.6 / -3.8 | -14.6 / -28.6 |
| 6800 | 17.1 / 19.3 | 19.5 / 16.0 | 9.6 / 0.9 | 3.1 / -0.1 | -10.7 / -13.6 | -4.3 / -4.2 | -14.4 / -29.3 |

### What the tables show

- Hiss: the snarl 4 to 12 kHz level is 7 to 30 dB above engine-sim at every RPM. flat12 has the most hiss of the tuned profiles (-14 to -21 dB), and it also has the largest AM setting (`am = 0.3`, the others have 0.2).
- Floor: the snarl floor stays at -8 to -17 dB over the full RPM range. The engine-sim floor falls to -25 to -43 dB above idle for flat6, inline4 and v8. The flat12 reference is an exception: its own floor is -5 to -14 dB.
- RPM trend: the snarl HNR below 500 Hz falls with RPM (flat6 30 to 20 dB, inline4 31 to 16 dB, v8 28 to 21 dB). The engine-sim HNR rises (flat6 23 to 36 dB, v8 23 to 42 dB). At idle, snarl and engine-sim are close. This agrees with the report that idle sounds authentic and high RPM does not.
- Tilt: snarl is 1 to 5 dB per octave flatter than engine-sim for flat6, inline4 and v8. The engine-sim tilt gets steeper with RPM (inline4 -4.9 to -8.7 dB per octave), but the snarl tilt does not.
- Half throttle (`holds-t050`) gives the same result. At 2000 RPM and at the top hold, the snarl floor is -13 to -21 dB and the engine-sim floor is -24 to -42 dB (flat6, inline4, v8). The snarl 4 to 12 kHz level is -15 to -29 dB, and the engine-sim level is -34 to -52 dB.
- Sweeps (`sweep-wot`): the snarl 4 to 12 kHz level is 11 to 30 dB above engine-sim over the full sweep. The HNR has no meaning in a sweep window, because the harmonics move.

### Catalogue engines

Full-throttle holds at HEAD. Each cell is snarl / engine-sim where a reference exists.

| Engine | RPM | HNR 20-500 Hz | HNR 0.5-2 kHz | Floor | 4-12 kHz |
| --- | ---: | ---: | ---: | ---: | ---: |
| `es-v12-60-aero` | 600 | 28.4 / 24.4 | 0.7 / 0.1 | -11.8 / -9.2 | -32.5 / -11.4 |
| `es-v12-60-aero` | 3000 | 23.4 / 34.9 | 12.6 / 6.8 | -9.4 / -18.9 | -29.4 / -21.0 |
| `es-v12-75-race` | 3810 | 21.2 / 11.3 | 13.0 / 11.4 | -8.6 / -6.7 | -28.5 / -26.7 |
| `es-v12-75-race` | 9000 | 22.2 / 23.8 | 21.1 / 15.6 | -14.6 / -17.4 | -30.3 / -30.7 |
| `es-v12-75-race` | 18000 | 4.7 / 15.5 | 23.5 / 25.5 | -17.0 / -13.9 | -24.4 / -29.5 |
| `el-v12-60-aero` | 800 | 20.5 / - | 1.8 / - | -5.9 / - | -34.8 / - |
| `el-v12-60-aero` | 3100 | 24.1 / - | 15.9 / - | -11.5 / - | -39.3 / - |
| `es-v10-72` | 2000 | 26.1 / 34.2 | 7.8 / 20.2 | -11.5 / -28.7 | -31.5 / -58.9 |
| `es-v10-72` | 9000 | 9.3 / 35.2 | 17.8 / 33.1 | -11.7 / -29.3 | -29.4 / -45.4 |
| `es-i4-dohc-cam-switch` | 2000 | 27.4 / 48.8 | 7.5 / 24.0 | -11.7 / -42.3 | -32.4 / -52.1 |
| `es-i4-dohc-cam-switch` | 8400 | 22.0 / 30.2 | 20.4 / 36.9 | -13.9 / -26.3 | -30.9 / -34.8 |
| `el-i4-dohc-cam-switch` | 2000 | 30.4 / - | 6.7 / - | -14.5 / - | -23.3 / - |
| `el-i4-dohc-cam-switch` | 8600 | 24.9 / - | 21.4 / - | -13.5 / - | -18.5 / - |

The catalogue V12 and V10 references are not valid above about 3 kHz. They run at a simulation rate of 7000 Hz (`es-v12-60-aero`), 5000 Hz (`es-v12-75-race`) and 6500 Hz (`es-v10-72`). The Merlin reference has a rising tilt of +3 dB per octave from images of the simulation rate. Use these references only for the floor and the HNR below 2 kHz. The `es-i4-dohc-cam-switch` reference runs at 20000 Hz and is valid to 10 kHz.

The catalogue engines show the same floor error as the tuned engines. The snarl floor stays at -6 to -17 dB. The engine-sim floor is -17 to -19 dB for the Merlin above 2000 RPM, -20 to -29 dB for the V10 at six of its nine holds, and -24 to -43 dB for the cam-switch I4. For the cam-switch I4, the snarl tilt is within 2 dB per octave of engine-sim, but its 4 to 12 kHz level is still 4 to 21 dB higher.

## Layer mutes

Each value is the change in dB against the HEAD render of `holds-t100`: the 4 to 12 kHz level at 2000 RPM / the 4 to 12 kHz level at the top hold / the floor at the top hold. The top holds are 6800 RPM (flat12), 6000 RPM (v8), 8400 RPM (inline4) and 3000 RPM (`es-v12-60-aero`).

| Mute | flat12 | v8 | inline4 | es-v12-60-aero |
| --- | ---: | ---: | ---: | ---: |
| `am0` | -18.7 / -3.6 / -9.6 | -19.5 / -8.4 / -12.2 | -16.0 / -1.2 / -5.2 | -11.9 / -9.3 / -13.4 |
| `jet0` | +0.0 / +0.0 / +0.0 | +0.0 / -0.1 / +0.0 | +0.0 / +0.0 / +0.0 | -0.1 / -0.2 / +0.0 |
| `intake0` | +0.0 / +0.0 / +0.0 | +0.0 / +0.0 / +0.0 | -0.1 / +0.0 / -1.1 | -0.1 / +0.0 / +0.0 |
| `tick0` | +0.0 / +0.0 / +0.0 | +0.0 / +0.0 / +0.1 | +0.0 / +0.0 / +0.0 | +0.0 / +0.0 / +0.0 |
| `fan0` | +0.0 / +0.0 / +0.0 | +0.0 / +0.0 / +0.0 | +0.0 / +0.0 / +0.0 | +0.0 / +0.0 / +0.0 |
| `steep0` | -0.9 / -0.4 / -0.9 | -0.5 / -0.4 / -0.5 | -0.9 / +0.2 / -0.1 | -0.9 / -0.6 / -0.6 |
| `overlap0` | +0.3 / +0.7 / +0.4 | +0.6 / +1.6 / +1.2 | +0.1 / +1.1 / +0.8 | +0.7 / +1.4 / +0.5 |
| `cyc0` | +0.0 / +0.0 / -0.3 | +0.0 / +0.0 / -0.1 | +0.0 / +0.1 / +0.0 | +0.0 / +0.0 / +0.0 |
| `jit0` | +0.0 / -0.3 / +0.2 | +0.0 / -0.2 / +0.0 | +0.0 / +0.3 / +0.1 | +0.0 / +0.1 / +0.1 |
| `idle0` | +0.0 / -0.1 / +0.0 | +0.0 / +0.0 / +0.0 | +0.0 / +0.0 / +0.0 | +0.0 / +0.0 / +0.0 |
| `allnoise0` | -19.6 / -3.6 / -9.8 | -22.2 / -9.1 / -14.3 | -19.3 / -1.3 / -11.0 | -20.9 / -16.7 / -14.1 |
| `out-12` | +0.0 / +0.0 / +0.1 | +0.0 / +0.0 / +0.1 | +0.3 / +0.3 / +0.2 | +0.0 / +0.1 / +0.1 |

Keys used with `--set`:

- `am0`: `noise.am=0`
- `jet0`: `noise.jet=0`
- `intake0`: `noise.intake=0`
- `tick0`: `mechanical.tick_gain=0`
- `fan0`: `mechanical.fan_gain=0`
- `steep0`: `exhaust.steepening=0`
- `overlap0`: `exhaust.overlap=0`
- `cyc0`: `pulse.cycle_variation=0`
- `jit0`: `pulse.jitter=0`
- `idle0`: `idle.variation=0`, `idle.timing_deg=0`, `idle.imbalance=0`, `idle.hunt=0`, `idle.rock=0`
- `allnoise0`: `noise.am=0`, `noise.jet=0`, `noise.intake=0`, `mechanical.tick_gain=0`
- `out-12`: `voice.output_db` set 12 dB below the profile value, which keeps the peak limiter and the soft clipper out of action. The files are under `out/tmp/hiss/mute/<profile>/out-20.wav`.

With `noise.am=0`, the HNR from 2 to 6 kHz rises from about 0 dB to 12 to 25 dB above 2000 RPM. The hiss covers real harmonics there.

With `noise.am=0`, the inline4 floor at 7000 to 8400 RPM is still -16 to -18 dB. With all noise layers at zero, it is -22 to -25 dB. The rest is the intake noise, which rises above the cam switch (`intake_cam_gain = 1.2`). The noise AM covers it at HEAD.

With all noise at zero, the 4 to 12 kHz level at the top hold is still -16 to -18 dB for flat12 and inline4, 11 to 19 dB above engine-sim. This content is harmonic (HNR 2 to 6 kHz of 24 to 27 dB). It comes from the pulse shape. The snarl tilt with all noise at zero flattens with RPM (inline4 -9.7 to -4.8 dB per octave, flat12 -10.1 to -6.7 dB per octave), but the engine-sim tilt gets steeper.

## Harness load

A replay log holds throttle 1 at fixed RPM with load 1, 0.7 or 0.5. The logs and renders are in `out/tmp/hiss/load/`.

| Engine | RPM | 4-12 kHz at load 1 / 0.7 / 0.5 | Floor at load 1 / 0.7 / 0.5 |
| --- | ---: | ---: | ---: |
| flat12 | 2000 | -16.7 / -18.6 / -20.4 | -9.9 / -11.9 / -12.9 |
| flat12 | 6000 | -14.7 / -15.9 / -16.7 | -9.8 / -11.3 / -13.3 |
| v8 | 2000 | -19.9 / -22.0 / -24.0 | -12.5 / -14.4 / -16.2 |
| v8 | 6000 | -19.3 / -21.0 / -22.5 | -12.6 / -14.5 / -16.3 |
| inline4 | 3000 | -23.1 / -25.1 / -26.9 | -15.3 / -17.2 / -18.9 |
| inline4 | 8000 | -15.2 / -15.6 / -15.9 | -11.6 / -13.0 / -14.1 |
| es-v12-60-aero | 2000 | -30.0 / -32.0 / -33.7 | -9.6 / -11.5 / -13.3 |
| es-v12-60-aero | 3000 | -29.4 / -31.2 / -32.7 | -9.4 / -11.3 / -12.9 |

Load 1 against load 0.7 adds 1.4 to 2.1 dB to the floor and up to 2.1 dB to the 4 to 12 kHz level. Before `4b61331`, the harness load was the acceleration ratio, which is below 1 at most points of a run. From the load 0.5 and 0.7 rows, the change makes the hiss about 2 to 4 dB louder in the harness. This is an estimate, because this analysis did not record the old harness load. It is not the source. `snarl-render` always sets the load to the throttle, so the render tools hear the full hiss at all times.

## Cause

The noise AM multiplies the summed pulse wave of each path by `1 + amDepth*nz` (`internal/model/synth.go`, the `process` function). `nz` is white noise through one one-pole low pass at 2 kHz (`amLP`), so it falls by only 6 dB per octave above 2 kHz. `amDepth` is `am * load^0.7`. It has no RPM term.

Each pulse is positive, so the pulse sum has a large steady part. The product puts a copy of the noise around every harmonic, and the copy around the steady part is broadband noise. The level of this noise follows the pulse level, which rises with RPM, so the hiss is always present and gets louder with RPM and load.

The hiss is most audible on the 12-cylinder engines for two reasons. flat12 has `am = 0.3`, against 0.2 for the other tuned profiles. Also, 12 overlapping pulses give a smoother sum, so the steady part that carries the noise is large relative to the tone. A prototype that applies the AM only to the pulse wave above its 30 Hz mean (option `ac` below) lowers the floor of `es-v12-60-aero` by 4 to 5 dB and the floor of the other engines by 1 to 4 dB.

No profile has a valve count. The only valve layer is the valve tick (`mechanical.tick_gain`), and muting it changes nothing. "Higher valve engines" probably means engines with more cylinders or higher RPM, and both increase the hiss as above.

At high RPM, engine-sim gets cleaner: its harmonics rise above its noise. The snarl noise AM stays at the same depth, so the tone loses clarity as RPM rises. The brighter pulses then add harmonic content at high frequency that engine-sim does not have.

## Fix options

The options were tested in a scratch copy of the repository under `/tmp/hiss/fix`, selected by the `HISS_FIX` environment variable. With no option, the copy renders identical files to HEAD. Each cell is the floor / 4 to 12 kHz level at 2000 RPM, then the floor / 4 to 12 kHz level and the HNR 20 to 500 Hz at the top hold, all in dB. The RMS level changes by less than 0.5 dB for all options.

| Option | flat6 | inline4 | v8 | flat12 | es-v12-60-aero |
| --- | --- | --- | --- | --- | --- |
| HEAD | -15 / -20, -10 / -14, 20 | -17 / -24, -11 / -15, 16 | -12 / -20, -13 / -19, 21 | -10 / -16, -11 / -14, 17 | -10 / -30, -9 / -29, 23 |
| `amhalf` | -19 / -25, -14 / -16, 23 | -21 / -30, -14 / -15, 18 | -18 / -26, -18 / -24, 24 | -14 / -22, -15 / -17, 20 | -15 / -35, -14 / -34, 26 |
| `ac` | -17 / -22, -12 / -14, 21 | -18 / -26, -12 / -15, 17 | -15 / -23, -15 / -22, 23 | -11 / -18, -12 / -15, 19 | -14 / -34, -13 / -33, 26 |
| `lp` | -17 / -28, -12 / -16, 15 | -18 / -38, -11 / -16, 13 | -15 / -35, -14 / -27, 16 | -9 / -31, -11 / -18, 13 | -11 / -40, -10 / -38, 19 |
| `lpn` | -21 / -29, -15 / -16, 21 | -22 / -40, -15 / -16, 17 | -20 / -38, -20 / -28, 22 | -14 / -34, -16 / -18, 18 | -16 / -42, -16 / -39, 24 |
| `rpm` | -16 / -22, -16 / -16, 24 | -18 / -26, -16 / -16, 19 | -15 / -22, -22 / -26, 26 | -11 / -18, -19 / -18, 22 | -15 / -35, -19 / -37, 27 |
| `lpnrpm` | -22 / -29, -16 / -16, 24 | -23 / -40, -16 / -16, 19 | -21 / -39, -24 / -28, 26 | -15 / -34, -20 / -18, 22 | -20 / -42, -22 / -39, 27 |
| `cap` | -15 / -20, -10 / -14, 20 | -17 / -24, -10 / -17, 16 | -12 / -20, -11 / -18, 20 | -10 / -16, -10 / -16, 17 | -9 / -30, -8 / -28, 23 |
| `full` | -22 / -29, -16 / -18, 24 | -23 / -40, -16 / -20, 20 | -21 / -39, -23 / -30, 25 | -15 / -34, -20 / -23, 22 | -20 / -42, -21 / -39, 27 |
| engine-sim | -26 / -37, -28 / -40, 36 | -41 / -52, -27 / -35, 31 | -25 / -33, -30 / -38, 42 | -6 / -27, -14 / -29, 19 | -17 / -19, -19 / -21, 35 |

The prototypes:

- `amhalf`: `noise.am` at half its profile value. This is a profile change only.
- `ac`: the AM acts on the pulse wave minus its 30 Hz one-pole mean, per path.
- `lp`: a second one-pole at 800 Hz after the AM noise, both at 800 Hz, scaled to the RMS of the HEAD noise.
- `lpn`: the same two poles at 800 Hz with unity gain, so that the noise density below 800 Hz stays the same.
- `rpm`: `amDepth` times `1 - 0.75*min(rpm/redline, 1)`.
- `lpnrpm`: `lpn` and `rpm` together.
- `cap`: the pulse attack scales by `0.5*redline/rpm` above half the redline, so that the rise time in seconds stops getting shorter.
- `full`: `lpnrpm` and `cap` together.

### Option 1 (recommended): band-limit the AM noise and taper its depth with RPM

This is `lpnrpm`. Add a second one-pole to `amLP` and lower both cutoffs to about 800 Hz, keeping unity gain. Multiply `amDepth` by a factor that falls with RPM, for example `1 - k*min(rpm/redline, 1)`, with `k` as a new optional `[noise]` key with a default of 0.75.

- Result: at 2000 RPM, the floor falls by 5 to 10 dB and the 4 to 12 kHz level by 9 to 19 dB. At the top hold, the floor falls by 5 to 13 dB, and the HNR below 500 Hz rises by 3 to 5 dB. The RPM trend of the HNR becomes flat or rising, as in engine-sim. The loudness does not change.
- Cost: one extra one-pole per sample (two multiply-adds) and one multiply per chunk, which is small against the budget of about 1060 cycles per stereo frame (`docs/scoping.md` section 4). The calibration cache and the regression tests need new values, and each profile needs a listening pass, because the timbre at idle also changes.
- Risk: medium. The noise AM is the timbre-with-load control (`docs/scoping.md` section 2), so the change needs listening at part load and at idle. Use `lp` instead of `lpn` only if the low band loses texture, because `lp` lowers the HNR below 500 Hz by 5 dB.
- Effort: S.

### Option 2: lower `noise.am` in the profiles

This is `amhalf`. Halve `am` in the tuned profiles and in the 42 catalogue profiles, which all have `am = 0.2`.

- Result: the floor falls by 4 to 6 dB and the 4 to 12 kHz level by 5 to 6 dB at 2000 RPM, with smaller gains at the top hold. The hiss spectrum stays bright and the RPM trend stays wrong.
- Cost: no code change. It can ship at once as a stop-gap, but it reduces the load timbre change by half.
- Effort: XS.

### Option 3: limit the pulse brightness at high RPM

This is `cap`, and it adds to option 1 as `full`. The pulse attack stops getting sharper in time above half the redline. Alternatively, an RPM-dependent low pass on the pulse wave gives the same result.

- Result: at the top hold, the 4 to 12 kHz level falls by a further 2 to 5 dB for flat12, inline4 and v8. It does not change anything below half the redline. It removes some of the tilt error at high RPM.
- Cost: one multiply per chunk. The inline4 `cam_attack` needs a new value.
- Risk: low. The HNR from 2 to 6 kHz at the top hold falls by about 3 dB, because the pulses lose some upper harmonics while the noise stays.
- Effort: S.

Recommended order: option 1, then option 3 if high RPM still sounds too bright, then a separate look at the inline4 intake noise above the cam switch. Option 2 is a stop-gap only.

## Fix

Option 1 is in the model, with two new `[noise]` keys:

- `am_cutoff_hz` (default 800): the cutoff of the AM noise low pass at the redline. At idle, the low pass is one pole at 2 kHz, as at HEAD. Towards the redline, it moves to two unity-gain poles at this cutoff. 0 keeps the idle low pass at every RPM.
- `am_redline` (default 0.25): the AM depth at the redline as a fraction of the depth at idle.

Both change along a smoothstep from the idle RPM to the redline, so that idle does not change and 2000 RPM changes little. The coefficients update once per control block. The cost is one one-pole per sample, and `BenchmarkRender` shows no change outside the noise of the machine. The four tuned profiles use the defaults: a lower cutoff or a lower `am_redline` lowers the 4 to 12 kHz level and raises the HNR by less than 1 dB more at the high holds. The catalogue profiles use the defaults.

Results on `holds-t100` (`TestHissHighRPM` and `TestHissIdle` in `hiss_test.go`):

- Idle: no change in any measure or in the RMS level.
- 2000 RPM: the 4 to 12 kHz level falls by 0.9 to 2.1 dB, the HNR below 500 Hz rises by 0.2 to 0.5 dB, and the RMS level falls by at most 0.08 dB, for the tuned profiles.
- From 4000 RPM up: the 4 to 12 kHz level falls by 1.2 to 10.4 dB, the HNR below 500 Hz rises by 1.7 to 5.1 dB, and the floor falls by 4 to 12 dB. From idle to the top hold, the HNR below 500 Hz now falls by 2 to 12 dB, against 7 to 15 dB at HEAD (v8 and flat12 2 dB, flat6 6 dB, inline4 12 dB). In engine-sim, it rises.
- The sweep-wot loudness falls by 0.1 to 0.7 LU and stays within the 1 LU tolerance of `TestOutputCalibration`, so `output_db` does not change. A change of `output_db` would also change the idle level.
- Pops: `TestHissPops` finds the pop waves unchanged (correlation 1.000). Requested pops at full throttle are 0.15 to 0.48 dB quieter, because the engine level that they follow is lower. Own pops on overrun change by at most 0.01 dB.

From 4000 RPM up, the flat6 and inline4 4 to 12 kHz level is still 18 to 26 dB above engine-sim. This content is the pulse harmonics (option 3) and the inline4 intake noise above the cam switch, which the noise AM no longer covers.

## Caveats

- The flat12 engine-sim reference is noisy below 2 kHz itself (floor -5 to -14 dB). It is not a good target for the floor of flat12. Use flat6 and inline4 for the floor and the 4 to 12 kHz level.
- The catalogue V12 references are not valid above about 3 kHz (see above).
- The HNR at idle uses half orders 5 to 8 Hz apart, close to the 0.73 Hz resolution with the Hann window. Treat the idle HNR values as approximate.

## Reproduce

The scratch analyser and the prototype copy of the repository that produced the tables above were under `/tmp/hiss`, which does not persist. The method is now in `hiss_test.go`, and these repository commands reproduce the measures:

- Measures of the fix: `nix develop -c go test -run 'TestHiss' -v .` runs `TestHissHighRPM`, `TestHissIdle` and `TestHissPops`. `TestHissHighRPM` logs the 4 to 12 kHz level, the HNR from 20 to 500 Hz, the floor and the RMS change of each `holds-t100` hold, with the values of the noise AM of HEAD `a553aef` beside them. The `legacyAM` overrides in `hiss_test.go` give that noise AM.
- Renders: `nix develop -c go run ./cmd/snarl-render --format f32 --tier 1 [--set section.key=value] --out out/tmp/hiss/<name>.wav <profile> scenarios/<profile>/<scenario>.scn`. The layer mutes use `--set` with the keys in the "Layer mutes" section, for example `--set noise.am=0`.
- Comparison with engine-sim: `nix develop -c just compare out/tmp/hiss/<name>.wav out/reference/engine-sim/<profile>/<scenario>.wav --scenario scenarios/<profile>/<scenario>.scn`.
- The fix options 2 and 3 have no switch in the repository. Option 1 is the model at HEAD, and `noise.am_cutoff_hz=0` with `noise.am_redline=1` turns it off.
