# Profile catalogue

This directory holds every engine profile of the two upstream projects, [engine-sim](https://github.com/ange-yaghi/engine-sim) and [EngineLab](https://github.com/zolaski333/EngineLab), converted to snarl profiles. The tests in `catalogue_test.go` load them from disk. They are not embedded in the library and do not ship to the game. The four tuned profiles in `profiles/*.toml` are separate and do not change with this catalogue.

To listen to a catalogue engine, run `just run-harness -profile es-v10-72`, then step through every profile with the Left and Right arrows or D-pad left and right. `snarl-render` takes a catalogue id as PROFILE and in `--replay` logs.

`just import-profiles` writes every file in this directory, and the scenarios in `scenarios/catalogue/`. Do not edit the files by hand. Change `tools/import` and run the import again. The import does these steps:

1. It builds the engine-sim renderer (`reference/engine-sim`) and runs `engine-sim-render --dump-json` on every engine script of the package. A script without a `main` node gets a wrapper with a default vehicle and a one-gear transmission (`tools/import/engine-sim-main.sh`). The dump holds the engine that the script builds, and the idle rpm of an 8 s free run at speed control 0.
2. It fetches EngineLab at the pinned commit (`tools/import/enginelab.nix`) and converts `engines/*.engine.yaml` and `parts/exhausts.yaml` to JSON with yq. It runs nothing from EngineLab.
3. `go run ./tools/import` maps both sources to snarl profiles, calibrates the output level of each profile by rendering it, and writes the profiles, this README and the scenarios.

The intermediate dumps go to `out/import/`.

## Identifiers

The identifiers and descriptions name no maker and no model. The prefix `es-` marks an engine from engine-sim and `el-` an engine from EngineLab. The tables give the upstream file of each profile. Cylinders and banks are the snarl cylinders and exhaust banks of the profile.

### engine-sim

Files are relative to `assets/engines/` of engine-sim.

| Id | Upstream file | Layout | Cylinders | Banks | Idle rpm | Redline rpm | Limiter rpm | Cut ms | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `es-single-atv` | `atg-video-1/01_honda_trx520.mr` | single | 1 | 1 | 1270 | 5000 | 6000 | 500 | Calibration stops at -17.77 LUFS. |
| `es-v2-90-industrial` | `atg-video-1/02_kohler_ch750.mr` | V2, 90 degree | 2 | 1 | 1590 | 3600 | 5000 | 500 |  |
| `es-v2-45-big-twin` | `atg-video-1/03_harley_davidson_shovelhead.mr` | V2, 45 degree | 2 | 2 | 820 | 5000 | 5500 | 500 |  |
| `es-i4-moto-litre` | `atg-video-1/04_hayabusa.mr` | inline-4 | 4 | 2 | 1410 | 11000 | 11500 | 50 |  |
| `es-i4-dohc-cam-switch` | `atg-video-1/05_honda_vtec.mr` | inline-4 | 4 | 2 | 770 | 8400 | 9400 | 50 |  |
| `es-f4-boxer` | `atg-video-1/06_subaru_ej25.mr` | flat-4 | 4 | 2 | 830 | 6500 | 6500 | 80 |  |
| `es-i5-23` | `atg-video-1/07_audi_i5.mr` | inline-5 | 5 | 2 | 960 | 6000 | 6500 | 100 |  |
| `es-r5-radial` | `atg-video-1/08_radial_5.mr` | radial-5 | 5 | 2 | 840 | 3000 | 3500 | 200 |  |
| `es-f4-boxer-equal-header` | `atg-video-2/01_subaru_ej25_eh.mr` | flat-4 | 4 | 1 | 770 | 6500 | 6800 | 160 |  |
| `es-f4-boxer-unequal-header` | `atg-video-2/02_subaru_ej25_uh.mr` | flat-4 | 4 | 1 | 770 | 6500 | 6800 | 160 |  |
| `es-i6-30` | `atg-video-2/03_2jz.mr` | inline-6 | 6 | 2 | 1200 | 6000 | 6500 | 100 | Stalls at speed control 0, so idle_rpm is the fallback 1200 rpm. |
| `es-v6-60` | `atg-video-2/04_60_degree_v6.mr` | V6, 60 degree | 6 | 2 | 720 | 5500 | 5600 | 200 |  |
| `es-v6-90-odd-fire` | `atg-video-2/05_odd_fire_v6.mr` | V6, 90 degree | 6 | 2 | 930 | 5500 | 5600 | 200 |  |
| `es-v6-90-even-fire` | `atg-video-2/06_even_fire_v6.mr` | V6, 90 degree | 6 | 2 | 920 | 5500 | 5600 | 200 |  |
| `es-v8-crossplane-pushrod` | `atg-video-2/07_gm_ls.mr` | V8, 90 degree | 8 | 2 | 690 | 6500 | 6800 | 200 |  |
| `es-v8-flatplane-dohc` | `atg-video-2/08_ferrari_f136_v8.mr` | V8, 90 degree | 8 | 2 | 970 | 9000 | 9300 | 100 |  |
| `es-r9-radial` | `atg-video-2/09_radial_9.mr` | radial-9 | 9 | 2 | 600 | 3000 | 3500 | 200 | Stalls at speed control 0, so idle_rpm is the fallback 600 rpm. |
| `es-v10-72` | `atg-video-2/10_lfa_v10.mr` | V10, 72 degree | 10 | 2 | 900 | 9000 | 9500 | 100 |  |
| `es-v12-60-aero` | `atg-video-2/11_merlin_v12.mr` | V12, 60 degree | 12 | 2 | 600 | 3000 | 3500 | 50 | Stalls at speed control 0, so idle_rpm is the fallback 600 rpm. |
| `es-v12-75-race` | `atg-video-2/12_ferrari_412_t2.mr` | V12, 75 degree | 12 | 2 | 3810 | 18000 | 18500 | 100 |  |
| `es-i5-22` | `audi/i5.mr` | inline-5 | 5 | 2 | 1500 | 7500 | 7500 | 500 | Stalls at speed control 0, so idle_rpm is the fallback 1500 rpm. |
| `es-i6-28-dohc` | `bmw/M52B28.mr` | inline-6 | 6 | 2 | 1030 | 7000 | 8000 | 500 |  |
| `es-v8-bigblock` | `chevrolet/chev_truck_454.mr` | V8, 90 degree | 8 | 2 | 930 | 5500 | 7000 | 500 |  |
| `es-v8-bigblock-alt` | `chevrolet/engine_03_for_e1.mr` | V8, 90 degree | 8 | 2 | 870 | 5500 | 7000 | 500 |  |
| `es-v2-90-industrial-10k` | `kohler/kohler_ch750.mr` | V2, 90 degree | 2 | 1 | 1520 | 3600 | 5000 | 500 |  |

### EngineLab

Files are relative to `engines/` of EngineLab.

| Id | Upstream file | Layout | Cylinders | Banks | Idle rpm | Redline rpm | Limiter rpm | Cut ms | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `el-i4-dohc-cam-switch` | `01_honda_k20a_like.engine.yaml` | inline-4 | 4 | 1 | 950 | 8600 | 8700 | 55 |  |
| `el-i6-30-turbo` | `02_toyota_2jz_gte_like.engine.yaml` | inline-6 | 6 | 1 | 760 | 7200 | 7000 | 70 |  |
| `el-v8-crossplane-pushrod` | `03_gm_ls3_like.engine.yaml` | V8, 90 degree | 8 | 2 | 720 | 6600 | 6700 | 65 |  |
| `el-f4-boxer-turbo` | `04_subaru_ej25_like.engine.yaml` | flat-4 | 4 | 1 | 820 | 6800 | 6800 | 70 |  |
| `el-i5-turbo` | `05_audi_i5_like.engine.yaml` | inline-5 | 5 | 1 | 780 | 7100 | 7300 | 65 |  |
| `el-i4-moto-litre` | `06_hayabusa_i4_like.engine.yaml` | inline-4 | 4 | 1 | 1250 | 11200 | 11200 | 45 |  |
| `el-v2-45-big-twin` | `07_harley_v_twin_like.engine.yaml` | V2, 45 degree | 2 | 2 | 760 | 5600 | 5900 | 85 | Calibration stops at -16.77 LUFS. |
| `el-v12-60-aero` | `08_merlin_v12_like.engine.yaml` | V12, 60 degree | 12 | 2 | 800 | 3800 | 3200 | 90 |  |
| `el-f6-aircooled` | `09_porsche_flat6_like.engine.yaml` | flat-6 | 6 | 2 | 780 | 7600 | 7400 | 60 |  |
| `el-r5-radial` | `10_radial5_like.engine.yaml` | radial-5 | 5 | 1 | 640 | 2400 | 2450 | 100 |  |
| `el-i2-270` | `11_yamaha_cp2_mt07_like.engine.yaml` | inline-2 | 2 | 1 | 1400 | 10000 | 10500 | 50 |  |
| `el-i3-240` | `12_yamaha_cp3_mt09_like.engine.yaml` | inline-3 | 3 | 1 | 1300 | 11000 | 11500 | 50 |  |
| `el-i4-crossplane-moto` | `13_yamaha_cp4_mt10_like.engine.yaml` | inline-4 | 4 | 1 | 1300 | 12000 | 12500 | 45 |  |
| `el-i4-diesel-turbo` | `14_vw_2_0_tdi_like.engine.yaml` | inline-4 | 4 | 1 | 800 | 5000 | 5000 | 70 |  |
| `el-i2-270-full-system` | `15_cp2_full_system_like.engine.yaml` | inline-2 | 2 | 1 | 1400 | 10000 | 10500 | 50 |  |
| `el-i2-270-lab` | `16_audio_physics_lab_689_twin.engine.yaml` | inline-2 | 2 | 1 | 1400 | 10000 | 10500 | 50 | The limiter keeps the fuel, so limiter_kind is ignition. |
| `el-i2-two-stroke` | `17_yamaha_rd350lc_like.engine.yaml` | inline-2 | 4 | 2 | 1300 | 9500 | 10000 | 50 | Two-stroke: each cylinder fires every 360 degrees, so each is two snarl cylinders 360 degrees apart. |

## Field mapping

### engine-sim

| snarl value | Source in the dump |
| --- | --- |
| `cylinder.crank_deg` | Spark plug angle of the cylinder (`connect_wire`), minus the angle of the first cylinder, in the 720 degree cycle. The timing advance is the same for every cylinder, so it does not change the intervals. |
| `cylinder.bank` | Exhaust system of the cylinder. Each exhaust system that a cylinder uses is one snarl bank, in the order of first use. |
| `cylinder.primary_m` | `primary_length` of the cylinder plus `primary_tube_length` of its exhaust system. |
| `path.collector_m` | `length` of the exhaust system, the pipe length that engine-sim delays the pulse by. |
| `path.pipe_radius_m`, `path.outlet_radius_m` | Radius of `collector_cross_section_area`. |
| `path.gain` | `audio_volume` of the exhaust system over the loudest system of the engine. |
| `exhaust.crossover` | `none`: engine-sim has no crossover. |
| `idle_rpm` | Mean rpm over the last 2 s of the free run at speed control 0, rounded to 10 rpm. An engine that stalls gets a fifth of the redline, rounded to 50 rpm and at least 500 rpm. |
| `redline_rpm` | `redline` of the engine node. |
| `limiter_rpm`, `limiter_ms` | `rev_limit` and `limiter_duration` of the ignition module, the cut capped at 500 ms. |
| `limiter_kind` | `fuel`: engine-sim cuts every cylinder, as the tuned profiles read it. |

The dump also holds bank angles, crank throws and journal angles, bore, stroke, rod length, compression ratio, intake and exhaust runner sizes, sound attenuation, the impulse response and the simulation frequency. snarl has no values for them, so the import does not use them.

### EngineLab

| snarl value | Source in the YAML |
| --- | --- |
| `cylinder.crank_deg` | `crank_offset_deg` of the cylinder, or the position in `firing_order` times the cycle over the cylinder count, as EngineLab sets it, plus `ignition_offset_deg`. A two-stroke cylinder fires every 360 degrees, so it is two snarl cylinders 360 degrees apart. |
| `cylinder.bank` | With a component `graph`: the cylinders that meet at the same first junction. Without: each entry of `exhaust_paths`, or one bank for an engine without `exhaust_paths`. |
| `cylinder.primary_m` | Graph: the pipes from the cylinder to its first junction. A single-cylinder path has no junction, so its first pipe is the primary. Scalar exhaust: `primary_length_mm`, or `exhaust_primary_length_mm` of the cylinder. |
| `path.collector_m` | Graph: the components from the junction to the outlet, without the silencer. Scalar exhaust: `midpipe_length_mm`. At least 0.2 m. |
| `path.pipe_radius_m` | Graph: the diameter of the first component after the junction. Scalar exhaust: `collector_diameter_mm`. |
| `path.outlet_radius_m` | `outlet_diameter_mm`, or the diameter of the `outlet` component. |
| `path.chamber_m`, `path.chamber_area_ratio` | Silencer length, and its area over the pipe area. Graph: `volume_l` over `length_mm` of the `muffler`. Scalar exhaust: `muffler_chamber_diameter_mm` over `collector_diameter_mm`, squared. No chamber when the ratio is 1 or less. |
| `path.gain` | `audio_volume` of the path over the loudest path. |
| `exhaust.crossover`, `exhaust.coupling` | `partial` with `crossover_coupling` (at most 0.5) for a `crossover` component. `full` when two banks reach the same merge. `none` otherwise. |
| `idle_rpm`, `redline_rpm` | `idle_rpm`, `redline_rpm`. |
| `limiter_rpm`, `limiter_ms` | `ignition.rev_limit_rpm` (default the redline) and `ignition.limiter_duration_s` (default 0.08 s). |
| `limiter_kind` | `ignition` when `limiter_keeps_fuel` is true, `fuel` otherwise. |

The exhaust preset is the `uses.exhaust` part of `parts/exhausts.yaml`, or the `exhaust` node of the engine, over the EngineLab defaults (480 mm primaries, 58 mm collector, 65 mm outlet, no silencer, no mid pipe). The YAML also holds bore, stroke, inertia, the intake runner, cams, the timing curve, injection, the vehicle and the voicing. snarl has no values for them, so the import does not use them.

### Defaults

Neither source has these values, so every profile has the same ones. They are near the middle of the four tuned profiles.

| Section | Values |
| --- | --- |
| `[pulse]` | `width_deg` 160, `attack` 25, `decay` 7, `part_load_db` -0.8, `cycle_variation` 0.04, `jitter` 0.003, `gain_variation` 0.12, `timing_variation_deg` 2 |
| `[exhaust]` | `speed_of_sound` 565, `outlet_direct` 0.4, `steepening` 0.3, `overlap` 1.0, `low_shelf_db` 0, `low_shelf_hz` 250 |
| `[noise]` | `am` 0.2, `jet` 0.02, `jet_hz` 1500 |
| `[pops]` | `density` 0.15, `character` sport, `gain` 1, `body_hz` 200, `body_ms` 35, `jump_ms` 3, `crack` 2.5, `crack_ms` 16, weights pop 0.4, boom 0.2, crackle 0.3, burble 0.1 |
| `[overrun]` | `level_db` -1.2, `idle_level_db` -1.2 |
| `[idle]` | `fade_rpm` idle + 1400, `variation` 0.15, `timing_deg` 1, `imbalance` 0.25, `hunt` 0.004, `rock` 0.01, `hunt_hz` 1.2 |
| `[voice]` | `gain` 1 |
| `[[path]]` | `reflection` -0.4, `damping_hz` 2500, `wall_loss_db` 2.5, `wall_loss_hz` 2000. engine-sim paths: `chamber_m` 0.3, `chamber_area_ratio` 7 |
| Engine | `seed` from a hash of the id, `cam_switch_rpm` 0, the limiter rates and levels of the model defaults |

### Calibration

The import renders each profile with the snapshots of `snarl-render`:

1. `leveller_db`: a 3 s closed-throttle hold at the idle rpm, measured over the last 2 s, at a leveller gain of -40 dB and of +40 dB. Below the knee of the leveller, the level follows the gain, and far above it, the level stays at the level of the knee. The gain puts the hold 3 dB below the knee, so that the leveller leaves the idle character alone.
2. `output_db`: the `sweep-wot` scenario, measured over its length, moved to -16 LUFS in up to 8 steps, as `TestOutputCalibration` checks.

## Scenarios

`scenarios/catalogue/<id>/` holds `holds-t000.scn`, `holds-t050.scn`, `holds-t100.scn`, `sweep-wot.scn` and `overrun.scn` in the format of `docs/scenario-format.md`. The top rpm is the redline, or 100 rpm below the limiter when the limiter cuts at or below the redline. The holds are at idle, at each multiple of 1000 rpm more than 500 rpm above idle and 300 rpm below the top, and at the top. There is no limiter scenario.

`just ref-set catalogue/<id>` renders the engine-sim reference of every scenario of one engine from engine-sim into `out/reference/engine-sim/catalogue/<id>/`, and `just ref-set catalogue` renders all of them. `scenarios/catalogue/engine-sim-scripts.txt` gives the script of each id. EngineLab has no Linux reference renderer, so the `el-` engines have no reference.

## Tests

`catalogue_test.go` runs these checks on every profile in this directory:

- The profile loads with `ParseProfile` and validates.
- `Render` allocates nothing.
- The `sweep-wot` and `overrun` scenarios and a 3 s full-throttle hold at the limiter rpm give finite output below 0.99.
- At a half-throttle hold, the strongest component from 20 Hz to 1.75 times the firing frequency is at the firing frequency within 2 Hz. This check is for even profiles only.
- The `sweep-wot` scenario is within 1 LU of -16 LUFS.

The test does not apply the by-ear tests of the tuned profiles: the overrun level windows, the limiter against engine-sim and the pop level bands. Those check values that were tuned by ear, and the catalogue has default values for them.

A profile is even when its cylinders fire at equal intervals, the primary pipe delays keep the arrivals at the outlet within 15 degrees of equal at the test rpm, and no path is below half the gain of the loudest. The test treats these profiles as uneven. For them, it checks that the strongest component is on an engine order, a multiple of half the crank frequency, within 2 Hz:

| Id | Reason |
| --- | --- |
| `el-i2-270`, `el-i2-270-full-system`, `el-i2-270-lab` | 270 degree crank: 270 and 450 degree intervals |
| `el-i4-crossplane-moto` | Cross-plane crank: 270, 180, 90 and 180 degree intervals |
| `el-v2-45-big-twin`, `es-v2-45-big-twin` | 45 degree V-twin on one crank pin: 315 and 405 degree intervals |
| `es-v2-90-industrial`, `es-v2-90-industrial-10k` | 90 degree V-twin on one crank pin: 270 and 450 degree intervals |
| `es-v6-90-odd-fire` | 90 degree V6 on common crank pins: 150 and 90 degree intervals |
| `es-v12-75-race` | 75 degree V12: 75 and 45 degree intervals |
| `es-f4-boxer-unequal-header` | Even firing, but the unequal primaries spread the arrivals |
| `es-r9-radial`, `es-v12-60-aero` | Even firing, but the primaries differ by up to 1.3 m, so the arrivals are uneven |
| `es-r5-radial` | Even firing, but one exhaust system has a fifth of the gain of the other |
| `es-v8-bigblock-alt` | Even firing, but one exhaust system has a tenth of the gain of the other, so the cross-plane pattern of one bank leads |

The test skips one check that a profile cannot pass:

| Id | Check | Reason |
| --- | --- | --- |
| `es-single-atv` | Calibration | One pulse per cycle gives a high crest factor. The output peak limiter caps the level, so the sweep stops near -17.2 LUFS for any `output_db`. |

## Licences

The profiles are derived from the engine definitions of both projects. Both are under the MIT licence below.

- engine-sim, <https://github.com/ange-yaghi/engine-sim>, commit `85f7c3b959a908ed5232ede4f1a4ac7eafe6b630`. Copyright 2022 AngeTheGreat (Ange Yaghi).
- EngineLab, <https://github.com/zolaski333/EngineLab>, commit `d345ba42b8201367c91b0f1dcf83cf283af4caec`. Copyright (c) 2026 EngineLab Contributors.

```text
Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
