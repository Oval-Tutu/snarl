# Formant bank fit to the engine-sim impulse responses

Measured on branch `overnight/engine-sim-match` at `195ba4e`. No model code or profile changed. The fit tool is `tools/fit` (new), with its maths in `internal/analysis/formant.go`. The gap of the current snarl sound against every engine-sim target is in [snarl-gap.md](snarl-gap.md).

## Answer

A cascade of 7 to 11 biquads follows each engine-sim impulse response within 1 dB (weighted error, 40 Hz to 6 kHz). The fitted values are snarl's own: a centre frequency, a Q and a gain per section, not taps of the response.

| Profile | Impulse response | Chosen N | Weighted error (dB) | Third-octave RMS 50 Hz-2 kHz (dB) | Largest third-octave error 50 Hz-6 kHz (dB) | Multiply-adds per sample |
| --- | --- | --- | --- | --- | --- | --- |
| flat12 | `minimal_muffling_01` | 8 | 1.00 | 1.30 | 1.94 | 41 |
| flat6 | `minimal_muffling_02` | 8 | 0.90 | 0.77 | 1.77 | 41 |
| inline4 | `mild_exhaust` | 7 | 0.87 | 0.54 | 1.09 | 36 |
| v8 | `smooth_39` | 11 | 0.87 | 0.86 | 2.43 | 56 |

The current snarl exhaust path is 8 to 11 dB (third-octave RMS, 50 Hz to 2 kHz) from the same target. It puts its peak at 0.8 to 2 kHz, where the chamber comb and the outlet rise meet. At 250 to 400 Hz it is 8 to 20 dB too weak for flat12, flat6 and v8 (inline4 matches there). At 3 to 5 kHz it is 19 to 29 dB too strong for flat6, inline4 and v8, and 4 to 6 dB for flat12, whose engine-sim response is brighter. The fitted bank removes that gap.

Recommendation: replace the wall loss, the low shelf, the expansion chamber, the mode cutoff and the outlet with one fitted bank per profile. Run the bank once, on the sum of the paths, before the crackle and the jet noise join. Keep the collector reflection and the wave steepening per path. This costs 36 to 56 multiply-adds per sample once, against 25 per path now (50 for the two-path profiles, and 100 while a pop sounds). The steady cost changes by -9 (flat12, flat6), +11 (inline4) and +6 (v8) multiply-adds per sample, and it falls by 44 to 59 while a pop sounds. Section 5 gives the reasons and the alternatives.

## 1. The fit

### 1.1 Filter form

The bank is a cascade of biquads from the Audio EQ Cookbook: peaking sections, at most one low pass and at most one high pass, and one gain. I chose a cascade over a parallel resonator bank for these reasons:

- A cascade adds in dB, so the fit works on the dB error directly, and the magnitude of each section is independent of the phase of the others. A parallel bank sums complex responses, so its magnitude depends on the phase and the sign of each resonator, and the fit needs more free values for the same error.
- Each section has a meaning that a person can read and tune: a centre, a Q and a gain. The model can design the coefficients at any sample rate from these three values, as `dsp.Biquad.SetLowPass` does now.
- The cost is the same: 5 multiply-adds per section in transposed direct form II.

The fit target is the magnitude of the impulse response alone, as engine-sim loads it (trimmed, at most 10,000 taps). The 1.9 kHz input low pass and the derivative are not part of the bank: the parallel model change ports them as their own stages. They are the same in the target and in the model, so they cancel in the dB difference. They count only in the audibility floor (section 1.2).

### 1.2 Error measure

- Grid: 1/24 octave from 40 Hz to 6 kHz (174 points). The target is the power response averaged over 1/6 octave around each point.
- Weight: the number of equivalent rectangular bandwidths (ERB, Glasberg and Moore) per octave at each point, so that each auditory filter counts the same. This weights 1 kHz about 5 times more than 40 Hz.
- Audibility floor: the level of each point on the engine-sim harmonic path (the 1.9 kHz low pass, the derivative and the response) relative to its loudest point passes through a soft floor at -40 dB (width 2 dB). An error in a valley that is more than 40 dB down counts little.
- Gain: the bank gain is free, so only the shape counts.
- Error: the weighted RMS of the dB differences.

`--smooth 0` smooths over one ERB instead of 1/6 octave. Below 500 Hz an ERB is wider than a third octave, and it blurs the formant peaks that decide the strongest engine order. With ERB smoothing, the inline4 fit at N = 5 has a weighted error of 0.95 dB but a third-octave RMS of 1.01 dB, against 0.58 dB with 1/6 octave. For this reason 1/6 octave is the default.

### 1.3 Method

The fit adds one section at a time, from 1 to 12. Each new section starts at one of the 4 largest extrema of the weighted difference (a peaking section with the gain of the difference and the Q of its half width), or as the one low pass at the top of the grid, or as the one high pass at the bottom. Then all sections and the gain move together by Levenberg-Marquardt steps (at most 300, forward-difference Jacobian). The start with the lowest error stays. The chosen N is the smallest from 4 to 12 with a weighted error of at most 1 dB (`--tolerance`). Each profile takes about 7 s.

