# Upstream engine-sim scenarios

`just ref-upstream` renders five engine-sim engines exactly as the pinned package ships them (`nix build -f reference/engine-sim`, scripts in `share/engine-sim/engines/`). `tools/fit --upstream-scenarios` writes the scenarios in each directory from the engine dump. Do not edit them by hand: run the recipe again.

| Engine | Upstream script | Engine name in the script |
| --- | --- | --- |
| `i4-vtec` | `atg-video-1/05_honda_vtec.mr` | Honda VTEC inline-4 |
| `v8-ls` | `atg-video-2/07_gm_ls.mr` | GM LS cross-plane V8 |
| `f4-ej25` | `atg-video-1/06_subaru_ej25.mr` | Subaru EJ25 flat-4 |
| `v12-412` | `atg-video-2/12_ferrari_412_t2.mr` | Ferrari 412 T2 V12, 75 degree |
| `v12-merlin` | `atg-video-2/11_merlin_v12.mr` | Rolls-Royce Merlin V12 |

All five scripts define `public node main`, so the recipe renders them with no wrapper.

## Scenarios

- `holds-t000`, `holds-t050`, `holds-t100`: 3 s holds at throttle 0, 0.5 and 1, with 0.5 s sweeps between them, from idle to the top rpm in steps of about 1000 rpm. The top is the redline, or 100 rpm below the rev limit when the limiter cuts at or below the redline.
- `sweep-wot`: a full-throttle sweep from idle to the top rpm over 8 s.
- `overrun`: full throttle at the top rpm, closed throttle held, then a closed-throttle sweep down to idle.
- `idle-free`: the dynamometer holds idle at throttle 0 for 1 s, then the engine runs free at throttle 0 for 10 s.
- `limiter-free`: the dynamometer holds 1000 rpm below the top at full throttle for 0.5 s, then the engine runs free at full throttle for 5 s into its limiter.
- `simrate`, `simrate-2x`: a 3 s full-throttle hold at the hold step nearest three quarters of the top. The recipe renders `simrate-2x` from a temporary copy of the script with only the simulation frequency doubled. It is a diagnostic render, not a reference.

Idle is the idle rpm of the dump rounded to 10 rpm. For an engine that stalls at speed control 0, idle is the fallback of `tools/import`: a fifth of the redline, rounded to 50 rpm, and at least 500 rpm.

## Cycle table grid

`just cycles-grid` writes `<engine>/grid/` with `tools/cycles scenarios` from the first and last hold of `holds-t100.scn` (`docs/research/cycle-tables.md`). Do not edit these files by hand: run the recipe again.

- `t000.scn` to `t100.scn`: one file per throttle row (0, 0.25, 0.5, 0.75 and 1), with a hold at idle, at every multiple of 250 rpm and at the top. Each hold is 0.5 s of settling and at least 24 whole engine cycles, and 0.25 s sweeps join the holds.
- `cut.scn`: ignition-off holds at throttle 0 and 1 at every fourth grid RPM and the top, each after a 0.5 s hold with the ignition on. The segment `cut RPM THROTTLE SECONDS` is a hold with the ignition off (`engine-sim-render --cut`).
- `step.scn`: full throttle for 1.5 s, closed for 3 s, then full again for 3 s, at the grid RPM nearest 60 per cent of the top, for the intake lag.

Only the renderer recipe reads `cut.scn`. `snarl-render` and `snarl-compare` do not read the `cut` segment.

## Full-range holds

`just ref-fullrange` writes `fullrange-*.scn` in each engine directory with `tools/scorecard --write-fullrange` (`docs/research/fullrange-scorecard.md`). Do not edit these files by hand: run the recipe again.

- `fullrange-t000.scn` to `fullrange-t100.scn`: one file per throttle row of the cycle table grid, with a hold at idle (the first hold of `holds-t100.scn`) and at every multiple of 250 rpm below 0.998 of the limiter rpm of the profile. Each hold is 0.5 s of settling and at least 24 whole engine cycles and 1 s, and 0.25 s sweeps join the holds.
- `fullrange-mid-t100.scn`: full-throttle holds halfway between the rpm of the other files, where playback blends two neighbouring grid points.

`tools/scorecard` scores the hold windows of these files only, after the 0.5 s of settling.

## The free segment

`idle-free.scn` and `limiter-free.scn` use the segment `free THROTTLE SECONDS`: the dynamometer is off and the engine runs free. Only the engine-sim renderer (`--free`) and `tools/fit --health` read it. It is not part of `docs/scenario-format.md`, so `snarl-render` and `snarl-compare` do not read these two files.
