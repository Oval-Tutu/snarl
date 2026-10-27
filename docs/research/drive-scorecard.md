# Drive scorecard: snarl against engine-sim in a scripted drive

Measured on the branch `pop-model` at `d7065aa` with the changes of this study in the working tree, and with the committed cycle tables and profiles. snarl renders at 48 kHz, tier 1, with `--dyno --no-pops`, as `just scorecard` does. The tools are `snarl-harness -scenario`, `just ref-drive`, `tools/scorecard --scenarios drive --drive` and `analysis.AlignTrack` (`internal/analysis/track.go`).

## Answer

In a scripted drive with idle, full-throttle pulls through three gears, lifts, coasts, downshifts and part throttle, snarl has no more noise above 2 kHz than engine-sim while the engine revs up. In the 42 full-throttle pull windows of each engine, the median worst band above 2 kHz is within 0.4 dB of the median of a second engine-sim render against the first, and the largest mean difference of a band is -0.8 to +0.3 dB. The tonality and the noise between the firings match within 0.2 dB on average.

The drive finds one short hissy stretch that the steady holds cannot show: the first 0.25 s after a lift-off from full throttle. There, snarl has 4 to 10 dB more level from 2 to 8 kHz than engine-sim, relative to the loudness of the window. The excess is beyond the engine-sim self spread on `v12-merlin` (all three lifts, +7.9 to +10.0 dB at 2 kHz) and on `f4-ej25` (two of three lifts, +5.7 and +8.6 dB at 2 kHz). It is at the edge of the spread on `i4-vtec` and `v8-ls` (+4.0 to +4.5 dB), and absent on `v12-412`. The next window is within the self spread again, so an average over the drive hides the burst.

The cause is the change from the full-throttle sound to the closed-throttle sound, which runs faster in snarl than in engine-sim:

- On `f4-ej25`, the intake lag of the profile (0.05 s) is too short for a lift at 5000 to 6000 rpm. A lag of 0.2 s takes the three lift windows to +2.8, -1.4 and +2.2 dB, within the self spread. No lag takes them to +10.2, +4.5 and +18.1 dB.
- On `v12-merlin`, the closing transient of the runtime causes the excess. The offline playback of the same tables with the same lag and no closing transient gives -5.3, -5.1 and +2.1 dB, within the self spread. The runtime, which adds the closing transient, gives +9.1, +10.0 and +7.9 dB.

The only other window beyond the self spread in the noise between the firings is the full-throttle launch at 1300 to 2300 rpm on `i4-vtec` and `f4-ej25` (+3.0 and +4.8 dB). That miss is an error of the measure, not noise in snarl: with the RPM track delayed by the 21.5 ms of the snarl control path, the `i4-vtec` window drops to -0.7 dB.

Recommendations, in order:

1. Listen to the lift-offs of `out/drive/snarl/<engine>/drive.wav` against `out/reference/upstream/<engine>/drive.wav` at 6.0, 14.5 and 24.0 s. When the hiss of the harness is there, the lift-off is the condition.
2. Measure the intake lag at a lift with a falling RPM, not at a held RPM, and check the closing transient on `v12-merlin` against the closing steps it came from.
3. Score the drive again after the table work of the other worker is committed. The numbers here are the record of the committed tables, because a partial run on the new tables overwrote `out/drive/snarl/` and `out/drive/score.md` (section 7).

## 1. The drive scenarios

`snarl-harness -profile <engine> -scenario <file>` writes the RPM and the throttle of the scripted drive of the harness (`driveScript` in `cmd/snarl-harness/script.go`) in the scenario format. The same drive model and `LoadFilter` run as in `-script`. Each tick of 1/60 s becomes a sweep from the RPM of the tick before to the RPM of the tick, with the throttle of the tick. Ticks at a constant RPM and throttle join one hold. `TestScriptScenario` checks that the scenario reaches the RPM and the throttle of every tick, and that it lasts 27.5 s.

`just ref-drive` writes `scenarios/upstream/<engine>/drive.scn` for the five engines and renders each with engine-sim twice, with the settings of `just ref-fullrange`: `--volume 0.5`, 48 kHz, a 3 s pre-roll into `out/reference/upstream/<engine>/drive.wav` and a 3.5 s pre-roll into `out/reference/upstream/calib/<engine>/drive.wav`. The 10 renders ran in parallel in 182 s.

