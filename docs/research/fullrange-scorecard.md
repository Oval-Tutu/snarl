# Full-range scorecard: snarl against engine-sim from idle to the limiter

Measured on the branch `pop-model` at `7e9ac06` with the changes of this study in the working tree. The engine defaults, the profiles, the cycle tables and the pops did not change. snarl renders at 48 kHz, tier 1, with `--dyno --no-pops` (no idle wander in the dynamometer holds, no own pops), as `just scorecard` does. The tools are `tools/scorecard` and `snarl-compare --align` (`internal/analysis/align.go`, `internal/analysis/hiss.go`).

## Answer

The full-range scorecard covers the five engines at the five throttles of the cycle table grid: 770 holds every 250 rpm from idle to below the limiter, 149 full-throttle holds halfway between them, and the existing full-throttle sweep and overrun. Each new measure is calibrated against a second engine-sim render.

In the holds, snarl does not have more noise above 2 kHz than engine-sim. Above 2 kHz, the worst third-octave band of each hold is within 1.5 dB of engine-sim at 464 of the 475 holds of `i4-vtec`, `v8-ls`, `f4-ej25` and `v12-merlin`. Only 7 of those holds have more high-frequency level than engine-sim, by at most 3.8 dB. snarl is never less tonal above 2 kHz than engine-sim by more than 1.2 dB. Where the noise between the firing pulses misses, snarl has less of it, not more. In the full-throttle sweeps, the worst band of a window is 1.5 to 2.7 dB from engine-sim on four engines, with signs that change from window to window, against 2.1 to 2.3 dB for engine-sim against itself.

The real-time path of the harness gives the same samples, bit for bit, as `snarl-render` for the same snapshots. The hiss that the listener hears in the harness therefore does not come from the real-time path. It also does not show in the dynamometer holds and sweeps that this scorecard covers. Before the tuning phase, listen to the full-range pairs of section 8, or record a harness session, to find the condition that makes the hiss.

The misses that remain, and their causes:

| Engine | Where | Size | Cause |
| --- | --- | --- | --- |
| `v12-merlin` | throttle 0, 600 to 1000 rpm | bands above 2 kHz +1.6 to +3.8 dB, score 78.9 to 99.2 | residual noise model: too much high-frequency residual at closed-throttle idle |
| `i4-vtec` | 1250 to 1500 rpm, throttle 0.25 to 1 | score 86.6 to 92.1, residual 3.6 to 4.5 dB too high at full throttle, all from 20 to 500 Hz | residual noise model: too much low-frequency residual |
| `f4-ej25` | 1000 to 1250 rpm, all throttles | score 90.8 to 93.8, from the residual level | residual noise model |
| `i4-vtec` | 9000 to 9250 rpm, above the top grid point at 8400 rpm | bands -4.0 dB at throttle 0.5, +2.5 to +2.6 dB at throttle 0.75 and 1 | table coverage: the tables end at 8400 rpm, and playback holds the edge point up to the limiter at 9400 rpm |
| `v12-412` | full throttle halfway between grid points, 15125 to 15625 and 17625 to 18125 rpm | bands -4.3 to -6.8 dB, score 56.6 to 74.9 | blend between neighbouring RPM tables |
| `i4-vtec`, `v8-ls`, `f4-ej25`, `v12-merlin` | full throttle halfway between the idle point and the next grid point | score 85.1 to 87.2, from the waveform, the residual and the spectrum | blend between neighbouring RPM tables, where the cycle shape changes most |
| `v12-412` | throttle 0 at 3810 to 13250 rpm, throttle 0.25 to 0.75 above 15500 rpm | bands +1.8 to +3.0 dB below 7000 rpm, -1.5 to -4.7 dB above 8500 rpm | residual noise model, on an engine that misfires at random and differs from itself by up to 2.9 dB |

The runtime, the resampler and the float16 storage of the tables cause none of these misses. The offline playback of the tables through the exact synthesiser port, at 44.1 kHz with an independent resampler, gives the same misses, and snarl against that playback scores 99.4 to 100.

## 1. Coverage

`tools/scorecard --write-fullrange` writes the scenarios `scenarios/upstream/<engine>/fullrange-t000.scn` to `fullrange-t100.scn` and `fullrange-mid-t100.scn` (`scenarios/upstream/README.md`). `just ref-fullrange` renders them with engine-sim. Idle is the first hold of `holds-t100.scn`. The top hold is the last multiple of 250 rpm below 0.998 of the limiter rpm of the profile, where the snarl limiter starts.

| Engine | Idle (rpm) | Top hold (rpm) | Limiter (rpm) | Top grid point of the tables (rpm) | Holds per throttle | Mid holds |
| --- | --- | --- | --- | --- | --- | --- |
| `i4-vtec` | 770 | 9250 | 9400 | 8400 | 35 | 34 |
| `v8-ls` | 690 | 6750 | 6800 | 6500 | 25 | 24 |
| `f4-ej25` | 830 | 6250 | 6500 | 6400 | 23 | 22 |
| `v12-412` | 3810 | 18250 | 18500 | 18000 | 59 | 58 |
| `v12-merlin` | 600 | 3250 | 3500 | 3000 | 12 | 11 |

Each hold is 0.5 s of settling and at least 24 whole cycles and 1 s, as in the grid of `tools/cycles scenarios`. A 0.25 s sweep joins two holds. The scorecard scores the hold windows only, after the 0.5 s of settling. One engine-sim render covers one throttle row of one engine. The holds on the multiples of 250 rpm are grid points of the cycle tables, so they test the tables at the points from which the tables were built. The mid holds test the blend between two points. The holds above the top grid point test playback outside the grid.

`just ref-fullrange` renders each file twice with the settings of `just ref-upstream` (`--volume 0.5`, 48 kHz, the unchanged upstream script): with the 3 s pre-roll into `out/reference/upstream/<engine>/`, and with a 3.5 s pre-roll into `out/reference/upstream/calib/<engine>/`, the second render of the calibration. It also renders the calibration of `sweep-wot` and `overrun`. The 70 renders ran 16 at a time in 741 s. The first pass without the mid files and the sweep calibration ran 50 renders in 610 s.

## 2. New measures

`snarl-compare --align` and `tools/scorecard` have three new measures in every window (`internal/analysis/hiss.go`). B is the render and A the reference. The score counts each of them above its floor, in steps that each cost a tenth of the score, as the other measures do (`docs/research/alignment-scorecard.md` section 2.6).

1. **Band level above 2 kHz.** The level of each third-octave band from 2 kHz to 16 kHz (10 bands at 48 kHz), relative to the K-weighted level of the window in each file. The distance is the size of the largest difference, B less A. The spectral distance of the existing scorecard cannot see these bands, because its floor at 60 dB below the strongest band of each frame removes them, and its loudness match spreads a band error over the whole spectrum.
2. **Tonality above 2 kHz.** In the octave bands 2 to 4, 4 to 8 and 8 to 16 kHz of a hold, the band power over the power of the noise between the engine-order harmonics, in dB. In the crank angle domain, the harmonics are the synchronous average cycle and the noise is the residual, with the bias of the average removed. Noise only gives 0 dB, and engine harmonics give more. The distance is the mean absolute difference over the bands. A first form, the power of the harmonics alone over the noise, was not stable: engine-sim against itself differed by up to 8.8 dB in the bands where the harmonics are far below the noise.
3. **Noise between the firing pulses above 2 kHz.** In the same octave bands of a hold, the mean band envelope over the cycle in 96 sectors of 2 ms frames, and the power of its quietest quarter of the sectors, relative to the K-weighted level of the window. The distance is the mean absolute difference over the bands.

| Distance | Floor | Step |
| --- | --- | --- |
| Band level above 2 kHz | 1.6 dB | 1 dB |
| Tonality above 2 kHz | 0.5 dB | 1 dB |
| Gap between firings above 2 kHz | 2.2 dB | 1 dB |

The tests `TestHissSelf`, `TestHissNoisier` and `TestHissTonality` check that two renders of one test engine with other noise differ by less than 0.5 dB in each measure, that ten times the noise raises the top bands by 14 to 21 dB, fills the gap by 12 dB or more and takes the tonality of the 2 to 4 kHz band below 1 dB, and that a tone on the engine cycle raises the tonality of its band by 8 dB or more.

## 3. engine-sim against itself

The reference against the second engine-sim render, over all the full-range holds of each engine:

| Engine | Holds | Least score | Band level median / largest (dB) | Tonality median / largest (dB) | Gap median / largest (dB) |
| --- | --- | --- | --- | --- | --- |
| `i4-vtec` | 209 | 100.0 | 0.52 / 1.03 | 0.06 / 0.30 | 0.13 / 2.18 |
| `v8-ls` | 149 | 99.2 | 0.64 / 1.42 | 0.04 / 0.17 | 0.19 / 1.66 |
| `f4-ej25` | 137 | 100.0 | 0.65 / 1.54 | 0.05 / 0.30 | 0.15 / 2.09 |
| `v12-412` | 353 | 85.1 | 0.61 / 2.88 | 0.05 / 1.17 | 0.19 / 3.06 |
| `v12-merlin` | 71 | 100.0 | 0.51 / 1.40 | 0.02 / 0.22 | 0.17 / 0.81 |

The floors of section 2 are at or above the largest value of `i4-vtec`, `v8-ls`, `f4-ej25` and `v12-merlin`, so engine-sim against itself scores 99.2 to 100 on those engines. `v12-412` misfires at random below 11000 rpm (`docs/research/reference-health.md`), and it differs from itself by more than the floors, as it does in the residual measure (`docs/research/cycle-tables.md` section 6.2).

On the existing measures, the worst values of engine-sim against itself stay below their floors on the four healthy engines, except the burst Mod distance of `v8-ls` (1.45 dB against the floor of 1.3 dB). The cycle waveform error of `v12-412` falls to 10.8 dB against the floor of 14 dB.

In the sweeps, engine-sim against itself has a worst band of 2.05 dB (`v8-ls`) and 2.26 dB (`i4-vtec`) above 2 kHz. In the overrun, it has 1.72 dB (`v8-ls`) and 2.41 dB (`v12-412`). Two sweeps of engine-sim pass the resonances at other times, as section 3.2 of `docs/research/alignment-scorecard.md` describes.

## 4. Real-time against offline

`TestSourceMatchesOfflineRender` (`cmd/snarl-harness/internal/stream/stream_test.go`) drives two `i4-vtec` engines with the same snapshot per 256-frame block: a full-throttle sweep from 1500 to 7500 rpm, then a closed-throttle overrun, with the own pops on. One engine plays through `stream.Source`, the audio source of the harness. The other renders in the loop of `snarl-render`: set the snapshot of the block, then render the block. The 153600 samples of both are the same, bit for bit, in both channels of the source, and both engines report the same number of pops.

The two paths share `Engine.Set` and `Engine.Render` at the same block size, and the engine has no clock of its own. The output can differ in one way only: the harness sets the snapshot on its game tick at 60 Hz, and the audio goroutine reads it at the next block that it renders. A recording that `snarl-render --replay` plays back applies each snapshot at the first block that starts at or after its time. A snapshot can therefore land up to one player buffer earlier or later in the harness than in the replay. That moves control changes in time. It does not add noise.

## 5. Results per engine

The full tables are in section 9. Each cell holds the score, the worst band above 2 kHz, the tonality difference and the gap difference, with the misses marked: S for a score below 95, H for a band more than 1.5 dB from engine-sim, T and G for a tonality or gap distance above the largest of engine-sim against itself for that engine.

The scenario totals, the mean of the hold scores:

| Engine | t000 | t025 | t050 | t075 | t100 | mid-t100 |
| --- | --- | --- | --- | --- | --- | --- |
| `i4-vtec` | 98.5 | 98.5 | 97.1 | 97.6 | 98.4 | 97.8 |
| `v8-ls` | 100.0 | 100.0 | 100.0 | 99.9 | 99.9 | 97.5 |
| `f4-ej25` | 98.0 | 99.2 | 99.0 | 99.4 | 98.9 | 96.8 |
| `v12-412` | 95.3 | 97.8 | 98.8 | 99.7 | 99.7 | 97.0 |
| `v12-merlin` | 93.5 | 99.7 | 100.0 | 100.0 | 100.0 | 94.8 |

The misses of each engine, with the measure that costs the most steps at the worst point:

