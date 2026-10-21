# engine-sim target for snarl

Measured on branch `overnight/engine-sim-match` at `7ed553f`, with no change to model code or profiles. The four tuned profiles are flat12, flat6, inline4 and v8. Each item below is marked "from engine-sim" (engine-sim has it, and snarl copies its form) or "new" (engine-sim does not have it).

## Answer

engine-sim does four things to the exhaust pressure. Each one has a measured target that snarl must match.

1. A peak leveller holds the output peak constant. Above a knee, a 26 to 48 dB change of the synthesiser input gives 2.2 to 4.3 dB of output change. Over a full-throttle sweep, the peak level stays in a 1.3 to 2.3 dB band, and the 0.5 s loudness in a 1.4 to 3.0 LU band. Where the loudness falls a little as the RPM rises (inline4 by 2.3 LU above 5000 RPM), the crest factor rises and the peak stays. Only a closed throttle at idle falls below the knee. This is the level target. snarl has no signal-following gain, so its level dips with RPM. The cheapest match is a port of the engine-sim leveller (from engine-sim, about 6 operations per sample).
2. The harmonic content is the derivative of the pressure (`hf_gain`), band-limited by a fourth-order 1.9 kHz low pass, through the impulse response. Above 3 kHz, the output has no harmonic content: at least 74 per cent of the power in every band from 3.2 kHz up is noise, and for flat12, flat6 and v8 nearly all of it. The impulse response puts fixed formants on the harmonics. The strongest harmonic is the order nearest the formant, for example the order at 400 to 417 Hz for the v8 at 2000, 3000, 5000 and 6000 RPM. This is the spectral-shape target. A harmonic series that reaches above 2 kHz, or that has no fixed formant, is the likely cause of the synthetic sound at high RPM. The cheapest match is the 1.9 kHz low pass on the pulse (from engine-sim, two biquads) and a resonator bank fitted to the formant curve of section 4 (new).
3. The noise is the pressure multiplied by low-passed noise (pure amplitude modulation with no constant part for flat12, flat6 and v8), through the same impulse response. At full throttle, the residual (the part that does not repeat from cycle to cycle) is -6 to -13 dB relative to the signal for flat12, -17 to -32 dB for flat6, -24 to -42 dB for inline4 and -17 to -30 dB for v8. For flat6 and v8 it falls by 12 to 16 dB from idle to the top hold. The noise follows the engine cycle, with a peak 2 to 11 dB above its mean and a trough 3 to 18 dB below it. A noise that is steady over the cycle, or that is not shaped by the formants, sounds like static. The cheapest match is the existing noise AM with the depth and band of section 5 (from engine-sim).
4. The 10 Hz DC block acts below 10 Hz and the 0.45 fs low pass above 19.8 kHz, so neither shapes an audible band. The jitter adds to the noise between harmonics. The coherence measurement includes it, but does not separate it from the noise modulation.

Section 6 gives the per-profile targets in tables. Section 7 lists what remains.

## 1. Method

All commands ran in `nix develop`. No ad-hoc script ran.

- `just analyse WAV SCENARIO OUT` (new recipe in the `justfile`) writes, for one render, the spectrogram (`showspectrumpic`), the waveform with a linear and a logarithmic amplitude (`showwavespic`), the sample histogram (`ahistogram`), the ffmpeg logs `ebur128.log`, `astats.log` (every 100 ms) and `aspectralstats.log` (centroid, flatness, rolloff and flux per 4096 samples), and the `snarl-compare` tables `describe.md` and `residual.md`.
- `snarl-compare --describe` (new option) gives per scenario window: the ungated loudness, the RMS and peak level, the crest factor, the aspectralstats centroid, flatness and rolloff, the octave band balance, and the power of each engine order relative to the window.
- `snarl-compare --residual` now also gives `AM peak` and `AM trough`: the highest and the lowest residual power of 96 crank-angle sectors of the cycle, relative to the mean. This is the depth of the noise modulation over the cycle.
- `snarl-compare --transfer CSV --engine JSON` (new option) rebuilds the synthesiser input from the per-step runner pressures in the CSV, as `PistonEngineSimulator::writeToSynthesizer` does: pressure above one atmosphere, times 1600, times `sound_attenuation`, `audio_volume / cylinders`, `1 / length^2` and the impulse response volume, delayed by `length / 343 m/s` in steps of a 10 kHz rate, summed. It resamples the input to the WAV rate, finds the one latency that maximises the coherence over the whole file, and gives per hold the gain, and per third-octave band the coherent gain, the noise share (1 minus the coherence) and the output balance.
- `snarl-compare --ir` (new option) measures an impulse response as engine-sim loads it (trimmed and capped at 10,000 taps), its energy decay, its third-octave response, the responses of the first 4096 to 128 taps and of all-pole fits of order 8 to 64, and the harmonic path response (1.9 kHz low pass, derivative, impulse response).

