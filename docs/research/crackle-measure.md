# Short exterior crackles: measures for the pop model

Status: measured. Date: 2026-10-22. Branch `pop-model` after `51f94ef`. This report gives the measures of the 30 short exterior crackle clips of the muffler crackle pack ([crackles.md](crackles.md)), and of the own pops of snarl as a baseline. The model phase selects a pop model from these measures. Every number below comes from `tools/crackle` on the clips or on snarl renders. No audio and no measure of a single clip is in the repository. The range of a distribution, its minimum and its maximum, is a statistic of the class, and it cannot rebuild a clip. The measures of each clip, shock and frame are in `out/crackle/`.

## 1. Answer

The 30 clips are one population of crackle trains, not separate classes. No clip is a single bang. Each clip is one burst of 368 to 637 ms (two clips have two bursts) with 28 to 61 shocks, of which 2 to 14 are within 12 dB of the strongest. The shocks come every 8.6 ms (median of the clip medians), and the strong shocks every 32 ms (pooled median). A low body near 52 Hz sounds through the whole burst and carries 78 per cent of the energy (the share below 125 Hz is -1.1 dB). k-means on six features gives a mean silhouette of 0.25 to 0.28 for 2 to 5 classes, which is no clear structure.

The measures that separate the clips from the current snarl pops are the crack and the body:

- Crack energy: the share above 2 kHz is -16.6 dB in the clips and -24.3 to -27.4 dB in the four snarl kinds, so snarl has 7.7 to 10.8 dB less crack energy for its level.
- Shock crest factor: the strong shocks of the clips have a crest factor of 8.5 dB, and those of snarl 6.3 to 7.0 dB.
- Crack between the shocks: above 2 kHz, the clips have only 3.5 dB less power between the shock windows than inside them. The snarl pop has 10.4 dB less.
- Body: the clips have a body near 52 Hz with 78 per cent of the energy below 125 Hz. The snarl pop has its body near 183 Hz and 5 per cent below 125 Hz (-13.1 dB).
- Strong shock spacing: 32.2 ms in the clips, 8.0 to 10.2 ms in snarl (pooled medians).

The rise of a shock is about the same: 2.25 ms in the clips and 1.81 to 2.41 ms in snarl, 10 to 90 per cent above 1 kHz.

The two channels are near mono: correlation 0.995, delay below 0.003 ms, level difference 0.13 dB, side 25.8 dB below mid. A mean of the two channels loses nothing that the measures can see.

The clips give a measured source for `KindCrackle`, a partial source for `KindBoom` (the deep body and the burst length, but not the bright onset), and no source for `KindPop` or `KindBurble` (section 7).

## 2. Clips and method

The clips are `Short_Cracles/crackle_1.wav` to `crackle_30.wav` of `Unity Engine/Muffler Sound Pack/Assets/Audio/Muffler/` in `out/pack/muffler/MufflerCrackleSounds/`. The `Interior/` folder and `Long_Cracles/` are not used. Each clip is 16-bit stereo at 44.1 kHz and 637.35 ms long. The mean of the two channels has a sample peak of -0.5 to -0.3 dBFS.

`just crackle-measure` runs `tools/crackle` on the 30 clips. The tool measures the mean of the two channels, apart from the channel measures. The definitions:

| Measure | Definition |
| --- | --- |
| Quiet | A sample at or below -80 dBFS on every channel. |
| Burst | A part of the file between quiet spans of at least 20 ms. |
| Active span | From the first burst to the last burst whose peak is within 30 dB of the highest burst. A later burst is a fragment. |
| Shock | A peak of the 1 ms RMS envelope of the signal through a fourth-order Butterworth band pass from 1 to 20 kHz that is the highest within 3 ms, within 30 dB of the highest peak of the clip, and 6 dB above the lowest envelope since the shock before. The file counts as quiet before the active span. |
| Strong shock | A shock within 12 dB of the strongest shock of its clip. |
| Shock onset | The 10 per cent point of the rise of the 0.25 ms RMS envelope above 1 kHz, from its lowest value since the shock before to its peak. The gap is from onset to onset. |
| Rise above 1 kHz | 10 to 90 per cent of the same rise. |
| Full-band rise | 10 to 90 per cent of the running full-band sample peak, from the onset to the full-band peak of the shock span. |
| Shock span | From the onset to the next onset, at most 20 ms. The full-band peak and the shock crest factor are over this span. |
| Crest factor | Sample peak over RMS level, in dB. |
| Bands | Octave bands with edges at 15.625, 31.25, 62.5, 125, 250, 500 Hz, 1, 2, 4, 8, 16 and 20 kHz, each a fourth-order Butterworth high pass and low pass (`analysis.BandPass`). Band energy over time is the mean power in 5 ms frames. |
| Shock window | From 0.5 ms before the onset for 6 ms. "Between the shocks" is the active span outside all shock windows. |
| Body | The highest peak of the Hann-windowed spectrum of the active span from 10 to 500 Hz. |
| Tail | From 10 ms after the onset of the last shock to the end of the active span, when it is at least 30 ms long. A 2 ms moving average takes the cracks out. The half cycles are the spans between zero crossings. The tail frequency comes from the median half cycle, and the decay from a straight line through the half-cycle peaks in dB, from the highest peak until they fall 30 dB. |
| Channels | Cross correlation over the root of the product of the channel energies, at no delay and at the best delay within 5 ms. |
| Classes | k-means with 50 seeds on six standardised features (log active length, log shock count, log strong shock count, share above 2 kHz, share below 125 Hz, crest factor), for 2 to 5 classes, scored by the mean silhouette. |

The band filters smear the band energy in time. The tool measures the smear as the time from the loudest 5 ms frame of the impulse response of each band filter to the first frame 20 dB below it: 80 ms for 15.625 to 31.25 Hz, 40 ms for 31.25 to 62.5 Hz, 20 ms for 62.5 to 125 Hz, 15 ms for 125 to 250 Hz, and 5 ms for every band from 250 Hz. A band level below 125 Hz in a 5 ms frame or a 6 ms shock window holds energy from up to 20 to 80 ms before. The tail measure uses no band filter for this reason.

All values below are median (p25 to p75, range min to max) over the 30 clips, unless the row says "pooled", which is over all shocks of all clips.

## 3. Clip edges and active length

| Measure | Value |
| --- | --- |
| Start of the active span | 0.0 ms in all 30 clips |
| Sample peak of the first 10 ms | -5.1 dBFS (-8.7 to -3.5, range -15.5 to -0.3) |
| Active length | 544 ms (476 to 574, range 368 to 637) |
| Bursts | 1 in 28 clips, 2 in 2 clips, with 22.9 and 45.7 ms of quiet between them |
| Level of the last 5 ms against the loudest 5 ms | -72.6 dB (-74.9 to -70.7, range -81.3 to -3.6) |
| Fall over the last 20 ms of the active span | -18.0 dB (-19.7 to -11.5, range -20.7 to +2.7) |
| Fragment after the active span | in 23 clips, from 608.3 ms (600.8 to 617.9), with a peak of -47.3 dBFS (-58.1 to -32.2) |

The clips have no quiet lead-in: the sound starts at the first sample, at a level 5.1 dB below full scale. So the onset of the event is cut, and the first shock of each clip has no measured rise. A quiet span (at or below -80 dBFS) exists only at the end of a clip, not before the event. In 23 clips, a fragment 32 to 58 dB below full scale starts 600 to 618 ms into the clip, after a quiet span. The tool keeps the fragment out of the active span.

The end of the active span falls 18 dB in its last 20 ms. I cannot measure whether that fall is the acoustic decay of the event or a fade of the edit. Seven clips have sound to the last sample, with the last 5 ms at -3.6 to -23.8 dB against the loudest 5 ms, so their events are cut at the end.

## 4. Shocks

