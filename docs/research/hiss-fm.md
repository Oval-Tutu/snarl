# The FM-like hiss of snarl

Measured on branch `upstream-profiles` at `e28ddaf`, with no change to model code or profiles. Engines: the five catalogue profiles converted from upstream engine-sim engines (`es-i4-dohc-cam-switch`, `es-v8-crossplane-pushrod`, `es-f4-boxer`, `es-v12-75-race`, `es-v12-60-aero`) and the tuned `v8`. Each fix option is marked "from engine-sim" or "new".

## Answer

The hiss comes from the input jitter, and it is in short bursts, one at each firing. engine-sim has the same jitter and the same noise source, but its impulse response spreads each burst over 12 to 46 ms, so its hiss is a steady texture. The snarl formant bank rings for only a few milliseconds and keeps the bursts. At a low firing rate, the bursts are a flutter of 40 to 60 per second, which sounds like a badly tuned FM radio.

Evidence:

- With `noise.input_jitter=0`, the snarl residual from 2 to 16 kHz falls by 8 to 57 dB. With `noise.am=0` or `exhaust.overlap=0`, it changes by at most 1.5 dB. The input jitter makes all of the hiss.
- The new burst measure (`snarl-compare --burst`) gives the cycle-locked modulation of the hiss envelope. The worst hold is `es-v12-60-aero` at 600 RPM at closed throttle: +2.5 dB in snarl against -9.2 dB in engine-sim (2 to 4 kHz), with an envelope crest factor of 10.0 dB against 5.1 dB. engine-sim has 3 to 5 dB more hiss there, but with no flutter.
- The gap is largest for the engines whose impulse response puts its high frequencies late or spreads them (`minimal_muffling_01`, `smooth_39`), and at low RPM. It is small for the engines with `mild_exhaust`, whose high frequencies come in the first 3 ms, as in the snarl bank.

Recommended fix: a high-frequency diffuser after the formant bank. It spreads the part above about 2 kHz over the time spread of the engine-sim impulse response with a short chain of allpass filters (new in form, engine-sim behaviour), for about 18 operations per sample. Section 5 has the options.

## 1. The burst measure

`snarl-compare --burst --scenario S.scn A.wav [B.wav ...]` (new option, `cmd/snarl-compare/burst.go`, `internal/analysis/burst.go`) takes the residual of each hold, as `--residual` does: whole engine cycles in the crank angle domain, less the mean cycle. In the bands 2 to 4, 4 to 8 and 8 to 16 kHz, it takes the band power of 5 ms Hann frames, 96 frames per cycle, as the band envelope, and gives:

| Column | What it measures | Steady noise | Noise in bursts |
| --- | --- | --- | --- |
| Level | Residual band power relative to the whole signal | | |
| Mod | Variance of the band envelope over its squared mean | -7, -10 and -13 dB in the three bands | above +3 dB |
| Locked | The part of Mod that repeats with the engine cycle | at the floor (-200 dB) | above +3 dB when the bursts follow the cycle |
| Crest | 99th percentile of the envelope over its mean | 2 to 4 dB | 7 to 9 dB |
| Flatness | Spectral flatness in the band, weighted by the frame power | 0.55 to 0.65 | the same, a tone gives below 0.1 |

The tests (`TestBurstSteadyAndBurst`, `TestBurstTone`, `TestResidualBurst`, `TestBursts`) check that a steady noise and a noise with the same power in the first quarter of each cycle give values that differ by 12 dB or more in Mod, a Locked at the floor against above +3 dB, and the same level and flatness. `Residual` now shares its cycle search with the new measure (`residualCycles`), and its tests pass unchanged.

## 2. Where the hiss is

Renders: `out/hiss2/snarl/<id>/holds-t000.wav` and `holds-t100.wav`. Tables: `out/hiss2/burst-<id>-t000.md` and `-t100.md`, each with engine-sim first and snarl second. The references `es-f4-boxer` and `es-v8-crossplane-pushrod` were rendered with `just ref-set catalogue/<id>`.

The holds with the largest gap, 2 to 4 kHz band, engine-sim against snarl:

| Engine | Impulse response | Hold (RPM, throttle) | Locked (dB) | Mod (dB) | Crest (dB) | Level (dB) |
| --- | --- | --- | --- | --- | --- | --- |
| `es-v12-60-aero` | `minimal_muffling_01` | 600, 0 | -9.2 / +2.5 | -4.1 / +5.2 | 5.1 / 10.0 | -22.6 / -27.1 |
| `es-v12-60-aero` | `minimal_muffling_01` | 600, 1 | -4.9 / +3.5 | -2.2 / +4.9 | 6.0 / 9.2 | -14.9 / -26.9 |
| `es-v12-75-race` | `minimal_muffling_01` | 3810, 1 | -13.1 / -6.3 | -3.9 / -2.6 | 5.1 / 5.7 | -30.4 / -27.2 |
| `es-v8-crossplane-pushrod` | `smooth_39` | 690, 1 | -0.1 / +4.7 | +1.7 / +6.1 | 7.5 / 10.0 | -27.8 / -29.8 |
| `es-v8-crossplane-pushrod` | `smooth_39` | 5000, 1 | -17.8 / -7.1 | -5.1 / -2.4 | 4.7 / 5.8 | -32.0 / -28.2 |
| `v8` (tuned) | `smooth_39` | 675, 0 | +1.5 / +3.6 | +4.5 / +6.6 | 9.4 / 10.3 | -29.1 / -30.6 |
| `es-f4-boxer` | `mild_exhaust` | 6000, 1 | -4.7 / +0.8 | -1.7 / +4.1 | 5.7 / 8.9 | -39.2 / -38.7 |
| `es-f4-boxer` | `mild_exhaust` | 2000, 1 | +1.5 / +4.8 | +3.1 / +6.3 | 9.0 / 10.1 | -45.0 / -41.9 |
| `es-i4-dohc-cam-switch` | `mild_exhaust` | 770, 1 | +2.9 / +5.7 | +4.3 / +6.7 | 9.2 / 10.0 | -50.6 / -55.7 |

The 4 to 8 and 8 to 16 kHz bands give the same order. Two forms of flutter show:

- At low RPM, where the firings are more than about 5 ms apart, the hiss bursts at each firing (the idle rows).
- At higher RPM, the firings are closer than the 5 ms frames, and Locked describes how the hiss level moves over the engine cycle, from one cylinder or bank to the next. The `es-v8-crossplane-pushrod` hold at 5000 RPM (cycle of 24 ms) and the `es-v12-75-race` hold at 3810 RPM are this form. snarl also has 3.8 dB more hiss than engine-sim at the V8 hold.

Where the hiss does not flutter more than in engine-sim:

- From 3000 RPM up, the gap is 3 dB or less for `es-v12-60-aero` and `es-i4-dohc-cam-switch`, and for `es-v8-crossplane-pushrod` at 4000 RPM. At 6000 and 6500 RPM the snarl V8 is smoother than engine-sim.
- `es-v12-75-race` from 5000 RPM up is within 3 dB, except at 16000 and 17000 RPM, where snarl has non-repeating tonal content from 2 to 4 kHz (flatness 0.20 to 0.45) at 6 to 10 dB over engine-sim. This is not hiss, and this report does not follow it further.

This agrees with the report "it depends on the RPM and the profile": the flutter is strongest at low RPM, and on the engines with a late or long impulse response.

The images show the same thing. Over 0.25 s at 600 RPM at full throttle, the snarl spectrogram has one vertical stripe per firing from 2 kHz up, and the engine-sim spectrogram has none ([snarl](../../out/hiss2/spec-es-v12-60-aero-600-snarl.png) against [engine-sim](../../out/hiss2/spec-es-v12-60-aero-600-engine-sim.png)). The phase 4e report saw the same stripes on the tuned v8 idle ([snarl-gap.md](snarl-gap.md), "What the images show").

## 3. The suspects

Mutes with `--set` (`out/hiss2/mute/<id>/`), full-throttle holds, 2 to 4 kHz band, Locked and Level in dB:

| Render | `es-v12-60-aero` 600 | `es-v12-60-aero` 3000 | `es-f4-boxer` 2000 | `es-f4-boxer` 6000 |
| --- | --- | --- | --- | --- |
| engine-sim | -4.9, -14.9 | -6.2, -24.9 | +1.5, -45.0 | -4.7, -39.2 |
| Profile | +3.5, -26.9 | -9.0, -25.8 | +4.8, -41.9 | +0.8, -38.7 |
| `noise.input_jitter=0` | +2.6, -46.7 | -3.1, -51.7 | +4.1, -59.8 | +5.7, -46.5 |
| `noise.am=0` | +3.3, -27.3 | -8.8, -25.9 | +4.5, -42.0 | +0.5, -39.0 |
| `exhaust.overlap=0` | +3.4, -27.1 | -6.5, -24.3 | profile has 0 | profile has 0 |
| `voice.leveller_db` +12 dB | +2.6, -26.1 | -9.0, -25.8 | | |
| Every random layer except the noise at 0 | +1.7, -27.5 | -9.3, -25.9 | | |