References:

- Steps 2 and 3 use the stored references `out/reference/engine-sim/<profile>/{sweep-wot,holds-t000,holds-t050,holds-t100,overrun,limiter}.wav`. They date from 17 October, before patch 0005. A fresh render of `v8/holds-t100` with patch 0005 differs sample by sample (the random sequence moved), but its level is the same: -11.62 dBFS RMS against -11.61 dBFS.
- Step 4 needs one CSV row per step, which `just ref-set` does not write (`--csv-every 100`). Fresh renders of the three hold scenarios of each profile with `just ref-engine-sim ... --csv-every 1` are in `out/analysis/engine-sim/<profile>/full/`. The engine dumps are in `out/analysis/engine-sim/<profile>/engine.json`.
- The impulse responses come from the `engine-sim-render` package in the Nix store (`share/engine-sim/es/sound-library/`). Their tables are in `out/analysis/engine-sim/ir/ir.md`.

Limits of the method:

- The rebuilt input leaves out the two `0.1 * dynamicPressure` terms of the sound tap, because the CSV does not have them.
- With 30 averaged frames, the noise share of a band with no coherent content reads -0.1 to -0.2 dB, not 0 dB.
- The limiter reference comes from `just ref-limiter` (0.5 s hold, then a 5 s free run). The scenario windows of `limiter.scn` do not match its timeline, so the limiter rows describe the file in parts only.
- The order levels come from 0.125-order bins, so an order next to a strong formant can borrow some power from noise.

## 2. What the images show

Spectrograms (`spectrogram.png`, log frequency, log magnitude):

- All profiles have horizontal bands at fixed frequencies that do not move with the RPM. For the v8 these are near 400 Hz, 880 Hz and 1.4 kHz ([v8 holds-t000](../../out/analysis/engine-sim/v8/holds-t000/spectrogram.png)). The engine orders move through them in steps between the holds. These bands are the formants of the impulse response.
- The harmonic lines stop near 1.5 to 2 kHz in every file. Above that, the spectrogram is diffuse noise with no lines ([inline4 sweep-wot](../../out/analysis/engine-sim/inline4/sweep-wot/spectrogram.png), [flat6 sweep-wot](../../out/analysis/engine-sim/flat6/sweep-wot/spectrogram.png)).
- The noise above 2 kHz is lowest for inline4 (about -90 to -100 dB per bin) and highest for flat12 (about -45 to -55 dB per bin, with vertical stripes at the firing rate) ([flat12 sweep-wot](../../out/analysis/engine-sim/flat12/sweep-wot/spectrogram.png), [flat12 holds-t000](../../out/analysis/engine-sim/flat12/holds-t000/spectrogram.png)). In the v8 and inline4 hold files, the high-frequency noise at the idle hold is darker than at the higher holds ([inline4 holds-t000](../../out/analysis/engine-sim/inline4/holds-t000/spectrogram.png)).
- Everything above about 15 kHz is dark in every file.
- In the v8 overrun, the change from full throttle to closed throttle at 2 s changes the colour of the spectrum but not its brightness ([v8 overrun](../../out/analysis/engine-sim/v8/overrun/spectrogram.png)). The leveller hides the drop of the input.
- In the v8 limiter, each cut shows as a dark notch across all frequencies, about every 0.26 s, and the high-frequency noise falls in each notch ([v8 limiter](../../out/analysis/engine-sim/v8/limiter/spectrogram.png)).

Waveforms (`waveform.png`, linear peak amplitude):

