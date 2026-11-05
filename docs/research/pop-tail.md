# The decay after the pops

Status: measured 2026-11-03. Recommendations 1 and 2 implemented 2026-11-04 (section 8). The boom and the band below 200 Hz fixed 2026-11-05 (section 9). Branch `pop-model`. This report measures the sound after the pops in the muffler crackle clips, in the crackle model voices and in the snarl pops on the lift-offs, and the start of the engine-sim exhaust impulse responses. No audio and no measure of a single clip is in the repository. The values are statistics of the whole set.

## 1. Answer

The snarl pops of the kinds `pop` and `burble` (one shock, the single-shock mode of the voice) and `boom` (a short train) end much sooner than a clip. After the last strong shock of a clip, the sound holds for about 110 ms at 5 to 9 dB below the peak, then falls at 0.8 dB/ms. A snarl pop holds for 44 ms, then falls at the same rate. A boom has no hold: it falls 20 dB in 24 ms.

| Full band, end of the sound | Clips | Snarl pop, burble | Snarl boom | Snarl crackle |
| --- | --- | --- | --- | --- |
| Time to -20 dB, ms | 128 (97 to 142) | 62 (55 to 65) | 24 (19 to 70) | 94 (67 to 129) |
| Time to -40 dB, ms | 147 (118 to 162) | 83 (80 to 87) | 48 (41 to 92) | 115 (92 to 168) |
| Level 20 to 100 ms after the peak, dB | -5.0 (-6.1 to -4.4) | -8.1 (-10.4 to -6.7) | -27.8 (-31.0 to -7.2) | -5.9 (-8.5 to -2.6) |
| End of the slow first stage, ms | 111 (83 to 122) | 44 (39 to 50) | 29 (18 to 56) | 77 (50 to 108) |
| Slope after it, dB/ms | -0.81 (-0.86 to -0.71) | -0.86 (-0.93 to -0.63) | -0.73 (-0.89 to -0.34) | -0.83 (-0.96 to -0.78) |

Each cell is the median (p25 to p75). The snarl columns are the pops on the lift-offs of the five engines, after the pop limiter.

- "Very fast release" is the early start of the final fall, not its rate. The final fall of the snarl pops (-0.86 dB/ms) is the same as that of the clips (-0.81 dB/ms). It starts 67 ms sooner for a pop and a burble. A boom reaches -20 dB 104 ms sooner than a clip. In the band above 2 kHz, a snarl pop falls to -30 dB at 44 ms, against 109 ms for the clips.
- "Follow-on" is the slow first stage of the clips: about 110 ms at -0.06 dB/ms (-0.10 to -0.02) after the last strong shock. In it, the band above 2 kHz holds at -19.9 dB (-21.4 to -15.5) for 20 to 100 ms. The snarl pop gives -28.4 dB there, 8.5 dB less.
- The crackle kind matches the clips after a strong shock inside a train. In the full band, all its measures are inside the p25 to p75 of the clips. Its end is 35 ms short at -20 dB.
- The engine adds no tail to the pops. The pops before and after the pop limiter give the same medians, and the pops mix after the engine-sim exhaust impulse response.
- The impulse response of the two V12 profiles (`minimal_muffling_01.wav`) has a follow-on like that of the clips: -6.6 dB from 20 to 100 ms. The responses of v8-ls, i4-vtec and f4-ej25 fall 27 to 32 dB in that window.

I cannot measure whether the final fall of the clips at 0.8 dB/ms is acoustic or a fade of the edit ([crackle-measure.md](crackle-measure.md) section 3). The long clips cannot tell either: all 30 have sound at full level in their last 300 ms. So the 110 ms of the clips is a lower limit of the natural follow-on.

## 2. Recommendations

Recommendations 1 and 2 are implemented (section 8). Recommendation 3 is not. Best first:

1. **Hold the single-shock mode and the short train longer before the fade.** Source: the clip end statistic, the slow first stage of 111 ms (83 to 122). The fade of the voice starts `fade_delay_ms` (40.6 ms) after the last shock. A hold of about 110 ms for `Options.Single` and `Options.Short` will move the full-band time to -20 dB of a pop from 62 ms to about 125 ms, to -40 dB from 83 ms to about 150 ms, and the 20 to 100 ms level from -8.1 dB to about -5 dB. The fall rate stays. Cost per voice: no new work per sample. Each pop sounds about 70 ms longer, at 0.47 ms of CPU per second of voice, so about 0.03 ms per pop, and more voices overlap in a lift-off run (pops 110 to 260 ms apart). The boom also needs a higher bed and body against its shocks: its level 20 to 100 ms after the peak is -27.8 dB, against -8.1 dB for the single shock.
2. **Add weak trailing shocks after the single shock.** Source: the clip end statistic above 2 kHz, -19.9 dB (-21.4 to -15.5) from 20 to 100 ms and -30 dB at 109 ms (78 to 124). A hold alone keeps the band above 2 kHz at the bed level, near -28 dB. A few weak shocks from the measured level table over the hold will raise that band by about 8 dB and move its time to -30 dB from 44 ms to about 110 ms. Cost per voice: a few more grain plays per pop, one multiply-add per sample for each grain that sounds, for at most 200 ms each.
3. **Convolve the pop bus with the profile impulse response.** Source: the engine-sim impulse responses (section 6). This adds a follow-on on the two V12 profiles only (-6.6 dB from 20 to 100 ms, -20 dB at 80 ms). It adds almost none on the other three (-27 to -32 dB from 20 to 100 ms), and it changes the spectrum of the pops, which the clips already hold with their own exhaust. Cost: none per voice, one convolution on the pop bus of 1,893 to 6,907 taps. A direct convolution costs 83 to 305 million multiply-adds per second at 44.1 kHz. A partitioned FFT convolution costs much less. I did not measure either.

## 3. Method

`just pop-tail` runs `tools/crackle --pop-tail` (`tools/crackle/tail.go`) and writes `out/pop-tail/summary.md` and `out/pop-tail/events.csv`.

- **Envelope**: the RMS over a centred 5 ms window, in four bands: the full band, 10 to 200 Hz, 200 Hz to 2 kHz, and 2 to 20 kHz. Each band filter is a fourth-order Butterworth high pass and low pass (`analysis.BandPass`).
- **Peak**: the shocks are those of `tools/crackle` (section 2 of [crackle-measure.md](crackle-measure.md)): the peaks of a 1 ms envelope above 1 kHz. Each band takes its own envelope peak from 2.5 ms before to 5 ms after the shock peak.
- **Time to -10, -20, -30 and -40 dB**: the first time after the peak at which the envelope falls below the level and stays below it for 10 ms, so that a dip between two body cycles does not count.
- **Tail levels**: the mean power from 20 to 100 ms and from 100 to 300 ms after the peak, against the mean power over the 5 ms around the peak.
- **Form**: one line, and the best two lines, through the envelope in dB every 0.5 ms from the peak to -40 dB (or the end of the window, at most 500 ms). The decay is two-stage when two lines leave at most half the squared error of one line and the second line is at most half as steep as the first (a fast drop, then a slow rumble). A form with a slow first line and a fast second line is a hold, then a fall.
- **Events**: (a) each strong shock of the 30 short exterior clips, within 12 dB of the strongest shock, with the drops and the fit up to the next strong shock and the tail levels to the end of the sound. (b) The end event of each short clip: the last shock within 12 dB of the strongest shock in the last 1 s of the active span, to the end of the sound. (c) The same for the 30 long exterior clips. (d) 60 model trains of each voice mode with the seeds 5001 to 5060: `crackle` (a train of measured length), `boom` (a short train of 4 to 16 shocks, as the boom shape of `internal/core/pops.go`), and the single-shock mode, which the engine uses for `pop` and `burble`. (e) Each pop of each kind on the lift-offs of `pop_level_cases`, pops only, after and before the pop limiter, as in [pop-level.md](pop-level.md) section 3. A pop ends at the next pop or after 1 s.
- **Cut ends**: an end is cut when the active span reaches the last 10 ms of the sound, or when the envelope of its last 5 ms is within 30 dB of the end peak. A cut end has no decay to measure, so the tool leaves it out. The end of the sound of a clip is the first fragment after its active span (the edit marks of [crackle-measure.md](crackle-measure.md) section 3), so the 100 to 300 ms level of the clip ends has 7 values of 23.

| Group | Events | Ends left out as cut |
| --- | --- | --- |
| (a) Short clips, strong shocks | 231 | |
| (b) Short clips, end | 23 | 7 to the last sample |
| (c) Long clips, end | 0 | 30 to the last sample |
| (d) Model crackle, strong shocks and end | 510 and 60 | 0 |
| (d) Model boom, strong shocks and end | 423 and 60 | 0 |
| (d) Model single shock, end | 60 | 0 |
| (e) Engine pop and burble, end, each | 48 | 0 |
| (e) Engine crackle, strong shocks and end | 385 and 30 | 18, the next pop starts first |
| (e) Engine boom, strong shocks and end | 421 and 43 | 5, the next pop starts first |

The kinds `pop` and `burble` give the same medians: both play the single-shock mode, and the relative measures do not see the level. The tables below give (e). The model voices alone (d) give the same form. The single-shock mode alone differs from the engine pops by at most 1.1 ms in the full-band times to -10 to -40 dB and 0.7 dB in the 20 to 100 ms level. The crackle and the boom alone differ more (up to 16 ms and 2.5 dB), because the engine sets their lengths from the pop shape. The values of (d) are in `out/pop-tail/summary.md`.