The engine-sim renderer has no limit on the number of segments. It accepts one segment per tick, which is the finest step of the drive: 1442 segments per engine, 1439 sweeps and 3 holds, in an argument list of less than 100 KB.

| Engine | Idle (rpm) | Part-throttle hold (rpm) | Top at full throttle (rpm) | Top after a downshift (rpm) | Limiter (rpm) |
| --- | --- | --- | --- | --- | --- |
| `i4-vtec` | 770 | 5301 | 8919 | 9366 | 9400 |
| `v8-ls` | 690 | 3898 | 6458 | 6776 | 6800 |
| `f4-ej25` | 830 | 3807 | 6183 | 6478 | 6500 |
| `v12-412` | 3810 | 11522 | 17677 | 18442 | 18500 |
| `v12-merlin` | 880 | 2256 | 3353 | 3490 | 3500 |

The drive model works in shares of the range from idle to the limiter, so the timeline is the same for each engine. Idle is the idle RPM of the profile (880 rpm on `v12-merlin`, where the full-range holds start at 600 rpm). The timeline:

| Time (s) | Gear | Throttle | What happens |
| --- | --- | --- | --- |
| 0 to 1 | 1 | 0 | idle |
| 1 to 3.5 | 1 | 1 | launch, to 63 % of the range |
| 3.5 to 6 | 2 | 1 | pull to 88 % |
| 6 to 8.5 | 2, then 1 | 0 | lift, coast, downshift |
| 8.5 to 14.5 | 1 to 3 | 1 | pulls through three gears to 94 % |
| 14.5 to 18 | 3 to 1 | 0 | lift, coast, two downshifts, to 99.6 % after the second |
| 18 to 22.5 | 1, then 2 | 0.5 | part throttle, hold at 52.5 %, upshift |
| 22.5 to 24 | 2 | 1 | pull |
| 24 to 27.5 | 2 | 0 | lift and coast |

The drive does not reach the limiter. The fastest climb is the launch, 2800 to 3200 rpm/s on `i4-vtec` (0.37 of the range per second). A gear change moves the RPM by a quarter of the range or more in one tick.

## 2. How the drive is scored

`tools/scorecard --scenarios drive` renders the drive with snarl as `snarl-render` does, and cuts both files into windows of 0.25 s, one after the other (`--drive-window`). Each window gets the alignment scorecard of a sweep and the three hiss measures of `docs/research/fullrange-scorecard.md` section 2, from `analysis.AlignTrack`.

The tonality and the noise between the firings need whole engine cycles, and a drive window has no constant cycle period. `AlignTrack` takes the cycles along the crank phase of the RPM of the scenario. Each file can run behind the scenario: the dynamometer of engine-sim lags its target by a few rpm, and the snarl control path delays the sound by about 21.5 ms. On a linear RPM ramp, a delay is a constant RPM offset. `AlignTrack` therefore searches the RPM offset of each file, up to 5 % of the mean RPM, that gives the least residual. `TestAlignTrack` checks that a ramp of 3200 rpm/s with a tone at engine order 100 keeps the tone at least 15 dB over the noise along the track, and that a copy 20 ms behind gives the same tonality and gap within 0.5 dB.

A window has cycle measures when it holds at least three whole cycles in both files: above about 1500 rpm at 0.25 s. 99 to 110 of the 110 windows of each engine have them.

`--drive` prints, for each engine, the windows by class and the windows that miss a hiss floor: the band level beyond 1.6 dB, the tonality beyond 0.5 dB, the noise between the firings beyond 2.2 dB, with the RPM, the mean RPM rate and the throttle of each. A `!` marks a miss beyond the largest value of engine-sim against itself over the drive.

The hold floors do not fit windows of 0.25 s. engine-sim against itself misses the band floor of 1.6 dB in 63 to 98 of the 110 windows, with a largest worst band of 4.1 to 5.8 dB. Two engine-sim renders pass the resonances at slightly other times, and a short window has few frames. A miss of the band floor alone is therefore no evidence. This report counts a window as a real miss only when it is beyond the self spread, and it compares the classes of windows by the mean difference of each band, which the window noise does not move.