- In every full-throttle sweep, the peak envelope is flat from the first window to the last. Single pulses stand out at low RPM, and the space between the peaks fills in as the RPM rises ([v8 sweep-wot](../../out/analysis/engine-sim/v8/sweep-wot/waveform.png), [inline4 sweep-wot](../../out/analysis/engine-sim/inline4/sweep-wot/waveform.png), [flat6 sweep-wot](../../out/analysis/engine-sim/flat6/sweep-wot/waveform.png)). No RPM increase lowers the envelope.
- In the closed-throttle holds, the idle hold has about half the peak amplitude of the later holds, and every later hold has the same envelope ([v8 holds-t000](../../out/analysis/engine-sim/v8/holds-t000/waveform.png)). For flat12, the envelope grows over the first three holds and is flat from 4000 RPM ([flat12 holds-t000](../../out/analysis/engine-sim/flat12/holds-t000/waveform.png)). This is the knee of the leveller.
- Short overshoots appear at the start of each RPM step. The likely cause is the gain smoothing of the leveller, which takes about 10 samples to follow its instant peak attack.

Histograms (`histogram.png`, log count over the sample value from -1 to 1): the samples stay inside about -0.6 to +0.7 with a sharp edge, which is the leveller ceiling at `--volume 0.5`. The v8 has a rounded peak at 0 ([v8 sweep-wot](../../out/analysis/engine-sim/v8/sweep-wot/histogram.png)). The inline4 has a broad flat top, as a steady near-periodic wave gives ([inline4 sweep-wot](../../out/analysis/engine-sim/inline4/sweep-wot/histogram.png)).

## 3. What the engine-sim synthesiser does

### 3.1 The signal path in the code

Source: `src/synthesizer.cpp`, `src/leveling_filter.cpp`, `include/jitter_filter.h` and `src/piston_engine_simulator.cpp` of the pinned engine-sim (`85f7c3b`), read from the Nix store. Per exhaust channel and per 44.1 kHz sample:

1. Input `x`: the staging buffer of section 1, linearly interpolated from the simulation rate, then a fourth-order Butterworth low pass at 1900 Hz.
2. Jitter: a random fractional delay of 0 to `9 * jitter` samples, low-passed at 10 kHz.
3. `f = x - LP10Hz(x)` and `f' = (x - x_prev) * 44100`.
4. `r`: uniform noise through a fourth-order Butterworth low pass at 2 kHz. One filter state serves all channels, so with two exhaust systems it runs twice per sample.
5. `v = hf_gain * f' + (1 - hf_gain) * f * (noise * r + 1 - noise)`.
6. Convolution with the impulse response (at most 10,000 taps), then the sum over channels, a fourth-order low pass at 0.45 fs, and the leveller.
7. Leveller: `peak = max(0.999 * peak, |s|)`, `gain = clamp(30000 / peak, 1e-5, 1.9)`, smoothed by `0.9 * old + 0.1 * new` per sample, then times `volume` (0.5 in the references) and clamped to 16 bits.

The parameters of the four profiles:

| Profile | Script | Sim rate (Hz) | `hf_gain` | `noise` | `jitter` | Impulse response | Exhaust systems | Measured latency (ms) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| flat12 | `reference/engine-sim/engines/flat12.mr` | 10000 | 0.01 | 1.0 | 0.1 | `minimal_muffling_01` | 2 | 24 to 25 |
| flat6 | `reference/engine-sim/engines/flat6.mr` | 20000 | 0.01 | 1.0 | 0.5 | `minimal_muffling_02` | 1 | 20 |
| inline4 | `atg-video-1/05_honda_vtec.mr` | 20000 | 0.002 | 0.253 | 0.195 | `mild_exhaust` | 2 | 7 |
| v8 | `reference/engine-sim/engines/v8.mr` | 10000 | 0.01 | 1.0 | 0.6 | `smooth_39` (`default_0`) | 2 | 34 |

With `noise` at 1.0, the pressure term `f * r` is pure noise modulation: it has no constant part. The only harmonic content then comes from the derivative term `hf_gain * f'`. The derivative weights each harmonic by its frequency, so the harmonic share grows as the RPM moves the harmonics up. inline4 is different: 75 per cent of its pressure term passes with no noise.

