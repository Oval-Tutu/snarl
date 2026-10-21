# Engine profiles

This directory holds the six engine profiles of the library, converted faithfully from their upstream engines. Each value of a profile has one recorded source: a copy from the upstream engine, a measurement from the own upstream reference render of that engine, or a snarl default for a feature that upstream does not have. No value is set by ear. The library embeds the six profiles (`snarl.Profiles`), but not the sources files. The test catalogue in `profiles/catalogue/` does not change with them.

`just import-reference` writes the six profiles, their sources files and this README, and the pulse tables `internal/model/pulses/<id>.txt` of the engine-sim engines. Do not edit the files by hand. Change `tools/import` and run the recipe again. Two runs give the same files. To use other reference renders for an engine, for example `v12-412` rendered at 10 kHz, give its directory: `just import-reference v12-412=DIR`.

## Engines

| Id | Project | Upstream file | Commit | References | Pulse table |
| --- | --- | --- | --- | --- | --- |
| `i4-vtec` | engine-sim | `atg-video-1/05_honda_vtec.mr` | `85f7c3b` | `out/reference/upstream/i4-vtec` | measured, `i4-vtec` |
| `v8-ls` | engine-sim | `atg-video-2/07_gm_ls.mr` | `85f7c3b` | `out/reference/upstream/v8-ls` | measured, `v8-ls` |
| `f4-ej25` | engine-sim | `atg-video-1/06_subaru_ej25.mr` | `85f7c3b` | `out/reference/upstream/f4-ej25` | measured, `f4-ej25` |
| `v12-412` | engine-sim | `atg-video-2/12_ferrari_412_t2.mr` | `85f7c3b` | `out/reference/upstream/v12-412` | measured, `v12-412` |
| `v12-merlin` | engine-sim | `atg-video-2/11_merlin_v12.mr` | `85f7c3b` | `out/reference/upstream/v12-merlin` | measured, `v12-merlin` |
| `f6-aircooled` | EngineLab | `09_porsche_flat6_like.engine.yaml` | `d345ba4` | `out/reference/enginelab/f6-aircooled` | not measured, `f4-ej25` |

The engine-sim files are relative to `assets/engines/` of engine-sim at `85f7c3b`, the package in `reference/engine-sim`, rendered unchanged by `just ref-upstream`. The EngineLab file is relative to the EngineLab repository at `d345ba4` (`tools/import/enginelab.nix`), rendered unchanged by `just ref-enginelab`. `docs/research/reference-health.md` gives the health of the engine-sim references.

## Sources

`<id>.sources.toml` beside each profile has one `[[source]]` entry for each key that the profile sets. For a key of an array table, such as `cylinder.crank_deg`, one entry covers every entry of the array. The sources are a separate file because the profile parser rejects unknown tables, and the profile stays the plain model input. `reference_test.go` fails when a key has no entry, or when an entry names a key that the profile does not set.

| Kind | Meaning |
| --- | --- |
| `upstream` | Copied from the upstream engine: a field of the engine-sim dump, of the EngineLab engine file, or of the upstream source. |
| `measured` | Measured from the own upstream reference render of the engine, with the command and the input in the entry. |
| `snarl` | A snarl default for a feature that upstream does not have: the id, the seed, the pops, the final gear hold and the output calibration. |
| `not-measured` | The references cannot give the value, so the profile has the model default (`model.DefaultSpec`), or for a key with no usable default the rule in the entry. |

## Conversion rules