- `i4-vtec`
  - Score: 770 to 1000 rpm at throttle 0 (90.3, envelope), 770 rpm at throttle 0.5 and 0.75 (93.6 and 89.0), 1250 to 1500 rpm at throttle 0.25 to 1 (86.6 to 92.1, residual level), 9000 to 9250 rpm at throttle 0.5 (72.7, band level), 9250 rpm at throttle 0.75 and 1 (89.6 and 91.0, band level).
  - Band level: 9000 to 9250 rpm at throttle 0.5 (-4.0 and -3.9 dB), 9250 rpm at throttle 0.75 and 1 (+2.6 and +2.5 dB).
  - Tonality: 0.3 to 1.1 dB at 12 single holds, the largest at 9000 rpm, throttle 0.5.
  - Gap: 3750 rpm at throttle 0 (2.6 dB, with less noise between the pulses than engine-sim).
  - Mid holds: 885 rpm (85.5, residual level), 1375 rpm (94.3), 5875 rpm (86.4, band level -2.8 dB), 8875 and 9125 rpm (band level +2.1 dB).
- `v8-ls`
  - Score: no hold below 95.
  - Band level: no hold beyond 1.5 dB. The largest is 1.2 dB.
  - Tonality: 0.2 to 0.3 dB at 8 single holds.
  - Gap: 3750 rpm at throttle 0.25 to 1 (1.7 to 2.0 dB, less noise between the pulses).
  - Mid holds: 845 rpm (86.1, cycle waveform), tonality 0.3 to 0.8 dB at 2375 to 2625 and 4125 to 4875 rpm.
- `f4-ej25`
  - Score: 1000 to 1250 rpm at throttle 0 (91.7, residual level), 1250 rpm at throttle 0.25 to 1 (90.8 to 93.8, residual level).
  - Band level: 2750 to 3000 rpm at throttle 0 (-1.6 and -2.0 dB), 2000 and 3250 rpm at throttle 1 (+1.5 and +1.7 dB).
  - Tonality: 2750 rpm at throttle 0 (0.3 dB).
  - Mid holds: 915 to 1375 rpm (87.2 to 94.3), 2625 rpm (90.8, tonality 1.2 dB), 3625 rpm (band level -1.6 dB), 1875 rpm (gap 2.4 dB, less noise between the pulses).
- `v12-412`
  - Score and band level, throttle 0: 3810 to 7000 rpm with bands +1.8 to +3.0 dB (score 86.0 to 97.7), 8500 to 13250 rpm with bands -1.5 to -4.7 dB (score 72.1 to 100), 18000 to 18250 rpm (+1.8 and +1.5 dB). 15000 rpm scores 88.6 from the gap (-3.0 dB).
  - Score and band level, throttle 0.25: 10750 rpm (-4.0 dB, 77.9), 16250 to 18250 rpm (-3.3 to +3.3 dB, 78.6 to 97.2), 9750 and 13000 rpm (-1.9 and +1.6 dB).
  - Score and band level, throttle 0.5 to 1: 15500 to 18250 rpm, bands -3.6 to +3.2 dB, score 81.3 to 100, and 14000 rpm at throttle 1 (-1.6 dB).
  - Mid holds: 15125 to 15625 rpm (bands -4.3 to -6.8 dB, score 57.5 to 74.9), 17625 to 18125 rpm (bands +2.3, -6.0 and +2.2 dB, score 56.6 to 94.0), 13375 rpm (-2.2 dB), 10875 and 12125 rpm (-1.6 and -1.8 dB).
- `v12-merlin`
  - Score and band level: 600 to 1000 rpm at throttle 0 (bands +3.8, +3.0 and +1.6 dB, score 78.9, 85.0 and 99.2).
  - Gap: 1500 and 3000 rpm at throttle 0.25 to 0.75 (0.8 and 0.9 dB, against the engine-sim largest of 0.81 dB).
  - Mid holds: 675 and 875 rpm (85.5 and 85.1, cycle waveform), 1875 rpm (gap 1.0 dB).

## 6. Sweeps, overrun and limiter

The default scenarios of `tools/scorecard`, with the new measures in the score:

| Engine | Holds t000 | Holds t050 | Holds t100 | Sweep | Overrun |
| --- | --- | --- | --- | --- | --- |
| `i4-vtec` | 98.5 | 99.2 | 99.4 | 98.8 | 98.7 |
| `v8-ls` | 99.4 | 99.7 | 99.8 | 99.4 | 99.5 |
| `f4-ej25` | 98.8 | 99.2 | 99.0 | 99.4 | 98.8 |
| `v12-412` | 94.2 | 98.1 | 98.1 | 98.5 | 92.4 |
| `v12-merlin` | 92.7 | 99.7 | 99.8 | 100.0 | 98.4 |

engine-sim against itself scores 99.2 to 99.5 on the sweeps and 99.5 to 99.9 on the overruns.

- **Full-throttle sweep.** The worst band above 2 kHz in a sweep window is 1.49 dB (`v12-merlin`), 2.08 dB (`v8-ls`), 2.21 dB (`f4-ej25`), 2.73 dB (`i4-vtec`) and 3.23 dB (`v12-412`). engine-sim against itself gives 2.05 to 2.26 dB. The signs change from window to window. Two windows are beyond the engine-sim spread: `i4-vtec` from 1470 to 1932 rpm (+2.7 dB) and `v12-412` from 15423 to 16282 rpm (-3.2 dB, less high-frequency level), the same range as the blend miss of the mid holds. The band detail of engine-sim against itself in the sweeps comes from `v8-ls` and `i4-vtec` only.
- **Overrun.** The worst band is 2.62 to 3.79 dB, against 1.72 and 2.41 dB for engine-sim against itself. In the closed-throttle sweeps down from the top, snarl has 1.6 to 3.3 dB less high-frequency level than engine-sim on `i4-vtec`, `f4-ej25` and `v12-412`, except +2.1 dB on `v12-412` from 14009 to 12876 rpm. Near idle at the end of the sweep, it has more: `v8-ls` +2.6 dB from 1154 to 690 rpm, `v12-merlin` +2.1 to +3.8 dB from 983 to 600 rpm. The `v12-merlin` value matches the closed-throttle idle holds.
- **Limiter.** In the limiter holds of `limiter-free`, the bands above 2 kHz are within 1.6 dB of engine-sim on `i4-vtec`, `v8-ls` and `f4-ej25`. `v12-412` is within 1.2 dB except -3.3 dB at 2 kHz. `v12-merlin` is 2.5 to 4.5 dB below engine-sim. The sweep into the limiter does not follow the free run of engine-sim, so its windows compare other RPM, and they give no evidence either way.

## 7. Causes

Each suspect was tested in a tool flag or a recipe argument, with the defaults unchanged:

- `just fullrange-play play` plays the full-range scenarios from the same tables through the offline playback (`cycles.Player`) and the exact synthesiser port in float64 at 44.1 kHz. `tools/scorecard --against out/fullrange/play` scores the playback against engine-sim, with the 44.1 to 48 kHz conversion of `analysis.Resample`. This playback has no `esynth.Runtime`, no real-time player (`cycles.Live`), no runtime resampler and no snarl control path (RPM smoothing, limiter and idle).
- `just fullrange-play play-noresidual all --no-residual` plays the same scenarios without the residual noise of the tables.
- `tools/scorecard --ref out/fullrange/play --against out/fullrange/snarl` scores the snarl renders against the playback.

| Suspect | Result | Evidence |
| --- | --- | --- |
| Residual noise model | cause of the closed-throttle idle misses of `v12-merlin`, the low-RPM residual misses of `i4-vtec` and `f4-ej25`, and part of the `v12-412` misses | `v12-merlin` at 600 rpm and throttle 0: bands +3.8 dB in snarl, +4.0 dB in the playback, -3.3 dB without the residual. The residual model adds about 7 dB above 2 kHz there, where engine-sim needs about 3.3 dB. `i4-vtec` at 1250 and 1500 rpm, full throttle: residual -28.9 and -30.0 dB in snarl against -32.5 and -34.5 dB in engine-sim, all of the excess from 20 to 500 Hz, and the residual from 2 kHz up within 0.2 dB. Without the residual, these holds score 95 or more. `v12-412` at throttle 0: without the residual, the bands at 3810 rpm fall from +2.9 to -7.8 dB and at 11750 rpm from -5.1 to -6.3 dB, so the residual model sets their level. |
| Blend between neighbouring RPM tables | cause of the mid-hold misses | `v12-412` at full throttle: the grid points 15250, 15500 and 15750 rpm are within -1.2 to +0.6 dB above 2 kHz, the holds halfway between them are -4.3 to -6.8 dB. The playback gives -6.9 dB at 15625 rpm and -6.4 dB at 17875 rpm, so the blend of `cycles.Player` and of `cycles.Live`, which use the same weights, causes it. A linear blend of two cycles whose high harmonics have different phases cancels part of those harmonics. At low RPM, the mid holds between idle and the next point score 85.1 to 87.2 from the cycle waveform, the residual and the spectrum, where the cycle shape changes most from point to point (`docs/research/cycle-tables.md` section 3). |
| Table coverage above the top grid point | cause of the `i4-vtec` misses at 9000 and 9250 rpm | The tables end at 8400 rpm and playback takes the edge point above it. The holds from 8500 to 8750 rpm pass. At 9000 and 9250 rpm, the playback gives the same band errors as snarl (-4.1 dB at throttle 0.5, +2.6 and +2.7 dB at throttle 0.75 and 1), with and without the residual. `v8-ls`, `v12-412` and `v12-merlin` have one hold 250 rpm above their top grid point, and `f4-ej25` none. `v8-ls` and `v12-merlin` pass there. `v12-412` misses at 18250 rpm by 1.5 to 2.8 dB, about as much as at 18000 rpm, which is a grid point. |
| float16 storage of the tables | not a cause, from the format, not switched off | snarl and the playback read the same float16 tables, so their comparison does not test this suspect, and no test builds the tables without float16. The format stores each Fourier coefficient of the average cycle and each residual envelope value over the peak of its array as float16 (`internal/cycles/table.go`): a relative error of at most 2^-11, 66 dB below each value, for values down to 2^-14 of the peak, and an absolute error of at most 2^-24 of the peak below that. That changes each harmonic by less than 0.01 dB, far below the 1.5 dB of the misses. |
| Resampler | not a cause | The playback uses `analysis.Resample` (64-tap Blackman) in place of the runtime resampler (`dsp.Resampler`), and it gives the same misses. |
| Runtime | not a cause | snarl against the playback scores 99.4 to 100 in every full-range scenario. The worst band above 2 kHz differs by at most 1.7 dB, at single holds, with no run of misses. The real-time path of the harness is the same as `snarl-render` (section 4). |
| engine-sim itself | part of the `v12-412` misses | `v12-412` differs from itself by up to 2.88 dB above 2 kHz, from its random misfires, so a band error below that on this engine is not a miss of snarl. |

## 8. What this means for the hiss

The scorecard finds no condition in which snarl has broadly more high-frequency noise than engine-sim. In the holds, the high-frequency misses of the healthy engines are mostly below engine-sim, and the gap between the firing pulses is quieter in snarl where it misses. The largest excess is the closed-throttle idle of `v12-merlin`, which is not the rev-up that the listener describes.

The conditions that the listener uses and that this scorecard does not cover:

- The own pops and the crackle model, which the harness plays by default and the scorecard turns off (`--no-pops`).
- Fast RPM changes in a free run with gear changes. The sweeps here take 8 s from idle to the top.
- The limiter bounce in the harness, which follows the harness drive model and not a dynamometer.

Recommendation: before the tuning phase, the listener compares the full-range pairs at the high-RPM holds, `out/reference/upstream/<engine>/fullrange-t100.wav` against `out/fullrange/snarl/<engine>/fullrange-t100.wav`. When the hiss is in those pairs, the new measures do not see it, and the next step is a measure of what the listener hears there. When it is not, the next step is a harness recording with the pops on and off, replayed with `snarl-render --replay`, to find the condition.

## 9. Full-range tables

Each cell: score, band level of the worst band above 2 kHz (snarl less engine-sim, dB), tonality (mean difference, negative is noisier), gap between firings (mean difference, positive is more noise), and the misses: S score below 95, H band beyond 1.5 dB, T tonality and G gap distance above the engine-sim largest of the engine. The mid holds are in section 5.

### i4-vtec