### 3.2 Harmonic path and noise path (measured)

The coherence between the rebuilt input and the output splits the output into the part that the pressure explains (harmonic) and the rest (noise). Noise share of the whole output, in dB (`full/transfer-t100.md` of each profile):

| Profile | Idle hold | 2000 RPM | 3000 RPM | 4000 RPM | 5000 RPM | 6000 RPM | Top hold |
| --- | --- | --- | --- | --- | --- | --- | --- |
| flat12 | -11.4 (1000) | -12.5 | -14.4 | -15.5 | -12.4 | -14.1 | -16.8 (6800) |
| flat6 | -17.6 (800) | -25.8 | -28.5 | -30.2 | -28.9 | -30.7 | -28.6 (6600) |
| inline4 | -13.3 (800) | -26.1 | -29.3 | -39.6 | -39.6 | -34.4 | -28.4 (8400) |
| v8 | -10.7 (675) | -22.3 | -25.4 | -30.1 | -24.4 | -30.4 | -30.4 (6000) |

Per band, the noise share is -25 to -40 dB in the strongest bands below 500 Hz at 2000 RPM and above, and rises to -5 to -15 dB at 1 to 1.6 kHz. It is -0.1 to -4.4 dB in the 2.5 kHz band, and -0.1 to -1.3 dB in every band from 3.2 kHz up, in every hold of every profile. That is, above about 3 kHz the output is noise only. The residual measurement agrees: its in-band residual from 2 to 6 kHz is -0.0 to -0.8 dB for flat12, flat6 and v8, and -0.2 to -2.6 dB for inline4 (`holds-t100/residual.md`).

At 6000 RPM at full throttle, the measured v8 gain per band peaks in the 398 Hz band, as the predicted harmonic path does (section 4). The band values differ from the prediction by up to 6 dB, because the noise and the left-out dynamic pressure terms add to the estimate.

### 3.3 The leveller (measured)

The rebuilt input level and the output level of every hold (`full/transfer-t000.md`, `-t050.md`, `-t100.md`):

| Profile | Input range, all holds (dB) | Output range, all holds (dBFS RMS) | Holds below the knee | Output range above the knee (dBFS RMS) |
| --- | --- | --- | --- | --- |
| flat12 | 12.2 to 54.0 | -26.0 to -11.1 | t000 at 1000, 2000 and 3000 RPM (-26.0, -17.6, -15.0) | -13.3 to -11.1 over 36 dB of input |
| flat6 | 19.7 to 69.3 | -25.0 to -9.4 | t000 at 800 RPM (-25.0) | -13.7 to -9.4 over 32 dB of input |
| inline4 | 36.3 to 84.3 | -13.1 to -9.2 | none | -13.1 to -9.2 over 48 dB of input |
| v8 | 22.2 to 67.1 | -18.2 to -10.9 | t000 at 675 RPM (-18.2) | -13.1 to -10.9 over 26 dB of input |

The leveller is a peak-normalising gain: above the knee the output peak sits at the target, so the RMS level only follows the crest factor. Below the knee the gain is at its 1.9 limit, and the output falls with the input. The release is fast: the peak falls by 8.7 dB in 1000 samples (22.7 ms). At an idle with a firing interval near 20 ms, the gain rises between pulses, which lifts the tail and the noise between pulses. The larger noise share at the idle holds (-10.7 to -17.6 dB) agrees with this, but the measurement does not separate it from the other noise sources.

### 3.4 The impulse response (measured)

From `out/analysis/engine-sim/ir/ir.md`. The RMS difference is over the third-octave bands from 50 Hz to 2 kHz, relative to the response as engine-sim loads it.