- Firing order: engine-sim turns the crank in the direction of the starter speed. `i4-vtec` has a starter speed of -500 rpm, so the import negates its spark angles, and the engine fires 1-3-4-2.
- Exhaust delay: engine-sim delays each cylinder by its header primary plus its exhaust length at 343 m/s, in a delay filter set for 10000 steps per second, which runs once per simulation step. `primary_m` is that length, and `speed_of_sound` is 343 m/s times the simulation frequency over 10000, so that the profile has the delay of the reference render.
- Path gain: engine-sim weights each cylinder by its sound attenuation and the audio volume of its exhaust system over the square of its exhaust length. A path gain is the mean of that weight over the cylinders of the path, relative to the loudest path. snarl has no fixed gain per cylinder, so the spread of the weights inside one path is not in the profile.
- Collector: engine-sim has no reflection in the exhaust, so `reflection` is 0 and the collector passes the sound unchanged.
- Pulse table: `tools/fit --pulse --variation --throttle 1` on the per-step `holds-t100.csv` of the engine, one row per full-throttle hold. `measured_variation` 1 takes the firing gain spread of the table, and the cylinder spreads are sqrt(3) times the median spread of the cylinder means over the same holds.
- Formant bank: fitted to the impulse response that the engine-sim dump names, with the tools/import fit.
- Idle hunt: the RPM spread and the main wander frequency of the free run at throttle 0 (`idle-free`), over the window of `tools/fit --health`. The spread is that of the RPM averaged over one engine cycle, so that the change of the crank speed within a cycle does not count. The hunt depth is that spread over the mean RPM, over the spread of the hunt modulation of the model per unit of hunt.
- Limiter: the median cut, fall rate and climb rate of the bounces of the free run at full throttle (`limiter-free`), with the RPM averaged over one engine cycle.
- Output level: the tools/import calibration. `leveller_db` puts the closed-throttle hold at idle 3 dB below the knee of the leveller, and `output_db` puts the sweep-wot scenario at -16 LUFS.
- Pops: the model defaults, with a density of 0.15, the tools/import default, so that the own pops play. The pop bank is fitted to the tools/import default pop stages.
- Removed stages: the model has no collector overlap, no limiter jitter, no idle variation, timing, imbalance or rocking, and no overrun or part-load level shaping (`docs/scoping.md` section 2). The load does not change the pulse level. As in engine-sim, the leveller sets the level of the part load and of the overrun. The profiles set no by-ear cylinder spread.

## Limits of the references

`docs/research/reference-health.md` gives these faults. The converter does not correct them, so the profiles have them too.

- `v12-412` is not healthy below 11000 rpm at its own 5000 Hz simulation rate: weak firings and misfires from `burning_efficiency_randomness: 1.0`, and a runner peak 17 per cent higher at 10000 Hz. Its pulse table, cylinder spreads and idle hunt include these holds. To use renders at another rate, run `just import-reference v12-412=DIR`. The `speed_of_sound` follows the simulation frequency of the dump in `DIR`.
- `v12-merlin` does not start from its starter at speed control 0. Its references start from a 600 rpm hold, its `idle_rpm` is the last second of the free run, and its idle hunt includes the climb from 600 rpm to that idle. Its pulse table has three rows: 600, 2000 and 3000 rpm.
- For `v8-ls` and `v12-merlin`, the main idle wander is at 0.22 Hz, the lowest frequency of the spectrum. It is a slow drift, not a periodic hunt.
- `f6-aircooled` has no per-cylinder exhaust pressure, no closed-throttle holds above idle and no limiter run, so its pulse table, formant bank and limiter rates are not measured. EngineLab loads no impulse response for it, so it has no formant bank.

## Per engine

### i4-vtec

Measured from `out/reference/upstream/i4-vtec`:

| Key | Value | Source |
| --- | --- | --- |
| `limiter_ms` | 55.8 | the median cut, from a peak of the RPM to the next trough where the lines through the climbs and the falls meet, of the 38 bounces of the free run at full throttle of out/reference/upstream/i4-vtec/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce): 55.8 ms (the dump limiter_duration_s is 0.05 s) |
| `limiter_fall_rpm_s` | 8509 | the median fall rate in the cuts, the slope of a straight line through the RPM from half a cycle after a peak to half a cycle before the trough, of the 38 bounces of the free run at full throttle of out/reference/upstream/i4-vtec/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce), with median peaks and troughs of 9381 and 8898 rpm |
| `limiter_rise_rpm_s` | 11609 | the median climb rate, the slope of a straight line through the RPM from half a cycle after a trough to half a cycle before the next peak, of the 38 bounces of the free run at full throttle of out/reference/upstream/i4-vtec/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce) |
| `pulse.table` | "i4-vtec" | tools/fit --pulse --variation --throttle 1 --engine out/reference/upstream/i4-vtec/engine.json --scenarios scenarios/upstream/i4-vtec on out/reference/upstream/i4-vtec/holds-t100.csv: one row per hold from 770 to 8400 rpm, in internal/model/pulses/i4-vtec.txt |
| `pulse.cycle_variation` | 0 | measured_variation 1 takes the measured firing gain spread of the table, so there is no extra uniform spread |
| `pulse.measured_variation` | 1 | the firing gain spread of the spread lines of the table (tools/fit --pulse --variation of the table, over its 9 holds) |
| `pulse.gain_variation` | 0.0069 | sqrt(3) times the median spread of the cylinder mean gains (tools/fit --pulse --variation of the table, over its 9 holds), 0.0040 |
| `pulse.timing_variation_deg` | 0.074 | sqrt(3) times the median spread of the cylinder mean onsets (tools/fit --pulse --variation of the table, over its 9 holds), 0.043 degrees |
| `exhaust.formant_gain_db` | -28.74 | the formant bank fitted to the impulse response mild_exhaust.wav that out/reference/upstream/i4-vtec/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `idle.hunt` | 0.032 | the spread of the RPM averaged over one engine cycle over the mean RPM of the free run at throttle 0 of out/reference/upstream/i4-vtec/idle-free.csv from 2 to 11 s, 17.3 over 767.1 rpm (23.7 rpm with no averaging), over 0.7036, the spread of the hunt modulation of the model per unit of hunt |
| `idle.hunt_hz` | 0.59 | the main wander frequency of the free run at throttle 0 of out/reference/upstream/i4-vtec/idle-free.csv from 2 to 11 s, 0.586 Hz at 15.5 rpm |
| `formant.kind` | per entry | the formant bank fitted to the impulse response mild_exhaust.wav that out/reference/upstream/i4-vtec/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.hz` | per entry | the formant bank fitted to the impulse response mild_exhaust.wav that out/reference/upstream/i4-vtec/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.q` | per entry | the formant bank fitted to the impulse response mild_exhaust.wav that out/reference/upstream/i4-vtec/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.gain_db` | per entry | the formant bank fitted to the impulse response mild_exhaust.wav that out/reference/upstream/i4-vtec/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |

Not measured:

| Key | Value | Reason |
| --- | --- | --- |
| `limiter_pump_db` | -1 | no reviewed tool measures it from the limiter reference: model default |
| `limiter_notch_db` | -3 | no reviewed tool measures it from the limiter reference: model default |
| `limiter_bark_db` | 1 | no reviewed tool measures it from the limiter reference: model default |
| `pulse.cam_gain` | 0 | each row of the pulse table has a peak of 1, and no reviewed tool measures the level step at the cam switch: model default |
| `pulse.jitter` | 0.003 | the dynamometer holds the RPM in the references, so they cannot show a slow RPM and gain wander under load: model default |
| `idle.fade_rpm` | 2170 | the free runs give the hunt at idle only: idle_rpm plus 1400 rpm, the tools/import rule |

### v8-ls

Measured from `out/reference/upstream/v8-ls`:

| Key | Value | Source |
| --- | --- | --- |
| `limiter_ms` | 206.8 | the median cut, from a peak of the RPM to the next trough where the lines through the climbs and the falls meet, of the 11 bounces of the free run at full throttle of out/reference/upstream/v8-ls/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce): 206.8 ms (the dump limiter_duration_s is 0.2 s) |
| `limiter_fall_rpm_s` | 3696 | the median fall rate in the cuts, the slope of a straight line through the RPM from half a cycle after a peak to half a cycle before the trough, of the 11 bounces of the free run at full throttle of out/reference/upstream/v8-ls/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce), with median peaks and troughs of 6818 and 6053 rpm |
| `limiter_rise_rpm_s` | 11601 | the median climb rate, the slope of a straight line through the RPM from half a cycle after a trough to half a cycle before the next peak, of the 11 bounces of the free run at full throttle of out/reference/upstream/v8-ls/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce) |
| `pulse.table` | "v8-ls" | tools/fit --pulse --variation --throttle 1 --engine out/reference/upstream/v8-ls/engine.json --scenarios scenarios/upstream/v8-ls on out/reference/upstream/v8-ls/holds-t100.csv: one row per hold from 690 to 6500 rpm, in internal/model/pulses/v8-ls.txt |
| `pulse.cycle_variation` | 0 | measured_variation 1 takes the measured firing gain spread of the table, so there is no extra uniform spread |
| `pulse.measured_variation` | 1 | the firing gain spread of the spread lines of the table (tools/fit --pulse --variation of the table, over its 7 holds) |
| `pulse.gain_variation` | 0.0993 | sqrt(3) times the median spread of the cylinder mean gains (tools/fit --pulse --variation of the table, over its 7 holds), 0.0573 |
| `pulse.timing_variation_deg` | 2.945 | sqrt(3) times the median spread of the cylinder mean onsets (tools/fit --pulse --variation of the table, over its 7 holds), 1.700 degrees |
| `exhaust.formant_gain_db` | -15.97 | the formant bank fitted to the impulse response smooth_39.wav that out/reference/upstream/v8-ls/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `idle.hunt` | 0.0176 | the spread of the RPM averaged over one engine cycle over the mean RPM of the free run at throttle 0 of out/reference/upstream/v8-ls/idle-free.csv from 2 to 11 s, 8.6 over 696.2 rpm (9.3 rpm with no averaging), over 0.7036, the spread of the hunt modulation of the model per unit of hunt |
| `idle.hunt_hz` | 0.22 | the main wander frequency of the free run at throttle 0 of out/reference/upstream/v8-ls/idle-free.csv from 2 to 11 s, 0.220 Hz at 5.4 rpm |
| `formant.kind` | per entry | the formant bank fitted to the impulse response smooth_39.wav that out/reference/upstream/v8-ls/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.hz` | per entry | the formant bank fitted to the impulse response smooth_39.wav that out/reference/upstream/v8-ls/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.q` | per entry | the formant bank fitted to the impulse response smooth_39.wav that out/reference/upstream/v8-ls/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.gain_db` | per entry | the formant bank fitted to the impulse response smooth_39.wav that out/reference/upstream/v8-ls/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |

Not measured:

| Key | Value | Reason |
| --- | --- | --- |
| `limiter_pump_db` | -1 | no reviewed tool measures it from the limiter reference: model default |
| `limiter_notch_db` | -3 | no reviewed tool measures it from the limiter reference: model default |
| `limiter_bark_db` | 1 | no reviewed tool measures it from the limiter reference: model default |
| `pulse.jitter` | 0.003 | the dynamometer holds the RPM in the references, so they cannot show a slow RPM and gain wander under load: model default |
| `idle.fade_rpm` | 2090 | the free runs give the hunt at idle only: idle_rpm plus 1400 rpm, the tools/import rule |

### f4-ej25

Measured from `out/reference/upstream/f4-ej25`:

| Key | Value | Source |
| --- | --- | --- |
| `limiter_ms` | 100.2 | the median cut, from a peak of the RPM to the next trough where the lines through the climbs and the falls meet, of the 27 bounces of the free run at full throttle of out/reference/upstream/f4-ej25/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce): 100.2 ms (the dump limiter_duration_s is 0.08 s) |
| `limiter_fall_rpm_s` | 5819 | the median fall rate in the cuts, the slope of a straight line through the RPM from half a cycle after a peak to half a cycle before the trough, of the 27 bounces of the free run at full throttle of out/reference/upstream/f4-ej25/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce), with median peaks and troughs of 6548 and 5966 rpm |
| `limiter_rise_rpm_s` | 17123 | the median climb rate, the slope of a straight line through the RPM from half a cycle after a trough to half a cycle before the next peak, of the 27 bounces of the free run at full throttle of out/reference/upstream/f4-ej25/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce) |
| `pulse.table` | "f4-ej25" | tools/fit --pulse --variation --throttle 1 --engine out/reference/upstream/f4-ej25/engine.json --scenarios scenarios/upstream/f4-ej25 on out/reference/upstream/f4-ej25/holds-t100.csv: one row per hold from 830 to 6400 rpm, in internal/model/pulses/f4-ej25.txt |
| `pulse.cycle_variation` | 0 | measured_variation 1 takes the measured firing gain spread of the table, so there is no extra uniform spread |
| `pulse.measured_variation` | 1 | the firing gain spread of the spread lines of the table (tools/fit --pulse --variation of the table, over its 7 holds) |
| `pulse.gain_variation` | 0.0145 | sqrt(3) times the median spread of the cylinder mean gains (tools/fit --pulse --variation of the table, over its 7 holds), 0.0084 |
| `pulse.timing_variation_deg` | 0.225 | sqrt(3) times the median spread of the cylinder mean onsets (tools/fit --pulse --variation of the table, over its 7 holds), 0.130 degrees |
| `exhaust.formant_gain_db` | -28.74 | the formant bank fitted to the impulse response mild_exhaust.wav that out/reference/upstream/f4-ej25/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `idle.hunt` | 0.0316 | the spread of the RPM averaged over one engine cycle over the mean RPM of the free run at throttle 0 of out/reference/upstream/f4-ej25/idle-free.csv from 2 to 11 s, 18.9 over 852.5 rpm (25.3 rpm with no averaging), over 0.7036, the spread of the hunt modulation of the model per unit of hunt |
| `idle.hunt_hz` | 0.68 | the main wander frequency of the free run at throttle 0 of out/reference/upstream/f4-ej25/idle-free.csv from 2 to 11 s, 0.684 Hz at 12.3 rpm |
| `formant.kind` | per entry | the formant bank fitted to the impulse response mild_exhaust.wav that out/reference/upstream/f4-ej25/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.hz` | per entry | the formant bank fitted to the impulse response mild_exhaust.wav that out/reference/upstream/f4-ej25/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.q` | per entry | the formant bank fitted to the impulse response mild_exhaust.wav that out/reference/upstream/f4-ej25/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.gain_db` | per entry | the formant bank fitted to the impulse response mild_exhaust.wav that out/reference/upstream/f4-ej25/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |

Not measured:

| Key | Value | Reason |
| --- | --- | --- |
| `limiter_pump_db` | -1 | no reviewed tool measures it from the limiter reference: model default |
| `limiter_notch_db` | -3 | no reviewed tool measures it from the limiter reference: model default |
| `limiter_bark_db` | 1 | no reviewed tool measures it from the limiter reference: model default |
| `pulse.jitter` | 0.003 | the dynamometer holds the RPM in the references, so they cannot show a slow RPM and gain wander under load: model default |
| `idle.fade_rpm` | 2230 | the free runs give the hunt at idle only: idle_rpm plus 1400 rpm, the tools/import rule |

### v12-412

Measured from `out/reference/upstream/v12-412`:

| Key | Value | Source |
| --- | --- | --- |
| `limiter_ms` | 101.7 | the median cut, from a peak of the RPM to the next trough where the lines through the climbs and the falls meet, of the 22 bounces of the free run at full throttle of out/reference/upstream/v12-412/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce): 101.7 ms (the dump limiter_duration_s is 0.1 s) |
| `limiter_fall_rpm_s` | 30580 | the median fall rate in the cuts, the slope of a straight line through the RPM from half a cycle after a peak to half a cycle before the trough, of the 22 bounces of the free run at full throttle of out/reference/upstream/v12-412/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce), with median peaks and troughs of 18510 and 15401 rpm |
| `limiter_rise_rpm_s` | 57323 | the median climb rate, the slope of a straight line through the RPM from half a cycle after a trough to half a cycle before the next peak, of the 22 bounces of the free run at full throttle of out/reference/upstream/v12-412/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce) |
| `pulse.table` | "v12-412" | tools/fit --pulse --variation --throttle 1 --engine out/reference/upstream/v12-412/engine.json --scenarios scenarios/upstream/v12-412 on out/reference/upstream/v12-412/holds-t100.csv: one row per hold from 3810 to 18000 rpm, in internal/model/pulses/v12-412.txt |
| `pulse.cycle_variation` | 0 | measured_variation 1 takes the measured firing gain spread of the table, so there is no extra uniform spread |
| `pulse.measured_variation` | 1 | the firing gain spread of the spread lines of the table (tools/fit --pulse --variation of the table, over its 15 holds) |
| `pulse.gain_variation` | 0.0329 | sqrt(3) times the median spread of the cylinder mean gains (tools/fit --pulse --variation of the table, over its 15 holds), 0.0190 |
| `pulse.timing_variation_deg` | 0.336 | sqrt(3) times the median spread of the cylinder mean onsets (tools/fit --pulse --variation of the table, over its 15 holds), 0.194 degrees |
| `exhaust.formant_gain_db` | -5.61 | the formant bank fitted to the impulse response minimal_muffling_01.wav that out/reference/upstream/v12-412/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `idle.hunt` | 0.0245 | the spread of the RPM averaged over one engine cycle over the mean RPM of the free run at throttle 0 of out/reference/upstream/v12-412/idle-free.csv from 2 to 11 s, 65.4 over 3794.4 rpm (66.6 rpm with no averaging), over 0.7036, the spread of the hunt modulation of the model per unit of hunt |
| `idle.hunt_hz` | 0.42 | the main wander frequency of the free run at throttle 0 of out/reference/upstream/v12-412/idle-free.csv from 2 to 11 s, 0.415 Hz at 68.5 rpm |
| `formant.kind` | per entry | the formant bank fitted to the impulse response minimal_muffling_01.wav that out/reference/upstream/v12-412/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.hz` | per entry | the formant bank fitted to the impulse response minimal_muffling_01.wav that out/reference/upstream/v12-412/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.q` | per entry | the formant bank fitted to the impulse response minimal_muffling_01.wav that out/reference/upstream/v12-412/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.gain_db` | per entry | the formant bank fitted to the impulse response minimal_muffling_01.wav that out/reference/upstream/v12-412/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |

Not measured:

| Key | Value | Reason |
| --- | --- | --- |
| `limiter_pump_db` | -1 | no reviewed tool measures it from the limiter reference: model default |
| `limiter_notch_db` | -3 | no reviewed tool measures it from the limiter reference: model default |
| `limiter_bark_db` | 1 | no reviewed tool measures it from the limiter reference: model default |
| `pulse.jitter` | 0.003 | the dynamometer holds the RPM in the references, so they cannot show a slow RPM and gain wander under load: model default |
| `idle.fade_rpm` | 5210 | the free runs give the hunt at idle only: idle_rpm plus 1400 rpm, the tools/import rule |

### v12-merlin

Measured from `out/reference/upstream/v12-merlin`:

| Key | Value | Source |
| --- | --- | --- |
| `idle_rpm` | 880 | the engine stalls at speed control 0 (out/reference/upstream/v12-merlin/engine.json idle.measured_rpm 0.0 rpm), so the mean RPM of the last 1 s of the free run at throttle 0 of out/reference/upstream/v12-merlin/idle-free.csv from 2 to 11 s, 877.8 rpm, rounded to 10 rpm |
| `limiter_ms` | 59.2 | the median cut, from a peak of the RPM to the next trough where the lines through the climbs and the falls meet, of the 39 bounces of the free run at full throttle of out/reference/upstream/v12-merlin/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce): 59.2 ms (the dump limiter_duration_s is 0.05 s) |
| `limiter_fall_rpm_s` | 914 | the median fall rate in the cuts, the slope of a straight line through the RPM from half a cycle after a peak to half a cycle before the trough, of the 39 bounces of the free run at full throttle of out/reference/upstream/v12-merlin/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce), with median peaks and troughs of 3507 and 3454 rpm |
| `limiter_rise_rpm_s` | 1523 | the median climb rate, the slope of a straight line through the RPM from half a cycle after a trough to half a cycle before the next peak, of the 39 bounces of the free run at full throttle of out/reference/upstream/v12-merlin/limiter-free.csv from 1.5 to 5.5 s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce) |
| `pulse.table` | "v12-merlin" | tools/fit --pulse --variation --throttle 1 --engine out/reference/upstream/v12-merlin/engine.json --scenarios scenarios/upstream/v12-merlin on out/reference/upstream/v12-merlin/holds-t100.csv: one row per hold from 600 to 3000 rpm, in internal/model/pulses/v12-merlin.txt |
| `pulse.cycle_variation` | 0 | measured_variation 1 takes the measured firing gain spread of the table, so there is no extra uniform spread |
| `pulse.measured_variation` | 1 | the firing gain spread of the spread lines of the table (tools/fit --pulse --variation of the table, over its 3 holds) |
| `pulse.gain_variation` | 0.208 | sqrt(3) times the median spread of the cylinder mean gains (tools/fit --pulse --variation of the table, over its 3 holds), 0.1201 |
| `pulse.timing_variation_deg` | 1.206 | sqrt(3) times the median spread of the cylinder mean onsets (tools/fit --pulse --variation of the table, over its 3 holds), 0.696 degrees |
| `exhaust.formant_gain_db` | -5.61 | the formant bank fitted to the impulse response minimal_muffling_01.wav that out/reference/upstream/v12-merlin/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `idle.hunt` | 0.0445 | the spread of the RPM averaged over one engine cycle over the mean RPM of the free run at throttle 0 of out/reference/upstream/v12-merlin/idle-free.csv from 2 to 11 s, 27.0 over 862.0 rpm (27.0 rpm with no averaging), over 0.7036, the spread of the hunt modulation of the model per unit of hunt |
| `idle.hunt_hz` | 0.22 | the main wander frequency of the free run at throttle 0 of out/reference/upstream/v12-merlin/idle-free.csv from 2 to 11 s, 0.220 Hz at 4.2 rpm |
| `formant.kind` | per entry | the formant bank fitted to the impulse response minimal_muffling_01.wav that out/reference/upstream/v12-merlin/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.hz` | per entry | the formant bank fitted to the impulse response minimal_muffling_01.wav that out/reference/upstream/v12-merlin/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.q` | per entry | the formant bank fitted to the impulse response minimal_muffling_01.wav that out/reference/upstream/v12-merlin/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |
| `formant.gain_db` | per entry | the formant bank fitted to the impulse response minimal_muffling_01.wav that out/reference/upstream/v12-merlin/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz) |