| RPM | t000 | t025 | t050 | t075 | t100 |
| --- | --- | --- | --- | --- | --- |
| 770 | 90.3 -1.0 +0.0 +0.1 S | 95.1 +0.5 +0.1 +0.2 | 93.6 +0.4 +0.1 +0.4 S | 89.0 +0.7 +0.1 +0.3 S | 95.5 +0.6 +0.1 +0.0 |
| 1000 | 94.6 -0.5 +0.0 -0.1 S | 100.0 +0.4 +0.3 +0.1 T | 100.0 -0.4 +0.2 -0.0 | 97.2 +0.5 +0.2 +0.0 | 100.0 +0.3 +0.3 +0.1 |
| 1250 | 95.9 -0.6 +0.0 -1.5 | 92.1 +0.5 +0.2 -0.0 S | 92.1 -0.6 +0.1 +0.1 S | 91.7 -0.8 +0.1 -0.0 S | 91.6 +0.6 +0.2 -0.1 S |
| 1500 | 100.0 -0.4 -0.0 -1.8 | 88.4 +0.7 +0.1 -0.0 S | 86.6 +0.5 -0.0 +0.1 S | 87.1 -0.7 +0.0 +0.1 S | 87.9 -0.7 +0.0 +0.1 S |
| 1750 | 100.0 -0.4 -0.2 -0.1 | 98.5 -0.6 +0.0 +0.1 | 100.0 +0.6 +0.1 +0.1 | 100.0 +0.5 +0.1 -0.0 | 100.0 -0.7 +0.0 -0.1 |
| 2000 | 100.0 -0.7 -0.0 -0.1 | 100.0 -0.7 -0.1 -0.7 | 100.0 +0.8 -0.0 -0.0 | 100.0 +0.7 -0.1 -0.7 | 100.0 -0.7 -0.0 -0.6 |
| 2250 | 99.7 -0.7 -0.1 +0.2 | 100.0 -1.0 -0.1 +0.0 | 100.0 -0.5 -0.0 +0.1 | 100.0 -0.8 -0.1 -0.2 | 100.0 -1.2 -0.2 +0.0 |
| 2500 | 100.0 -0.7 +0.1 -2.0 | 100.0 -0.6 -0.0 -2.1 | 100.0 -0.3 +0.1 -2.1 | 100.0 -0.7 +0.1 -0.0 | 100.0 +0.4 +0.1 -0.0 |
| 2750 | 100.0 -0.7 -0.0 -0.0 | 100.0 +0.7 +0.2 +0.3 T | 100.0 +0.9 +0.2 +0.0 | 100.0 +0.7 +0.1 +0.1 | 100.0 +0.6 +0.0 +0.2 |
| 3000 | 100.0 +0.3 -0.0 -1.2 | 100.0 -1.4 -0.1 -1.5 | 100.0 -0.5 -0.1 -1.6 | 100.0 -0.8 -0.1 -0.1 | 100.0 -0.5 -0.1 +0.1 |
| 3250 | 99.6 +0.7 -0.1 -0.0 | 100.0 +0.6 -0.0 -0.0 | 100.0 -0.6 +0.1 -0.1 | 100.0 +0.4 +0.1 -0.1 | 100.0 -0.5 +0.1 -0.2 |
| 3500 | 100.0 +0.4 -0.2 +0.0 | 100.0 -1.0 +0.1 +0.2 | 100.0 -0.8 +0.2 +0.1 | 100.0 -0.7 +0.1 -0.0 | 100.0 -0.9 +0.1 -0.1 |
| 3750 | 95.8 +0.8 +0.1 -2.6 G | 100.0 -0.7 -0.0 +0.0 | 100.0 -0.7 -0.0 +0.0 | 100.0 -0.7 -0.1 -2.1 | 100.0 -0.8 +0.0 -2.1 |
| 4000 | 100.0 -0.5 -0.0 -0.4 | 100.0 -0.6 +0.0 -0.1 | 100.0 +0.6 -0.0 +0.0 | 100.0 -0.6 +0.1 -0.2 | 100.0 -0.5 -0.0 -0.2 |
| 4250 | 100.0 -0.8 +0.0 +0.1 | 100.0 +0.8 +0.0 -0.0 | 100.0 -0.6 +0.1 -0.1 | 100.0 +1.0 +0.0 +0.1 | 100.0 +0.6 -0.1 -0.1 |
| 4500 | 100.0 -0.9 +0.0 -0.1 | 100.0 -0.8 +0.1 +0.1 | 100.0 -0.8 +0.0 +0.0 | 100.0 -0.7 +0.1 -0.0 | 100.0 -0.6 +0.0 -0.0 |
| 4750 | 100.0 -0.6 -0.0 +0.1 | 100.0 -0.3 +0.0 -0.0 | 100.0 -0.5 +0.0 +0.1 | 100.0 +0.7 +0.1 -0.0 | 100.0 -0.5 +0.0 +0.0 |
| 5000 | 100.0 -0.4 -0.2 -1.9 | 100.0 -0.5 -0.1 +0.1 | 100.0 -0.3 +0.1 -1.4 | 100.0 -0.3 -0.1 -1.5 | 100.0 -0.6 -0.1 -1.6 |
| 5250 | 100.0 -0.4 -0.4 -0.0 T | 100.0 -0.6 +0.0 +0.1 | 100.0 -0.3 -0.1 +0.1 | 100.0 +0.7 -0.2 -0.1 | 100.0 -0.4 -0.1 +0.0 |
| 5500 | 100.0 +0.3 +0.1 -0.1 | 100.0 -0.6 -0.0 -0.0 | 100.0 +0.5 +0.2 +0.1 | 100.0 -0.5 +0.1 +0.1 | 100.0 -0.8 +0.1 +0.0 |
| 5750 | 100.0 -0.5 -0.0 +0.0 | 100.0 -0.6 +0.0 +0.2 | 100.0 -0.5 +0.0 +0.0 | 100.0 -0.5 +0.1 +0.1 | 100.0 +0.5 +0.0 +0.1 |
| 6000 | 100.0 -0.4 +0.1 -1.7 | 100.0 +0.4 -0.0 -1.5 | 100.0 +0.4 -0.0 -0.0 | 100.0 -0.5 +0.0 -0.1 | 100.0 +0.5 +0.0 -1.6 |
| 6250 | 100.0 -0.3 +0.0 +0.2 | 100.0 +0.3 +0.1 +0.2 | 100.0 -0.5 +0.0 +0.1 | 100.0 -0.7 -0.1 -0.0 | 100.0 -0.6 +0.1 +0.2 |
| 6500 | 100.0 -0.5 -0.1 -0.0 | 100.0 -0.3 -0.0 +0.0 | 100.0 -0.8 +0.0 +0.1 | 100.0 -1.0 +0.1 -0.1 | 100.0 -0.6 +0.1 +0.2 |
| 6750 | 100.0 +0.6 +0.1 -0.1 | 100.0 +0.4 -0.0 +0.0 | 100.0 +0.5 -0.0 -0.0 | 100.0 +0.5 +0.0 -0.0 | 100.0 +0.3 +0.1 -0.1 |
| 7000 | 100.0 +0.3 +0.0 +0.0 | 100.0 +0.3 +0.1 +0.2 | 100.0 -0.6 +0.1 +0.2 | 100.0 +0.4 -0.1 +0.2 | 100.0 +0.6 -0.0 +0.2 |
| 7250 | 100.0 -0.4 +0.0 -0.0 | 100.0 +0.4 +0.0 +0.0 | 100.0 +0.5 -0.1 +0.0 | 100.0 -0.6 -0.0 +0.1 | 100.0 +0.5 +0.1 +0.2 |
| 7500 | 100.0 +0.5 -0.4 -1.8 T | 100.0 +0.3 -0.1 -0.1 | 100.0 +0.4 -0.0 -0.2 | 100.0 +0.5 -0.1 -1.3 | 100.0 +0.5 -0.2 -1.3 |
| 7750 | 100.0 -0.4 +0.0 +0.2 | 100.0 -0.7 +0.1 +0.1 | 100.0 -0.2 +0.1 +0.1 | 100.0 +0.4 -0.1 +0.1 | 100.0 +0.6 +0.1 -0.0 |
| 8000 | 100.0 -0.5 +0.0 -0.2 | 100.0 +0.6 +0.1 -0.2 | 100.0 -0.5 +0.0 -0.2 | 100.0 -0.6 -0.1 +0.0 | 100.0 +0.9 +0.0 -0.1 |
| 8250 | 100.0 +0.7 -0.1 -0.1 | 100.0 -0.5 +0.0 -0.0 | 100.0 +0.4 -0.0 -0.0 | 100.0 -0.6 +0.0 -0.0 | 100.0 +0.5 +0.1 +0.0 |
| 8500 | 100.0 +0.6 +0.1 +0.2 | 100.0 -1.1 +0.0 -0.0 | 100.0 +0.5 -0.2 +0.1 | 100.0 +0.8 -0.1 +0.0 | 99.4 +1.3 -0.1 +0.0 |
| 8750 | 100.0 -0.3 -0.2 +0.0 | 100.0 -1.0 +0.3 -0.0 T | 95.7 +0.5 -0.9 -0.0 T | 100.0 +1.2 +0.0 -0.0 | 100.0 +1.0 -0.2 +0.0 |
| 9000 | 100.0 -0.4 -0.1 -0.0 | 100.0 -1.3 +0.0 +0.1 | 72.7 -4.0 -1.1 +0.1 SHT | 100.0 +0.7 +0.3 +0.2 | 100.0 +1.1 +0.2 +0.2 |
| 9250 | 100.0 -0.6 -0.2 +0.2 T | 98.8 -1.0 +0.6 -0.1 T | 75.9 -3.9 -0.8 -0.0 SHT | 89.6 +2.6 +0.3 +0.3 SHT | 91.0 +2.5 +0.3 +0.3 SHT |

### v8-ls

