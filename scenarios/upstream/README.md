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

## The free segment

`idle-free.scn` and `limiter-free.scn` use the segment `free THROTTLE SECONDS`: the dynamometer is off and the engine runs free. Only the engine-sim renderer (`--free`) and `tools/fit --health` read it. It is not part of `docs/scenario-format.md`, so `snarl-render` and `snarl-compare` do not read these two files.
