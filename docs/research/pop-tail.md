# The decay after the pops

Status: measured, no change made. Date: 2026-11-03. Branch `pop-model`. This report measures the sound after the pops in the muffler crackle clips, in the crackle model voices and in the snarl pops on the lift-offs, and the start of the engine-sim exhaust impulse responses. No audio and no measure of a single clip is in the repository. The values are statistics of the whole set.

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

Not implemented. Best first:

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