Bounds: 20 Hz to 0.45 times the sample rate, Q from 0.3 to 20, gain from -36 to +36 dB.

### 1.4 Test

`TestFitFormantsRecovers` builds a response from three known peaking sections (160 Hz, Q 4, +12 dB; 800 Hz, Q 2, -8 dB; 2.5 kHz, Q 1.5, +6 dB; gain -6 dB) and fits it. With three sections, the error is below 0.05 dB, each centre is within 2 per cent, each Q within 10 per cent and each gain within 0.3 dB. Other tests check the section responses, the soft floor, the gain search and the smoothing. `tools/fit` has tests for its profile reader, the collector comb of a path and the choice of N.

### 1.5 Error against N

Weighted error in dB (`out/analysis/fit/<profile>.md` has the third-octave columns too):

| N | flat12 | flat6 | inline4 | v8 |
| --- | --- | --- | --- | --- |
| 4 | 1.46 | 1.50 | 1.29 | 2.21 |
| 5 | 1.34 | 1.26 | 1.19 | 2.02 |
| 6 | 1.22 | 1.13 | 1.07 | 1.75 |
| 7 | 1.12 | 1.03 | 0.87 | 1.60 |
| 8 | 1.00 | 0.90 | 0.77 | 1.42 |
| 9 | 0.93 | 0.82 | 0.70 | 1.19 |
| 10 | 0.90 | 0.77 | 0.65 | 1.06 |
| 11 | 0.88 | 0.73 | 0.61 | 0.87 |
| 12 | 0.84 | 0.69 | 0.46 | 0.79 |

The v8 response needs the most sections. Its 1024-tap lead-in is not the cause, because the fit uses the magnitude only. The cause is the narrow dip at 79 Hz and the strong, narrow 400 Hz formant on top of a broad hill. With 8 sections for every profile, the v8 error is 1.42 dB (1.19 dB third-octave RMS).

### 1.6 Fitted values

The proposed fragments are `out/analysis/fit/flat12.toml`, `flat6.toml`, `inline4.toml` and `v8.toml`. Each report beside them (`<profile>.md`) has the section table, the harmonic path per third octave for engine-sim, the fitted bank and each snarl path, and the stage tables of section 4.

Some sections reach the gain bound (for example v8 section 1, 77 Hz, Q 14.8, -36 dB, and the +36 dB, Q 0.3 to 0.9 sections of flat6 and v8). They are broad gain hills and narrow dips that pairs of sections build together. They are valid, but the model must design the coefficients in float64 and check a float32 render for noise near the narrow low sections.

## 2. Proposed key format

One bank per profile, because engine-sim uses one impulse response for every exhaust system of all four profiles (`impulse_response: ir_lib.<name>` on each `exhaust_system` of the scripts).

```toml
[exhaust]
# Gain of the formant bank in dB. The fit sets the highest response of
# the bank from 40 Hz to 6 kHz to 0 dB.
formant_gain_db = -12.93

# One table per section, in any order. kind is "peak", "lowpass" or
# "highpass". hz is the centre (peak) or the corner (lowpass, highpass)
# in hertz, q the Q, and gain_db the gain at the centre of a peak in dB.
# A lowpass or highpass has no gain_db.
[[formant]]
kind = "peak"
hz = 153.5
q = 0.978
gain_db = 22.91
```

I propose a top-level `[[formant]]` table list, not `[[path.formant]]`:

- The profile files use a TOML subset with one level of nesting (`toml.go`). `[[path.formant]]` is a second level.
- The bank runs once on the sum of the paths (section 5), so it belongs to the exhaust, not to one path.
- A profile without `[[formant]]` keeps the current path stages, so the change can come one profile at a time.

The coefficients follow from `hz`, `q` and `gain_db` with the Audio EQ Cookbook formulas at the render rate. The fit ran at 48 kHz. At 44.1 kHz the warping below 6 kHz changes the response by less than 0.1 dB.

## 3. Cost per sample

A biquad in transposed direct form II (`dsp.Biquad.Process`) is 5 multiply-adds. The bank is 5N + 1 multiply-adds per sample: 41 for flat12 and flat6 (N = 8), 36 for inline4 (N = 7) and 56 for v8 (N = 11). It has no division and no call into the math package, as the `dsp` rule asks. The state is 2 floats per section.

## 4. The current path stages against the target

`tools/fit --profile` builds the impulse response of each linear stage of each path from the `dsp` parts, as `internal/model/path.go` builds them at `195ba4e`. It leaves out the steepening (not linear), the path gain and the air delay. The tables are in `out/analysis/fit/<profile>.md`. The profiles changed in the working tree during this run, but only in `[noise]` and `[voice]`, which the path response does not read.