| Measure | Value |
| --- | --- |
| Shocks per clip | 40 (37 to 48, range 28 to 61) |
| Shocks within 6 dB of the strongest | 4 (3 to 5, range 1 to 8) |
| Strong shocks per clip | 8 (6 to 8, range 2 to 14) |
| Shocks per second of active length | 77 (70 to 91, range 56 to 105) |
| Median gap between shocks | 8.6 ms (8.0 to 9.7, range 6.9 to 11.8) |
| Median gap between strong shocks | 51.2 ms (17.2 to 74.0, range 10.5 to 356.2) |
| Median rise above 1 kHz | 2.25 ms (2.00 to 2.82, range 1.63 to 3.38) |
| Median rise above 1 kHz of the strong shocks | 2.39 ms (1.50 to 3.44, range 0.41 to 4.59) |
| Onset of the strongest shock | 165.0 ms (129.0 to 235.8, range 2.6 to 477.9) |

Pooled over the shocks:

| Measure | All shocks (1,251) | Strong shocks (231) |
| --- | --- | --- |
| Gap to the shock before, ms | 8.9 (5.8 to 13.2, range 0.7 to 169.0) | 32.2 (12.5 to 77.9, range 2.0 to 356.2), to the strong shock before |
| Level against the strongest shock, dB | -21.0 (-24.0 to -15.1, range -30.0 to 0.0) | -5.6 (-8.7 to -2.0, range -12.0 to 0.0) |
| Full-band sample peak, dBFS | -7.3 (-10.3 to -5.0, range -22.7 to -0.3) | -2.5 (-4.0 to -0.5, range -11.4 to -0.3) |
| Rise above 1 kHz, ms | 2.31 (1.22 to 4.08, range 0.00 to 17.60) | 2.49 (1.17 to 4.23, range 0.20 to 10.34) |
| Full-band rise, ms | 2.04 (0.57 to 3.98, range 0.00 to 17.78) | 1.79 (0.67 to 3.68, range 0.02 to 9.41) |
| Crest factor over the shock span, dB | 5.7 (4.6 to 7.3, range 0.8 to 14.7) | 8.5 (7.0 to 9.6, range 3.6 to 13.2) |

A clip is a train: a dense series of small shocks about 9 ms apart, with a strong shock about every 32 ms, and the strongest shock 165 ms into the clip. The shocks within 6 dB of the strongest number 1 to 8 per clip, so no clip has one bang above the rest. The rise times of 0.00 ms are first shocks at the cut start of a clip.

The detector finds 1,251 shocks. [crackles.md](crackles.md) section 11 found 1,249 clicks on the short set with the same peak rules. The two extra shocks are first shocks at the cut start, which the rule "quiet before the active span" now counts. The median rise here (2.25 ms) is longer than the attack of section 11 (1.16 ms). The definitions differ: here the rise starts at the lowest envelope since the shock before, and the earlier report gives the median of the file medians of its own attack measure.

## 5. Energy in bands

Share of each band in the energy, in dB, median (p25 to p75):