## 4. After a strong shock, inside a train

Median (p25 to p75), with [n] the count of events with a value when some have none. Inside a train, the next strong shock comes about 32 ms later, so few events reach -20 dB before it.

| Measure | Band | (a) Clips | (e) Snarl crackle |
| --- | --- | --- | --- |
| Time to -10 dB, ms | 200 Hz to 2 kHz | 7.3 (6.0 to 15.1) [127] | 22.2 (12.4 to 49.1) [63] |
| Time to -10 dB, ms | above 2 kHz | 4.8 (4.4 to 6.7) [129] | 9.9 (8.0 to 12.7) [148] |
| 20 to 100 ms, dB | full band | -3.1 (-4.9 to -1.2) | -2.1 (-4.7 to 0.5) |
| 20 to 100 ms, dB | 200 Hz to 2 kHz | -8.9 (-12.4 to -5.8) | -4.8 (-8.4 to -1.5) |
| 20 to 100 ms, dB | above 2 kHz | -9.3 (-17.1 to -4.1) | -8.5 (-14.8 to -3.2) |
| 100 to 300 ms, dB | full band | -4.4 (-7.0 to -2.6) | -5.3 (-11.7 to -2.1) |
| Two-stage, per cent of fits | above 2 kHz | 60 | 64 |
| Fast slope, dB/ms | above 2 kHz | -0.92 (-1.96 to -0.37) | -0.78 (-1.19 to -0.43) |

After a strong shock, the clips fall 10 dB in 5 to 7 ms above 200 Hz, then hold about 9 dB down while the train goes on. The snarl crackle falls more slowly above 200 Hz and holds 1 to 4 dB higher. The snarl `boom` gives -3.9 dB (-10.0 to 0.3) full band from 20 to 100 ms, and -59.3 dB from 100 to 300 ms, because its train ends.

## 5. At the end of the sound

| Measure | Band | (b) Clips | (e) Pop, burble | (e) Boom | (e) Crackle |
| --- | --- | --- | --- | --- | --- |
| Time to -10 dB, ms | full band | 70.9 (44.8 to 117.4) | 48.9 (26.5 to 51.2) | 13.7 (6.6 to 47.8) | 56.2 (45.1 to 99.7) |
| Time to -20 dB, ms | full band | 128.2 (97.3 to 142.0) | 62.0 (55.5 to 64.7) | 24.3 (19.4 to 70.1) | 93.6 (66.7 to 128.8) |
| Time to -30 dB, ms | full band | 136.2 (106.8 to 151.4) | 69.1 (66.5 to 71.3) | 34.3 (27.2 to 77.3) | 110.7 (77.4 to 151.9) |
| Time to -40 dB, ms | full band | 146.7 (117.5 to 161.6) | 83.2 (80.1 to 86.6) | 47.7 (41.0 to 91.8) | 115.0 (92.1 to 167.5) |
| Time to -20 dB, ms | below 200 Hz | 137.4 (108.8 to 152.1) | 96.1 (94.4 to 98.8) | 57.6 (48.8 to 101.6) | 127.0 (97.2 to 166.2) |
| Time to -40 dB, ms | below 200 Hz | 161.2 (132.2 to 177.0) | 201.1 (186.3 to 204.0) | 155.2 (114.3 to 176.2) | 226.0 (202.8 to 262.2) |
| Time to -20 dB, ms | 200 Hz to 2 kHz | 45.6 (28.3 to 87.3) | 45.4 (42.7 to 46.5) | 21.8 (19.2 to 49.9) | 86.1 (53.4 to 116.6) |
| Time to -30 dB, ms | 200 Hz to 2 kHz | 112.9 (83.1 to 130.3) | 55.4 (52.7 to 57.5) | 31.6 (29.0 to 66.0) | 99.3 (66.8 to 138.7) |
| Time to -20 dB, ms | above 2 kHz | 40.5 (34.0 to 75.5) | 20.4 (18.8 to 22.6) | 21.8 (16.6 to 28.2) | 44.1 (27.3 to 66.8) |
| Time to -30 dB, ms | above 2 kHz | 109.1 (78.3 to 124.2) | 44.3 (42.2 to 47.0) | 33.6 (27.8 to 48.1) | 83.2 (53.5 to 119.7) |
| 20 to 100 ms, dB | full band | -5.0 (-6.1 to -4.4) | -8.1 (-10.4 to -6.7) | -27.8 (-31.0 to -7.2) | -5.9 (-8.5 to -2.6) |
| 20 to 100 ms, dB | below 200 Hz | 0.6 (-5.2 to 4.0) | -2.1 (-4.7 to 1.3) | -12.9 (-18.0 to -3.5) | -4.0 (-7.1 to 0.3) |
| 20 to 100 ms, dB | 200 Hz to 2 kHz | -14.5 (-19.3 to -11.1) | -16.0 (-18.5 to -14.7) | -30.8 (-32.1 to -13.5) | -10.5 (-13.4 to -8.2) |
| 20 to 100 ms, dB | above 2 kHz | -19.9 (-21.4 to -15.5) | -28.4 (-29.6 to -27.0) | -28.3 (-33.0 to -23.5) | -20.2 (-23.6 to -15.7) |
| 100 to 300 ms, dB | full band | -18.6 (-23.9 to -15.2) [7] | -70.5 (-74.2 to -67.1) [30] | -70.6 (-106.2 to -49.4) [20] | -32.0 (-65.7 to -11.6) [29] |
| One line, dB/ms | full band | -0.18 (-0.26 to -0.12) | -0.44 (-0.48 to -0.40) | -0.87 (-0.97 to -0.39) | -0.22 (-0.38 to -0.13) |
| First line, dB/ms | full band | -0.06 (-0.10 to -0.02) | -0.13 (-0.23 to -0.04) | -0.81 (-1.10 to -0.06) | -0.05 (-0.14 to 0.00) |
| Second line, dB/ms | full band | -0.81 (-0.86 to -0.71) | -0.86 (-0.93 to -0.63) | -0.73 (-0.89 to -0.34) | -0.83 (-0.96 to -0.78) |
| Break, ms | full band | 111.2 (82.8 to 121.7) | 43.7 (38.9 to 50.4) | 28.9 (17.7 to 56.1) | 76.8 (49.5 to 107.5) |
| Two-stage, per cent of fits | full band | 0 | 2 | 21 | 0 |
| Two-stage, per cent of fits | above 2 kHz | 52 | 54 | 21 | 13 |

