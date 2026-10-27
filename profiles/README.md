# Engine profiles

This directory holds the five engine profiles of the library, converted faithfully from their upstream engine-sim engines. Each value of a profile has one recorded source: a copy from the upstream engine, a measurement from the own upstream reference renders of that engine, or a snarl default for a feature that upstream does not have. No value is set by ear. The library embeds the five profiles (`snarl.Profiles`), the cycle tables in `cycles/` and the impulse responses in `ir/` that they name, but not the sources files.

The test catalogue in `profiles/catalogue/` is in the format before the cycle tables. It has no tables, so snarl cannot load it: `snarl-render` and `snarl-harness` skip its profiles with a note (`snarl.ErrNoTables`).

`just import-reference` writes the five profiles, their sources files and this README, and copies the impulse responses that the cycle tables name from the pinned engine-sim package into `ir/`. It reads the cycle tables of `just cycles-grid` and the reference renders of `just ref-upstream`. Do not edit the files by hand. Change `tools/import` and run the recipe again. Two runs give the same files. To use other reference renders for an engine, give its directory: `just import-reference v12-412=DIR`.

## Engines

| Id | Upstream file | Commit | References | Table points | Channels | Impulse responses |
| --- | --- | --- | --- | --- | --- | --- |
| `i4-vtec` | `atg-video-1/05_honda_vtec.mr` | `85f7c3b` | `out/reference/upstream/i4-vtec` | 273 | 2 | `sound-library/new/mild_exhaust.wav`, `sound-library/new/mild_exhaust.wav` |
| `v8-ls` | `atg-video-2/07_gm_ls.mr` | `85f7c3b` | `out/reference/upstream/v8-ls` | 190 | 2 | `sound-library/smooth/smooth_39.wav`, `sound-library/smooth/smooth_39.wav` |
| `f4-ej25` | `atg-video-1/06_subaru_ej25.mr` | `85f7c3b` | `out/reference/upstream/f4-ej25` | 185 | 2 | `sound-library/new/mild_exhaust.wav`, `sound-library/new/mild_exhaust.wav` |
| `v12-412` | `atg-video-2/12_ferrari_412_t2.mr` | `85f7c3b` | `out/reference/upstream/v12-412` | 469 | 2 | `sound-library/new/minimal_muffling_01.wav`, `sound-library/new/minimal_muffling_01.wav` |
| `v12-merlin` | `atg-video-2/11_merlin_v12.mr` | `85f7c3b` | `out/reference/upstream/v12-merlin` | 93 | 2 | `sound-library/new/minimal_muffling_01.wav`, `sound-library/new/minimal_muffling_01.wav` |

The files are relative to `assets/engines/` of engine-sim at `85f7c3b`, the package in `reference/engine-sim`, rendered unchanged by `just ref-upstream` and `just cycles-grid`. The impulse responses are relative to its `es/` directory. `docs/research/reference-health.md` gives the health of the references.

## Sources

`<id>.sources.toml` beside each profile has one `[[source]]` entry for each key that the profile sets. For a key of an array table, such as `channel.impulse_response`, one entry covers every entry of the array. The sources are a separate file because the profile parser rejects unknown tables, and the profile stays the plain engine input. `reference_test.go` fails when a key has no entry, or when an entry names a key that the profile does not set.

| Kind | Meaning |
| --- | --- |
| `upstream` | Copied from the upstream engine: a field of the engine-sim dump, or a file of the engine-sim package. |
| `measured` | Measured from the own upstream reference renders of the engine, with the command and the input in the entry. |
| `snarl` | A snarl default for a feature that upstream does not have: the id, the seed, the volume, the pops and the final gear hold. |
| `not-measured` | The references cannot give the value, so the profile has the core default (`core.DefaultSpec`), or for a key with no usable default the rule in the entry. |

## Conversion rules

- Cycle tables: `tables` names `cycles/<id>.bin`, the synthesiser input of each exhaust channel over one engine cycle on a grid of RPM and throttle holds, with the ignition on and off, and the slices of the closing transient after a full-throttle close, measured by `tools/cycles build` from engine-sim renders of the unchanged script (`docs/research/cycle-tables.md`).
- Impulse responses: each `[[channel]]` names the impulse response of its exhaust system, as the tables name it, copied unchanged into `ir/`.
- Intake lag: the time constants of the tables, fitted by `tools/cycles build --step` to the throttle steps of `grid/step.scn`.
- Limiter: engine-sim cuts the ignition. The cut length, the fall rate and the climb rate are the medians of the bounces of the free run at full throttle (`limiter-free`), with the RPM averaged over one engine cycle. A cut plays the ignition-off tables.
- Idle wander: the RPM spread and the main wander frequency of the free run at throttle 0 (`idle-free`), over the window of `tools/fit --health`. The spread is that of the RPM averaged over one engine cycle, so that the change of the crank speed within a cycle does not count. The hunt depth is that spread over the mean RPM, over the spread of the idle wander of the engine per unit of hunt.
- Volume: 0.25, half the `--volume` of the reference renders, so that the pops have 6 dB of room above the engine. The engine-sim leveller sets the level before it.
- Pops: the core defaults, with a density of 0.15, so that the own pops play, and no limiter bangs, because the engine-sim cut has no afterfire.