| Band, Hz | Strong shock windows (231) | All shock windows (1,251) | Between the shocks (30 clips) | Active span (30 clips) |
| --- | --- | --- | --- | --- |
| 15.6 to 31.3 | -21.3 (-30.1 to -15.5) | -16.4 (-22.0 to -13.0) | -14.5 (-15.4 to -13.1) | -14.9 (-15.6 to -13.6) |
| 31.3 to 62.5 | -7.0 (-14.2 to -4.2) | -2.5 (-5.7 to -1.4) | -2.8 (-3.2 to -2.4) | -2.7 (-3.2 to -2.5) |
| 62.5 to 125 | -10.1 (-14.8 to -5.8) | -7.4 (-11.1 to -4.9) | -6.9 (-7.5 to -6.4) | -6.8 (-7.2 to -6.5) |
| 125 to 250 | -14.6 (-18.3 to -11.8) | -18.2 (-22.0 to -13.4) | -15.0 (-16.3 to -13.9) | -15.6 (-16.7 to -14.5) |
| 250 to 500 | -9.5 (-12.6 to -6.1) | -14.5 (-19.6 to -8.9) | -9.3 (-10.6 to -8.7) | -10.0 (-11.0 to -9.5) |
| 500 to 1k | -9.0 (-11.7 to -7.1) | -17.6 (-22.8 to -11.2) | -14.6 (-15.3 to -13.9) | -14.0 (-14.4 to -13.1) |
| 1k to 2k | -11.3 (-14.0 to -8.7) | -21.3 (-25.3 to -15.8) | -17.7 (-18.5 to -16.7) | -16.7 (-17.4 to -14.9) |
| 2k to 4k | -13.8 (-17.1 to -10.3) | -24.3 (-28.2 to -19.4) | -19.7 (-20.5 to -18.2) | -18.3 (-18.8 to -16.8) |
| 4k to 8k | -20.3 (-25.3 to -15.6) | -30.5 (-35.1 to -24.8) | -22.3 (-26.0 to -21.0) | -22.1 (-23.5 to -19.8) |
| 8k to 16k | -32.8 (-39.2 to -24.7) | -43.0 (-47.8 to -36.8) | -30.7 (-34.6 to -27.9) | -29.5 (-31.2 to -25.9) |
| 16k to 20k | -58.7 (-66.9 to -51.3) | -69.6 (-74.9 to -62.7) | -54.4 (-58.4 to -49.3) | -53.5 (-55.6 to -49.1) |

Whole clips: the share above 2 kHz is -16.6 dB (-17.5 to -14.6, range -20.4 to -11.3), and the share below 125 Hz is -1.1 dB (-1.3 to -0.9, range -2.2 to -0.8). The 31.3 to 62.5 Hz band alone holds -2.7 dB, about half of the energy.

A strong shock window is bright: from 500 Hz to 4 kHz its bands are 4.5 to 5.4 dB above the active span. The weak shocks are less bright than the sound between the shocks. Between the shocks, the mean power is 1.2 dB (-1.7 to -0.7, range -2.7 to -0.2) below the mean power in the shock windows, and above 2 kHz it is 3.5 dB (-5.3 to -1.6, range -9.1 to +2.0) below. The crack energy is therefore spread over the whole train, not held in short shocks with quiet between them.

Band energy over time, median over the clips, in dB against the loudest 5 ms frame of the clip summed over all bands, at the time from the frame of the strongest shock (30 clips have each frame from -10 to 160 ms, 28 have the frames before 0 ms, and 17 have the frame at 320 ms):

| ms | 15.6-31 | 31-62 | 62-125 | 125-250 | 250-500 | 500-1k | 1k-2k | 2k-4k | 4k-8k | 8k-16k | 16k-20k | above 2k |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| -10 | -22.9 | -9.9 | -16.4 | -30.0 | -24.3 | -27.0 | -29.6 | -33.1 | -41.2 | -54.1 | -81.1 | -31.9 |
| -5 | -26.1 | -10.3 | -17.5 | -26.2 | -26.5 | -28.7 | -30.8 | -34.6 | -40.4 | -50.6 | -74.8 | -34.0 |
| 0 | -23.0 | -12.0 | -15.7 | -25.6 | -19.4 | -17.8 | -19.0 | -18.7 | -19.8 | -22.6 | -48.4 | -15.0 |
| 5 | -24.5 | -10.7 | -19.5 | -17.3 | -10.5 | -11.1 | -10.8 | -11.7 | -15.6 | -23.3 | -46.4 | -10.1 |
| 10 | -22.7 | -13.8 | -14.4 | -16.5 | -10.2 | -13.9 | -17.3 | -19.4 | -20.7 | -29.5 | -52.2 | -16.3 |
| 20 | -22.6 | -12.9 | -14.0 | -19.7 | -16.9 | -22.3 | -27.7 | -26.4 | -24.5 | -34.0 | -57.6 | -22.1 |
| 40 | -24.9 | -11.3 | -11.1 | -26.4 | -27.2 | -30.1 | -29.9 | -30.4 | -29.1 | -40.9 | -65.6 | -26.4 |
| 80 | -22.4 | -9.4 | -16.0 | -27.2 | -23.6 | -28.6 | -29.4 | -33.9 | -37.9 | -52.1 | -80.6 | -32.4 |
| 160 | -25.1 | -13.4 | -18.0 | -31.6 | -28.1 | -30.1 | -33.5 | -34.7 | -43.7 | -55.8 | -82.9 | -32.6 |
| 320 | -25.2 | -15.5 | -16.6 | -28.1 | -38.3 | -34.9 | -37.3 | -38.5 | -46.0 | -59.3 | -85.4 | -37.8 |