Not measured:

| Key | Value | Reason |
| --- | --- | --- |
| `limiter_pump_db` | -1 | no reviewed tool measures it from the limiter reference: model default |
| `limiter_notch_db` | -3 | no reviewed tool measures it from the limiter reference: model default |
| `limiter_bark_db` | 1 | no reviewed tool measures it from the limiter reference: model default |
| `pulse.jitter` | 0.003 | the dynamometer holds the RPM in the references, so they cannot show a slow RPM and gain wander under load: model default |
| `idle.fade_rpm` | 2280 | the free runs give the hunt at idle only: idle_rpm plus 1400 rpm, the tools/import rule |

### f6-aircooled

Measured from `out/reference/enginelab/f6-aircooled`:

| Key | Value | Source |
| --- | --- | --- |
| `idle.hunt` | 0.0033 | the spread of the RPM averaged over one engine cycle over the mean RPM of the free run at throttle 0 of out/reference/enginelab/f6-aircooled/idle-free/engine-order-map.csv (24 RPM values per second) from 9.5 to 14.5 s, after the 8.5 s pre-roll of reference/enginelab/scenarios/idle-free.json, 1.8 over 775.2 rpm (1.8 rpm with no averaging), over 0.7036, the spread of the hunt modulation of the model per unit of hunt |
| `idle.hunt_hz` | 0.23 | the main wander frequency of the free run at throttle 0 of out/reference/enginelab/f6-aircooled/idle-free/engine-order-map.csv (24 RPM values per second) from 9.5 to 14.5 s, after the 8.5 s pre-roll of reference/enginelab/scenarios/idle-free.json, 0.234 Hz at 1.5 rpm |