## Limits of the references

`docs/research/reference-health.md` gives these faults. The converter does not correct them, so the tables and the profiles have them too.

- `v12-412` is not healthy below 11000 rpm at its own 5000 Hz simulation rate: weak firings and misfires from `burning_efficiency_randomness: 1.0`, and a runner peak 17 per cent higher at 10000 Hz. Its tables and its idle wander include these holds.
- `v12-merlin` does not start from its starter at speed control 0. Its references start from a 600 rpm hold, its `idle_rpm` is the last second of the free run, and its idle wander includes the climb from 600 rpm to that idle. Its grid starts at 600 rpm.
- For `v8-ls` and `v12-merlin`, the main idle wander is at 0.22 Hz, the lowest frequency of the spectrum. It is a slow drift, not a periodic hunt.
- The EngineLab engine `f6-aircooled` is not in the set: EngineLab writes no synthesiser input, so it has no cycle tables.

## Per engine

### i4-vtec

Measured from `out/reference/upstream/i4-vtec` and its grid renders:

| Key | Value | Source |
| --- | --- | --- |
| `tables` | "i4-vtec" | profiles/cycles/i4-vtec.bin of just cycles-grid i4-vtec: tools/cycles build of the engine-sim renders of the grid scenarios in scenarios/upstream/i4-vtec/grid at the simulation rate of the unchanged script, from idle to below the limiter, in out/cycles/grid/i4-vtec: 273 points, of which 78 are slices of the closing steps of close.scn, with the harmonics above the Nyquist frequency of the steps and the modulation of the average cycle in 8 parts per cycle (docs/research/cycle-tables.md, docs/research/fullrange-scorecard.md section 12) |
| `throttle.close_lag_s` | 0.0524 | the intake lag of profiles/cycles/i4-vtec.bin, 0.052 s: tools/cycles build --step out/cycles/grid/i4-vtec/step fits exp(-t/tau) to the share of the state before the step in each cycle after the full to closed throttle step of scenarios/upstream/i4-vtec/grid/step.scn at 5000 rpm (out/cycles/grid/i4-vtec/build.md) |
| `throttle.open_lag_s` | 0.0371 | the intake lag of profiles/cycles/i4-vtec.bin, 0.037 s: tools/cycles build --step out/cycles/grid/i4-vtec/step fits exp(-t/tau) to the share of the state before the step in each cycle after the closed to full throttle step of scenarios/upstream/i4-vtec/grid/step.scn at 5000 rpm (out/cycles/grid/i4-vtec/build.md) |
| `limiter.cut_ms` | 55.8 | the median cut, from a peak of the RPM to the next trough where the lines through the climbs and the falls meet, of the 38 bounces of the free run at full throttle of out/reference/upstream/i4-vtec/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce): 55.8 ms (the dump limiter_duration_s is 0.05 s) |
| `limiter.fall_rpm_s` | 8509 | the median fall rate in the cuts, the slope of a straight line through the RPM from half a cycle after a peak to half a cycle before the trough, of the 38 bounces of the free run at full throttle of out/reference/upstream/i4-vtec/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce), with median peaks and troughs of 9381 and 8898 rpm |
| `limiter.rise_rpm_s` | 11609 | the median climb rate, the slope of a straight line through the RPM from half a cycle after a trough to half a cycle before the next peak, of the 38 bounces of the free run at full throttle of out/reference/upstream/i4-vtec/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce) |
| `idle.hunt` | 0.032 | the spread of the RPM averaged over one engine cycle over the mean RPM of the free run at throttle 0 of out/reference/upstream/i4-vtec/idle-free.csv from 2 to 11 s, 17.3 over 767.1 rpm (23.7 rpm with no averaging), over 0.7036, the spread of the idle wander of the engine per unit of hunt |
| `idle.hunt_hz` | 0.59 | the main wander frequency of the free run at throttle 0 of out/reference/upstream/i4-vtec/idle-free.csv from 2 to 11 s, 0.586 Hz at 15.5 rpm |