| Profile | Impulse response | Taps loaded (ms) | Energy -20 dB / -40 dB (ms) | Strongest band | First 2048 taps | First 1024 taps | First 512 taps | All-pole 16 | All-pole 64 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| flat12 | `minimal_muffling_01` | 5979 (135.6) | 74.6 / 113.4 | 158 Hz | 0.71 dB | 5.11 dB | 7.55 dB | 5.22 dB | 4.31 dB |
| flat6 | `minimal_muffling_02` | 5636 (127.8) | 58.3 / 98.3 | 251 Hz | 0.42 dB | 2.63 dB | 6.19 dB | 9.71 dB | 5.43 dB |
| inline4 | `mild_exhaust` | 1893 (42.9) | 25.3 / 37.0 | 126 Hz | all taps | 0.41 dB | 2.29 dB | 4.32 dB | 3.72 dB |
| v8 | `smooth_39` | 6907 (156.6) | 45.0 / 107.7 | 398 Hz | 0.66 dB | 13.00 dB | 37.77 dB | 5.44 dB | 3.00 dB |

What this shows:

- The first 2048 taps (46 ms) reproduce every response within 0.7 dB. The tail after that is inaudible in the band levels.
- The v8 response starts with about 1024 taps of low level: in most bands, its first 1024 taps are 18 to 19 dB below the full response. A pure delay can replace that lead-in.
- Linear prediction at 44.1 kHz fits the responses poorly: 3 to 10 dB even with 64 poles, because the formants below 500 Hz are narrow. An all-pole fit is not a usable shortcut.

## 4. The formant targets

The harmonic path of each profile (`--ir`, the 1.9 kHz low pass, the derivative and the impulse response), in dB relative to its strongest band. These curves are the fixed spectral shape that the engine-sim synthesiser puts on the pressure harmonics. They are from engine-sim.

| Band (Hz) | flat12 | flat6 | inline4 | v8 |
| --- | --- | --- | --- | --- |
| 50 | -41.2 | -41.4 | -17.8 | -31.0 |
| 79 | -24.5 | -28.5 | -13.8 | -36.5 |
| 100 | -18.7 | -21.9 | -11.4 | -22.3 |
| 126 | -16.9 | -20.4 | -1.4 | -27.6 |
| 158 | -12.2 | -11.3 | 0.0 | -17.9 |
| 200 | -14.7 | -7.6 | -6.9 | -16.1 |
| 251 | -12.0 | -1.7 | -16.3 | -11.3 |
| 316 | -8.2 | -0.8 | -13.2 | -12.3 |
| 398 | -6.1 | 0.0 | -6.6 | 0.0 |
| 501 | -7.4 | -4.4 | -7.8 | -6.3 |
| 631 | -10.7 | -8.5 | -8.0 | -4.4 |
| 794 | -11.1 | -9.5 | -7.4 | -7.2 |
| 1000 | -2.8 | -3.5 | -7.6 | -3.0 |
| 1259 | -1.2 | -3.1 | -6.8 | -7.8 |
| 1585 | 0.0 | -2.2 | -7.8 | -6.5 |
| 1995 | -2.6 | -6.0 | -10.2 | -10.1 |
| 2512 | -4.4 | -12.3 | -16.5 | -15.5 |
| 3162 | -11.0 | -22.4 | -27.5 | -27.8 |
| 3981 | -16.6 | -31.5 | -35.5 | -35.5 |
| 5012 | -25.2 | -41.8 | -41.9 | -45.8 |
| 7943 | -42.1 | -62.6 | -61.6 | -67.0 |
| 12589 | -64.7 | -85.6 | -78.6 | -85.9 |

The orders confirm the formants. The strongest whole order of each full-throttle hold (`holds-t100/describe.md`) lands near the same frequency at every RPM:

| Profile | Strongest order and its frequency, by hold |
| --- | --- |
| flat12 | 6 (100 Hz) at 1000, 6 (200 Hz) at 2000, 6 (300 Hz) at 3000, 6 (400 Hz) at 4000, 2 (167 Hz) at 5000, 4 (400 Hz) at 6000, 2 (227 Hz) at 6800 |
| flat6 | 18 (240 Hz) at 800, 9 (300 Hz) at 2000, 6 (300 Hz) at 3000, 6 (400 Hz) at 4000, 3 (250 Hz) at 5000, 3 (300 Hz) at 6000, 3 (330 Hz) at 6600 |
| inline4 | 10 (133 Hz) at 800, 4 (133 Hz) at 2000, 4 (200 Hz) at 3000, 2 (133 Hz) at 4000, 2 (167 Hz) at 5000, 2 (200 Hz) at 6000, 4 (467 Hz) at 7000, 2 (267 Hz) at 8000, 4 (560 Hz) at 8400 |
| v8 | 16 (180 Hz) at 675, 12 (400 Hz) at 2000, 8 (400 Hz) at 3000, 4 (267 Hz) at 4000, 5 (417 Hz) at 5000, 4 (400 Hz) at 6000 |