Not measured:

| Key | Value | Reason |
| --- | --- | --- |
| `limiter_fall_rpm_s` | 6000 | EngineLab reaches about 7240 rpm at full throttle with no load, below its rev limit, so no reference bounces off the limiter (reference/enginelab/README.md): model default |
| `limiter_rise_rpm_s` | 10000 | EngineLab reaches about 7240 rpm at full throttle with no load, below its rev limit, so no reference bounces off the limiter (reference/enginelab/README.md): model default |
| `limiter_pump_db` | -1 | EngineLab reaches about 7240 rpm at full throttle with no load, below its rev limit, so no reference bounces off the limiter (reference/enginelab/README.md): model default |
| `limiter_notch_db` | -3 | EngineLab reaches about 7240 rpm at full throttle with no load, below its rev limit, so no reference bounces off the limiter (reference/enginelab/README.md): model default |
| `limiter_bark_db` | 1 | EngineLab reaches about 7240 rpm at full throttle with no load, below its rev limit, so no reference bounces off the limiter (reference/enginelab/README.md): model default |
| `pulse.table` | "f4-ej25" | EngineLab writes no per-cylinder exhaust pressure (reference/enginelab/README.md), so no table is measured: the table of f4-ej25, the flat engine of the set, measured from unchanged upstream data |
| `pulse.measured_variation` | 0 | the engine has no measured table of its own: model default |
| `pulse.jitter` | 0.003 | the dynamometer holds the RPM in the references, so they cannot show a slow RPM and gain wander under load: model default |
| `exhaust.speed_of_sound` | 565 | EngineLab delays the exhaust in its finite-volume exhaust network, which gives no single speed of sound: model default |
| `exhaust.formant_gain_db` | 0 | EngineLab loads no impulse response for this engine (out/reference/enginelab/f6-aircooled/holds-t100/render-manifest.json impulse_responses loaded 0): its exhaust network, which snarl does not model, gives the colour, so there is no formant bank |
| `noise.hf_gain` | 0 | an engine-sim synthesiser setting, which EngineLab does not have: model default |
| `noise.am` | 0.35 | an engine-sim synthesiser setting, which EngineLab does not have: model default |
| `noise.am_cutoff_hz` | 2000 | an engine-sim synthesiser setting, which EngineLab does not have: model default |
| `noise.input_jitter` | 0 | an engine-sim synthesiser setting, which EngineLab does not have: model default |
| `idle.fade_rpm` | 2180 | the free runs give the hunt at idle only: idle_rpm plus 1400 rpm, the tools/import rule |
| `path.reflection` | per entry | EngineLab reflects at the junctions of its finite-volume exhaust network, which gives no single reflection gain: model default |

## Licences

- engine-sim is MIT, copyright 2022 Ange Yaghi (AngeTheGreat). The profiles hold values copied from its engine scripts and measured from its renders, and formant banks fitted to its impulse responses. They hold no code and no impulse response.
- EngineLab is MIT, copyright 2026 EngineLab Contributors (`LICENSE.md` in the source). The `f6-aircooled` profile holds values copied from its engine file and measured from its renders, and no code.