From the frame 5 ms before the strongest shock to the frame 5 ms after it, the bands from 250 Hz to 8 kHz rise 16 to 25 dB, and the 8 to 16 kHz band rises 28 dB at the frame of the shock. Above 2 kHz, the energy falls 10 dB from its loudest frame in 10 ms (10 to 15, range 5 to 25) and 20 dB in 40 ms (31 to 54, range 20 to 195). The 31.3 to 62.5 Hz band stays between -9.4 and -15.5 dB from 10 ms before the strongest shock to 320 ms after it: the body does not start at the shock and does not decay after it.

## 6. Body and tail

| Measure | Value |
| --- | --- |
| Body peak of the active span, 10 to 500 Hz | 52 Hz (46 to 56, range 34 to 79) |
| Tail after the last shock | 69 ms (55 to 74, range 0 to 144), at least 30 ms in 24 clips |
| Tail peak of the spectrum, 10 to 500 Hz | 28 Hz (27 to 30, range 25 to 39), n=24 |
| Half cycles in the tail | 3 (2 to 3, range 1 to 8), n=24 |
| Tail frequency from the median half cycle | 27 Hz (27 to 28, range 24 to 38), n=13 with at least 3 half cycles |
| Spread of the half cycles, interquartile range over median | 0.17 (0.16 to 0.24, range 0.11 to 0.68), n=13 |
| Tail decay to -20 dB | 22 ms (22 to 24, range 20 to 83), n=13 |
| Fit of the decay line, r squared | 1.00 (1.00 to 1.00, range 0.84 to 1.00), n=13 |
| Last half-cycle peak against the highest | -32.0 dB (-32.2 to -29.1, range -39.9 to -12.4), n=13 |

The body inside the train is a low resonance near 52 Hz that lasts as long as the train (section 5). After the last shock, the tail is short, 69 ms, and shows 2 to 3 half cycles of a lower component near 27 Hz that falls 20 dB in about 22 ms. Three half cycles are few points for a decay line, so the r squared of 1.00 is weak evidence. The tail decay matches the 18 dB fall over the last 20 ms of the active span (section 3), so the tail decay is possibly the fade of the edit and not an acoustic decay. The clips cannot separate the two. The decay of the 52 Hz body in the free air, without later shocks, is not measurable from these clips, because no clip has a strong shock followed by a long free tail.

## 7. Channels

| Measure | Value |
| --- | --- |
| Correlation at no delay | 0.995 (0.994 to 0.995, range 0.990 to 0.996) |
| Correlation at the best delay | 0.995 (0.994 to 0.995, range 0.990 to 0.996) |
| Delay of the right channel behind the left | -0.001 ms (-0.001 to -0.000, range -0.003 to 0.000) |
| Left level over right level | -0.13 dB (-0.15 to -0.11, range -0.22 to -0.07) |
| Side (L-R) over mid (L+R) | -25.8 dB (-26.2 to -25.3, range -27.0 to -22.9) |

The delay is less than one sample at 44.1 kHz (0.023 ms), and the best delay gives no more correlation than no delay. The model can use the mean of the two channels. The 0.13 dB level difference is too small to need a gain correction.

## 8. Classes