| RPM | t000 | t025 | t050 | t075 | t100 |
| --- | --- | --- | --- | --- | --- |
| 690 | 100.0 -0.3 +0.0 +1.0 | 100.0 +0.3 -0.0 +0.1 | 100.0 +0.3 -0.0 +0.0 | 100.0 +0.3 -0.0 +0.0 | 100.0 +0.3 -0.0 +0.1 |
| 1000 | 100.0 +0.6 +0.1 +0.0 | 100.0 -0.4 +0.0 -0.7 | 100.0 +0.3 +0.0 -0.6 | 100.0 +0.3 +0.0 -0.6 | 100.0 +0.3 +0.0 -0.6 |
| 1250 | 100.0 -0.7 -0.0 +0.1 | 100.0 -0.5 +0.0 -0.1 | 100.0 -0.6 +0.1 -0.1 | 100.0 +0.6 +0.1 -0.2 | 100.0 +0.5 +0.1 -0.2 |
| 1500 | 99.1 -0.5 -0.0 -0.2 | 100.0 +1.1 -0.0 -0.1 | 99.8 +1.0 -0.0 -0.1 | 99.0 +1.1 +0.0 -1.4 | 98.6 +1.1 +0.0 -1.4 |
| 1750 | 100.0 -0.4 -0.0 -0.0 | 100.0 -0.4 +0.0 -0.1 | 100.0 -0.5 +0.0 -0.1 | 100.0 +0.5 +0.0 -0.1 | 100.0 +0.6 +0.0 -0.1 |
| 2000 | 100.0 +0.7 -0.0 -0.1 | 100.0 +0.6 +0.0 +0.1 | 100.0 +0.5 +0.1 -0.0 | 100.0 +0.4 +0.1 -0.0 | 100.0 +0.4 +0.1 -0.0 |
| 2250 | 100.0 -0.5 -0.1 -0.1 | 100.0 -0.6 +0.0 +0.1 | 100.0 -0.7 +0.0 +0.1 | 100.0 -0.7 +0.0 +0.1 | 100.0 -0.8 +0.0 +0.1 |
| 2500 | 100.0 -0.5 -0.0 +0.0 | 100.0 +0.5 -0.0 -0.9 | 100.0 +0.5 -0.0 -0.8 | 100.0 +0.5 -0.0 -0.8 | 100.0 +0.5 -0.0 -0.8 |
| 2750 | 100.0 -0.4 +0.1 -0.2 | 100.0 -0.8 -0.0 +0.2 | 100.0 -0.7 -0.2 +0.2 T | 100.0 -0.7 -0.2 +0.2 T | 100.0 -0.6 -0.2 +0.2 T |
| 3000 | 100.0 -0.4 +0.0 -1.1 | 100.0 -0.8 -0.0 -0.9 | 100.0 -0.9 -0.0 -1.1 | 100.0 -0.9 -0.1 -1.0 | 100.0 -0.8 -0.1 -1.0 |
| 3250 | 100.0 -0.5 +0.0 +0.0 | 100.0 +0.4 -0.0 +0.2 | 100.0 -0.7 +0.0 +0.1 | 100.0 -0.6 +0.0 +0.1 | 100.0 -0.6 +0.0 +0.1 |
| 3500 | 100.0 -0.8 +0.0 -0.1 | 100.0 -0.7 -0.0 -0.0 | 100.0 -0.5 -0.0 -0.1 | 100.0 -0.4 +0.0 -0.1 | 100.0 -0.4 +0.0 -0.1 |
| 3750 | 100.0 -0.7 +0.1 -1.4 | 100.0 -0.7 +0.0 -1.7 G | 100.0 +0.8 +0.0 -1.9 G | 100.0 -0.7 -0.0 -1.9 G | 100.0 -0.7 -0.0 -2.0 G |
| 4000 | 100.0 +0.5 +0.0 +0.0 | 100.0 +0.9 +0.0 +0.0 | 100.0 +1.1 +0.1 +0.1 | 100.0 +1.1 +0.2 -0.0 | 100.0 +1.1 +0.2 -0.0 |
| 4250 | 100.0 +0.4 -0.2 +0.2 | 100.0 +0.6 +0.0 +0.0 | 100.0 +0.7 -0.2 +0.2 | 100.0 +1.0 -0.2 +0.2 T | 100.0 +0.8 -0.2 +0.2 T |
| 4500 | 100.0 -0.6 +0.0 -0.0 | 100.0 +0.7 +0.2 -0.2 | 100.0 -0.7 +0.1 -0.4 | 100.0 -0.5 +0.1 -0.3 | 100.0 +0.5 +0.1 -0.3 |
| 4750 | 100.0 -0.3 +0.0 -0.1 | 100.0 -1.2 -0.0 +0.0 | 100.0 -1.1 -0.1 +0.0 | 100.0 -1.2 -0.1 -0.1 | 100.0 -1.2 -0.1 -0.1 |
| 5000 | 100.0 +0.4 +0.1 +0.0 | 100.0 +0.7 -0.1 +0.2 | 100.0 +0.7 -0.1 +0.1 | 100.0 +0.9 -0.1 +0.2 | 100.0 +0.9 -0.1 +0.2 |
| 5250 | 100.0 -0.6 +0.1 -0.2 | 100.0 +0.9 +0.0 +0.2 | 100.0 +0.7 -0.3 -0.0 T | 100.0 +0.9 -0.1 +0.1 | 100.0 +0.9 -0.1 +0.1 |
| 5500 | 100.0 -0.9 +0.2 -0.0 T | 100.0 -1.0 -0.0 -0.0 | 100.0 -0.6 -0.0 -0.2 | 100.0 -0.5 -0.1 -0.1 | 100.0 -0.5 -0.1 -0.1 |
| 5750 | 100.0 -1.0 +0.0 +0.2 | 100.0 +0.3 +0.0 +0.1 | 100.0 -0.5 +0.0 +0.1 | 100.0 -0.3 -0.0 +0.0 | 100.0 +0.3 -0.0 +0.0 |
| 6000 | 100.0 +0.8 -0.0 -1.3 | 100.0 -0.5 -0.1 -0.1 | 100.0 -0.4 +0.0 -1.2 | 100.0 +0.4 +0.0 -1.2 | 100.0 +0.5 +0.0 -1.2 |
| 6250 | 100.0 +0.9 -0.0 +0.2 | 100.0 +0.5 -0.0 +0.1 | 100.0 +0.5 -0.0 +0.1 | 100.0 +0.6 -0.0 +0.2 | 100.0 +0.6 -0.0 +0.2 |
| 6500 | 100.0 +0.9 +0.0 +0.1 | 100.0 +0.8 -0.0 +0.2 | 100.0 +0.8 +0.0 +0.3 | 100.0 +0.8 -0.0 +0.3 | 100.0 +0.8 -0.0 +0.3 |
| 6750 | 100.0 -0.7 -0.2 +0.1 T | 100.0 +0.6 -0.0 -0.0 | 100.0 +0.8 +0.2 +0.0 | 100.0 +0.8 -0.0 -0.2 | 100.0 +0.8 -0.0 -0.3 |

### f4-ej25

| RPM | t000 | t025 | t050 | t075 | t100 |
| --- | --- | --- | --- | --- | --- |
| 830 | 95.9 -0.3 +0.0 +0.1 | 100.0 -0.7 +0.0 +0.2 | 96.6 -0.4 -0.0 -0.2 | 100.0 +0.4 -0.0 -0.1 | 95.2 +0.5 -0.0 -0.1 |
| 1000 | 91.7 +0.4 +0.0 +0.3 S | 100.0 -0.6 -0.0 +0.1 | 100.0 -1.1 +0.0 -0.1 | 100.0 -0.4 +0.0 +0.1 | 99.1 -0.4 +0.0 +0.2 |
| 1250 | 93.2 +0.7 +0.1 -0.8 S | 90.8 -0.4 -0.0 -0.0 S | 91.7 -0.4 -0.1 +0.1 S | 93.8 -0.4 -0.0 -0.0 S | 93.7 -0.5 +0.0 +0.0 S |
| 1500 | 98.8 -0.7 -0.0 -1.1 | 98.3 -0.6 -0.1 -0.1 | 100.0 -0.6 -0.0 +0.1 | 100.0 -0.7 -0.0 +0.1 | 99.4 -1.0 +0.0 -0.1 |
| 1750 | 100.0 -0.7 -0.0 +0.2 | 100.0 -0.8 -0.0 -0.1 | 100.0 -0.5 +0.0 -0.1 | 97.5 -0.8 +0.0 +0.1 | 100.0 -0.9 +0.0 -0.0 |
| 2000 | 100.0 -0.4 -0.0 -0.2 | 100.0 +0.8 +0.0 -0.4 | 100.0 +1.1 -0.0 +0.1 | 100.0 +0.8 -0.0 +0.1 | 100.0 +1.5 +0.0 -0.1 H |
| 2250 | 100.0 -1.3 -0.1 +0.0 | 100.0 -0.5 +0.1 -0.3 | 100.0 -0.7 -0.1 -0.1 | 100.0 -0.7 -0.1 -0.2 | 100.0 +0.4 -0.1 -0.1 |
| 2500 | 100.0 -0.6 +0.0 -0.9 | 100.0 +0.4 -0.1 -0.2 | 100.0 -0.9 +0.2 -1.5 | 100.0 -0.6 +0.0 -1.5 | 100.0 -0.6 +0.1 -0.2 |
| 2750 | 98.3 -1.6 +0.3 -0.3 HT | 100.0 +0.7 -0.0 +0.1 | 100.0 +1.1 -0.3 +0.1 | 100.0 -0.4 -0.0 -0.1 | 100.0 -0.9 +0.0 +0.0 |
| 3000 | 95.9 -2.0 -0.0 -1.1 H | 100.0 -0.9 +0.0 +0.0 | 100.0 -0.7 -0.0 -1.1 | 100.0 +1.2 +0.0 +0.1 | 100.0 +0.8 +0.0 -0.1 |
| 3250 | 100.0 -1.3 +0.1 +0.0 | 100.0 +0.7 -0.1 +0.2 | 100.0 -0.6 +0.1 -0.2 | 100.0 +0.5 +0.1 +0.0 | 99.4 +1.7 +0.1 +0.1 H |
| 3500 | 100.0 +1.0 +0.1 -0.1 | 100.0 -0.6 -0.1 -0.1 | 100.0 -0.6 +0.0 -0.1 | 100.0 -0.9 -0.0 +0.1 | 100.0 -0.8 +0.0 -0.1 |
| 3750 | 100.0 -1.1 +0.0 -1.7 | 100.0 +0.9 +0.2 +0.0 | 100.0 +1.0 +0.0 -1.8 | 100.0 +0.6 -0.0 -0.1 | 100.0 +1.1 +0.0 -1.8 |
| 4000 | 100.0 +0.6 +0.2 -0.2 | 100.0 +0.5 +0.1 +0.2 | 100.0 -0.5 +0.1 -0.3 | 100.0 +0.5 +0.1 -0.2 | 100.0 -0.5 +0.0 -0.2 |
| 4250 | 100.0 -0.6 -0.1 -0.0 | 100.0 -0.4 -0.0 +0.1 | 100.0 +0.7 +0.0 +0.1 | 100.0 +0.8 -0.0 +0.1 | 100.0 +0.5 -0.1 +0.1 |
| 4500 | 100.0 -0.5 -0.1 -0.1 | 100.0 -0.8 +0.1 +0.0 | 100.0 -0.5 +0.0 +0.1 | 100.0 -0.7 +0.0 +0.0 | 100.0 -0.6 +0.0 +0.0 |
| 4750 | 100.0 +0.6 -0.0 +0.1 | 100.0 -0.4 -0.1 -0.2 | 100.0 +0.6 -0.1 -0.0 | 100.0 +0.6 -0.1 -0.0 | 100.0 -0.5 -0.1 +0.1 |
| 5000 | 100.0 -0.8 -0.0 -1.5 | 100.0 -0.7 -0.0 +0.1 | 100.0 -0.5 +0.2 +0.1 | 100.0 -0.5 -0.0 -1.1 | 100.0 -0.9 +0.1 +0.0 |
| 5250 | 100.0 +0.4 -0.2 -0.2 | 100.0 -0.9 +0.0 -0.1 | 100.0 -1.0 -0.0 +0.0 | 100.0 -0.9 -0.1 -0.1 | 100.0 -0.5 -0.0 -0.1 |
| 5500 | 100.0 -0.9 -0.1 -0.1 | 100.0 +0.7 +0.2 -0.0 | 100.0 +0.3 +0.1 -0.1 | 100.0 +0.5 +0.0 -0.1 | 100.0 +0.4 +0.1 -0.1 |
| 5750 | 100.0 +0.4 -0.0 +0.1 | 100.0 -0.6 +0.0 -0.1 | 100.0 -0.5 -0.0 +0.1 | 100.0 -0.4 -0.0 +0.1 | 100.0 +0.2 -0.0 +0.0 |
| 6000 | 100.0 +0.6 +0.0 -1.0 | 100.0 +0.4 -0.0 -1.3 | 100.0 +0.3 -0.0 +0.0 | 100.0 -0.3 -0.0 +0.1 | 100.0 +0.7 +0.0 -1.1 |
| 6250 | 100.0 +0.4 -0.0 -0.2 | 100.0 +0.4 -0.0 +0.1 | 100.0 -0.5 +0.1 -0.0 | 100.0 -0.5 +0.1 -0.1 | 100.0 -0.5 +0.0 +0.1 |

### v12-412