- In the full band, the end of a clip is a hold, then a fall: a first line at -0.06 dB/ms for about 110 ms, then -0.81 dB/ms. It is not a fast drop with a slow rumble after it. The snarl pop has the same form with a hold of 44 ms. This hold is the `fade_delay_ms` of the model (40.6 ms), and the fall is its `fade_db_per_ms` (0.98 dB/ms).
- Above 2 kHz, half of the clip ends are two-stage: the band falls 10 dB in 6.9 ms (4.7 to 8.9), then holds near -20 dB until the final fall. The snarl pop falls as fast at first (9.2 ms), then holds 8.5 dB lower.
- Below 200 Hz, the snarl pop lasts longer than the clips at -40 dB (201 ms against 161 ms). The cause is the damped sine at 27 Hz of the model, which starts at the fade, or the ringing of the 10 Hz high pass of the band after the voice ends. I did not separate the two.

## 6. The engine-sim impulse responses

The first channel of each profile, trimmed as engine-sim trims it: the response ends after its last sample above 100 of 32767, at most 10,000 taps. The Schroeder times are those of `analysis.EnergyDecay`. One response per file, so no p25 to p75.

| Profiles | File | Taps | Schroeder -20 / -40 dB, ms | Full band -10 / -20 / -40 dB, ms | 20 to 100 ms, dB | 100 to 300 ms, dB | Below 200 Hz, 20 to 100 ms, dB | Above 2 kHz, 20 to 100 ms, dB |
| --- | --- | ---: | --- | --- | ---: | ---: | ---: | ---: |
| v8-ls | `smooth_39.wav` | 6,907 | 45.0 / 107.7 | 18.2 / 19.6 / 72.0 | -32.2 | -56.2 | -17.8 | -43.1 |
| i4-vtec, f4-ej25 | `mild_exhaust.wav` | 1,893 | 25.3 / 37.0 | 16.1 / 27.3 / 38.3 | -27.3 | none, the response ends at 43 ms | -25.5 | -22.3 |
| v12-412, v12-merlin | `minimal_muffling_01.wav` | 5,979 | 74.6 / 113.4 | 52.2 / 80.2 / 115.4 | -6.6 | -44.1 | -8.1 | -7.0 |

- `smooth_39.wav` is two-stage in the full band: a fast drop to -18.9 dB at 19.5 ms (first line -0.64 dB/ms), then a slow stage at -0.29 dB/ms.
- `minimal_muffling_01.wav` holds within 3 dB of its peak for 31 ms and then falls at -0.43 dB/ms in the bands above 200 Hz, a form like that of the clip ends with a shorter hold.
- `mild_exhaust.wav` is over in 43 ms.

## 7. Reproduce

```sh
just pop-tail
```