| k | 2 | 3 | 4 | 5 |
| --- | --- | --- | --- | --- |
| Mean silhouette | 0.262 | 0.269 | 0.248 | 0.281 |

By the usual reading of the silhouette (Kaufman and Rousseeuw), 0.26 to 0.50 is a weak structure that can be artificial, and below 0.25 there is no structure. So the clips do not fall into separate groups. Section 3 to 7 give the clips as one class for this reason. The classes at k=5 (`out/crackle/summary-classes.md`) show the directions in which the clips vary:

| Class | Clips | Active length, ms | Strong shocks | Median gap between strong shocks, ms | Share above 2 kHz, dB | Crest factor, dB |
| --- | --- | --- | --- | --- | --- | --- |
| A | 4 | 410 (381 to 437) | 8 (7 to 8) | 22.2 (17.9 to 38.1) | -16.7 (-17.6 to -15.5) | 12.0 (11.7 to 12.1) |
| B | 2 | 383 (380 to 387) | 6 (4 to 7) | 54.3 (32.4 to 76.2) | -14.4 (-14.5 to -14.2) | 14.9 (14.7 to 15.0) |
| C | 7 | 553 (537 to 595) | 4 (4 to 6) | 72.5 (68.6 to 90.0) | -16.6 (-16.9 to -16.2) | 12.6 (12.6 to 13.5) |
| D | 4 | 538 (517 to 544) | 8 (7 to 10) | 25.8 (18.4 to 34.0) | -12.0 (-12.8 to -11.4) | 12.4 (12.2 to 12.6) |
| E | 13 | 549 (544 to 637) | 8 (8 to 10) | 36.7 (13.2 to 67.1) | -17.0 (-17.6 to -16.7) | 12.2 (11.8 to 12.5) |

Class C has the fewest strong shocks and the longest gaps between them: a sparse train. Class D is the brightest. Class B has the highest crest factor and the lowest RMS level (-15.2 dB against -12.4 to -13.0 dB). Class A is the shortest. Every class is still a train of 28 or more shocks over a body near 50 Hz. No class is a single bang.

## 9. Baseline: the current snarl pops

`just crackle-baseline` renders `tools/crackle/lift.scn` with `snarl-render` on `v8-ls` at 44.1 kHz: six lift-offs at 5000 rpm, each 3 s at full throttle and 3 s of closed throttle. Each render has `pops.character="race"` and `pops.density=1`, and one kind with weight 1 and the others with weight 0. A render with `pops.density=0` is the reference. `tools/crackle --minus` subtracts the reference, and `--split 0.3` cuts the result into events at quiet spans of 0.3 s. Each kind gives 6 events, one per lift-off, and the residual is quiet (at or below -80 dBFS) between them. So the pops do not change the engine sound outside the pops. Each event is one lift-off run, which `internal/core/pops.go` limits to 3 pops. The tool does not count the pops in a run. The renders are mono.

| Measure | Clips (30) | `KindPop` (6) | `KindBoom` (6) | `KindCrackle` (6) | `KindBurble` (6) |
| --- | --- | --- | --- | --- | --- |
| Active length, ms | 544 | 565 | 688 | 697 | 596 |
| Crest factor, dB | 12.4 | 16.0 | 17.0 | 16.9 | 16.7 |
| Shocks per event | 40 | 15 | 42 | 56 | 24 |
| Strong shocks per event | 8 | 9 | 15 | 32 | 12 |
| Median gap between shocks, ms | 8.6 | 9.9 | 8.0 | 7.4 | 9.0 |
| Gap between strong shocks, pooled, ms | 32.2 | 8.0 | 10.2 | 8.4 | 9.0 |
| Median rise above 1 kHz, ms | 2.25 | 1.81 | 2.21 | 2.41 | 2.39 |
| Crest factor of the strong shocks, pooled, dB | 8.5 | 6.7 | 7.0 | 6.7 | 6.3 |
| Share above 2 kHz, dB | -16.6 | -27.4 | -24.3 | -27.0 | -27.2 |
| Share below 125 Hz, dB | -1.1 | -13.1 | -5.4 | -13.8 | -9.6 |
| Decay above 2 kHz to -20 dB, ms | 40 | 40 | 72 | 80 | 52 |
| Power between the shock windows against inside, dB | -1.2 | -7.7 | -4.4 | -5.0 | -5.0 |
| The same above 2 kHz, dB | -3.5 | -10.4 | -5.9 | -4.8 | -5.1 |
| Body peak, 10 to 500 Hz, Hz | 52 | 183 | 86 | 421 | 163 |
| Tail frequency from the half cycles, Hz | 27 (n=13) | 197 | 89 | 66 | 149 |
| Tail decay to -20 dB, ms | 22 (n=13) | 51 | 27 | 72 (n=5) | 40 |
| Fall over the last 20 ms, dB | -18.0 | -11.3 | -7.1 | -3.4 | -10.1 |