## 3. Results by class of window

The windows by class, snarl against engine-sim (S) and the second engine-sim render against the first (E). The worst band is the size of the largest band difference above 2 kHz of each window. The mean band is the band with the largest mean difference over the windows of the class, snarl or E less engine-sim.

| Engine | Class | Windows | Worst band median / p90, S (dB) | Same, E | Mean band, S (dB at Hz) | Mean band, E | Tonality / gap mean, S (dB) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `i4-vtec` | full-throttle pull | 42 | 1.9 / 2.7 | 1.9 / 2.6 | -0.4 at 3981 | +0.2 at 2512 | -0.0 / -0.2 |
| `i4-vtec` | coast | 31 | 1.8 / 3.7 | 1.8 / 2.9 | -1.2 at 1995 | +0.6 at 2512 | -0.1 / +0.0 |
| `i4-vtec` | part throttle | 16 | 1.4 / 1.8 | 1.5 / 2.0 | +0.4 at 6310 | +0.4 at 3981 | -0.2 / +0.0 |
| `v8-ls` | full-throttle pull | 42 | 1.8 / 2.6 | 1.9 / 3.0 | +0.3 at 3162 | +0.2 at 3162 | -0.0 / +0.0 |
| `v8-ls` | coast | 31 | 2.0 / 3.1 | 1.9 / 2.4 | -0.4 at 1995 | -0.3 at 3981 | -0.1 / +0.0 |
| `v8-ls` | part throttle | 16 | 2.0 / 2.8 | 2.2 / 3.0 | +0.3 at 15849 | +0.7 at 5012 | -0.1 / -0.2 |
| `f4-ej25` | full-throttle pull | 42 | 1.7 / 2.9 | 1.9 / 2.8 | -0.5 at 3162 | +0.2 at 7943 | -0.1 / +0.0 |
| `f4-ej25` | coast | 31 | 2.2 / 4.0 | 2.1 / 3.5 | -0.8 at 1995 | -0.4 at 2512 | -0.3 / +0.1 |
| `f4-ej25` | part throttle | 16 | 1.7 / 2.3 | 1.8 / 2.5 | -0.4 at 3162 | +0.7 at 1995 | -0.0 / -0.1 |
| `v12-412` | full-throttle pull | 42 | 2.1 / 3.2 | 1.7 / 2.7 | -0.8 at 1995 | -0.2 at 12589 | -0.0 / -0.0 |
| `v12-412` | coast | 31 | 2.2 / 4.3 | 1.7 / 3.1 | -1.0 at 2512 | -0.5 at 2512 | -0.0 / -0.0 |
| `v12-412` | part throttle | 16 | 1.6 / 2.3 | 2.2 / 2.8 | -0.4 at 5012 | -0.4 at 3981 | -0.3 / -0.2 |
| `v12-merlin` | full-throttle pull | 42 | 2.0 / 2.9 | 2.2 / 3.2 | -0.5 at 2512 | -0.3 at 15849 | -0.0 / -0.2 |
| `v12-merlin` | coast | 31 | 2.2 / 3.4 | 2.5 / 3.3 | +0.3 at 10000 | -0.4 at 2512 | -0.0 / +0.1 |
| `v12-merlin` | part throttle | 16 | 1.9 / 2.8 | 2.2 / 3.1 | +0.7 at 2512 | -0.4 at 3162 | -0.0 / +0.1 |

The engine-sim tonality and gap means of these classes are within 0.1 dB of 0. The lift class:

| Engine | Lift windows | Worst band median / p90, S (dB) | Same, E | Mean band, S (dB at Hz) | Mean band, E |
| --- | --- | --- | --- | --- | --- |
| `i4-vtec` | 4 | 4.0 / 4.5 | 1.7 / 2.8 | +2.9 at 3981 | +0.8 at 1995 |
| `v8-ls` | 4 | 4.1 / 4.5 | 2.0 / 2.8 | +2.7 at 6310 | -0.8 at 6310 |
| `f4-ej25` | 4 | 5.7 / 8.6 | 2.9 / 4.1 | +3.5 at 1995 | -1.5 at 5012 |
| `v12-412` | 4 | 4.1 / 4.1 | 2.0 / 3.5 | -2.0 at 2512 | -1.2 at 5012 |
| `v12-merlin` | 4 | 9.1 / 10.0 | 3.5 / 3.8 | +6.6 at 1995 | -0.8 at 3981 |