The recipe takes about 12 s. The test `TestMeasureDecayExponential` checks the times, the slope and the 20 to 100 ms level on an exponential decay with a time constant of 20 ms, and `TestMeasureDecayTwoStage` checks the slow slope and the break of a fast and a slow exponential.

## 8. The hold and the trailing shocks

Date: 2026-11-04. The pops of the kinds `pop`, `burble` and `crackle` now hold for about 110 ms after the last strong shock, as the clips do, and then fall at the same rate as before. A pop and a burble also carry weak trailing shocks in the hold, so that the band above 2 kHz holds at the level of the clips. The boom is longer, but half of the booms still end short. The loudness of the bang stays within 0.4 LU.

### 8.1 The change

- **Hold** (`hold_ms` of `internal/crackle/crackle.toml`, 111.2 ms). The fade of every train starts `fade_delay_ms` after the last shock, and at least `hold_ms` after the last strong shock: a shock within 12 dB of the strongest shock of the train before it, as the strong shocks of `tools/crackle`. The value is the median break of the full band after the end event of the 23 uncut clip ends (section 5). The hold applies to the single-shock mode, the short train and the trains of a length, so it also closes the short end of the `crackle` kind.
- **Trailing shocks** (`trail_shocks`, `trail_gap_ms`, `trail_level_db`). After the last shock of a single shock and of a short train, the voice plays a count of trailing shocks from the grain bank, each at a gap after the shock before and at a level over the strongest shock of the train, until the fade. The three quantile tables are the shocks in the hold of the 23 uncut clip ends: the shocks after the end event whose peak is at most the full-band break after its peak. The clips have 8 (5 to 12) such shocks per end, at gaps of 7.6 ms (5.3 to 12.4) and levels of -22.8 dB (-25.5 to -19.5), from 206 shocks.
- **Bed and body of the boom** (`short_body_db`, -1.4 dB). The fit sets the bed and the body of the trains against the loudest 5 ms frame of the clips. The loudest frame of a model boom is 1.4 dB below that of a model train (-7.3 against -5.9 dB, the medians of 60 trains each), so the measured clip ratio lowers the bed and the body of the boom by 1.4 dB and does not raise them. The boom ends short for another reason: its train builds up the slow ring of the shock activity (`act_ms`, `act_db`), which falls when the train ends. The trailing shocks hold the boom up only in part.
- **Calibration**. `calibrate` in `internal/core/calibrate.go` now renders the calibration voices with the short tail (`crackle.Params.ShortTail`: no hold, no trailing shocks, no boom body gain). With the long tail, the span that holds 90 per cent of the energy grows, and the bang of a pop rose by 1.0 LU, of a boom by 1.3 LU and of a burble by 1.0 LU. With the short tail, the kind gains are those of commit 4cd7850.
- **Sources**. `just crackle-fit` runs `tools/crackle --fit`, then `--fit-tail`, which sets the five keys from the clip ends and writes their sources (kinds measured and derived) to `crackle.sources.toml`. The fit trains of `--fit` have the short tail, so the other keys do not move. I did not run `--fit` again: only `just crackle-fit-tail`.

The voice adds two multiplies per sample (the gain of the bed and of the body) and the trailing shocks as grain plays. No new filter runs per sample.

### 8.2 The decay, before and after

The end event of the engine pops on the lift-offs, after the pop limiter, from `just pop-tail`. Median (p25 to p75). Before is section 5.

| Measure | Clips | Pop, before | Pop, after | Boom, before | Boom, after | Crackle, before | Crackle, after |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Full band, -20 dB, ms | 128.2 (97.3 to 142.0) | 62.0 (55.5 to 64.7) | 123.7 (120.1 to 132.7) | 24.3 (19.4 to 70.1) | 46.2 (21.6 to 126.7) | 93.6 (66.7 to 128.8) | 132.5 (126.5 to 140.5) |
| Full band, -40 dB, ms | 146.7 (117.5 to 161.6) | 83.2 (80.1 to 86.6) | 152.6 (140.8 to 156.8) | 47.7 (41.0 to 91.8) | 72.1 (43.7 to 152.5) | 115.0 (92.1 to 167.5) | 157.1 (150.9 to 169.1) |
| Full band, 20 to 100 ms, dB | -5.0 (-6.1 to -4.4) | -8.1 (-10.4 to -6.7) | -3.8 (-6.0 to -0.7) | -27.8 (-31.0 to -7.2) | -16.1 (-32.1 to -2.4) | -5.9 (-8.5 to -2.6) | -3.9 (-6.1 to -0.4) |
| Above 2 kHz, 20 to 100 ms, dB | -19.9 (-21.4 to -15.5) | -28.4 (-29.6 to -27.0) | -21.2 (-22.8 to -19.5) | -28.3 (-33.0 to -23.5) | -26.5 (-30.9 to -18.9) | -20.2 (-23.6 to -15.7) | -19.0 (-23.0 to -14.8) |