All values are medians. The other statistics are in `out/crackle/baseline/<kind>/summary.md`.

Against the clips, every snarl kind has 7.7 to 10.8 dB less energy above 2 kHz, strong shocks with a 1.5 to 2.2 dB lower crest factor, and strong shocks 8.0 to 10.2 ms apart instead of 32.2 ms. `KindPop` and `KindBurble` have their body at 163 to 183 Hz with 4 to 11 per cent of the energy below 125 Hz, and `KindCrackle` has its body peak at 421 Hz. `KindBoom` is closest in the low bands (body 86 Hz, -5.4 dB below 125 Hz). The snarl events have a higher crest factor than the clips (16.0 to 17.0 dB against 12.4 dB), because their energy is in fewer, shorter parts of the event. The rise above 1 kHz is about the same. This report does not measure the cause of the differences in the snarl signal path.

## 10. Pop kinds with and without a measured source

| Kind (`api.go`) | Source in the clips | Reason |
| --- | --- | --- |
| `KindCrackle`, a train of cracks over a deep body | Measured | The clips are this kind: 28 to 61 shocks at a median gap of 8.6 ms, strong shocks 32 ms apart, crack energy at -16.6 dB above 2 kHz through the whole train, over a body near 52 Hz. All sections give its distributions. |
| `KindBoom`, a short deep burst with a bright onset | Partial | The deep burst is measured: 78 per cent of the energy below 125 Hz, a body near 52 Hz, 368 to 637 ms long. The bright onset is not measurable: every clip starts with sound at its first sample, so the onset of the event is cut. The strongest shock is 165 ms (median) into the clip, not at its start. |
| `KindPop`, an isolated pop with a resonant body | None | No clip holds an isolated pop: the fewest shocks in a clip is 28, and the body sounds through the whole train. The shape of one strong shock (rise, crest factor, spectrum) is measured, but not the free decay of a body after one shock, and not the spacing of isolated pops. |
| `KindBurble`, a soft low pop with no crack | None | Every clip has crack energy above 2 kHz (at least -20.4 dB share). Each clip is peak-normalised, so the level of a soft pop against a loud one is not measurable. |

Not measurable from these clips for any kind: the level of a pop against a running engine (the clips have no engine), the level of one clip against another (each is peak-normalised), the onset of an event (cut), and the free decay of the body (no long free tail).

## 11. Reproduce

1. Run `just crackle-measure`. It writes `out/crackle/summary.md` (the clips as one class), `out/crackle/summary-classes.md` (the k-means classes), and `clips.csv`, `shocks.csv` and `bands.csv` with the measures of each clip, shock and frame.
2. Run `just crackle-baseline`. It writes the renders and `out/crackle/baseline/<kind>/` for the four kinds.

`go test ./tools/crackle` checks the tool on signals that the tests make: shock times, levels, rise and gaps of a click train, the delay and level difference of two channels, bursts and fragments, the frequency and decay of a damped tail, band shares of a tone, k-means on two groups, subtraction and splitting, and the CSV output.