The lift and tip-in classes of this run also hold the window that ends at the throttle step, from a rounding error of the segment times that is now fixed in `driveWindows`. Each engine has three lifts, so a fourth window changes each lift row. Section 4 and section 5 give the three lift windows one by one.

The tip-in windows and the gear-change windows are within the self spread, except one closed-throttle downshift of `v12-412` at 15.5 s with 6.4 dB less level at 2 kHz than engine-sim.

## 4. The misses

The windows beyond the largest value of engine-sim against itself over the drive. Band: worst band above 2 kHz, snarl less engine-sim. Tonality: mean difference, negative is noisier. Gap: mean difference of the noise between the firings, positive is more noise.

| Engine | Time (s) | RPM | Rate (rpm/s) | Throttle | Class | Miss |
| --- | --- | --- | --- | --- | --- | --- |
| `i4-vtec` | 1.25-1.50 | 1471-2260 | +3157 | 1 | launch | gap +3.0 dB (section 5.2) |
| `i4-vtec` | 4.25-4.50 | 5676-5982 | +1226 | 1 | pull | tonality +1.0 dB |
| `i4-vtec` | 6.00-6.25 | 8295-7863 | -1726 | 1 to 0 | lift | tonality +0.9 dB |
| `i4-vtec` | 12.00, 15.50, 16.50 | 7165-8963 | -7015 to +6664 | 1 or 0 | gear change | tonality -0.7 to -1.0 dB |
| `i4-vtec` | 22.75-23.25 | 5579-6210 | +1187 to +1336 | 1 | pull | tonality +0.7 and -0.1 (0.8) dB |
| `i4-vtec` | 24.00-24.25 | 7446-7015 | -1726 | 1 to 0 | lift | band +4.5 dB at 6310 Hz |
| `i4-vtec` | 24.75-25.00, 26.50-26.75 | 6152-5720, 3131-2700 | -1726 | 0 | coast | band -5.3 and -5.5 dB at 1995 Hz |
| `v8-ls` | 6.00-6.25 | 6018-5712 | -1222 | 1 to 0 | lift | tonality +0.8 dB |
| `v8-ls` | 6.75-7.00, 15.25-15.50 | 5101-4796, 5403-5098 | -1222 | 0 | downshift, coast | tonality -0.7 dB |
| `v8-ls` | 24.00-24.25 | 5417-5111 | -1222 | 1 to 0 | lift | gap -2.9 dB (less noise between the firings) |
| `f4-ej25` | 1.25-1.50 | 1291-1809 | +2074 | 1 | launch | gap +4.8 dB (section 5.2) |
| `f4-ej25` | 2.25-2.50 | 2837-3116 | +1116 | 1 | pull | tonality -0.9 dB |
| `f4-ej25` | 6.00-6.25 | 5774-5490 | -1134 | 1 to 0 | lift | band +5.7 dB at 1995 Hz |
| `f4-ej25` | 15.00-15.50, 16.50, 24.50 | 4366-6213 | -1134 to +4378 | 0 | coast, downshift | tonality -0.5 to -0.7 dB |
| `f4-ej25` | 24.00-24.25 | 5216-4933 | -1134 | 1 to 0 | lift | band +8.6 dB at 1995 Hz, gap +2.5 dB |
| `f4-ej25` | 25.50-26.00 | 3515-2948 | -1134 | 0 | coast | band -5.6 and -5.5 dB |
| `v12-412` | 0.50-0.75 | 3810 | 0 | 0 | idle | band +4.2 dB at 2512 Hz |
| `v12-412` | 12.75-13.00 | 15522-15901 | +1515 | 1 | pull | band -8.0 dB at 1995 Hz |
| `v12-412` | 14.50-14.75 | 17345-16611 | -2938 | 1 to 0 | lift | tonality -1.2 dB |
| `v12-412` | 15.50-15.75 | 14407-17124 | +10869 | 0 | downshift | band -6.4 dB at 1995 Hz |
| `v12-412` | 19.00-22.50 | 11522 | 0 | 0.5 | part-throttle hold | tonality -0.5 to -0.8 dB in 6 windows |
| `v12-412` | 25.25-26.50 | 11502-7830 | -2938 | 0 | coast | band -4.3 to -5.7 dB at 2512 Hz |
| `v12-merlin` | 0.00-0.25 | 880 | 0 | 0 | idle | band +4.8 dB at 2512 Hz |
| `v12-merlin` | 6.00-6.25 | 3164-3033 | -524 | 1 to 0 | lift | band +9.1 dB at 1995 Hz |
| `v12-merlin` | 14.50-14.75 | 3294-3163 | -524 | 1 to 0 | lift | band +10.0 dB at 1995 Hz |
| `v12-merlin` | 24.00-24.25 | 2907-2776 | -524 | 1 to 0 | lift | band +7.9 dB at 1995 Hz |