The burble gives the same values as the pop within 0.4 dB and 0.5 ms.

- A pop and a burble are now inside the clip p25 to p75 at -20 dB, at -40 dB and above 2 kHz. Their 20 to 100 ms level in the full band is 1.2 dB above the clip median and 0.6 dB above the clip p75. The bed holds at its full level until the fade, and the clips sink a little in the hold (first line -0.06 dB/ms).
- The crackle kind is inside the clip p25 to p75 at -20 dB and above 2 kHz, and 10 ms above the clip median at -40 dB.
- The boom is bimodal: half of the booms hold (p75 127 ms at -20 dB), and half still end within 25 ms. It is the only kind outside the clips at the median.
- Below 200 Hz, the pops last longer than the clips: -40 dB at 269 ms against 161 ms (201 ms before). The damped sine of the tail and the body start at the fade, which is now 70 ms later.
- In the hold, `tools/crackle` finds 11 (8 to 12) shocks after a pop against 8 (5 to 12) after a clip end. The detector also finds peaks of the bed, which give 3 shocks in the 44 ms of the old hold.

### 8.3 The level of the bang

The mean over the five engines of the medians over the pops, in LU over the engine with no pops, from `tools/crackle --pop-level`. Before is `--short-tail`, which gives the values of [pop-level.md](pop-level.md) to 0.1 LU. Bang is the K-weighted level over the first 50 ms from the start of the pop.

| Kind | Bang, before | Bang, after | Span, before | Span, after | Momentary, before | Momentary, after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| pop | -4.3 | -3.9 | -4.1 | -4.9 | -11.5 | -8.5 |
| boom | -6.4 | -6.6 | -6.1 | -6.7 | -9.8 | -8.9 |
| crackle | -7.5 | -7.5 | -7.2 | -7.1 | -6.6 | -6.5 |
| burble | -13.5 | -13.1 | -13.3 | -14.1 | -20.7 | -17.6 |

The bang moves by 0.4 LU at most. The momentary loudness over 400 ms rises by up to 3.1 LU, because the hold adds sound after the bang. `TestPopLevelOverEngine` passes with its bands unchanged.

### 8.4 The model check and the cost

`just crackle-model` checks 60 model trains against the 30 clips. The hold changes the trains of a length, so the check moves: 175 measures pass against 174 before. Three measures that passed now fail by 0.2 dB or less: the share of 62.5 to 125 Hz between the shocks, the share of 2 to 4 kHz in the active span, and the band 250 to 500 Hz at 320 ms from the strongest shock. Four that failed now pass. The active length is 529 ms against 514 ms before (clips 544 ms), and the tail after the last shock stays at 69 ms (clips 69 ms).

`BenchmarkPops` and `BenchmarkRender` on an AMD Ryzen 9 7940HS, 8 runs each, compared by benchstat:

| Benchmark | Before, µs per block | After, µs per block | Change |
| --- | ---: | ---: | ---: |
| `BenchmarkPops`, five profiles | 106.2 to 161.8 | 114.3 to 168.2 | +3.9 to +7.6 per cent, p = 0.000 |
| `BenchmarkRender`, five profiles | 92.7 to 167.1 | 94.2 to 168.1 | no significant change |

The pops cost 6 to 9 µs more per 256-frame block, 0.12 to 0.16 per cent of real time at 48 kHz. Most of it is more voices that sound at once: a pop now sounds about 70 ms longer, and the benchmark starts a pop every 64 ms. With the pops off, the engine output is byte-identical to commit 4cd7850 on the lift-offs of the five engines.

### 8.5 Listen

```sh
just listen-pop-tail
```

`out/listen-pop-tail/pop-tail-old-then-new.wav` (137 s, -19.0 LUFS) plays for each of the five engines 2 lift-offs with the old tail, then the same 2 lift-offs with the new tail. `out/listen-pop-tail/README.txt` gives the start of each part. `tools/crackle --pop-tail-listen` renders the old tail with `core.UseShortPopTail`, so the recipe needs no old commit.

## 9. The boom and the band below 200 Hz

Date: 2026-11-05. Every boom now holds as the clips do, and the band below 200 Hz of a pop, a burble and a boom falls to -20 dB at the clip time. At -40 dB below 200 Hz, these kinds still last 30 to 51 ms longer than the clip median. The bang keeps its loudness within 0.3 LU, and the model check does not change.

### 9.1 The causes