With no input jitter, the level falls by 8 to 26 dB from 2 to 4 kHz and by 37 to 57 dB from 8 to 16 kHz, so what is left there is not hiss. The Locked values of that row describe a small tonal rest (flatness 0.15 to 0.42), not the hiss.

The four hypotheses:

1. The hiss comes in bursts with each firing because the bank is short and the engine-sim impulse response is long. Confirmed for the engines with `minimal_muffling_01` and `smooth_39`. The `--ir` tables (`out/analysis/engine-sim/ir/ir.md`) show where the energy above 6 kHz lies in time. For `minimal_muffling_01`, the first 128 taps (2.9 ms) hold almost none of it (-66 dB at 6.3 kHz against -12 dB for the whole response), the first 512 taps (11.6 ms) are 3.5 to 5.8 dB short, and only 2048 taps (46 ms) give it all. For `smooth_39`, the first 1024 taps are 13 to 17 dB short and 2048 taps give it all. For `mild_exhaust`, the first 128 taps give it all within 1 dB. The snarl bank is a cascade of 4 to 10 biquads that ring for a few milliseconds above 2 kHz. The `mild_exhaust` engines have the smallest gap, and engine-sim itself is bursty there (Locked +1.5 to +5.4 dB at low RPM).
2. The snarl input jitter adds broadband noise where the engine-sim jitter mostly shifts the pressure. Rejected. The engine-sim `JitterFilter::fast_f` (`include/jitter_filter.h`) reads a 10-sample history at a random fractional delay of 0 to 9 times `jitter` samples, low-passed at 10 kHz, with linear interpolation, as snarl does. In both, the jitter turns the slope of the pressure into noise: the noise is about the delay noise times the derivative of the pulse, so it is loudest at each pulse rise. The one difference is the low pass of the delay noise: fourth order in engine-sim, second order in snarl.
3. The bank lets through more above 6 kHz. True in part, and not the cause of the flutter. Relative to the 4 kHz octave, the snarl 16 kHz octave is 6 dB louder than in engine-sim (`es-v12-60-aero` at 600 RPM, `out/hiss2/describe-es-v12-60-aero-t100.md`), and the flatness from 8 to 16 kHz is 0.15 to 0.34 in snarl against 0.05 to 0.10 in engine-sim at every hold of every engine. The engine-sim spectrum falls more steeply inside that band. The catalogue banks were fitted only up to 6 kHz (`tools/fit --hi` default). From the profile values, the second-order low pass of the `es-v12-60-aero` bank (5771 Hz, Q 0.587) falls about 14 dB from 6.3 to 15.8 kHz, against 17.5 dB for `minimal_muffling_01` (this is an estimate from the filter formula, not a measurement). The second-order jitter low pass adds to the same band, and the measure cannot split the two without a model change.
4. The collector overlap adds intermodulation products. Rejected: with `exhaust.overlap=0`, the level from 2 to 16 kHz changes by at most 1.5 dB, and Locked moves by at most 2.5 dB, towards engine-sim at some holds and away from it at others (`es-v8-crossplane-pushrod`: 690 RPM +4.7 to +2.8 dB, 5000 RPM -7.1 to -5.6 dB, engine-sim -0.1 and -17.8 dB).

Two smaller findings:

- The fixed cylinder spreads of the import (`pulse.gain_variation` 0.12, `timing_variation_deg` 2, not measured) add cycle-locked modulation at mid RPM. With both at 0, Locked falls on `es-v12-75-race` from -6.3 to -8.8 dB at 3810 RPM and from -11.5 to -17.0 dB at 5000 RPM (engine-sim -13.1 and -18.2 dB), and on `es-v8-crossplane-pushrod` from -7.1 to -10.1 dB at 5000 RPM (engine-sim -17.8 dB). On `es-f4-boxer` they change nothing.
- The leveller and the other random layers move Locked by at most 1.8 dB.

## 4. Listening set

`out/listen-hiss2/`, `es-v12-60-aero` at 600 RPM at closed throttle, 4 s each, all at -20 LUFS (`README.txt` says what to listen for). The scenario, the raw renders and the measures are in `out/hiss2/listen/`.

| File | Render | Locked, 2-4 kHz (dB) | Crest, 2-4 kHz (dB) |
| --- | --- | --- | --- |
| `1-snarl.wav` | profile | +2.3 | 9.1 |
| `2-snarl-no-input-jitter.wav` | `noise.input_jitter=0` | hiss gone (level -45.1 dB against -27.0 dB) | |
| `3-snarl-no-noise-am.wav` | `noise.am=0` (control) | +2.2 | 9.2 |
| `4-engine-sim.wav` | engine-sim | -9.3 | 5.1 |