The self spread over the drive, the largest of engine-sim against itself: band 4.3, 5.8, 5.3, 4.1 and 4.6 dB, tonality 0.7, 0.7, 0.6, 0.4 and 0.5 dB, gap 1.2, 0.9, 1.6, 1.1 and 1.1 dB, for `i4-vtec`, `v8-ls`, `f4-ej25`, `v12-412` and `v12-merlin`.

Only the lift windows and the two idle windows have more level above 2 kHz than engine-sim beyond the self spread. The other band misses have less.

The tonality numbers of this report, the tonality misses of 0.5 to 1.2 dB in this table and the tonality means of section 3, used a wrong rate in `hissCycle`, which put each tonality band 1 to 2 times lower in frequency than its name (section 7). The fix came after the snarl renders were overwritten, so the snarl tonality is not scored again. Scored again on the same files, the drive of engine-sim against itself and the offline playback of the tables have no tonality miss above 0.5 dB, against 0 to 3 per engine before the fix (`out/drive/score-self-ratefix.md`, `out/drive/score-play-ratefix.md`). Treat the snarl tonality misses as unconfirmed until the drive is scored again.

## 5. Causes

Each suspect was tested with a flag, with the defaults unchanged. The lift windows start at the lift: 6.00, 14.50 and 24.00 s. Each cell is the worst band above 2 kHz of the three windows, in dB, snarl or playback less engine-sim.

| Render | `i4-vtec` | `v8-ls` | `f4-ej25` | `v12-412` | `v12-merlin` |
| --- | --- | --- | --- | --- | --- |
| engine-sim against itself | +2.8, -1.4, -1.1 | +1.9, -2.8, +2.0 | -2.5, -4.1, -2.9 | -1.7, +3.5, +2.0 | -3.5, -3.4, +3.8 |
| snarl, as shipped | -2.7, +4.0, +4.5 | +3.2, +4.5, +4.1 | +5.7, +2.6, +8.6 | -2.5, -4.1, +3.4 | +9.1, +10.0, +7.9 |
| snarl, `--set throttle.close_lag_s=0` | -5.3, -4.2, -5.1 | | +10.2, +4.5, +18.1 | | +17.3, +13.5, +14.2 |
| snarl, `--set throttle.close_lag_s=0.2` | -3.8, +4.7, +4.8 | | +2.8, -1.4, +2.2 | | -6.0, +5.0, +4.5 |
| playback, lag, no closing transient | +3.4, +6.9, +4.1 | +3.7, +3.5, +2.9 | +6.2, +2.5, +6.8 | -2.9, -3.8, +2.4 | -5.3, -5.1, +2.1 |
| playback, no lag, no closing transient | +5.6, +7.1, +4.7 | +7.1, +7.0, +5.8 | +4.5, +3.7, +4.2 | -3.2, -3.8, +4.4 | +17.7, +12.6, +16.4 |

The playback is `tools/cycles play` of `drive.scn` from the same tables, with the exact synthesiser port in float64 at 44.1 kHz (`cycles.Player`). It has the intake lag of the tables, which is the lag of the profiles, but no closing transient. With `--no-lag`, the throttle rows change at once. The snarl runtime (`cycles.Live`) has the intake lag and the closing transient.

### 5.1 The lift-off