- **The boom.** The ring of the shock activity is not the cause. With the ring held at its level from the last shock to the fade, the model booms stay bimodal: 28 of 60 reach -20 dB within 50 ms, against 25 before. The renders of a short boom and of a held boom both hold for about 140 ms. The cause is the level of the strongest shock. The level table holds the levels against the strongest shock of each clip, so a train of 28 to 61 shocks draws a shock near its top. A boom of 4 to 16 shocks seldom does, so its strongest shock is only a few dB over the peaks of the bed. Those peaks then count as strong shocks (within 12 dB of the strongest) up to the start of the fade, and the decay from the last of them is the fade alone, 20 dB in about 25 ms. In the clips, the shocks in the hold are 22.8 dB (25.5 to 19.5) below the strongest shock.
- **The band below 200 Hz.** The body, the bed and the hold are not the cause. It is the tail, the damped sine at 27.3 Hz that starts from rest at the fade. It falls 20 dB in 22 ms, so its spectrum is flat from 0 to 27 Hz, and that sound holds up the 10 Hz edge of the band of the measure for about 160 ms after the fade. The hold moved the fade, and with it the tail, 70 ms later. The model single shocks give these times below 200 Hz, -20 dB / -40 dB: 165.6 / 271.7 ms with the tail, 130.8 / 164.3 ms with no tail, 148.2 / 217.2 ms with the tail 10 dB lower, and 174.7 / 229.9 ms with the tail through a 15.6 Hz high pass. The clips give 137.4 / 161.2 ms. With no tail, the full band and the band above 2 kHz stay inside the clip p25 to p75.

The two clip statistics disagree on the tail. The trains of a length in the model check match the clip tail measures (27 Hz, 3 half cycles, 22 ms to -20 dB) only with the tail at its full level. A tail that starts 40.6 ms after the last shock, in the hold, gives 135.2 / 199.0 ms below 200 Hz, and the model check passes 171 measures instead of 175, with three tail measures that fail.

### 9.2 The change

- **Boom** (commit 7a4985a). The levels of a short train (`Options.Short`) move together so that its strongest shock is at the top of `level_db`, as the strongest shock of each clip is. The voice draws the levels of the train ahead at the trigger, on a copy of its noise and of the level state, so that the train keeps the draws of the levels as drawn: only the move is new. The short tail (`Params.ShortTail`) keeps the levels as drawn. The calibration in `internal/core/calibrate.go` renders each boom of the calibration also with the long tail and with the levels as drawn, and takes the mean move of the K-weighted loudness of the first 50 ms out of the boom gain, so that the bang keeps its loudness. `just crackle-fit-tail` sets `short_body_db` again from the new booms: -1.15 dB, against -1.38 dB before.
- **Band below 200 Hz** (commit b936996). `hold_tail_db` (-11.35 dB) is the gain of the tail of a single shock and a short train over `tail_gain_db`. `tools/crackle --fit-tail` sets it by bisection from -60 to 0 dB, so that the median time to -20 dB below 200 Hz after the end event of 60 model single shocks is the median of the 23 uncut clip ends, 137.4 ms. Its source is derived. The trains of a length keep the tail, so the `crackle` kind keeps its long band below 200 Hz, and the model check does not change. The short tail has no such gain.

The voice adds no work per sample. At each trigger of a boom, it draws the levels of the train twice.

### 9.3 The decay, before and after

The end event of the engine pops on the lift-offs, after the pop limiter, from `just pop-tail`. Median (p25 to p75). Before is commit 1f271ef (section 8.2), after is commit b936996.

| Measure | Clips | Pop, before | Pop, after | Burble, before | Burble, after | Boom, before | Boom, after | Crackle, before and after |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Full band, -20 dB, ms | 128.2 (97.3 to 142.0) | 123.7 (120.1 to 132.7) | 120.6 (113.0 to 125.1) | 123.4 (119.7 to 132.7) | 120.3 (109.4 to 125.1) | 46.2 (21.6 to 126.7) | 119.3 (117.0 to 125.7) | 132.5 (126.5 to 140.5) |
| Full band, -40 dB, ms | 146.7 (117.5 to 161.6) | 152.6 (140.8 to 156.8) | 140.2 (131.2 to 145.5) | 153.0 (141.5 to 156.5) | 139.1 (131.2 to 145.5) | 72.1 (43.7 to 152.5) | 143.4 (135.3 to 150.1) | 157.1 (150.9 to 169.1) |
| Full band, 20 to 100 ms, dB | -5.0 (-6.1 to -4.4) | -3.8 (-6.0 to -0.7) | -4.1 (-8.4 to -0.6) | -3.4 (-6.0 to -0.7) | -3.7 (-8.1 to -0.6) | -16.1 (-32.1 to -2.4) | -3.4 (-6.3 to -1.7) | -3.9 (-6.1 to -0.4) |
| Above 2 kHz, 20 to 100 ms, dB | -19.9 (-21.4 to -15.5) | -21.2 (-22.8 to -19.5) | -21.2 (-22.8 to -19.4) | -20.9 (-22.6 to -19.3) | -20.7 (-22.6 to -19.1) | -26.5 (-30.9 to -18.9) | -19.4 (-21.7 to -15.5) | -19.0 (-23.0 to -14.8) |
| Below 200 Hz, -20 dB, ms | 137.4 (108.8 to 152.1) | 164.6 (157.9 to 167.7) | 131.5 (121.4 to 154.7) | 164.6 (157.8 to 167.4) | 131.4 (121.3 to 152.4) | 80.6 (53.4 to 163.7) | 132.8 (128.8 to 151.5) | 166.8 (155.3 to 173.9) |
| Below 200 Hz, -40 dB, ms | 161.2 (132.2 to 177.0) | 269.1 (231.4 to 274.0) | 212.1 (177.2 to 223.7) | 269.0 (229.7 to 273.9) | 208.4 (172.0 to 223.3) | 169.0 (121.8 to 264.4) | 191.1 (168.2 to 223.5) | 268.8 (228.8 to 275.7) |

