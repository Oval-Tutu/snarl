# Background static

Measured at HEAD `8b1fba5`, tier 1, f32, 48 kHz unless a table gives another rate. No model behaviour or profile value changed. The only model change is a test hook for the control block size, and the tuned-profile renders stay bit-identical. The [Fix](#fix) section is later: it puts options A, B and C in the model.

## Answer

The faint constant static has one model artefact and four noise layers as its sources. No processing stage adds measurable static.

1. The noise AM (`[noise] am`) is the static at idle and in the middle of the RPM range for every profile. With only the AM on, the noise floor between the harmonics is -12.7 to -18.9 dB at idle and at 3000 RPM, which is almost all of the floor of the full profile (-11.7 to -17.6 dB). It fills 500 Hz to 6 kHz. The hiss fix `8b1fba5` tapers it towards the redline only, so it keeps its full depth below about 3000 RPM.
2. The valve tick (`[mechanical] tick_gain`) is a high-passed noise burst at each firing with no upper band limit. It is the top source of the flat6 above 3000 RPM (floor -17.4 dB at 5440 RPM) and the main content from 16 to 24 kHz of every profile.
3. The intake noise is the top source of the inline4 near the redline (floor -17.0 dB at 6720 RPM).
4. The cycle variation and the jet noise add a floor of -19 to -33 dB and -24 to -45 dB. The cycle variation sits mostly below 500 Hz, where it sounds as roughness more than static.
5. The model artefact: each pulse starts on a whole sample, and the fraction of a sample from the crank is lost. The firing times move by up to one sample (20.8 microseconds) from cycle to cycle. This turns the upper harmonics into noise in every render, at every RPM and with every layer off. The silent engine (every random and noise layer off) has a residual of -47 to -51 dB at idle, -33 to -39 dB at 3000 RPM and -25 to -37 dB at 80 per cent of the redline, far above the float32 floor. Above 6 kHz, this noise is only 10.7 to 12.2 dB below the harmonics (18.3 dB for the v8 at 4800 RPM). In the stock profiles, the noise layers cover it by about 11 dB or more, but it is the floor that stays when the layers get quieter.

The slow jitter (`[pulse] jitter`) gives a large residual, but most of it lies within a few hertz of each harmonic: it is a pitch wander, not static. Wave steepening, collector overlap, the soft clipper, the peak limiter and the 32-sample control block change the residual by at most 2 dB. The model has no fractional delay.

Against engine-sim, snarl has 12 to 28 dB more non-periodic power from 500 Hz to 6 kHz at 3000 RPM and above, for flat6, inline4 and v8.

Recommendations, in this order: lower the noise AM at idle and middle RPM (option A), band-limit the valve tick (option B), and start each pulse at its fractional position with an end at zero (option C). Do not add oversampling or per-sample control ramps (options D and E), which address no measured source.

## Method

- Residual: `snarl-compare --residual --scenario S.scn A.wav [B.wav ...]` (code in `internal/analysis/residual.go`). For each hold after the settle time, it removes the DC and finds the cycle period within 0.5 per cent of the nominal RPM that gives the least residual: a coarse search with linear interpolation, then two parabola fits with a Kaiser-windowed sinc interpolator (128 taps, passband to about 21 kHz at 48 kHz). It resamples whole 720-degree cycles to the crank-angle domain with at least the input rate, subtracts the synchronous average cycle and scales the residual power by k/(k-1) for k cycles. It reports the residual power relative to the signal power in dB, and the residual in the bands 20 to 500 Hz, 0.5 to 2 kHz, 2 to 6 kHz, 6 to 16 kHz and 16 to 24 kHz, both relative to the whole signal and relative to the signal in the same band.
- Unit tests (`residual_test.go`): a periodic signal with a fractional cycle length and harmonics up to 20 kHz gives -135 to -143 dB. White noise 20, 40 and 60 dB below the signal is measured within 0.1 dB in total and within 1 dB in the bands from 500 Hz to 16 kHz.
- Floor: the noise floor between the harmonics with the method of `docs/research/hiss-analysis.md` (`hissMeasure` in `hiss_test.go`). The residual also counts a slow pitch or gain wander. The floor does not, because that wander stays within 0.15 of a half order of each harmonic. At idle, the half orders are 5 to 8 Hz apart, and the leakage of the Hann window limits the floor to about -28 to -36 dB.
- Holds: idle, 3000 RPM and 80 per cent of the redline, at throttle 1 and 0.5, 3 s each with 0.5 s settle (scenarios in `scenarios/static/<profile>/holds.scn`). Throttle 0.5 is load 0.5.
- Silent engine: these `--set` overrides turn off every random and noise layer and the pops:

  `noise.am=0 noise.jet=0 noise.intake=0 mechanical.tick_gain=0 pulse.cycle_variation=0 pulse.jitter=0 idle.variation=0 idle.timing_deg=0 idle.hunt=0 idle.rock=0 idle.imbalance=0 pops.density=0`

  The fixed per-cylinder gain and timing spreads (`gain_variation`, `timing_variation_deg`) and the fan sine stay on, because they repeat every cycle.

## Silent engine

Residual in dB relative to the signal, at throttle 1 / 0.5:

| Profile | Idle | 3000 RPM | 80 % redline |
| --- | ---: | ---: | ---: |
| flat12 (1000, 3000, 5440) | -47.1 / -47.2 | -33.3 / -32.8 | -26.3 / -26.2 |
| inline4 (800, 3000, 6720) | -50.1 / -49.9 | -38.1 / -39.0 | -25.0 / -25.0 |
| flat6 (800, 3000, 5440) | -48.1 / -48.2 | -39.4 / -39.0 | -26.9 / -26.9 |
| v8 (675, 3000, 4800) | -50.7 / -50.7 | -34.2 / -34.0 | -34.3 / -37.2 |

The float32 floor is below -120 dB. The residual is 70 to 95 dB above it.

Residual in each band relative to the signal in the same band, at throttle 1:

| Profile | Hold | 20-500 | 0.5-2k | 2k-6k | 6k-16k | 16k-24k |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| flat12 | 1000 | -48.3 | -30.1 | -19.2 | -11.5 | -4.2 |
| flat12 | 5440 | -35.7 | -27.3 | -18.0 | -10.7 | -4.4 |
| inline4 | 800 | -50.3 | -28.3 | -18.4 | -11.6 | -4.2 |
| inline4 | 6720 | -39.3 | -24.9 | -19.1 | -11.9 | -4.1 |
| flat6 | 800 | -48.6 | -28.0 | -18.3 | -11.3 | 0.8 |
| flat6 | 5440 | -36.1 | -27.4 | -18.4 | -11.9 | -3.5 |
| v8 | 675 | -51.4 | -29.7 | -19.3 | -11.8 | -3.9 |
| v8 | 3000 | -40.3 | -30.9 | -19.7 | -11.8 | -4.4 |

The ratio rises by about 10 dB per band towards high frequencies, as a timing error does: its noise grows with frequency times the timing error.

## Artefact sources

### Processing stages and sample rate

Total residual in dB at throttle 1, idle / 3000 RPM / 80 % redline. "Linear" has the steepening and the overlap off and the output 20 dB lower, which keeps the soft clipper near linear and the peak limiter out of action. The residual is a ratio, so the lower output level needs no other compensation.

| Change | flat12 | inline4 | flat6 | v8 |
| --- | --- | --- | --- | --- |
| silent | -47.1 / -33.3 / -26.3 | -50.1 / -38.1 / -25.0 | -48.1 / -39.4 / -26.9 | -50.7 / -34.2 / -34.3 |
| `exhaust.steepening=0` | -47.1 / -33.0 / -25.8 | -50.3 / -38.2 / -25.1 | -47.9 / -39.4 / -26.8 | -50.7 / -34.1 / -34.1 |
| `exhaust.overlap=0` | -46.7 / -32.5 / -25.8 | -50.1 / -38.1 / -25.3 | -46.2 / -38.5 / -26.9 | -50.2 / -33.1 / -33.6 |
| `voice.output_db` -20 dB | -47.1 / -33.3 / -26.2 | -49.9 / -38.1 / -24.9 | -48.1 / -39.4 / -26.9 | -50.7 / -34.2 / -34.3 |
| linear | -46.5 / -32.3 / -25.5 | -50.2 / -38.0 / -25.3 | -45.9 / -38.5 / -26.9 | -50.2 / -33.1 / -33.4 |
| `mechanical.fan_gain=0` | no change | no change | -50.4 / -39.5 / -26.6 | no change |
| silent, `--rate 96000` | -53.1 / -38.9 / -32.3 | -56.2 / -44.4 / -30.7 | -51.1 / -43.9 / -30.5 | -56.6 / -40.3 / -35.5 |
| linear, `--rate 96000` | -52.4 / -38.0 / -31.6 | -56.3 / -44.3 / -31.0 | -47.7 / -42.2 / -27.7 | -56.1 / -39.2 / -34.6 |

- No nonlinear stage is a source. Each stage changes the residual by at most 2.2 dB, and turning a stage off never lowers it by more than 0.5 dB. The steepening adds aliasing from 16 to 24 kHz: with it off, that band falls from -79 to -98 dB to -97 to -110 dB relative to the signal at 80 % redline. This is 50 dB or more below the other residual and is not audible.
- At 96 kHz, the residual falls by 5.6 to 6.3 dB for flat12, inline4 and v8 at idle and 3000 RPM. A timing error of up to one sample halves at twice the rate, which gives 6 dB. Aliasing would fall by more, and mostly in the top bands. The v8 at 4800 RPM is a special case: its cycle is 1200 samples at 48 kHz (see the next section). The flat6 falls by 3.0 to 4.5 dB, because its fan adds about -52 dB at idle.

### Control block size

`TestStaticControlBlock` (`internal/model/static_test.go`) renders the silent engine at throttle 1 with the control block at 32, 8 and 1 samples through the hook `controlBlock`. The residual in dB:

| Profile | Idle | 3000 RPM | 80 % redline |
| --- | --- | --- | --- |
| flat12 | -47.1 / -47.2 / -47.3 | -33.0 / -32.7 / -32.9 | -26.3 / -26.3 / -26.3 |
| inline4 | -50.1 / -49.9 / -49.7 | -38.7 / -39.4 / -38.3 | -25.0 / -25.0 / -24.9 |
| flat6 | -48.1 / -48.1 / -48.9 | -39.2 / -39.0 / -39.1 | -27.1 / -27.1 / -27.1 |
| v8 | -50.7 / -50.7 / -51.0 | -34.1 / -34.2 / -34.1 | -41.7 / -83.8 / -29.3 |

At a constant hold, the block-rate controls do not step, and the residual changes by at most 1.1 dB. The v8 at 4800 RPM has a cycle of exactly 1200 samples, so the result depends on the float32 value at which the smoothed RPM settles, not on stepping. The test also checks that the hook at 32 samples gives the same samples as the model with no hook.

### Pulse edges and the cycle length

At `8b1fba5`, `TestPulseEdges` (`internal/model/pulse_test.go`) ran one voice with the pulse shape of each tuned profile at 800, 3000 and 6000 RPM. The test now checks the fractional start of the [Fix](#fix) section. At `8b1fba5`, it found:

- The pulse starts at 0, so its start has no step.
- The pulse stops at its last sample at -50.2 to -60.6 dB below its peak (v8 -50.4, flat6 -54.8, inline4 -59.2, inline4 cam -60.6, flat12 -59.5 dB). The step is small, and it repeats every cycle except for its timing.
- A pulse that the crank places 0.9 of a sample earlier gives the same samples and is one sample shorter. The envelope starts on a whole sample, and the fraction of the sample from the crank (`crank.past`) and from the primary pipe delay is lost.

`TestStaticIntegerPeriod` measures the silent engine where the cycle is a whole number of samples, so that the rounding of the start times repeats in every cycle:

| Profile | 2812.5 RPM (2048 samples) | 2850 RPM (2021.05 samples) | 5625 RPM (1024 samples) | 5600 RPM (1028.57 samples) |
| --- | ---: | ---: | ---: | ---: |
| flat12 | -109.6 | -33.7 | -116.6 | -26.8 |
| inline4 | -117.7 | -39.1 | -91.9 | -29.8 |
| flat6 | -91.6 | -39.1 | -73.1 | -26.2 |
| v8 | -127.9 | -34.7 | -119.2 | -27.5 |

A whole number of samples per cycle removes 47 to 93 dB of residual. The rounding of the pulse start to the sample grid is the source of the silent-engine residual. Everything else in the model, with the fan and the nonlinear stages on, is at -73 to -128 dB.

There is no fractional delay to test. Every delay line reads a whole number of samples, and the primary pipe delay moves the pulse start, so its fraction is lost with the start fraction.

## Noise layers

Each layer is on alone over the silent engine, at its profile value. Total residual in dB at throttle 1, idle / 3000 RPM / 80 % redline:

| Layer | flat12 | inline4 | flat6 | v8 |
| --- | --- | --- | --- | --- |
| silent | -47.1 / -33.3 / -26.3 | -50.1 / -38.1 / -25.0 | -48.1 / -39.4 / -26.9 | -50.7 / -34.2 / -34.3 |
| AM | -14.6 / -12.3 / -21.3 | -17.6 / -18.1 / -23.1 | -16.8 / -18.9 / -23.2 | -12.6 / -13.8 / -23.9 |
| cycle variation | -21.5 / -18.1 / -18.4 | -31.8 / -29.5 / -23.0 | -31.4 / -29.9 / -23.3 | -28.3 / -23.0 / -23.3 |
| jitter | -33.5 / -17.2 / -18.7 | -36.9 / -24.2 / -14.1 | -34.0 / -25.9 / -11.7 | -34.6 / -19.1 / -16.4 |
| intake | -41.0 / -31.3 / -25.8 | -30.5 / -27.6 / -16.4 | -32.6 / -30.4 / -23.3 | -35.2 / -29.5 / -29.8 |
| tick | -43.9 / -31.3 / -25.6 | -46.7 / -36.6 / -24.8 | -30.9 / -23.7 / -15.9 | -44.0 / -31.4 / -30.8 |
| jet | -47.1 / -32.8 / -25.8 | -50.0 / -37.6 / -24.7 | -47.9 / -36.2 / -23.5 | -50.3 / -31.3 / -28.0 |
| all layers (stock) | -14.1 / -10.8 / -13.2 | -17.2 / -16.6 / -12.0 | -16.4 / -16.8 / -10.6 | -12.9 / -12.2 / -14.2 |

Noise floor between the harmonics in dB (`TestStaticLayerFloor` in `static_test.go`), at throttle 1:

| Layer | flat12 | inline4 | flat6 | v8 |
| --- | --- | --- | --- | --- |
| silent | -36.2 / -44.5 / -27.6 | -35.6 / -49.1 / -27.1 | -35.6 / -49.3 / -28.6 | -28.0 / -45.6 / -76.6 |
| AM | -14.6 / -12.8 / -21.8 | -17.6 / -17.9 / -24.4 | -16.7 / -18.9 / -24.1 | -12.7 / -13.7 / -24.2 |
| cycle variation | -21.2 / -19.1 / -18.8 | -33.3 / -29.5 / -24.2 | -30.5 / -31.0 / -24.5 | -25.7 / -23.3 / -24.5 |
| jitter | -35.7 / -32.3 / -27.5 | -34.8 / -40.6 / -26.6 | -35.4 / -38.9 / -28.8 | -27.8 / -34.1 / -30.3 |
| intake | -35.1 / -34.9 / -27.2 | -29.7 / -27.3 / -17.0 | -30.7 / -31.2 / -24.4 | -27.2 / -31.1 / -32.0 |
| tick | -35.9 / -36.3 / -26.9 | -35.5 / -42.2 / -26.8 | -30.7 / -25.2 / -17.4 | -27.9 / -35.6 / -34.8 |
| jet | -36.2 / -40.4 / -27.0 | -35.6 / -45.0 / -26.5 | -35.6 / -38.7 / -24.3 | -28.0 / -33.9 / -28.8 |
| all layers (stock) | -14.1 / -11.7 / -16.7 | -17.3 / -17.2 / -16.0 | -16.4 / -17.6 / -16.0 | -12.8 / -13.2 / -19.0 |

- The jitter gives up to -11.7 dB of residual but a floor of only -26.6 to -40.6 dB. It is a slow wander, with its power next to the harmonics. From the profile values, its timing wander is about 20 microseconds RMS (an estimate, not measured).
- The tick is the main residual above 16 kHz for every profile: in the stock render, that band is -22.0 to -54.6 dB relative to the signal, and the tick alone gives the same value within 1.3 dB. The tick high pass (`tick_hz`, 2.5 to 3 kHz) has no upper limit, so the noise runs to the Nyquist frequency.
- The `[idle]` effects act only below full load. At throttle 0.5 and idle, they give -6.6 dB (flat12), -15.6 dB (flat6) and -13.8 dB (v8), most of it from `variation` and `timing_deg` below 500 Hz. The inline4 has no `[idle]` values. At the holds, the pops add nothing.

## Against engine-sim

Residual of `holds-t100` in dB relative to the signal, snarl / engine-sim. engine-sim has its own noise. The flat6 and inline4 references are valid to 10 kHz and the v8 and flat12 references to 5 kHz (see `hiss-analysis.md`), so the table leaves out the bands above 6 kHz.

| Profile | Hold | Total | 0.5-2k | 2k-6k |
| --- | ---: | ---: | ---: | ---: |
| flat12 | 1000 | -14.1 / -5.9 | -20.1 / -23.1 | -17.9 / -28.3 |
| flat12 | 3000 | -11.1 / -9.2 | -16.6 / -26.9 | -15.5 / -29.8 |
| flat12 | 6000 | -14.0 / -9.7 | -18.4 / -24.1 | -17.9 / -29.3 |
| inline4 | 800 | -17.2 / -24.2 | -21.7 / -38.9 | -20.6 / -48.3 |
| inline4 | 3000 | -17.2 / -30.5 | -21.3 / -43.2 | -21.3 / -43.0 |
| inline4 | 6000 | -11.3 / -33.0 | -14.1 / -42.3 | -16.0 / -43.0 |
| flat6 | 800 | -16.4 / -16.6 | -22.7 / -24.2 | -18.8 / -25.1 |
| flat6 | 3000 | -16.9 / -29.2 | -22.9 / -34.8 | -20.8 / -33.9 |
| flat6 | 6000 | -12.6 / -30.5 | -19.9 / -36.5 | -18.1 / -36.4 |
| v8 | 675 | -12.9 / -16.5 | -18.6 / -25.5 | -15.0 / -26.3 |
| v8 | 3000 | -12.5 / -26.6 | -16.5 / -30.7 | -16.1 / -31.2 |
| v8 | 6000 | -12.0 / -30.3 | -13.6 / -34.6 | -18.1 / -34.7 |

Above 2 kHz, both are mostly noise (the residual is within 1 dB of the signal in the band for engine-sim and within 11 dB for snarl), so the level of the noise is the difference. From 3000 RPM up, snarl has 12 to 28 dB more non-periodic power from 500 Hz to 6 kHz for inline4, flat6 and v8. The flat12 reference is noisy itself below 500 Hz.

## Sources ranked by audibility

| Rank | Source | Where | Floor or residual | Band |
| ---: | --- | --- | --- | --- |
| 1 | Noise AM | all profiles, idle to about 4000 RPM | floor -12.7 to -18.9 dB | 0.5 to 6 kHz |
| 2 | Valve tick | flat6 at all RPM, all profiles above 16 kHz | floor -17.4 dB (flat6 5440 RPM) | 6 to 24 kHz |
| 3 | Intake noise | inline4 near the redline | floor -17.0 dB (6720 RPM) | 20 Hz to 2 kHz |
| 4 | Cycle variation | all, most in flat12 | floor -18.8 to -33.3 dB | 20 to 500 Hz |
| 5 | Jet noise | all, near the redline | floor -24.3 to -28.8 dB | 1 to 2 kHz |
| 6 | Pulse start on whole samples | every render, rises with RPM | floor -27 to -29 dB at 80 % redline, residual -25 to -51 dB (silent engine) | 2 to 24 kHz, -11 dB in band above 6 kHz |
| 7 | Jitter | all | floor -26.6 to -40.6 dB | next to the harmonics |

Not sources: wave steepening, collector overlap, soft clipper, peak limiter, control block stepping, fan, pops at a hold.

## Fix options

### Option A (recommended): lower the noise AM at idle and middle RPM

The AM keeps its full depth up to about 3000 RPM. Options: start the depth taper and the low pass move from idle at a steeper curve, use the two-pole `am_cutoff_hz` low pass at every RPM instead of the one pole at 2 kHz, or lower `noise.am` per profile. With `noise.am=0`, the floor at 2000 RPM falls by 9 to 13 dB (`hiss-analysis.md`), so a halved depth can give about 6 dB. The AM also gives the combustion texture, so set the amount by ear. Cost: S (model default or profile values, `hiss_test.go` bounds, a listening pass). Risk: low. Measure: `TestStaticLayerFloor` and `snarl-compare --residual` on `holds-t100`.

### Option B (recommended): band-limit the valve tick

Add a low pass, for example 8 to 10 kHz, after the tick high pass, or make it a band pass at `tick_hz`. Consider a lower `tick_gain` for the flat6 (0.05). This removes most of the content above 16 kHz for every profile and the flat6 floor at high RPM. Cost: XS for a profile value, S for a filter in the model with a new key or default. Risk: low, the tick level changes only above the new cutoff. Measure: the 16 to 24 kHz and 6 to 16 kHz bands of `snarl-compare --residual`.

### Option C (recommended): fractional pulse start and an end at zero

When a voice starts, set its envelope state to the value at the fraction of a sample by which the start lies before the sample (`e1 = ka^f`, `e2 = kb^f`, with `f = v.pos / step`). End the pulse at zero, for example with a fade over the last few samples or by subtracting the end value. The cost is two powers per firing and no cost per sample. The integer-period test bounds the gain: up to 47 to 93 dB less residual for the silent engine. The corner of the attack at a fractional position still aliases a little, so the real gain is smaller. In the stock profiles the noise layers cover this source by 11 dB or more, so its audible effect grows after options A and B. All renders change, so the calibration (`TestOutputCalibration`) and the hiss bounds need a check. Cost: S. Risk: low.

### Option D (not recommended): 2x oversampling around the nonlinear stages

With the steepening, the overlap and the soft clipper off, the residual changes by at most 2.2 dB. The only aliasing is above 16 kHz at -80 dB or lower relative to the signal. Oversampling would double the cost of the paths for no audible gain. Cost: M.

### Option E (not recommended): per-sample ramps for the stepped settings

The control block size from 1 to 32 samples changes the residual at a hold by at most 1.1 dB. Sweeps were not measured. Cost: M.

### Option F (not applicable): a better fractional delay

The model has no fractional delay. Option C covers the lost fraction of the primary pipe delay.

### Option G: less noise in a layer

Lower the inline4 intake above the cam switch (`intake_cam_gain` 1.2) if its high RPM sounds noisy, and keep the cycle variation, the jet and the jitter, which give roughness, flow and wander rather than static. Cost: XS each. Set by ear.

## Fix

Options A, B and C are in the model. The four tuned profiles set `[noise] am_idle = 0.5` (option A, medium) and `[mechanical] tick_lp_hz = 9000` (option B). These values are also the defaults, so the catalogue profiles get the same change.

### Option C: fractional pulse start and an end at zero

- Each pulse starts at its fractional sample position. When a voice starts, its envelope state is the envelope at the fraction of a sample by which the start lies before the sample: two `math.Exp` per firing, none per sample.
- The start of the envelope is a corner, a step of its slope. A two-sample polynomial band-limited ramp (polyBLAMP) rounds it: the sample before the start and the first sample each add a cubic of the fraction times the slope. So that the sample before the start is still to come, every pulse starts one sample (20.8 microseconds at 48 kHz) later than before.
- The pulse subtracts its envelope end value along its length, `env(1)*x`, so that it ends at zero. The peak changes by less than 0.01 dB.
- Cost per sample: one multiply-add per active voice, and a corner term on two samples per firing. `BenchmarkRender` (median of 5 runs of 2 s, 256-frame blocks) gave 12951, 10237, 15206 and 14955 ns per block before and 13704, 10697, 12998 and 13684 ns after, for flat12, inline4, flat6 and v8, with 0 allocations. The differences are in both directions and within the noise of the machine.

Residual of the silent engine in dB, at throttle 1 / 0.5 (`TestStaticFractionalStart` in `internal/model/static_test.go`, before at `7ed553f`):

| Profile | Idle before | Idle after | 3000 RPM before | 3000 RPM after | 80 % redline before | 80 % redline after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| flat12 | -47.1 / -47.1 | -111.9 / -111.9 | -33.0 / -32.9 | -90.5 / -91.2 | -26.3 / -26.2 | -85.9 / -85.4 |
| inline4 | -50.1 / -50.1 | -118.7 / -118.9 | -38.7 / -38.7 | -104.4 / -103.9 | -25.0 / -25.0 | -81.6 / -81.4 |
| flat6 | -48.1 / -48.0 | -52.3 / -52.0 | -39.2 / -39.2 | -53.7 / -53.4 | -27.1 / -27.1 | -48.2 / -47.9 |
| flat6, fan off | | -118.8 / -118.8 | | -104.5 / -104.3 | | -88.1 / -87.8 |
| v8 | -50.7 / -50.7 | -118.8 / -118.8 | -34.1 / -34.0 | -102.2 / -102.1 | -41.7 / -41.6 | -108.2 / -107.8 |

- For flat12, inline4 and v8, the residual falls by 57 to 69 dB, to -81 to -119 dB, in the range of the integer-period values of `8b1fba5` (-73 to -128 dB). The test fails above -75 dB with the fan off.
- The flat6 fan is the rest of its residual: -48 to -54 dB. The fan sine runs on its own float32 phase, whose rounding differs from that of the crank phase, so the fan frequency is not an exact multiple of the cycle frequency. This is a slow drift of a pure tone, not static. With a cycle of exactly 2048 samples, where both phases are exact, the flat6 residual is -121 dB.
- `TestStaticIntegerPeriod` now gives -116.3 to -121.0 dB at 2812.5 RPM, -86.1 to -105.1 dB at 2850 and 5600 RPM except the flat6 (-47.3 and -43.6 dB, the fan), and -79.1 to -117.6 dB at 5625 RPM. A whole number of samples per cycle no longer changes the residual much.
- With the fan off, the residual from 6 to 16 kHz is -54 to -74 dB relative to the signal in the same band, against -11 dB before. The band from 16 to 24 kHz keeps -3 to -28 dB relative to its own small signal, but this is part of the total residual of -81 dB or lower.

With every layer on (the stock profiles):

- Level: `TestStaticWholeSampleLevel` compares the fix with the pulses on the whole sample, through the test hook `SetWholeSampleStart`, on `holds-t000`, `holds-t100` and `sweep-wot`. The loudness changes by -0.012 to -0.020 LU and the RMS level by -0.003 to -0.012 dB. The hook gives renders that are bit-identical to `7ed553f`.
- Pops: `TestStaticWholeSamplePops` finds the pops alone, requested at full throttle and own pops of a lift-off, within -0.050 to 0.000 dB with a correlation of at least 0.99996.
- Floor between the harmonics, before options A and B: on `holds-t100`, the floor falls by 0 to 0.2 dB below 4000 RPM and by up to 1.1 dB at the top holds. On `holds-t000`, where the throttle is closed and the noise AM is almost off, it falls by up to 4.7 dB (inline4 at 8400 RPM, -20.7 to -25.4 dB).
- `TestOutputCalibration` and the hiss tests pass with no change of values.

In the stock profiles, the noise layers still cover most of the gain. Options A and B lower those layers.

### Options A and B: the AM depth at idle and the tick low pass

- `[noise] am_idle` (0.5, from 0 to 1): the AM depth at idle as a fraction of `am`. The depth moves from `am_idle` at idle to `am_redline` at the redline along the smoothstep of the hiss fix. At 1, the AM is the AM of `7ed553f`.
- `[mechanical] tick_lp_hz` (9000 Hz, 0 for off, else from 1000 to 20000 Hz): a two-pole Butterworth low pass after the valve tick high pass, with its coefficients set once in `New`. It runs only while the tick envelope sounds.

The choice came from a listening comparison of `am_idle` 0.7, 0.5 and 0.35 (a light, medium and strong option A) and `tick_lp_hz` 12000, 9000 and 6000 (option B). The medium option A and 9 kHz were chosen. 9 kHz removes mostly content above 16 kHz. On `holds-t100`, before the choice:

- Option A lowered the floor at idle by 2.3 to 2.8 dB (light), 4.2 to 5.3 dB (medium) and 5.6 to 7.8 dB (strong), and at 3000 RPM by 1.8 to 2.2, 3.1 to 3.9 and 4.0 to 5.4 dB. From 6000 RPM up, it changed the floor by at most 0.4 dB. At throttle 0, the AM is almost off, so option A changed the floor by at most 0.5 dB.
- Option B lowered the residual from 16 to 24 kHz, relative to the whole signal, by 3 to 14 dB at 12 kHz, 4 to 18 dB at 9 kHz and 4 to 22 dB at 6 kHz: the most for the flat6, which has the loudest tick, and the least for the inline4. The floor changed by at most 0.3 dB, except for the flat6, where it fell by up to 1.5 dB at full throttle and 2.9 dB at throttle 0 (6 kHz).

Floor between the harmonics on `holds-t100` in dB, before options A and B / with both / engine-sim (`TestStaticFixFloor` in `static_test.go`). "Before" has `noise.am_idle=1` and `mechanical.tick_lp_hz=0`, with option C:

| Profile | Idle | 3000 RPM |
| --- | ---: | ---: |
| flat12 (1000 RPM) | -14.1 / -18.3 / -4.9 | -11.6 / -15.0 / -8.6 |
| flat6 (800 RPM) | -16.4 / -21.6 / -16.4 | -17.7 / -21.5 / -29.2 |
| inline4 (800 RPM) | -17.3 / -22.4 / -24.9 | -17.5 / -21.4 / -29.1 |
| v8 (675 RPM) | -12.8 / -18.1 / -16.5 | -13.2 / -16.8 / -26.6 |

- The floor falls by 4.2 to 5.3 dB at idle and by 3.4 to 3.9 dB at 3000 RPM. Option A gives almost all of it. Option B adds at most 0.7 dB (flat6 at 3000 RPM).
- At idle, the floor is now at or below the engine-sim floor, except for the inline4 (2.5 dB above). At 3000 RPM, it is still 7.7 dB above engine-sim for flat6 and inline4 and 9.8 dB above for the v8. The flat12 reference is noisy itself.
- `TestStaticFixFloor` fails when a final floor at idle or 3000 RPM is above its bound (-17.5 and -14.5 dB flat12, -21 and -21 dB flat6, -22 and -21 dB inline4, -17.5 and -16.5 dB v8) or less than 3 dB below the floor before.
- `TestHissIdle` now checks option A against the noise AM of `a553aef`: on `holds-t100`, the floor is at least 3.5 dB lower at idle and 2000 RPM (measured 4.2 to 6.2 dB), and on every holds scenario the 4 to 12 kHz level and the floor do not rise, the HNR does not fall and the RMS level stays within 0.3 dB (measured -0.26 to 0.00 dB).
- `TestHissHighRPM`: for each tuned profile, the worst 4 to 12 kHz level of the high holds changes by at most 0.3 dB and the worst HNR does not fall, so their bounds do not change. The tick low pass raises the 4 to 12 kHz level of the top hold of `es-v12-60-aero` and `el-v12-60-aero` by 1.3 and 1.4 dB, to -37.4 and -48.2 dB, so their bounds are now -36.5 and -47.5 dB. The tick, the jet and the intake noise share one noise source, and the low pass changes the phase of the tick against the other two. With the other layers off, the tick low pass lowers the 4 to 12 kHz level as expected.
- Level: `sweep-wot` is at -16.34, -16.18, -16.44 and -16.85 LUFS for flat12, inline4, flat6 and v8 (before -16.21, -16.13, -16.31 and -16.68), within 1 LU of -16 LUFS, so `output_db` does not change. The v8 is 0.15 LU from the bound. In `TestLoadLevelOrder`, the closed and the half throttle levels rise by up to 0.22 and 0.14 dB relative to full throttle, and no margin of the test moves by more than 0.2 dB. The overrun level and the pop levels move by at most 0.1 dB.
- Cost of option B: `BenchmarkTickLowPass` (in `bench_layers_test.go`) runs the state of `BenchmarkRender` and the four states of `BenchmarkStates` with `tick_lp_hz` 0 and 9000 one after the other, 10 runs of 0.5 s. The geometric mean goes from 12.79 to 12.96 microseconds per 256-frame block (+1.36 per cent, about 170 ns), that is from 0.240 to 0.243 per cent of real time, with 0 allocations. The single states change by -5.5 to +9.0 per cent in both directions, which is within the noise of the machine.

## Reproduce

- Residual of any render: `nix develop -c just compare A.wav B.wav --residual --scenario S.scn` or `nix develop -c go run ./cmd/snarl-compare --residual --settle 0.5 --scenario S.scn A.wav [B.wav ...]`. The output is Markdown on standard output.
- Renders: `nix develop -c go run ./cmd/snarl-render --format f32 [--rate 96000] --set <key>=<value> ... --out out/tmp/static/wav/<profile>/<name>.wav <profile> scenarios/static/<profile>/holds.scn`, with the silent-engine overrides of the method, less the keys of a layer to turn it back on. The scenario files hold `hold <rpm> <throttle> 3` lines for the idle, 3000 RPM and 80 % redline holds at throttle 1 and 0.5.
- Tests: `nix develop -c go test -run 'TestStatic|TestPulseEdges' -v ./internal/model/` for the control block, the integer cycle, the fractional start, the level and pop checks of the fix and the pulse edges, `nix develop -c go test -run 'TestStaticLayerFloor|TestStaticFixFloor' -v .` for the floor per layer and the floor of options A and B, and `nix develop -c go test -run Residual -v ./internal/analysis/` for the measurement. The static tests do not run with `-short`.
- Cost of option B: `nix develop -c go test -run '^$' -bench BenchmarkTickLowPass -benchtime 0.5s -count 10 . > out/bench/tick-lp.txt`, then `nix develop -c benchstat -col /tick_lp_hz out/bench/tick-lp.txt`.