Not measured:

| Key | Value | Reason |
| --- | --- | --- |
| `idle.fade_rpm` | 2170 | the free runs give the wander at idle only: idle_rpm plus 1400 rpm, the tools/import rule |

### v8-ls

Measured from `out/reference/upstream/v8-ls` and its grid renders:

| Key | Value | Source |
| --- | --- | --- |
| `tables` | "v8-ls" | profiles/cycles/v8-ls.bin of just cycles-grid v8-ls: tools/cycles build of the engine-sim renders of the grid scenarios in scenarios/upstream/v8-ls/grid at the simulation rate of the unchanged script, from idle to below the limiter, in out/cycles/grid/v8-ls: 190 points, of which 51 are slices of the closing steps of close.scn, with the harmonics above the Nyquist frequency of the steps and the modulation of the average cycle in 8 parts per cycle (docs/research/cycle-tables.md, docs/research/fullrange-scorecard.md section 12) |
| `throttle.close_lag_s` | 0.0682 | the intake lag of profiles/cycles/v8-ls.bin, 0.068 s: tools/cycles build --step out/cycles/grid/v8-ls/step fits exp(-t/tau) to the share of the state before the step in each cycle after the full to closed throttle step of scenarios/upstream/v8-ls/grid/step.scn at 4000 rpm (out/cycles/grid/v8-ls/build.md) |
| `throttle.open_lag_s` | 0.0284 | the intake lag of profiles/cycles/v8-ls.bin, 0.028 s: tools/cycles build --step out/cycles/grid/v8-ls/step fits exp(-t/tau) to the share of the state before the step in each cycle after the closed to full throttle step of scenarios/upstream/v8-ls/grid/step.scn at 4000 rpm (out/cycles/grid/v8-ls/build.md) |
| `limiter.cut_ms` | 206.8 | the median cut, from a peak of the RPM to the next trough where the lines through the climbs and the falls meet, of the 11 bounces of the free run at full throttle of out/reference/upstream/v8-ls/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce): 206.8 ms (the dump limiter_duration_s is 0.2 s) |
| `limiter.fall_rpm_s` | 3696 | the median fall rate in the cuts, the slope of a straight line through the RPM from half a cycle after a peak to half a cycle before the trough, of the 11 bounces of the free run at full throttle of out/reference/upstream/v8-ls/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce), with median peaks and troughs of 6818 and 6053 rpm |
| `limiter.rise_rpm_s` | 11601 | the median climb rate, the slope of a straight line through the RPM from half a cycle after a trough to half a cycle before the next peak, of the 11 bounces of the free run at full throttle of out/reference/upstream/v8-ls/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce) |
| `idle.hunt` | 0.0176 | the spread of the RPM averaged over one engine cycle over the mean RPM of the free run at throttle 0 of out/reference/upstream/v8-ls/idle-free.csv from 2 to 11 s, 8.6 over 696.2 rpm (9.3 rpm with no averaging), over 0.7036, the spread of the idle wander of the engine per unit of hunt |
| `idle.hunt_hz` | 0.22 | the main wander frequency of the free run at throttle 0 of out/reference/upstream/v8-ls/idle-free.csv from 2 to 11 s, 0.220 Hz at 5.4 rpm |

Not measured:

| Key | Value | Reason |
| --- | --- | --- |
| `idle.fade_rpm` | 2090 | the free runs give the wander at idle only: idle_rpm plus 1400 rpm, the tools/import rule |

### f4-ej25

Measured from `out/reference/upstream/f4-ej25` and its grid renders:

| Key | Value | Source |
| --- | --- | --- |
| `tables` | "f4-ej25" | profiles/cycles/f4-ej25.bin of just cycles-grid f4-ej25: tools/cycles build of the engine-sim renders of the grid scenarios in scenarios/upstream/f4-ej25/grid at the simulation rate of the unchanged script, from idle to below the limiter, in out/cycles/grid/f4-ej25: 185 points, of which 51 are slices of the closing steps of close.scn, with the harmonics above the Nyquist frequency of the steps and the modulation of the average cycle in 8 parts per cycle (docs/research/cycle-tables.md, docs/research/fullrange-scorecard.md section 12) |
| `throttle.close_lag_s` | 0.05 | the intake lag of profiles/cycles/f4-ej25.bin, 0.050 s: tools/cycles build --step out/cycles/grid/f4-ej25/step fits exp(-t/tau) to the share of the state before the step in each cycle after the full to closed throttle step of scenarios/upstream/f4-ej25/grid/step.scn at 3750 rpm (out/cycles/grid/f4-ej25/build.md) |
| `throttle.open_lag_s` | 0.0524 | the intake lag of profiles/cycles/f4-ej25.bin, 0.052 s: tools/cycles build --step out/cycles/grid/f4-ej25/step fits exp(-t/tau) to the share of the state before the step in each cycle after the closed to full throttle step of scenarios/upstream/f4-ej25/grid/step.scn at 3750 rpm (out/cycles/grid/f4-ej25/build.md) |
| `limiter.cut_ms` | 100.2 | the median cut, from a peak of the RPM to the next trough where the lines through the climbs and the falls meet, of the 27 bounces of the free run at full throttle of out/reference/upstream/f4-ej25/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce): 100.2 ms (the dump limiter_duration_s is 0.08 s) |
| `limiter.fall_rpm_s` | 5819 | the median fall rate in the cuts, the slope of a straight line through the RPM from half a cycle after a peak to half a cycle before the trough, of the 27 bounces of the free run at full throttle of out/reference/upstream/f4-ej25/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce), with median peaks and troughs of 6548 and 5966 rpm |
| `limiter.rise_rpm_s` | 17123 | the median climb rate, the slope of a straight line through the RPM from half a cycle after a trough to half a cycle before the next peak, of the 27 bounces of the free run at full throttle of out/reference/upstream/f4-ej25/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce) |
| `idle.hunt` | 0.0316 | the spread of the RPM averaged over one engine cycle over the mean RPM of the free run at throttle 0 of out/reference/upstream/f4-ej25/idle-free.csv from 2 to 11 s, 18.9 over 852.5 rpm (25.3 rpm with no averaging), over 0.7036, the spread of the idle wander of the engine per unit of hunt |
| `idle.hunt_hz` | 0.68 | the main wander frequency of the free run at throttle 0 of out/reference/upstream/f4-ej25/idle-free.csv from 2 to 11 s, 0.684 Hz at 12.3 rpm |

Not measured:

| Key | Value | Reason |
| --- | --- | --- |
| `idle.fade_rpm` | 2230 | the free runs give the wander at idle only: idle_rpm plus 1400 rpm, the tools/import rule |

### v12-412

Measured from `out/reference/upstream/v12-412` and its grid renders:

| Key | Value | Source |
| --- | --- | --- |
| `tables` | "v12-412" | profiles/cycles/v12-412.bin of just cycles-grid v12-412: tools/cycles build of the engine-sim renders of the grid scenarios in scenarios/upstream/v12-412/grid at the simulation rate of the unchanged script, from idle to below the limiter, in out/cycles/grid/v12-412: 469 points, of which 142 are slices of the closing steps of close.scn, with the harmonics above the Nyquist frequency of the steps and the modulation of the average cycle in 8 parts per cycle (docs/research/cycle-tables.md, docs/research/fullrange-scorecard.md section 12) |
| `throttle.close_lag_s` | 0.019 | the intake lag of profiles/cycles/v12-412.bin, 0.019 s: tools/cycles build --step out/cycles/grid/v12-412/step fits exp(-t/tau) to the share of the state before the step in each cycle after the full to closed throttle step of scenarios/upstream/v12-412/grid/step.scn at 10750 rpm (out/cycles/grid/v12-412/build.md) |
| `throttle.open_lag_s` | 0.0155 | the intake lag of profiles/cycles/v12-412.bin, 0.015 s: tools/cycles build --step out/cycles/grid/v12-412/step fits exp(-t/tau) to the share of the state before the step in each cycle after the closed to full throttle step of scenarios/upstream/v12-412/grid/step.scn at 10750 rpm (out/cycles/grid/v12-412/build.md) |
| `limiter.cut_ms` | 101.7 | the median cut, from a peak of the RPM to the next trough where the lines through the climbs and the falls meet, of the 22 bounces of the free run at full throttle of out/reference/upstream/v12-412/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce): 101.7 ms (the dump limiter_duration_s is 0.1 s) |
| `limiter.fall_rpm_s` | 30580 | the median fall rate in the cuts, the slope of a straight line through the RPM from half a cycle after a peak to half a cycle before the trough, of the 22 bounces of the free run at full throttle of out/reference/upstream/v12-412/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce), with median peaks and troughs of 18510 and 15401 rpm |
| `limiter.rise_rpm_s` | 57323 | the median climb rate, the slope of a straight line through the RPM from half a cycle after a trough to half a cycle before the next peak, of the 22 bounces of the free run at full throttle of out/reference/upstream/v12-412/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce) |
| `idle.hunt` | 0.0245 | the spread of the RPM averaged over one engine cycle over the mean RPM of the free run at throttle 0 of out/reference/upstream/v12-412/idle-free.csv from 2 to 11 s, 65.4 over 3794.4 rpm (66.6 rpm with no averaging), over 0.7036, the spread of the idle wander of the engine per unit of hunt |
| `idle.hunt_hz` | 0.42 | the main wander frequency of the free run at throttle 0 of out/reference/upstream/v12-412/idle-free.csv from 2 to 11 s, 0.415 Hz at 68.5 rpm |