## 5. Noise targets

The residual of each full-throttle hold (`holds-t100/residual.md`): the part that does not repeat from cycle to cycle, relative to the whole signal, in dB, and its modulation over the cycle.

| Profile | Hold (RPM) | Residual | 2-6 kHz | 6-16 kHz | AM peak | AM trough |
| --- | --- | --- | --- | --- | --- | --- |
| flat12 | 1000 | -6.0 | -28.4 | -30.3 | +4.1 | -6.8 |
| flat12 | 3000 | -9.1 | -29.8 | -31.4 | +4.7 | -12.5 |
| flat12 | 6800 | -12.8 | -30.3 | -31.9 | +3.0 | -10.4 |
| flat6 | 800 | -16.7 | -25.2 | -33.1 | +5.4 | -6.5 |
| flat6 | 3000 | -29.1 | -33.9 | -42.1 | +2.8 | -4.1 |
| flat6 | 6600 | -28.6 | -35.4 | -43.8 | +3.5 | -4.5 |
| inline4 | 800 | -23.9 | -48.2 | -50.1 | +11.2 | -15.9 |
| inline4 | 3000 | -30.5 | -43.0 | -44.9 | +6.5 | -18.3 |
| inline4 | 8400 | -24.6 | -35.5 | -37.4 | +4.3 | -7.3 |
| v8 | 675 | -16.7 | -26.4 | -34.7 | +5.6 | -9.4 |
| v8 | 3000 | -26.6 | -31.2 | -38.9 | +3.1 | -5.7 |
| v8 | 6000 | -30.3 | -34.8 | -42.4 | +3.0 | -4.4 |

At closed throttle at idle (`holds-t000/residual.md`), the residual is -3.2 dB for flat12 (1000 RPM), -16.1 dB for flat6 (800), -23.0 dB for inline4 (800) and -11.3 dB for v8 (675). The 6 to 16 kHz part is -30.5, -37.8, -52.9 and -35.3 dB. engine-sim has no separate idle noise: its high-frequency noise at idle is the same modulated pressure noise as at any other RPM, 30 to 53 dB below the signal.

flat12 has a residual far above its coherence noise (section 3.2). Part of its cycle-to-cycle change is in the pressure input itself, not in the synthesiser.

## 6. Per-profile targets

Level targets are at `--volume 0.5`. snarl calibrates its sweep to -16 LUFS, so compare differences, not absolute values. Loudness is the ungated K-weighted level of each window. Crest, centroid, flatness and rolloff are from the full-throttle holds. The band columns give the octave band power relative to the window.

### flat12

| Hold (RPM) | Closed (LUFS) | Half (LUFS) | Full (LUFS) | Crest (dB) | Centroid (Hz) | Flatness | Rolloff (Hz) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1000 | -27.5 | -14.4 | -14.1 | 8.3 | 1372 | 0.033 | 3509 |
| 2000 | -18.1 | -12.8 | -12.8 | 8.1 | 1525 | 0.035 | 4182 |
| 3000 | -15.8 | -11.9 | -12.1 | 7.7 | 1469 | 0.035 | 3901 |
| 4000 | -12.2 | -12.0 | -12.0 | 7.0 | 1251 | 0.030 | 2123 |
| 5000 | -14.0 | -12.8 | -12.8 | 8.0 | 1403 | 0.032 | 3565 |
| 6000 | -13.4 | -13.3 | -13.1 | 8.3 | 1335 | 0.031 | 3126 |
| 6800 | -13.5 | -12.7 | -12.8 | 7.9 | 1298 | 0.030 | 2801 |

Full-throttle sweep: -13.3 to -11.9 LUFS, peak -4.6 to -3.3 dBFS. Bands at 3000 RPM, from the 31.5 Hz octave to the 16 kHz octave: -38.9, -22.1, -10.7, -0.5, -19.6, -28.2, -36.7, -30.9, -31.0, -50.6 dB. Half orders: -1.8 to -9.7 dB relative to the window.