| RPM | t000 | t025 | t050 | t075 | t100 |
| --- | --- | --- | --- | --- | --- |
| 3810 | 97.7 +1.8 -0.1 +0.5 H | 100.0 +0.5 -0.0 -0.1 | 100.0 +0.7 +0.0 -0.5 | 100.0 +1.4 +0.1 -0.2 | 100.0 -0.8 +0.1 -0.2 |
| 4000 | 86.0 +2.9 +0.0 +0.3 SH | 100.0 -1.0 -0.1 +0.3 | 99.2 +1.0 -0.1 +0.0 | 100.0 -1.3 -0.1 -0.1 | 100.0 -0.8 -0.0 -0.0 |
| 4250 | 92.6 +2.3 -0.1 +0.7 SH | 100.0 +0.5 +0.1 +0.0 | 100.0 +0.8 -0.1 -0.2 | 100.0 -0.6 -0.0 -0.2 | 100.0 +1.0 -0.0 +0.0 |
| 4500 | 100.0 +1.2 +0.0 +0.4 | 100.0 -1.0 +0.0 +0.1 | 100.0 -0.9 -0.0 +0.0 | 100.0 +0.5 -0.1 +0.1 | 100.0 +0.5 +0.0 +0.0 |
| 4750 | 99.0 +1.3 +0.0 +0.0 | 100.0 +1.4 -0.1 +0.6 | 100.0 +0.7 -0.1 +0.3 | 100.0 +0.8 -0.0 +0.0 | 100.0 +0.7 -0.1 +0.2 |
| 5000 | 86.4 +3.0 +0.0 +0.6 SH | 100.0 +1.0 -0.0 +0.3 | 100.0 -0.9 -0.1 -0.1 | 100.0 -0.8 -0.0 -0.4 | 100.0 -0.6 -0.2 -0.2 |
| 5250 | 100.0 +1.2 +0.0 +0.2 | 100.0 +1.2 +0.2 +0.6 | 100.0 +1.0 -0.1 +0.1 | 100.0 -0.5 -0.1 +0.1 | 100.0 +0.6 -0.1 +0.2 |
| 5500 | 100.0 +1.2 -0.0 +0.3 | 100.0 -0.5 +0.5 -0.3 | 100.0 -0.8 -0.4 -0.0 | 100.0 -0.6 -0.4 -0.0 | 100.0 +0.5 -0.3 -0.1 |
| 5750 | 100.0 +1.3 -0.1 +0.2 | 100.0 +0.9 -0.0 +0.4 | 100.0 +0.9 +0.0 +0.1 | 100.0 +0.6 +0.0 +0.2 | 100.0 +0.7 +0.0 +0.0 |
| 6000 | 100.0 +1.3 -0.1 -1.1 | 100.0 -0.8 -0.0 +0.0 | 100.0 -0.9 +0.0 -0.6 | 100.0 -0.6 +0.0 -0.4 | 100.0 -0.6 +0.0 -0.0 |
| 6250 | 95.1 +2.1 -0.2 +0.1 H | 100.0 +0.4 -0.1 -0.0 | 100.0 +1.0 -0.0 -0.1 | 100.0 +0.6 -0.0 -0.2 | 100.0 +0.8 -0.0 -0.3 |
| 6500 | 97.6 +1.8 +0.0 +0.0 H | 100.0 +0.9 -0.0 -0.0 | 100.0 -0.5 -0.0 -0.3 | 100.0 +0.7 -0.0 -0.1 | 100.0 +0.4 -0.0 -0.1 |
| 6750 | 95.4 +2.1 -0.1 +0.1 H | 100.0 +0.6 +0.0 +0.1 | 100.0 -1.1 +0.1 -0.1 | 100.0 -1.1 +0.1 -0.2 | 100.0 -0.3 +0.1 -0.1 |
| 7000 | 91.2 +2.5 -0.4 +0.1 SH | 100.0 +0.6 -0.1 +0.0 | 100.0 +0.3 -0.1 +0.1 | 100.0 +0.6 -0.1 -0.0 | 100.0 +0.8 -0.2 +0.1 |
| 7250 | 100.0 +1.4 +0.0 +0.3 | 100.0 +0.9 -0.1 +0.0 | 100.0 -0.6 -0.0 -0.1 | 100.0 -0.4 +0.0 +0.0 | 100.0 -0.4 -0.0 -0.1 |
| 7500 | 99.5 +0.7 +0.0 -2.2 | 100.0 +0.4 -0.4 -0.8 | 100.0 -0.4 +0.1 +0.1 | 100.0 -0.4 +0.1 -0.1 | 100.0 +0.4 +0.1 -0.0 |
| 7750 | 100.0 +0.6 -0.1 +0.0 | 100.0 +0.8 -0.1 +0.2 | 100.0 +0.7 +0.0 +0.3 | 100.0 +0.4 +0.1 -0.0 | 100.0 -0.5 +0.1 +0.2 |
| 8000 | 100.0 -0.4 +0.1 -0.0 | 100.0 +0.4 +0.1 +0.0 | 100.0 -0.7 -0.1 +0.0 | 100.0 +0.4 +0.1 +0.0 | 100.0 -0.7 -0.1 -0.1 |
| 8250 | 100.0 +0.8 -0.0 -0.1 | 100.0 +0.3 +0.0 -0.0 | 100.0 -0.4 +0.0 -0.1 | 100.0 +0.4 -0.0 -0.1 | 100.0 +0.5 +0.1 -0.1 |
| 8500 | 94.2 -2.2 -0.1 -0.1 SH | 100.0 -0.2 +0.1 +0.0 | 100.0 +0.6 -0.0 +0.1 | 100.0 -0.5 +0.0 -0.0 | 100.0 +0.5 -0.0 +0.0 |
| 8750 | 100.0 -1.6 -0.2 -0.3 H | 100.0 -0.9 +0.2 +0.0 | 100.0 -0.6 +0.0 +0.2 | 100.0 -1.0 +0.1 -0.2 | 100.0 +0.7 +0.1 +0.0 |
| 9000 | 99.2 -1.7 -0.4 -0.1 H | 100.0 -0.6 -0.1 +0.1 | 100.0 +0.5 -0.2 +0.0 | 100.0 -0.5 -0.3 +0.4 | 100.0 -0.3 -0.2 +0.1 |
| 9250 | 100.0 -1.3 -0.3 -0.2 | 100.0 -1.4 +0.3 -0.2 | 100.0 +0.6 +0.0 -0.0 | 100.0 -0.7 -0.0 -0.0 | 100.0 +1.0 +0.1 +0.1 |
| 9500 | 80.9 -3.6 +0.1 -0.5 SH | 100.0 -0.6 +0.0 +0.2 | 100.0 -0.3 -0.0 +0.1 | 100.0 -0.5 -0.0 +0.1 | 100.0 -0.8 -0.0 -0.0 |
| 9750 | 92.1 -2.4 -0.1 +0.1 SH | 97.3 -1.9 +0.3 -0.0 H | 100.0 -0.7 +0.2 -0.0 | 100.0 -1.0 +0.1 -0.1 | 100.0 -0.6 +0.2 -0.0 |
| 10000 | 92.1 -2.4 -0.2 -1.4 SH | 100.0 +0.5 -0.1 +0.0 | 100.0 +0.6 -0.2 +0.0 | 100.0 +0.4 -0.2 -0.3 | 100.0 +0.4 -0.2 -0.2 |
| 10250 | 76.2 -4.2 -0.3 -0.7 SH | 100.0 -0.9 -0.2 -0.0 | 100.0 -0.8 -0.2 -0.1 | 100.0 -1.1 -0.1 -0.1 | 100.0 -0.7 -0.2 -0.1 |
| 10500 | 79.5 -3.8 -0.4 -0.6 SH | 100.0 -1.1 +0.0 +0.0 | 100.0 -0.5 +0.1 +0.3 | 100.0 +0.5 +0.1 -0.0 | 100.0 +0.8 +0.2 +0.2 |
| 10750 | 86.5 -2.8 -0.6 -0.7 SH | 77.9 -4.0 +0.0 -0.5 SH | 100.0 -0.8 -0.3 -0.2 | 100.0 -0.7 -0.3 -0.1 | 100.0 -1.1 -0.3 -0.2 |
| 11000 | 86.3 -3.0 -0.5 -0.6 SH | 100.0 -1.5 -0.1 -0.3 | 100.0 -1.0 -0.2 -0.4 | 100.0 -0.9 -0.2 -0.1 | 100.0 -0.7 -0.2 -0.3 |
| 11250 | 84.2 -3.2 -0.4 -0.6 SH | 100.0 +1.1 -0.2 -0.1 | 99.9 -0.4 -0.5 -0.1 | 97.8 -0.7 -0.7 -0.3 | 99.9 -0.6 -0.5 -0.1 |
| 11500 | 83.6 -3.3 +0.0 -0.5 SH | 100.0 -0.5 -0.0 -0.1 | 100.0 -0.7 +0.0 -0.1 | 100.0 -0.4 -0.0 +0.0 | 100.0 -0.5 +0.0 -0.1 |
| 11750 | 72.1 -4.7 +0.0 -0.4 SH | 100.0 +1.3 -0.0 +0.4 | 100.0 -0.7 -0.0 +0.1 | 100.0 +0.4 -0.0 +0.0 | 100.0 +0.7 +0.0 +0.2 |
| 12000 | 88.3 -2.8 +0.1 -1.7 SH | 100.0 +0.8 -0.1 -0.4 | 100.0 +0.4 +0.0 +0.2 | 100.0 +0.8 -0.1 -0.1 | 100.0 -0.3 -0.0 -0.0 |
| 12250 | 94.1 -2.2 +0.1 -0.5 SH | 100.0 -0.5 +0.0 +0.3 | 100.0 -0.9 -0.0 +0.0 | 100.0 -1.4 -0.1 +0.1 | 100.0 -1.2 -0.1 -0.1 |
| 12500 | 100.0 -1.4 +0.4 -0.2 | 100.0 -0.6 -0.2 +0.2 | 100.0 -0.6 +0.0 -0.1 | 100.0 +0.4 -0.0 +0.0 | 100.0 +0.5 -0.0 +0.1 |
| 12750 | 100.0 -1.5 +0.3 -0.1 H | 100.0 +0.4 -0.2 -0.1 | 100.0 -0.7 -0.0 +0.0 | 100.0 -0.8 -0.0 -0.1 | 100.0 -1.2 -0.0 -0.0 |
| 13000 | 85.9 -3.0 +0.4 -0.6 SH | 100.0 +1.6 +0.1 -0.1 H | 100.0 +0.5 +0.0 -0.0 | 100.0 -0.6 +0.0 -0.1 | 100.0 +0.4 +0.0 +0.1 |
| 13250 | 97.7 -1.8 +0.4 -0.3 H | 100.0 +1.1 -0.2 -0.0 | 100.0 -0.5 -0.1 -0.1 | 100.0 -0.7 -0.0 +0.1 | 100.0 -0.7 -0.1 -0.0 |
| 13500 | 100.0 -0.5 +0.2 -0.1 | 100.0 +1.2 +0.0 +0.1 | 100.0 +0.5 +0.1 -0.1 | 100.0 +0.3 +0.0 +0.0 | 100.0 +0.7 +0.1 -0.1 |
| 13750 | 100.0 -1.1 +0.2 -0.4 | 100.0 +1.0 -0.1 +0.1 | 100.0 -0.7 -0.0 -0.2 | 100.0 -1.2 +0.1 -0.3 | 100.0 -0.6 -0.0 -0.3 |
| 14000 | 100.0 -0.5 +0.1 +0.1 | 100.0 +0.9 -0.2 -0.2 | 100.0 -0.4 +0.0 -0.2 | 100.0 -1.2 -0.0 -0.3 | 99.8 -1.6 +0.0 -0.1 H |
| 14250 | 100.0 -0.4 +0.3 -0.2 | 100.0 -0.8 -0.2 -0.1 | 100.0 -1.1 -0.1 -0.2 | 100.0 -0.8 +0.1 -0.2 | 100.0 -1.0 +0.1 -0.4 |
| 14500 | 100.0 +0.6 -0.1 +0.2 | 100.0 -0.6 -0.0 +0.1 | 100.0 +0.3 -0.1 -0.1 | 100.0 -0.5 +0.0 -0.2 | 100.0 -1.2 -0.0 -0.2 |
| 14750 | 100.0 +1.0 +0.1 +0.1 | 100.0 -0.4 +0.2 -0.3 | 100.0 -0.7 +0.1 -0.2 | 100.0 -0.7 +0.3 -0.3 | 100.0 -0.8 +0.1 -0.1 |
| 15000 | 88.6 -0.5 -0.9 -3.0 S | 100.0 +0.9 +0.1 -0.8 | 100.0 -0.5 -0.0 -0.1 | 100.0 -0.8 -0.0 -0.6 | 100.0 -0.6 +0.0 -0.1 |
| 15250 | 100.0 +0.3 +0.3 +0.1 | 100.0 +0.3 -0.4 +0.2 | 100.0 +0.6 -0.2 +0.1 | 100.0 -1.3 -0.0 -0.1 | 100.0 +0.6 -0.0 -0.0 |
| 15500 | 100.0 -1.2 -0.0 +0.2 | 100.0 +0.8 -0.5 -0.0 | 83.7 +3.2 -0.6 +0.0 SH | 100.0 -1.0 -0.0 -0.2 | 100.0 -1.0 -0.1 -0.1 |
| 15750 | 100.0 -0.8 +0.1 +0.1 | 100.0 -1.1 -0.2 -0.1 | 100.0 +0.7 -0.4 +0.3 | 100.0 -1.5 -0.1 +0.1 H | 100.0 -1.2 -0.1 +0.3 |
| 16000 | 100.0 -1.0 +0.0 +0.0 | 100.0 -1.4 -0.3 -0.1 | 81.3 -3.6 -0.1 -0.3 SH | 94.3 -2.2 -0.0 +0.0 SH | 87.9 -2.8 +0.1 +0.0 SH |
| 16250 | 100.0 -0.8 +0.1 -0.1 | 93.4 -2.3 +0.0 -0.0 SH | 94.5 -2.1 +0.2 +0.1 SH | 99.5 -0.6 +0.1 -0.1 | 98.7 -0.9 +0.1 +0.0 |
| 16500 | 100.0 +0.9 +0.0 -0.1 | 94.9 -2.1 -0.3 +0.1 SH | 100.0 -0.6 -0.1 +0.2 | 100.0 -0.7 -0.0 +0.1 | 100.0 +0.4 -0.1 +0.2 |
| 16750 | 100.0 +0.8 +0.1 -0.0 | 80.3 +3.3 -0.9 +0.4 SH | 100.0 +1.2 +0.0 +0.3 | 100.0 -0.7 -0.0 +0.2 | 100.0 -1.1 +0.1 +0.3 |
| 17000 | 100.0 +0.9 +0.2 +0.2 | 83.2 -3.3 -0.4 -0.1 SH | 100.0 -1.3 +0.0 +0.1 | 99.2 +1.7 +0.1 +0.2 H | 100.0 +1.4 +0.2 +0.3 |
| 17250 | 100.0 -0.8 +0.0 -0.0 | 83.4 -3.3 -0.5 +0.1 SH | 91.3 -2.5 -0.2 +0.0 SH | 98.2 -1.8 -0.1 +0.1 H | 100.0 -1.6 -0.2 +0.1 H |
| 17500 | 100.0 +1.3 +0.1 -0.1 | 78.6 -3.3 -1.1 +0.0 SH | 87.7 -2.8 -0.6 +0.1 SH | 97.7 -1.7 -0.6 -0.1 H | 97.0 +1.9 -0.4 +0.2 H |
| 17750 | 100.0 +1.1 +0.0 +0.2 | 97.2 -1.6 -0.8 -0.1 H | 96.6 +1.9 -0.3 +0.0 H | 97.8 +1.5 -0.7 -0.1 | 99.7 +1.0 -0.5 +0.0 |
| 18000 | 98.3 +1.8 +0.1 -0.3 H | 96.2 -1.9 -0.5 +0.1 H | 99.5 +0.6 +0.5 +0.2 | 99.3 +0.4 +0.5 +0.1 | 100.0 -0.7 +0.4 +0.2 |
| 18250 | 99.9 +1.5 +0.5 +0.2 H | 88.3 -2.8 -0.3 -0.0 SH | 94.3 +2.2 +0.1 -0.1 SH | 96.8 +1.9 +0.1 +0.1 H | 99.0 +1.7 -0.1 -0.1 H |