What each stage does from 50 Hz to 5 kHz, in dB relative to its own mean from 50 Hz to 2 kHz (v8 path 1; the other profiles are alike):

| Stage | Response | Multiply-adds per path | Role against the target |
| --- | --- | --- | --- |
| Collector reflection | Comb, -3.7 to +4.3 dB, peaks at odd multiples of c/(4L) (128, 385, 642 Hz for 1.10 m) | 3 | Different per path, so it gives the bank-to-bank difference and the half orders. Keep |
| Wall loss | High shelf, +0.3 dB to -2.3 dB at 5 kHz | 3 | A slope that the bank holds. Replace |
| Low shelf | Low shelf, -1.7 to +1.2 dB | 3 | A slope that the bank holds. Replace |
| Expansion chamber | Comb, -4.5 to +5.9 dB, peaks every 807 Hz (c/(2 x 0.35 m)) | 2 | Puts fixed peaks at 794 Hz, 1.6 kHz, 2.4 kHz and up. They show as horizontal lines above 2 kHz in the snarl spectrograms. engine-sim has none there. Replace |
| Mode cutoff | Flat to 4 kHz, -1.2 dB at 5 kHz (cut-off 5.8 kHz) | 10 | No effect in the audible band of the target. The 1.9 kHz input low pass and the bank do its job. Replace |
| Outlet | Rises from -1.5 dB to +8 dB at 5 kHz | 7 | Lifts the top, the opposite of the target slope. Replace |

The whole linear path costs 28 multiply-adds per path and is 6.6 dB (flat12) to 12.8 dB (flat6) from the fit target (weighted error, best gain). Per third octave, the snarl paths are 8.1 to 10.9 dB RMS from engine-sim from 50 Hz to 2 kHz, and 19 to 29 dB too strong at 3 to 5 kHz (flat12: 4 to 6 dB).

## 5. Recommendation

Replace the wall loss, the low shelf, the expansion chamber, the mode cutoff and the outlet with the fitted bank. Run the bank once, on the sum of the paths, before the crackle and the jet noise join it.

Reasons:

- The bank is linear and engine-sim applies one impulse response to every exhaust system and sums them. One bank on the sum gives the same result as one bank per path, at the cost of one.
- In engine-sim the impulse response is the last stage before the leveller. With the bank last, the harmonics that the steepening adds above 2 kHz pass through the bank's slope, as engine-sim has no harmonic content above 3 kHz.
- The pop stages run the linear stages twice while a pop sounds. The bank on the sum runs once, so a pop no longer doubles the exhaust cost.
- The crackle and the jet noise keep their own band, because they join after the bank.

Cost, in multiply-adds per sample, steady (while a pop sounds):

| Profile | Paths | Five stages now | Bank | Change |
| --- | --- | --- | --- | --- |
| flat12 | 2 | 50 (100) | 41 | -9 (-59) |
| flat6 | 2 | 50 (100) | 41 | -9 (-59) |
| inline4 | 1 | 25 (50) | 36 | +11 (-14) |
| v8 | 2 | 50 (100) | 56 | +6 (-44) |

The collector reflection (3 per path) and the steepening stay per path.

Risks:

- Medium: the collector comb (-3.7 to +4.3 dB) lies on top of the fitted bank. Measure the harmonic path with and without it. If it moves the strongest order away from the targets of `engine-sim-target.md` section 4, lower `reflection` or refit the bank to the target divided by the collector response.
- Low: the steepening then acts on a wave with no chamber or outlet colour, so its harmonics change. The bank and the 1.9 kHz low pass bound them.
- Low: float32 noise of the narrow low sections (section 1.6).

Alternatives:

- One bank per path before the steepening: the closest to the current structure, but it costs 2 x (5N + 1) per sample on two-path profiles (82 to 112) and twice that during pops.
- A fixed N = 8 for every profile: 41 multiply-adds everywhere, with a v8 error of 1.42 dB.
- A partitioned FFT convolution with a snarl-owned 2048-tap response (`engine-sim-target.md` section 7): closer (0.7 dB), but about 100 operations and 5.8 ms of latency.

Measure the change by the strongest-order table of `engine-sim-target.md` section 4, the band columns of its section 6, and the `snarl-compare --gap` tables of [snarl-gap.md](snarl-gap.md): the strongest order must land at the formant, and the power above 2.84 kHz must fall to the engine-sim values.

## 6. How to repeat

```sh
nix develop -c go run ./tools/fit --ir <sound-library>/smooth/smooth_39.wav --profile profiles/v8.toml --out out/analysis/fit/v8.toml > out/analysis/fit/v8.md
```

The impulse responses are in `share/engine-sim/es/sound-library/` of the `engine-sim-render` package (`new/minimal_muffling_01.wav` for flat12, `new/minimal_muffling_02.wav` for flat6, `new/mild_exhaust.wav` for inline4, `smooth/smooth_39.wav` for v8).