### flat6

| Hold (RPM) | Closed (LUFS) | Half (LUFS) | Full (LUFS) | Crest (dB) | Centroid (Hz) | Flatness | Rolloff (Hz) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 800 | -26.2 | -14.7 | -14.7 | 10.0 | 1168 | 0.026 | 2307 |
| 2000 | -14.0 | -12.4 | -12.8 | 7.4 | 921 | 0.021 | 1322 |
| 3000 | -13.1 | -13.0 | -12.9 | 6.9 | 846 | 0.021 | 960 |
| 4000 | -12.7 | -12.1 | -12.3 | 6.3 | 932 | 0.022 | 1229 |
| 5000 | -11.3 | -10.9 | -10.6 | 4.1 | 851 | 0.021 | 1151 |
| 6000 | -10.7 | -10.6 | -10.2 | 3.6 | 938 | 0.023 | 1350 |
| 6600 | -10.3 | -11.1 | -10.9 | 5.2 | 939 | 0.022 | 1320 |

Full-throttle sweep: -13.4 to -10.4 LUFS, rising with RPM, peak -5.8 to -3.5 dBFS. Bands at 3000 RPM: -48.0, -34.9, -4.9, -2.8, -8.5, -30.8, -32.7, -36.7, -41.5, -61.9 dB. Half orders: -19.3 to -24.4 dB.

### inline4

| Hold (RPM) | Closed (LUFS) | Half (LUFS) | Full (LUFS) | Crest (dB) | Centroid (Hz) | Flatness | Rolloff (Hz) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 800 | -15.3 | -13.9 | -13.9 | 6.9 | 337 | 0.012 | 278 |
| 2000 | -11.4 | -11.4 | -11.4 | 4.4 | 348 | 0.013 | 257 |
| 3000 | -12.9 | -13.4 | -13.4 | 7.8 | 560 | 0.017 | 594 |
| 4000 | -10.6 | -10.9 | -10.9 | 4.0 | 397 | 0.014 | 276 |
| 5000 | -10.5 | -10.7 | -10.7 | 3.8 | 419 | 0.014 | 368 |
| 6000 | -12.0 | -11.7 | -11.8 | 6.4 | 693 | 0.019 | 738 |
| 7000 | -12.5 | -12.4 | -13.0 | 7.7 | 909 | 0.025 | 856 |
| 8000 | -12.3 | -11.7 | -12.7 | 7.1 | 1033 | 0.027 | 1223 |
| 8400 | -12.6 | -12.2 | -13.5 | 8.3 | 1088 | 0.028 | 1190 |

Full-throttle sweep: -12.9 to -10.6 LUFS, peak -5.9 to -3.6 dBFS. Bands at 3000 RPM: -26.5, -17.6, -3.5, -2.9, -16.8, -28.1, -42.6, -45.4, -44.3, -61.9 dB. Half orders: -3.7 to -23.7 dB. The half-throttle and full-throttle holds are the same up to 5000 RPM, so the upstream script does not follow the throttle there.

### v8

| Hold (RPM) | Closed (LUFS) | Half (LUFS) | Full (LUFS) | Crest (dB) | Centroid (Hz) | Flatness | Rolloff (Hz) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 675 | -18.9 | -13.6 | -13.6 | 8.5 | 1047 | 0.025 | 1920 |
| 2000 | -12.9 | -12.6 | -12.7 | 8.2 | 965 | 0.021 | 1083 |
| 3000 | -12.8 | -12.5 | -12.5 | 7.3 | 934 | 0.021 | 1077 |
| 4000 | -12.9 | -11.9 | -12.0 | 6.1 | 868 | 0.019 | 1018 |
| 5000 | -13.8 | -12.3 | -12.3 | 7.0 | 1049 | 0.024 | 1339 |
| 6000 | -12.4 | -11.7 | -11.6 | 5.6 | 913 | 0.021 | 1015 |

Full-throttle sweep: -13.5 to -11.8 LUFS, peak -5.1 to -3.0 dBFS. Bands at 3000 RPM: -41.1, -29.4, -18.0, -4.4, -2.1, -21.0, -30.3, -34.4, -37.8, -55.5 dB. Half orders: -0.7 to -6.8 dB (the two exhaust systems have different lengths).