The tonality and gap marks of `v12-412` count against its own spread of 1.17 and 3.06 dB, so no tonality or gap mark shows. The score of 88.6 at 15000 rpm and throttle 0 comes from the gap, 3.0 dB against the floor of 2.2 dB.

### v12-merlin

| RPM | t000 | t025 | t050 | t075 | t100 |
| --- | --- | --- | --- | --- | --- |
| 600 | 78.9 +3.8 -0.0 -0.0 SH | 100.0 -0.3 +0.0 -0.3 | 100.0 -0.2 +0.0 -0.1 | 100.0 +0.2 +0.0 -0.4 | 100.0 +0.3 +0.0 -0.3 |
| 750 | 85.0 +3.0 +0.0 -0.4 SH | 98.8 -0.3 -0.0 -0.1 | 100.0 -0.3 -0.0 +0.0 | 100.0 -0.2 -0.0 +0.0 | 100.0 -0.2 -0.0 +0.0 |
| 1000 | 99.2 +1.6 +0.0 +0.0 H | 100.0 -0.4 +0.0 -0.2 | 100.0 -0.4 +0.0 +0.0 | 100.0 -0.4 +0.0 +0.1 | 100.0 -0.4 +0.0 +0.1 |
| 1250 | 100.0 +0.5 -0.0 -0.3 | 100.0 -0.3 +0.0 -0.3 | 100.0 -0.3 +0.0 -0.2 | 100.0 -0.3 -0.0 -0.2 | 100.0 -0.3 -0.0 -0.2 |
| 1500 | 100.0 -0.5 -0.0 -0.4 | 100.0 -0.6 -0.0 -0.2 G | 100.0 -0.5 -0.0 -0.3 | 100.0 -0.5 -0.0 -0.3 | 100.0 +0.5 -0.0 -0.3 |
| 1750 | 100.0 -0.8 +0.1 -0.3 | 100.0 -0.3 +0.0 -0.0 | 100.0 -0.5 +0.0 -0.1 | 100.0 -0.4 -0.0 -0.1 | 100.0 -0.4 -0.0 -0.0 |
| 2000 | 100.0 +0.3 +0.0 +0.0 | 100.0 +0.7 -0.0 -0.2 | 100.0 +0.4 -0.0 -0.0 | 100.0 -0.3 -0.0 -0.0 | 100.0 -0.4 -0.0 -0.0 |
| 2250 | 100.0 -0.7 -0.0 -0.1 | 100.0 -0.6 +0.0 +0.0 | 100.0 -0.5 +0.1 -0.1 | 100.0 -0.6 +0.1 -0.1 | 100.0 -0.6 +0.1 -0.1 |
| 2500 | 100.0 +0.5 -0.1 -0.4 | 100.0 +0.3 +0.0 -0.2 | 100.0 -0.6 +0.0 +0.1 | 100.0 +0.6 +0.0 +0.1 | 100.0 +0.6 +0.0 +0.0 |
| 2750 | 100.0 +1.0 -0.0 +0.2 | 100.0 -0.6 -0.1 +0.1 | 100.0 +0.5 -0.0 -0.1 | 100.0 +0.6 -0.0 -0.2 | 100.0 +0.6 -0.0 -0.1 |
| 3000 | 100.0 +0.2 +0.0 -0.3 | 100.0 +0.3 +0.0 -0.2 G | 100.0 +0.1 -0.0 -0.2 G | 100.0 +0.2 -0.0 -0.2 G | 100.0 +0.2 -0.0 -0.1 |
| 3250 | 100.0 -1.0 -0.0 -0.1 | 96.6 -1.0 -0.0 -0.5 | 100.0 -0.4 +0.0 -0.3 | 100.0 -0.5 +0.0 -0.3 | 100.0 -0.6 +0.0 -0.4 |

The gap column is the mean signed difference over the three bands. A G mark counts the mean absolute difference, so a cell can show a small signed value with a G.

## 10. Reproduce

```sh
# Write the full-range scenarios and render the engine-sim references and the calibration (about 12 minutes on 16 cores).
just ref-fullrange

# Score snarl against engine-sim, with the self spread of engine-sim and the matrix of the holds.
go run ./tools/scorecard --scenarios fullrange-t000,fullrange-t025,fullrange-t050,fullrange-t075,fullrange-t100,fullrange-mid-t100 \
  --dyno --no-pops --calib out/reference/upstream/calib --matrix --out out/fullrange/snarl > out/fullrange/score.md

# The default scenarios, with the new measures and the sweep and overrun calibration.
go run ./tools/scorecard --dyno --no-pops --calib out/reference/upstream/calib --out out/fullrange/snarl-default

# Suspects: the tables through the exact port, with and without the residual, then scored.
just fullrange-play play
just fullrange-play play-noresidual all --no-residual
go run ./tools/scorecard --scenarios fullrange-t000,fullrange-t025,fullrange-t050,fullrange-t075,fullrange-t100,fullrange-mid-t100 \
  --dyno --no-pops --against out/fullrange/play --calib out/reference/upstream/calib --matrix --out out/fullrange/unused
go run ./tools/scorecard --scenarios fullrange-t000,fullrange-t025,fullrange-t050,fullrange-t075,fullrange-t100,fullrange-mid-t100 \
  --dyno --no-pops --ref out/fullrange/play --against out/fullrange/snarl --matrix --out out/fullrange/unused

# The band detail of one window, for example the sweep.
go run ./cmd/snarl-compare --align --scenario scenarios/upstream/v8-ls/sweep-wot.scn \
  out/reference/upstream/v8-ls/sweep-wot.wav out/fullrange/snarl-default/v8-ls/sweep-wot.wav
```

The full-range scorecard takes about 15 minutes. The outputs of this study are in `out/fullrange/`: `score.md`, `score-default.md`, `score-play.md`, `score-play-noresidual.md`, `score-snarl-vs-play.md`, the snarl renders in `snarl/` and `snarl-default/`, and the playbacks in `play/` and `play-noresidual/`.

## 11. Limits

- The holds on the grid RPM are the points from which the tables were built, and engine-sim renders the same scenario the same way. The holds therefore test the tables at their own points. The mid holds and the holds above the top grid point test playback away from them.
- The calibration is one second render per scenario with another pre-roll. It does not cover other seeds of engine-sim, which has no seed option.
- The miss limit of the tonality and the gap is the largest value of engine-sim against itself for the engine. A single outlier hold of engine-sim sets it, for example the gap of `i4-vtec` at 2.18 dB, where the median is 0.13 dB.
- The scorecard covers dynamometer holds, sweeps and the limiter free run, with the own pops off. It does not cover the harness drive model, the gear changes or the pops.
- The residual bands of `i4-vtec` at 1250 and 1500 rpm come from `snarl-compare --residual` at full throttle only.
- The tonality numbers of this report used a wrong rate: the tonality bands of `hissCycle` were 1 to 2 times lower in frequency than their names, by a factor that depends on the RPM. The fix is in `internal/analysis/hiss.go` (`TestHissTonalityBand`). Scored again on the same renders (`out/fullrange/score-ratefix.md`), the largest tonality distance of engine-sim against itself falls from 0.30, 0.17, 0.30, 1.17 and 0.22 dB to 0.19, 0.11, 0.19, 0.36 and 0.09 dB for `i4-vtec`, `v8-ls`, `f4-ej25`, `v12-412` and `v12-merlin`. The largest of snarl falls from 1.2 dB to 0.7 dB (`i4-vtec` at 9125 rpm). The tonality misses are now `i4-vtec` above 8750 rpm (0.3 to 0.7 dB), `f4-ej25` at 3500 rpm at throttle 0 (0.2 dB), and `v12-412` at 15000 and 17500 rpm (0.4 dB). The other measures did not change.

## 12. Fixes of the misses

Measured on `pop-model` after `d7065aa`, with the tonality rate fix of section 11 in both the base and the fixed scorecards, so that the two compare on the same analysis. The base is `out/fix/base2/score.md`, the result `out/fix/f2/score.md`, and `tools/cycles scorediff` lists the holds that pass in the base and lose more than 1 point.

### 12.1 Answer

Three changes to the tables fix most of the misses. Of the 919 full-range holds, 867 pass after the fixes (a score of 95 or more and the worst band above 2 kHz within 1.5 dB), against 814 before:

1. **Table range.** The grid now goes on to the last multiple of 250 rpm below 0.998 of the limiter. `i4-vtec` at 9000 and 9250 rpm goes from 77.3 to 91.0 up to 100 at every throttle.
2. **Aliased harmonics.** The cause of the `v12-412` mid-hold misses is not the blend. The engine-sim physics makes harmonics above the Nyquist frequency of its 5 kHz steps, and the steps fold them to tones that the tables held as noise. The tables now hold these harmonics. The mid holds at 15375, 15625, 17625 and 18125 rpm go from 57.5 to 94.0 up to 100, and the hold at 17875 rpm from 58.1 to 94.2.
3. **Modulation of the cycle.** Most of the residual of `i4-vtec`, `f4-ej25` and `v8-ls` is a random gain of the average cycle, which follows the harmonics, not flat band noise. The tables now hold that gain. `i4-vtec` at 1250 to 1500 rpm goes from 86.6 to 92.1 up to 97.6 to 100, and `f4-ej25` at 1000 to 1250 rpm goes from 90.8 to 93.8 up to 98.9 to 100.

A 125 rpm grid below 2000 rpm fixes the low-RPM mid holds, but it makes the `i4-vtec` 1000 rpm holds worse (section 12.5), so the grid recipe does not use it. The fixes cost 1.15 % of the render on the geometric mean of the benchmarks, at most 1.9 % on one profile (`f4-ej25`), and the tables grow by 319 kB (6 %).

| Engine | t000 | t025 | t050 | t075 | t100 | mid-t100 |
| --- | --- | --- | --- | --- | --- | --- |
| `i4-vtec` | 98.5 to 98.8 | 98.6 to 99.8 | 97.4 to 99.8 | 97.6 to 99.1 | 98.4 to 99.8 | 97.7 to 99.4 |
| `v8-ls` | 100.0 | 100.0 | 100.0 | 99.9 | 99.9 | 97.7 |
| `f4-ej25` | 98.0 to 99.7 | 99.2 to 99.8 | 99.0 to 99.6 | 99.4 to 99.9 | 98.9 to 99.4 | 97.0 to 97.6 |
| `v12-412` | 95.3 | 98.0 to 99.8 | 98.8 to 99.8 | 99.8 to 100.0 | 99.7 to 100.0 | 97.1 to 99.6 |
| `v12-merlin` | 93.5 to 93.8 | 99.7 to 99.8 | 100.0 | 100.0 | 100.0 | 94.8 to 94.9 |