- Every measure of section 8 that was inside the clip p25 to p75 stays inside: the pop and the burble at -20 dB, at -40 dB and above 2 kHz, and the crackle, which does not change.
- The boom is now inside the clip p25 to p75 at -20 dB, at -40 dB and above 2 kHz. Its 20 to 100 ms level in the full band is 1.0 dB above the clip p75, as that of the pop is.
- Below 200 Hz, the pop, the burble and the boom are inside the clips at -20 dB. At -40 dB they are 14 to 35 ms above the clip p75. The `crackle` kind keeps its long band below 200 Hz.

### 9.4 The level of the bang

The mean over the five engines of the medians over the pops, in LU over the engine with no pops, from `just pop-level`.

| Kind | Bang, before | Bang, after | Span, before | Span, after | Momentary, before | Momentary, after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| pop | -3.9 | -3.9 | -4.9 | -4.8 | -8.5 | -8.6 |
| boom | -6.6 | -6.3 | -6.7 | -7.5 | -8.9 | -10.7 |
| crackle | -7.5 | -7.5 | -7.1 | -7.1 | -6.5 | -6.5 |
| burble | -13.1 | -13.1 | -14.1 | -14.0 | -17.6 | -17.7 |

The bang moves by 0.3 LU at most. The boom is 1.8 LU lower over 400 ms, because its bed and body are lower against its bang. `TestPopLevelOverEngine` passes with its bands unchanged.

The level of the boom bang is sensitive to the draws. With the levels of a short train drawn at the trigger in another order and no move, the boom bang over the engine moves by 1.2 LU. So the voice draws the levels ahead on a copy of its noise, and the train keeps the draws of commit 1f271ef.

### 9.5 The model check and the cost

`just crackle-model` gives the same result before and after: 175 measures pass, and no measure changes. The change touches only the single shock and the short train, and the check renders trains of a length.

`BenchmarkPops` and `BenchmarkRender` on an AMD Ryzen 9 7940HS, 8 runs each, compared by benchstat. The first pair of runs, before then after, showed +14 to +33 per cent on both benchmarks, also on `BenchmarkRender`, which plays no pops and whose output does not change. The load average of the PC was 4 to 6 during the runs. The pair below alternates the two test binaries in 4 rounds of 2 runs each.

| Benchmark | Before, µs per block | After, µs per block | Change |
| --- | ---: | ---: | ---: |
| `BenchmarkPops`, five profiles | 113.8 to 168.9 | 115.2 to 167.1 | no significant change, geomean -0.3 per cent over both |
| `BenchmarkRender`, five profiles | 94.1 to 168.3 | 93.2 to 169.6 | no significant change |

With the pops off, the engine output is byte-identical to commit 1f271ef on the lift-offs of the five engines.

### 9.6 Listen

```sh
just listen-pop-tail-fixed out/listen-pop-tail-2/current out/listen-pop-tail-2
```

`out/listen-pop-tail-2/pop-tail-current-then-fixed.wav` (207 s, -19.1 LUFS) plays for each of the five engines 2 lift-offs with the tail of commit 1f271ef, then the same 2 lift-offs with the fixed tail. The boom is rare in the lift-offs: 2 in 5 first pops of a lift-off run and 1 in 5 later pops. So each engine then plays a boom part: the first lift-off with only the boom in use, with the current tail, then with the fixed tail. `out/listen-pop-tail-2/README.txt` gives the start of each part. The silence detection of ffmpeg finds the same starts in the file. `tools/crackle --pop-tail-fixed` cannot render an earlier tail, so the cuts of the current tail in `out/listen-pop-tail-2/current/cuts` came from the same recipe at commit 005af2a, before the fixes.