- **The change to the closed-throttle sound runs too fast.** With no lag, the rows change at once, and the excess grows: on `v12-merlin` to +17.7 dB in the playback and +17.3 dB in the runtime, on `f4-ej25` to +18.1 dB in the runtime, and on `i4-vtec` and `v8-ls` to +7.1 dB in the playback. A lag of 0.2 s lowers it on `f4-ej25` and `v12-merlin`, and not on `i4-vtec`. In the first 0.25 s after a lift, engine-sim keeps more of the balance of the full-throttle sound than snarl does.
- **`f4-ej25`: the intake lag is too short.** The playback with the lag and the runtime give the same excess (+6.2 and +6.8 against +5.7 and +8.6 dB), so the closing transient does not cause it. A lag of 0.2 s in place of 0.05 s brings the three windows within the self spread. The lag of the profile comes from a step at a held 3750 rpm (`profiles/f4-ej25.sources.toml`). The lifts of the drive are at 5000 to 6000 rpm with the RPM falling.
- **`v12-merlin`: the closing transient causes it.** With the same lag of 0.095 s, the playback without the closing transient is within the self spread, and the runtime with it is 7.9 to 10.0 dB over. The closing transient weights the closed-throttle row with slices of the closing steps of `close.scn`, while the lag still holds part of the full-throttle row. This test does not show whether the slices or their weighting cause the excess. A switch of the closing transient in `cycles.Live` would show it.
- **`i4-vtec` and `v8-ls`**: the excess is at the edge of the self spread, and it does not follow the lag in one direction. No cause is proven.
- **Not the cause**: the load, the own pops, the RPM smoothing and the resampler. The playback has none of them and shows the excess on `f4-ej25`.

### 5.2 The launch

At the full-throttle launch, 1300 to 2300 rpm at 2000 to 3200 rpm/s, the gap of `i4-vtec` and `f4-ej25` is 3.0 and 4.8 dB over engine-sim. The playback gives +0.3 and -0.1 dB there, so the miss comes from the snarl runtime or from the measure. The launch has the largest change of RPM rate in the drive: the climb rate doubles below 40 km/h and changes at once above it. A constant RPM offset then does not follow the 21.5 ms delay of the snarl sound, the average cycle smears, and the gap fills. With `--drive-lead 0.0215`, which delays the RPM track of the snarl file by that time, the `i4-vtec` gap falls from +3.0 to -0.7 dB. The `f4-ej25` window then finds no cycles. The band levels of these windows, which do not depend on the cycles, are within the self spread. The launch miss is an error of the measure, not a hiss.

### 5.3 The other suspects

| Suspect | Result | Evidence |
| --- | --- | --- |
| Fast RPM change | not a cause | The 42 pull windows at up to 3200 rpm/s match engine-sim in the band level, the tonality and the gap. |
| Crossing a grid point of the tables | not a cause in the pulls | A pull window moves by 75 to 800 rpm, so most cross a grid point of 250 rpm, and the class matches. The `v12-412` pull miss at 15522 to 15901 rpm (-8.0 dB) is the blend miss of the full-range mid holds at 15125 to 15625 rpm. |
| Intake lag | cause of the lift excess on `f4-ej25` | section 5.1 |
| Closing transient | cause of the lift excess on `v12-merlin` | section 5.1 |
| Ignition crossfade | not tested | The drive does not reach the limiter and has no ignition cut, so the crossfade does not run. |
| Residual noise model | cause of the two idle misses | `v12-merlin` at 880 rpm (+4.8 dB) and `v12-412` at 3810 rpm (+4.2 dB), closed throttle, as in the full-range holds. |
| Overrun level | less level above 2 kHz on the coast | `i4-vtec`, `f4-ej25` and `v12-412` coasts are 4.3 to 5.7 dB under engine-sim in single windows, as in the full-range overrun sweeps. |

## 6. What this means for the hiss

The drive does not show a hiss while the engine revs up. The lift-off does show an airy burst of about a quarter of a second, strongest on `v12-merlin` and `f4-ej25`. A listener who revs and lifts in turn can hear that burst at each lift. When the hiss of the harness comes at the lift and not during the pull, the lift-off is the condition, and the fixes are the intake lag of `f4-ej25` and the closing transient of `v12-merlin`. When the hiss comes during the pull, this scorecard does not see it, and the next step is a harness recording with the pops on and off, replayed with `snarl-render --replay`.