The default scenarios (`out/fix/base-default/score.md` to `out/fix/f2-default/score.md`): `v12-412` holds-t050 98.1 to 99.6, holds-t100 98.1 to 99.5 and sweep-wot 98.5 to 99.6; `i4-vtec` holds-t100 99.4 to 99.7 and overrun 98.7 to 99.1; `f4-ej25` holds-t000 98.8 to 99.2; `v12-merlin` holds-t000 92.7 to 93.5 and limiter-free 75.0 to 78.2. The losses are 0.1 to 0.6 points: `v8-ls` limiter-free 98.4 to 98.2, `f4-ej25` sweep-wot 99.4 to 99.3 and idle-free 42.6 to 42.0, `v12-412` holds-t000 94.6 to 94.5, `i4-vtec` and `v12-merlin` idle-free-rpm 0.1 and 0.3 points.

### 12.2 Table range and aliased harmonics

`tools/cycles scenarios --limiter` takes the grid on to the last multiple of the step below 0.998 of the limiter RPM, the top of the full-range scenarios, when that is above the last hold of `holds-t100.scn`. `just cycles-grid` reads `limiter_rpm` from the profile. The new tops are 9250 rpm (`i4-vtec`), 6750 rpm (`v8-ls`), 18250 rpm (`v12-412`) and 3250 rpm (`v12-merlin`). `f4-ej25` keeps 6400 rpm, above 6250 rpm. The throttle step stays at 60 % of the last hold, so the intake lag does not change. engine-sim renders the grid the same way each time: the `f4-ej25` tables of the new render are the same bytes as before.

The `v12-412` holds halfway between grid points missed in one third-octave band, 1995 Hz at 15125 to 15625 rpm and 2512 Hz at 17875 rpm, by -4.3 to -6.8 dB. All other bands were within 0.8 dB. Tables built from engine-sim holds at 15125, 15375 and 15625 rpm themselves (`out/fix/v12-fine`) missed by the same -4.7 to -6.9 dB at those RPM, so the blend is not the cause. `tools/cycles blend` confirms that the linear blend of neighbouring points loses no level in any band above 78 Hz on `v12-412` (-0.0 to -0.5 dB).

The cause is aliasing in engine-sim. Harmonic 24, twice the firing rate, is near 3 kHz at these RPM, above the 2.5 kHz Nyquist frequency of the 5 kHz steps. The steps fold it to 5000 Hz less 24 times the cycle rate, for example 1948 Hz at 15250 rpm and 1922 Hz at 15375 rpm. `tools/cycles spectrum` shows the tone 44 to 46 dB below the input power, all of it in the residual, where the band noise of the model was 15 to 21 dB lower near it. At the output, `tools/cycles wavspectrum` shows engine-sim 12 dB above the playback at 1920 to 1940 Hz at 15375 rpm. At 15250 rpm the output tone is 16 dB weaker than at 15375 rpm, although the input tones are equal, and that hold passed.

`aliasHarmonics` (`internal/cycles/measure.go`) measures these harmonics. Over many cycles, the steps fall at other phases of the cycle, so a least-squares fit to the residual of all samples separates up to 64 harmonics above the Nyquist frequency. A harmonic stays when its power is 10 times what the residual noise near its folded frequency gives the fit, 80 dB or less below the series, and its folded frequency is below 3.8 kHz, twice the 1900 Hz of the fourth-order input low pass of the synthesiser. Playback reads the series at the steps, so the harmonics fold to the same tones, and between grid points the tones move with the RPM as in engine-sim. The 20 kHz engines keep none, as their harmonics fold above 3.8 kHz. `TestMeasureAliases` checks that a harmonic above the Nyquist frequency comes back with its magnitude and phase.

With the aliased harmonics, tables built from the holds at 15000 to 15750 rpm play every hold within 0.9 dB and score 100. With every second point left out, the holds at 15375 and 15625 rpm also score 100. The hold at 15125 rpm scores 77.3 there, because at 15000 rpm a cycle has exactly 40 steps. The folded harmonics then fall on harmonics, the fit cannot separate them, and halfway to 15250 rpm half of the tone is at the wrong frequency (-3.0 dB).

### 12.3 Modulation of the cycle

The residual model of section 7 matched the input residual of engine-sim in power per band (`tools/cycles spectrum`, within 0.2 dB on `i4-vtec` at 1500 rpm), but not in shape within a band. The real residual follows the harmonic envelope. On `i4-vtec` at 1500 rpm and full throttle, the residual dips with the harmonics at 275 and 325 Hz, and the flat band noise is 5 to 19 dB too high from 220 to 360 Hz. At the output, that gave 3.6 to 4.5 dB too much residual.

A random gain of the average cycle in 8 equal parts of each cycle explains 53 to 88 % of the residual power at the grid holds of `i4-vtec`, `f4-ej25` and `v8-ls` that were measured (`tools/cycles spectrum`, the modulation table, 9 holds from 1000 to 6000 rpm). It explains 24 % at the closed-throttle idle of `v12-merlin` and 33 % on `v12-412` at idle. `slotModulation` measures the gain of each part as a least-squares fit, shrunk where the cycle is quiet. It also measures the correlation of neighbouring parts and the RMS gain that, with independent values per part and the same interpolation, gives the explained part its power. The band envelopes hold the rest, so the residual power stays the same. `Live` and `Player` play the gain as one Gaussian value per part, with the measured correlation to the part before, interpolated between the part centres, times the average cycle without its mean.

The gain applies only where each part holds 16 steps or more, so that the fit takes less than a sixteenth of the residual noise, and where the RMS gain is 0.1 or less. A larger gain is not a small change of one cycle shape: the misfires of `v12-412` at idle (0.24 to 0.33) and the closing slices are not. With the gain on `v12-412` at idle, the hold at 3810 rpm fell from 97.7 to 76.2, with 4.1 dB too little level above 2 kHz. A first estimator, a least-squares fit at the part centres with a Wiener shrink, gave gains 2 times larger and made `f4-ej25` at 830 to 1000 rpm worse (90.4 to 93.1). The estimator in the tables takes the mean gain of each part. `TestMeasureModulation` checks that a modulated cycle comes back with its power within -2.5 to 0.5 dB, with the band noise 6 dB or more below. `TestLiveModulation` checks the RMS gain of the playback.

### 12.4 Cost

The benchmarks ran on this PC with `-test.cpu 1`, 6 runs each, the old and the new tables in turn, on the same code (`out/fix/bench5-old.txt`, `out/fix/bench5-new4.txt`, benchstat):

| Benchmark | Old tables (% real time) | New tables (% real time) | Change |
| --- | --- | --- | --- |
| Render `i4-vtec` | 3.090 | 3.136 | not significant (p = 0.29) |
| Render `v8-ls` | 2.409 | 2.405 | not significant |
| Render `f4-ej25` | 3.085 | 3.143 | +1.90 % |
| Render `v12-412` | 1.736 | 1.738 | not significant |
| Render `v12-merlin` | 1.917 | 1.916 | not significant |

Over all 25 benchmarks of `BenchmarkRender` and `BenchmarkStates`, the geometric mean rises by 1.15 %. The largest rise is +6.82 % in the limiter state of `i4-vtec`. The old tables play on the new code without a modulation, as on the code before.

The aliased harmonics do not change the cost of the wave table reads: the render of `v12-412` and `v12-merlin`, which keep them, does not change. The modulation adds per step and channel three multiply-adds per weight in the pass that blends the wave tables, and one Gaussian value per part. A first form, with the blend in its own pass and with aliased harmonics on the 20 kHz engines, cost 2.3 % on the geometric mean of the render benchmarks and 4.6 % on `i4-vtec`.

The tables grow from 5 103 840 to 5 423 121 bytes (+319 281, +6.3 %): 1 512 145 (`i4-vtec`, 273 points), 883 052 (`v8-ls`, 190), 1 164 878 (`f4-ej25`, 185), 1 398 987 (`v12-412`, 469) and 464 059 (`v12-merlin`, 93). The range gives 143 587 bytes. The aliased harmonics and the modulation values in the JSON header give the rest.

### 12.5 The low-RPM blend

The blend between neighbouring points loses level where the cycle changes most from point to point. `tools/cycles blend` gives -2 to -9 dB in some bands between the grid points below 2000 rpm, for example -9.1 dB from 156 to 312 Hz between 1500 and 1750 rpm on `f4-ej25`. A grid with every multiple of 125 rpm below 2000 rpm (`tools/cycles scenarios --fine-below 2000 --fine-step 125`, new engine-sim renders) measured:

- The first-gap mid holds go from 86.1 (`v8-ls` 845 rpm) to 97.3 or more and from 86.8 (`f4-ej25` 915 rpm) to 92.4. `v12-merlin` 875 rpm becomes a grid point, and 675 rpm stays at 85.6.
- New engine-sim holds halfway between the 125 rpm points, at 812.5 to 1937.5 rpm (`out/fix/valid`), score 97.8 on average on `v8-ls` with the 250 rpm grid and 99.4 with the 125 rpm grid, and 97.5 and 98.7 on `f4-ej25`.
- The `i4-vtec` holds at 1000 rpm fall from 96.1 to 98.7 to 87.4 to 90.0. The grid reaches 1000 rpm from 875 rpm, while the full-range scenario reaches it from 770 rpm, and the cycle error of the hold grows from 40.5 to 32.7 dB below the signal. engine-sim does not settle to one state at 1000 rpm within the 0.5 s of settling. With the 250 rpm grid the scenario and the grid have the same history, so the hold repeats the grid point.
- The tables grow by a further 0.84 MB.

A hold that passes must not get worse, so the recipe keeps the 250 rpm step.

### 12.6 What still misses

The 52 holds below the target, and the reason for each:

| Engine | Where | Size | Reason |
| --- | --- | --- | --- |
| `v12-412` | throttle 0 at 3810 to 13750 rpm and 17500 to 18250 rpm, throttle 0.25 and 0.5 at 16250 to 17500 rpm | throttle 0: worst bands of 1.6 to 4.3 dB (too high below 7000 rpm, too low from 8500 to 13750 rpm), scores 75.4 to 100; throttle 0.25 and 0.5: 1.9 to 2.3 dB, scores 92.9 to 100 | engine-sim misfires at random and differs from itself by up to 2.88 dB (section 3). The misfires are not a modulation of one cycle (section 12.3), and the band noise does not hold their shape above 2 kHz. |
| `v12-412` | mid hold at 15125 rpm | 85.1, -3.1 dB at 1995 Hz | at 15000 rpm a cycle has exactly 40 steps, so the folded harmonics cannot be separated from the harmonics (section 12.2). |
| `v12-merlin` | throttle 0 at 600 to 750 rpm | 81.0 and 84.8, +3.6 and +3.0 dB | the residual has a notch from 1.8 to 2.4 kHz inside the top band, 1750 to 3500 Hz, where the model is 5 to 7.5 dB too high. A tenth band to split the top band costs about 4.5 % of the render: the band noise is about 41 % of the render on `i4-vtec` (CPU profile), and a tenth band adds a ninth to it. |
| `v8-ls`, `f4-ej25`, `v12-merlin` | mid holds at 845, 915, 675 and 875 rpm | 85.2 to 86.8 | the blend between idle and the next point (section 12.5). |
| `i4-vtec` | mid hold at 5875 rpm | 85.5, -2.8 dB at 1995 Hz | the cycle changes most between 5750 and 6000 rpm: the blend loses 2.6 to 3.3 dB in every band from 156 Hz to 10 kHz there (`tools/cycles blend`), likely the VTEC switch of the cam profile. A grid point at 5875 rpm would hold it. |
| `i4-vtec` | idle, 770 rpm, throttle 0 and 0.75 | 90.4 and 93.9, from the envelope | the idle holds of `docs/research/cycle-tables.md` section 6.3: few cycles per window and the largest change of shape per step. These did not change. |
| `f4-ej25` | throttle 0 at 2750 to 3000 rpm, single holds at 2000, 3250 and 3625 rpm | bands 1.6 to 2.0 dB | these did not change, and they are within 0.5 dB of the 1.5 dB limit. |

The holds that passed and lose more than 1 point: `i4-vtec` at 1000 rpm (throttle 0.25 100.0 to 97.7, throttle 0.75 97.2 to 96.1, throttle 1 100.0 to 98.7) and the mid hold at 1125 rpm (98.8 to 97.5), where the modulation leaves 2.2 dB less output residual than engine-sim. Also `i4-vtec` at 8750 rpm, throttle 0.5 (100.0 to 98.9), and `v12-412` at throttle 0 at 9250 and 13750 rpm (100.0 to 98.2 and 95.3), within the 2.88 dB that engine-sim differs from itself there. `f4-ej25` at 2000 rpm and `v12-412` at 17500 and 18250 rpm pass the score but move 0.1 to 0.3 dB past the 1.5 dB band limit. All of them still score 95.3 or more.