### Overrun and limiter

On overrun, the leveller keeps the level up as the RPM falls: v8 -12.4 to -13.8 LUFS from 6000 down to 900 RPM, inline4 -10.6 to -13.1, flat6 -10.3 to -13.8. flat12 falls below the knee under 4000 RPM, from -14.3 to -22.6 LUFS at 1200 RPM. At the limiter the crest factor rises by 3 to 6 dB above the top of the sweep (v8 10.2 to 10.5 dB against 6.4 dB, inline4 13.2 to 13.4 dB against 7.3 dB, with peaks at 0 dBFS). `overrun/describe.md` and `limiter/describe.md` have the windows.

## 7. Cheapest implementation per target

| Target | engine-sim behaviour | Cheapest implementation | Origin | Cost per sample |
| --- | --- | --- | --- | --- |
| Level flat over RPM and throttle above the knee, within the sweep bands of section 6 | Peak leveller (section 3.3) | Port the leveller: peak follower with instant attack and a release of 0.999 per sample at 44.1 kHz (0.999 to the power 44100/48000 at 48 kHz), gain `min(target / peak, 1.9)`, smoothed by a one-pole of 0.1 per sample. Set the pre-gain per profile so that only the closed-throttle idle holds of section 3.3 fall below the knee. To keep the rule of no division per sample, update the gain target once per 16 to 32 samples and keep the smoothing per sample | From engine-sim | About 6 operations, plus one division per block |
| No harmonic content above 2 kHz | Fourth-order Butterworth at 1.9 kHz on the input | Two biquads on the pulse sum before the noise AM, per path | From engine-sim | 10 multiply-adds |
| Fixed formants (section 4) | Impulse response convolution | A bank of 6 to 10 peaking or resonator biquads fitted to the harmonic path curve of each profile, in place of the impulse response. The fit target is the table of section 4, and the acceptance test is the strongest-order table of section 4 and the band columns of section 6. engine-sim impulse responses cannot ship (licence, `reference/engine-sim/README.md`), so the fit must give snarl-owned coefficients | New (the form of the filter), from engine-sim (the curve) | 30 to 50 multiply-adds |
| Same, as a quality reference | Same | A partitioned FFT convolution with a snarl-owned 2048-tap response (46 ms) is within 0.7 dB of the engine-sim response. A direct 2048-tap FIR costs 2048 multiply-adds per sample and does not fit the budget | From engine-sim | About 100 operations with 256-sample partitions (estimate), plus 5.8 ms of latency |
| Noise between harmonics and above 2 kHz (section 5) | Pressure times noise low-passed at 2 kHz, through the impulse response | The existing noise AM, with the noise applied to the pulse before the formant filters, so that the formants and the cycle shape the noise. For flat12, flat6 and v8, use AM with no constant part on the pressure term, mixed with the derivative path at `hf_gain`. Tune the depth per profile to the residual and AM columns of section 5 | From engine-sim | Already in snarl |
| Noise share falls with RPM | The derivative weighting raises the harmonic part with frequency, the noise part does not | A first difference (one subtraction) on the harmonic path, weighted by `hf_gain`, gives this fall with no RPM curve. This follows from the code, and the noise shares of section 3.2 agree | From engine-sim | 2 operations |

Not worth a separate stage: the 0.45 fs low pass (no audible band at 48 kHz), the 10 Hz DC block (snarl has one), the jitter as its own stage (its effect is part of the measured noise share).

## 8. What remains

- A fitting tool for the resonator bank of section 7 (a reviewed `snarl-compare` option with a test), and the fitted coefficients for the four profiles.
- A snarl render through `just analyse` and `snarl-compare --describe` and `--residual`, to give the gap to each target of sections 5 and 6. This analysis did not measure snarl.
- The split of the measured noise into the noise AM part and the jitter part. The coherence gives their sum only.
- The effect of the shared noise filter state in engine-sim on the profiles with two exhaust systems (flat12, inline4 and v8). The code runs that filter twice per sample, which raises its cut-off, but no measurement here isolates it.