No mute moves the hiss towards engine-sim: the jitter mute removes it, and engine-sim has hiss. So file 2 confirms the source, not the fix.

## 5. Fix options

CPU costs are per output sample and are estimates. A render costs 0.21 to 0.25 per cent of real time per 256-frame block now (snarl-gap.md phase 4e), and the bank 21 to 36 multiply-adds.

| Option | Origin | What it does | CPU cost | Recommendation |
| --- | --- | --- | --- | --- |
| High-frequency diffuser after the formant bank | New in form, engine-sim behaviour (the time spread of the impulse response) | Split the bank output at about 2 kHz (one low-pass biquad, the high part is the input less the low part). Pass the high part through 3 or 4 Schroeder allpass filters with delays from about 3 to 20 ms and a gain near 0.6, set per profile from the time spread above 2 kHz of its engine-sim impulse response, and add it back. The allpass keeps the band levels, so the residual level and the spectrum stay, and the bursts spread into a steady texture. The stage does not run for an impulse response with a short high band (`mild_exhaust`). | About 18 operations, and about 2000 samples of delay memory at 48 kHz | Yes, first. Test it with `--burst` (Locked within 3 dB of engine-sim on the table of section 2) and by ear on `out/listen-hiss2/`. Check that the allpass does not ring at 2 kHz |
| Velvet-noise FIR on the high part | New | The same split, with a sparse FIR of 30 to 60 random taps of plus or minus a decaying gain over 20 to 40 ms in place of the allpass chain. Closer to the noise-like tail of a recorded response, with no feedback, so no ringing | 30 to 60 additions and a few multiplies, the same delay memory | Yes, as the alternative if the allpass chain rings |
| Partitioned FFT convolution with a snarl-owned 2048-tap response | From engine-sim | The engine-sim form. Within 0.7 dB of the engine-sim response (engine-sim-target.md section 7) | About 100 operations with 256-sample partitions, plus 5.8 ms of latency | No, for the cost and the latency. The engine-sim responses cannot ship, so it also needs a snarl-owned response |
| Measured cylinder spreads for the catalogue profiles | From engine-sim (measured, `tools/fit --pulse --variation`) | Replace the imported `gain_variation` 0.12 and `timing_variation_deg` 2 with the measured spreads, as flat6 and v8 do | None | Yes, in the import. It takes 2.5 to 5.5 dB of the `es-v12-75-race` gap at 3810 to 5000 RPM and 3 dB of the `es-v8-crossplane-pushrod` gap at 5000 RPM. Check the level at 11000 RPM and up, which rises by up to 2 dB with no spread |
| Fit the catalogue banks to 16 kHz | From engine-sim (the response curve) | `tools/fit --hi 16000`, as the tuned profiles do, so that the bank falls as steeply as the response from 6 to 16 kHz | 0 to 2 more sections, at most 10 multiply-adds | Yes, cheap. It fixes the colour of the top octave, not the flutter |
| Fourth-order jitter low pass | From engine-sim | One more biquad on the shared jitter delay noise, as in `JitterFilter` | About 5 multiply-adds per sample, shared by the paths | Only if the 16 kHz refit leaves the 8 to 16 kHz flatness above 0.12. Test it before and after |
| Mute or lower the input jitter | From engine-sim (its parameter) | Removes the hiss | Saves about 7 operations per path | No. engine-sim has the hiss, about 4 dB louder than snarl at the worst hold |
| Smooth the jitter noise at its source | New | Replace the jitter noise, which follows the pulse slope, with noise on a slow envelope | About 10 operations | No. It changes the engine-sim form of the jitter, and the diffuser gives the same result after the bank, where engine-sim does it |

## 6. Limits

- The burst measure takes the residual, so it describes the hiss only where the harmonics repeat. At 16000 RPM and up on `es-v12-75-race`, the residual holds tonal content too.
- The time spread of each impulse response above 6 kHz comes from the band levels of its truncated FIR filters in the `--ir` tables, not from a band-limited energy decay.
- The remaining gap on the `mild_exhaust` engines (2.8 to 5.5 dB of Locked) has no measured cause. A likely one is the pulse shape: `es-f4-boxer` uses the `inline4` pulse table and `es-v12-60-aero` the `flat12` table, both measured on other engines, and the jitter noise follows the slope of the pulse.
- The renders are at 48 kHz (snarl) and 44.1 kHz (engine-sim). The burst frames are 5 ms at both rates.