The harness drives the engine with the load of `snarl.LoadFilter`, and the scenario format gives the load equal to the throttle. The load moves only the idle wander and the pop heat (`internal/core/core.go`), so this difference does not change the sound with `--no-pops`.

## 7. Limits

- The snarl renders and the suspect tests ran on the committed tables. The other worker then rebuilt `profiles/cycles/*.bin`, uncommitted. A partial later run on the new tables overwrote `out/drive/snarl/` and emptied `out/drive/score.md`, so the numbers of this report are the record of the committed tables. The files `out/drive/score-lag0.md`, `score-lag0.2.md`, `score-play.md`, `score-play-nolag.md` and `score-lead.md` are from the committed tables. Score the drive again after the new tables are committed.
- The tonality numbers of this report used a wrong rate, now fixed. `hissCycle` (`internal/analysis/hiss.go`) took the spectrum of `c.c.sig`, which has `m` samples per cycle, at the rate `c.fs`, which is for `nextPow2(m)` samples per cycle. Each tonality band was lower in frequency than its name, by a factor of 1 to 2 that depends on the RPM. `TestHissTonalityBand` checks that a tone at 3.9 kHz counts in the 2 to 4 kHz band. The gap used `c.sig` and was correct.
- The parabola step of `residualCycles` (`internal/analysis/residual.go`) had no bound, now fixed: on a signal with no steady period it could move the period out of the search and index out of the signal. Each fit now keeps the period in the search. `TestResidualCyclesBounded` checks this on a ramp, on noise and on a ramp with a burst, which panicked before the fix. The renders of this report did not reach the panic.
- Windows below about 1500 rpm have no cycle measures at 0.25 s, so the idle and the start of the launch have the band level only.

## 8. Reproduce

```sh
just ref-drive
go run ./tools/scorecard --scenarios drive --dyno --no-pops --calib out/reference/upstream/calib --drive --out out/drive/snarl
go run ./tools/scorecard --engines f4-ej25,v12-merlin,i4-vtec --scenarios drive --dyno --no-pops --set throttle.close_lag_s=0.2 --drive --out out/drive/lag0.2
go build -o out/drive/cycles ./tools/cycles
out/drive/cycles play --tables profiles/cycles/v12-merlin.bin --es "$(nix build -f reference/engine-sim --print-out-paths --no-link)/share/engine-sim/es" --out-dir out/drive/play/v12-merlin scenarios/upstream/v12-merlin/drive.scn
go run ./tools/scorecard --scenarios drive --against out/drive/play --drive --out out/drive/unused
go run ./tools/scorecard --engines i4-vtec,f4-ej25 --scenarios drive --dyno --no-pops --drive --drive-lead 0.0215 --out out/drive/lead
```

Add `--no-lag` to `cycles play` for the playback without the lag. Repeat the `cycles play` line for each engine.

## 9. After the fixes of the tables

Scored again on the tables of `1318de7`: the grid to the limiter, the aliased harmonics, the modulation of the cycle (`docs/research/fullrange-scorecard.md` section 12) and the closing slices at their middle (section 13 there). The tonality fix of section 7 is in every score of this section. The before column is the same tables without the closing slice fix (`out/fix4/base/score.md`), the after column `out/drive/score.md`.

### 9.1 Answer

The lift-off burst is gone on `v12-merlin` and `f4-ej25`. The first closing slice of each closing step held the average of 2 or 3 cycles of a cycle that changes fast. The average jumps at its end and clicks when it plays over and over. Each slice now holds the cycle at its middle. The intake lag of `f4-ej25` is not the cause: closing steps of engine-sim at the RPM of the lifts give 0.050 s, the lag of the profile. The windows beyond the engine-sim self spread fall from 24 to 20.

The worst band above 2 kHz of the lift windows at 6.0, 14.5 and 24.0 s, snarl less engine-sim:

| Engine | Before (dB) | After (dB) | Self spread (dB) |
| --- | --- | --- | --- |
| `i4-vtec` | -3.3, +4.3, +4.3 | +2.9, +4.4, +4.2 | 4.3 |
| `v8-ls` | +3.8, +4.6, +4.1 | +3.8, +4.6, +3.9 | 5.8 |
| `f4-ej25` | +5.7, +2.6, +8.7 | +3.1, within 1.6, +3.4 | 5.3 |
| `v12-412` | -2.3, -3.7, +3.4 | -2.8, -3.8, +3.4 | 4.1 |
| `v12-merlin` | +10.7, +13.3, +10.9 | -6.0, -4.1, -2.5 | 4.6 |

The lift class: worst band median 4.2, 3.9, 3.1, 3.4 and 4.1 dB for `i4-vtec`, `v8-ls`, `f4-ej25`, `v12-412` and `v12-merlin`, against 1.4 to 3.5 dB for engine-sim against itself. `i4-vtec` at 14.5 s (+4.4 dB, 0.1 dB over the spread) and `v8-ls` at 24.0 s (the gap, -2.9 dB) stay beyond the spread, as before the fix. `v12-merlin` at 6.0 s is beyond it on the low side (-6.0 dB). The other misses of section 4 do not change: the coast and downshift windows with less level than engine-sim, the two idle windows, and the launch.

### 9.2 The intake lag by RPM

The closing lag of engine-sim, fitted at each closing step of `close.scn` (`tools/cycles build --step out/cycles/grid/<engine>/close`, in `out/fix4/lag-<engine>.md`; `just cycles-grid` now prints the same fits in the line "Closing lag by RPM" of `build.md`), grows at low RPM:

| Engine | Closing lag by RPM (rpm: s) | Profile (s) |
| --- | --- | --- |
| `i4-vtec` | 770: 0.226, 1750: 0.103, 3750: 0.055, 5750: 0.048, 7750: 0.033, 9250: 0.031 | 0.052 |
| `v8-ls` | 690: 0.312, 1750: 0.136, 3750: 0.071, 5750: 0.054, 6750: 0.048 | 0.068 |
| `f4-ej25` | 830: 0.190, 1750: 0.090, 3750: 0.057, 5750: 0.050, 6400: 0.050 | 0.050 |
| `v12-412` | 3810: 0.046, 7750: 0.027, 11750: 0.018, 15750: 0.012, 18250: 0.011 | 0.019 |
| `v12-merlin` | 600: 0.173, 1500: 0.061, 2500: 0.082, 3250: 0.056 | 0.095 |

A test of `cycles.Live` with this lag, linear in the RPM between the steps and scaled by the lag of the profile over the lag of the tables, scored the drive (`out/fix4/rl/score.md`). The `v12-merlin` lifts go to -3.9, -3.2 and +2.6 dB, inside the spread, and the `i4-vtec` lift at 14.5 s to +2.7 dB. But the `i4-vtec` lift at 6.0 s goes to -4.4 dB, a new window beyond the spread of 4.3 dB. The windows beyond the spread fall from 20 to 19, with one new miss, so the lag by RPM is not kept. snarl keeps one closing lag per profile.

### 9.3 The other lifts

The lifts of `i4-vtec`, `v8-ls` and `v12-412` stay at the edge of the spread, and no cause is proven. The engine-sim lifts of `out/fix4/lift` (fullrange-scorecard section 13.2) give the same excess at a held RPM as with the RPM falling: `i4-vtec` +5.8 and +4.9 dB, `v8-ls` +4.5 and +3.8 dB, `v12-412` +3.0 and -3.0 dB. The falling RPM is therefore not the cause. Without the lag while the slices play, `i4-vtec` turns to -5.8 and -4.8 dB, and the lag that follows the RPM moves its drive lifts to -4.4, +2.7 and +2.9 dB. The sign follows the balance of the open-throttle and closed-throttle sound in the first 0.25 s, and no setting measured from engine-sim brings all three lifts inside the spread. The offline playback without a closing transient gave +3.4 to +6.9 dB on `i4-vtec` (section 5), so the slices are not the cause either.

### 9.4 Reproduce

```sh
go run ./tools/scorecard --scenarios drive --dyno --no-pops --calib out/reference/upstream/calib --drive --out out/drive/snarl > out/drive/score.md
```

The lift renders of `docs/research/fullrange-scorecard.md` section 13.2 are in `out/fix4/lift`, with their scenarios in `out/fix4/lift/scn`.