Not measured:

| Key | Value | Reason |
| --- | --- | --- |
| `idle.fade_rpm` | 5210 | the free runs give the wander at idle only: idle_rpm plus 1400 rpm, the tools/import rule |

### v12-merlin

Measured from `out/reference/upstream/v12-merlin` and its grid renders:

| Key | Value | Source |
| --- | --- | --- |
| `idle_rpm` | 880 | the engine stalls at speed control 0 (out/reference/upstream/v12-merlin/engine.json idle.measured_rpm 0.0 rpm), so the mean RPM of the last 1 s of the free run at throttle 0 of out/reference/upstream/v12-merlin/idle-free.csv from 2 to 11 s, 877.8 rpm, rounded to 10 rpm |
| `tables` | "v12-merlin" | profiles/cycles/v12-merlin.bin of just cycles-grid v12-merlin: tools/cycles build of the engine-sim renders of the grid scenarios in scenarios/upstream/v12-merlin/grid at the simulation rate of the unchanged script, from idle to below the limiter, in out/cycles/grid/v12-merlin: 93 points, of which 25 are slices of the closing steps of close.scn, with the harmonics above the Nyquist frequency of the steps and the modulation of the average cycle in 8 parts per cycle (docs/research/cycle-tables.md, docs/research/fullrange-scorecard.md section 12) |
| `throttle.close_lag_s` | 0.0953 | the intake lag of profiles/cycles/v12-merlin.bin, 0.095 s: tools/cycles build --step out/cycles/grid/v12-merlin/step fits exp(-t/tau) to the share of the state before the step in each cycle after the full to closed throttle step of scenarios/upstream/v12-merlin/grid/step.scn at 1750 rpm (out/cycles/grid/v12-merlin/build.md) |
| `throttle.open_lag_s` | 0.0561 | the intake lag of profiles/cycles/v12-merlin.bin, 0.056 s: tools/cycles build --step out/cycles/grid/v12-merlin/step fits exp(-t/tau) to the share of the state before the step in each cycle after the closed to full throttle step of scenarios/upstream/v12-merlin/grid/step.scn at 1750 rpm (out/cycles/grid/v12-merlin/build.md) |
| `limiter.cut_ms` | 59.2 | the median cut, from a peak of the RPM to the next trough where the lines through the climbs and the falls meet, of the 39 bounces of the free run at full throttle of out/reference/upstream/v12-merlin/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce): 59.2 ms (the dump limiter_duration_s is 0.05 s) |
| `limiter.fall_rpm_s` | 914 | the median fall rate in the cuts, the slope of a straight line through the RPM from half a cycle after a peak to half a cycle before the trough, of the 39 bounces of the free run at full throttle of out/reference/upstream/v12-merlin/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce), with median peaks and troughs of 3507 and 3454 rpm |
| `limiter.rise_rpm_s` | 1523 | the median climb rate, the slope of a straight line through the RPM from half a cycle after a trough to half a cycle before the next peak, of the 39 bounces of the free run at full throttle of out/reference/upstream/v12-merlin/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce) |
| `idle.hunt` | 0.0445 | the spread of the RPM averaged over one engine cycle over the mean RPM of the free run at throttle 0 of out/reference/upstream/v12-merlin/idle-free.csv from 2 to 11 s, 27.0 over 862.0 rpm (27.0 rpm with no averaging), over 0.7036, the spread of the idle wander of the engine per unit of hunt |
| `idle.hunt_hz` | 0.22 | the main wander frequency of the free run at throttle 0 of out/reference/upstream/v12-merlin/idle-free.csv from 2 to 11 s, 0.220 Hz at 4.2 rpm |

Not measured:

| Key | Value | Reason |
| --- | --- | --- |
| `idle.fade_rpm` | 2280 | the free runs give the wander at idle only: idle_rpm plus 1400 rpm, the tools/import rule |

## Licences

engine-sim is MIT, copyright 2022 Ange Yaghi (AngeTheGreat). The profiles hold values copied from its engine scripts and measured from its renders. The cycle tables hold measurements of its synthesiser input. `ir/` holds unchanged copies of its impulse responses.