### 12.7 Tools and reproduce

The new commands of `tools/cycles`: `blend` (the level of the blend halfway between neighbouring points per residual band), `wavspectrum` (the spectrum of one hold in several renders), `scorediff` (the holds that pass in one full-range scorecard and lose in another), and the band, model and modulation tables of `spectrum`. `build --no-aliases` and `build --no-modulation` build the tables of the earlier model.

```sh
# Render the grids to below the limiter and build the tables (about 6 minutes per engine).
just cycles-grid
just import-reference

# The full-range and default scorecards.
go run ./tools/scorecard --scenarios fullrange-t000,fullrange-t025,fullrange-t050,fullrange-t075,fullrange-t100,fullrange-mid-t100 \
  --dyno --no-pops --calib out/reference/upstream/calib --matrix --out out/fix/f2/snarl > out/fix/f2/score.md
go run ./tools/scorecard --dyno --no-pops --calib out/reference/upstream/calib --out out/fix/f2-default/snarl > out/fix/f2-default/score.md
go run ./tools/cycles scorediff out/fix/base2/score.md out/fix/f2/score.md

# The evidence: aliased tones and the residual against its model at one hold, the blend per band.
go run ./tools/cycles spectrum --record out/cycles/grid/v12-412/t100 --rpm 15250 --groups 16 --frame 256
go run ./tools/cycles spectrum --record out/cycles/grid/i4-vtec/t100 --rpm 1500 --groups 1000 --frame 16384
go run ./tools/cycles blend --tables profiles/cycles/f4-ej25.bin
```

## 13. The lift-off

The drive scorecard (`docs/research/drive-scorecard.md`) found 4 to 10 dB too much level from 2 to 8 kHz in the first 0.25 s after a lift from full throttle. It was largest on `v12-merlin` and `f4-ej25`, and it blamed the closing transient and the intake lag. Measured on the tables of section 12.

### 13.1 Answer

The cause is in the measure of the closing slices, not in the intake lag. The first slice of each closing step holds only 2 or 3 cycles of a cycle that changes fast. The average of those cycles holds each phase at another time, so the average cycle jumps at its end. Played over and over, the jump clicks, with harmonics up to the Nyquist frequency. Each slice now holds the cycle at its middle. In the drive, the lift windows of `v12-merlin` go from +10.7, +13.3 and +10.9 dB to -6.0, -4.1 and -2.5 dB, and those of `f4-ej25` from +5.7, +2.6 and +8.7 dB to +3.1, within 1.6 and +3.4 dB, inside its self spread of 5.3 dB. The full-range and default scorecards do not change, the intake lag stays as measured, and the tables shrink to 5 395 744 bytes (-27 377 bytes). The runtime does not change.

### 13.2 Evidence

New engine-sim renders of a lift at a held RPM and a lift at the falling RPM of the drive lifts, per engine (`out/fix4/lift`: 3250 rpm on `v12-merlin`, 5750 rpm on `f4-ej25` and `v8-ls`, 7750 rpm on `i4-vtec` and 15750 rpm on `v12-412`, at a closing step RPM of each grid), show the excess at the held RPM too: +11.3 dB on `v12-merlin` and +6.9 dB on `f4-ej25`. The slices come from closing steps at these RPM, so the playback of the slices is at fault, not the falling RPM.

On `v12-merlin` at 3250 rpm, the first slice runs from 0.04 to 0.15 s after the close and holds 2 cycles. With the average of the cycles, `tools/cycles spectrum --length 0.11` gives a residual 30 to 40 dB above the input from 700 Hz to 3.5 kHz: the average cycle holds content that the input has not. With the cycle at the middle of the slice, the residual is 0.3 to 5.5 dB below the input. In the output, engine-sim has 5 dB more from 0.7 to 2.8 kHz in the first 0.25 s after the lift, relative to the window level. snarl had 16 to 23 dB more (`snarl-compare --describe` on quarter-second holds).

Three other suspects, each tested on the lift renders and the drive:

- **Without the intake lag while the slices play**, the slices taking the whole closed-throttle share at once: worse, +13.0 to +14.3 dB on `v12-merlin` and +11.3 to +21.7 dB on `f4-ej25`. The lag stays.
- **Without the band noise of the first slices**: no change on `v12-merlin` (+13.3 dB). The excess is in the average cycle, not in the noise.
- **A longer intake lag on `f4-ej25`**: `tools/cycles build --step` on the closing steps of `close.scn` fits the closing lag at each RPM of the steps. On `f4-ej25` it is 0.050 s at 4750, 5750 and 6400 rpm and 0.057 s at 3750 rpm, against 0.050 s in the profile, from the throttle step of `step.scn` at 3750 rpm. The 0.2 s that the drive scorecard tried has no support in engine-sim, and with the slices fixed, the `f4-ej25` lifts pass with 0.05 s. The lag grows at low RPM: 0.19 s at 830 rpm on `f4-ej25` and 0.17 s at 600 rpm on `v12-merlin` (`out/fix4/lag-*.md`). A lag by RPM would follow that, but no scored window needs it.

### 13.3 The fix

`syncSnapshot` (`internal/cycles/measure.go`) measures a slice with `MeasureOptions.Drift`, which `CloseSlices` sets. It averages the cycles and fits their least-squares slope over the cycle index. Then it takes each phase of the average back to the middle of its cycle with the slope, which is the cycle at the middle of the slice. The playback blends the slices by the time since the close, as before, so the change from slice to slice comes from the blend, not from a jump within a cycle. The band filters of the slice residual start at the slice, settled on its first residual, with no warm-up on the input before it, which is another state. A slice has no aliased harmonics and no modulation. `TestMeasureDrift` checks that a cycle that grows by 7 % per cycle comes back as the cycle at the middle with an error 40 dB or more below it, and 15 dB or more below the error of the average. `tools/cycles build --no-drift` builds the slices as before.

### 13.4 Results

The drive (`out/fix4/base/score.md` to `out/fix4/snap/score.md`), the worst band above 2 kHz of the lift windows at 6.0, 14.5 and 24.0 s:

| Engine | Before (dB) | After (dB) | engine-sim self spread over the drive (dB) |
| --- | --- | --- | --- |
| `i4-vtec` | -3.3, +4.3, +4.3 | +2.9, +4.4, +4.2 | 4.3 |
| `v8-ls` | +3.8, +4.6, +4.1 | +3.8, +4.6, +3.9 | 5.8 |
| `f4-ej25` | +5.7, +2.6, +8.7 | +3.1, pass, +3.4 | 5.3 |
| `v12-412` | -2.3, -3.7, +3.4 | -2.8, -3.8, +3.4 | 4.1 |
| `v12-merlin` | +10.7, +13.3, +10.9 | -6.0, -4.1, -2.5 | 4.6 |

The windows of the drive beyond the self spread fall from 24 to 20. `v12-merlin` at 6.0 s (-6.0 dB) is now beyond the spread on the low side. The `i4-vtec` and `v8-ls` lifts stay at the edge of the spread, as before. The full-range scorecard has the same 867 passing holds of 919, and the default scenarios do not change (`out/fix4/full/score.md`, `out/fix4/default/score.md`).

## 14. The 3 s hold of `v12-412` at 12500 rpm

`just listen-ab` found one miss in the ten 3 s holds: `v12-412/hold-70`, full throttle at 12500 rpm, score 64.6 and +3.7 dB in the 1995 Hz band. Measured on `pop-model` at `0cf7ab4`, with the tables of section 13.

### 14.1 Answer

The miss is inside the variation of engine-sim against itself. Nothing in snarl changes. At 12500 rpm a cycle of `v12-412` has exactly 48 of the 5 kHz steps, so the steps fold harmonic 30 (3125 Hz) onto harmonic 18 at 1875 Hz. The two add with the phase of the crank against the steps, and the history of the render sets that phase. The same 3 s hold, reached on other paths, has a tone at 1800 to 1900 Hz from -38.1 to -31.0 dB of the hold power in engine-sim. snarl plays -31.0 dB, the phase of the grid render. Two engine-sim renders of the same scenario on different paths score 66.9 against each other with +5.4 dB at 1995 Hz, more than snarl misses by.

### 14.2 Evidence

The tone, power from 1800 to 1900 Hz relative to the power of the 12500 rpm hold (`tools/cycles wavspectrum`). The 1 Hz groups put all of it at 1874 to 1876 Hz.

| Render | Path to 12500 rpm | Tone (dB) | snarl score of the hold |
| --- | --- | --- | --- |
| engine-sim `listen-hold-70`, 3 s and 3.5 s pre-roll | the start of the render | -35.8, -35.8 | 64.6 |
| engine-sim `listen-hold-70-up` | from a hold at 12250 rpm | -38.1 | 68.9 |
| engine-sim `listen-hold-70-down` | from a hold at 12750 rpm | -31.0 | 100.0 |
| engine-sim `fullrange-t100` | full-range holds from idle | -31.4 | 100.0 |
| engine-sim grid `t100`, the source of the tables | grid holds from idle | -31.4 | |
| snarl and the tables through the exact port | any | -31.0 | |

1. **engine-sim against itself.** The calibration pair, the same scenario at a 3.5 s pre-roll, scores 100 against the 3 s pre-roll and has the same tone to 0.1 dB. The pre-roll only starts the same simulation earlier, so the phase does not change. That pair does not test this variation. `listen-hold-70-up` and `listen-hold-70-down` have the same timeline, so `tools/scorecard --against` scores the second against the first: 66.9 at the 12500 rpm hold, +3.0 dB mean and +5.4 dB most in the 1995 Hz band. snarl against the first scores 68.9 and +2.3 dB. engine-sim renders at exactly 12500 rpm on the dynamometer, so the phase holds over the whole hold.
2. **The other suspects.** The length: the tone of engine-sim is -35.8, -36.0 and -36.1 dB from 0.5, 1.5 and 2.5 s to the end of the hold, and the playback -31.0, -30.9 and -30.8 dB. No slow change. The aliased harmonics: tables built with `tools/cycles build --no-aliases` play the same -31.0 dB, as the fit has no separate harmonic 30 to find when it falls on harmonic 18. The residual model: `tools/cycles play --no-residual` plays -30.8 dB, so the tone is in the average cycle. The misfires: the other bands above 2 kHz are within 0.4 dB, and the tonality and gap distances are 0.0 and 0.2 dB.
3. **The neighbouring grid points.** The 3 s holds at 12250 and 12750 rpm score 100, and so do their calibration renders. There, a cycle has 48.98 and 47.06 steps, so the folded harmonic 30 is 100 Hz from harmonic 18 (folded to 1937.5 and 1812.5 Hz, against harmonic 18 at 1837.5 and 1912.5 Hz) and adds no phase dependence.

The same fold occurs at every RPM where a cycle has a whole number of steps: 15000 rpm (40 steps, section 12.2), 10000 rpm (60) and 7500 rpm (80). There the tables hold one phase of engine-sim, and engine-sim on another path can differ by several dB in one band. Do not fix the tables there: no phase is more correct than another.

### 14.3 Reproduce

`just ref-listen` now renders each `listen-hold-*.scn` file of an engine, with any segments, at the 3 s pre-roll and the 3.5 s pre-roll of the calibration. The new scenarios are `listen-hold-70-up`, `listen-hold-70-down`, `listen-hold-12250` and `listen-hold-12750` in `scenarios/upstream/v12-412/`. The outputs are in `out/fix5/`.

```sh
just ref-listen v12-412
go run ./tools/scorecard --engines v12-412 --scenarios listen-hold-70,listen-hold-70-up,listen-hold-70-down,listen-hold-12250,listen-hold-12750 \
  --dyno --no-pops --calib out/reference/upstream/calib --windows --summary out/fix5/listen --out out/fix5/listen > out/fix5/listen.md

# engine-sim against itself: the render from 12750 rpm in the place of the render from 12250 rpm.
mkdir -p out/fix5/es-down/v12-412
ln -sf "${PWD}/out/reference/upstream/v12-412/listen-hold-70-down.wav" out/fix5/es-down/v12-412/listen-hold-70-up.wav
go run ./tools/scorecard --engines v12-412 --scenarios listen-hold-70-up --dyno --no-pops --against out/fix5/es-down \
  --windows --summary out/fix5/es-self --out out/fix5/es-self > out/fix5/es-self.md

go run ./tools/cycles wavspectrum --scenario scenarios/upstream/v12-412/listen-hold-70-down.scn --rpm 12500 \
  --from 1800 --to 1900 --groups 1 out/reference/upstream/v12-412/listen-hold-70-down.wav
```
