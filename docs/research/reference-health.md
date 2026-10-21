# Health of the upstream engine-sim references

Measured on branch `upstream-profiles` with the engine-sim package pinned in `reference/engine-sim` (engine-sim `85f7c3b`). The five scripts are unchanged. No engine changed to make a check pass.

## Answer

Three engines are healthy and two have faults that a measurement must allow for.

- `i4-vtec`, `f4-ej25` and `v8-ls` are healthy at every load hold. Every cylinder fires on its spark angle, with no misfires. At load, the peak runner pressure varies by at most 3.3 per cent from firing to firing, and by 11 per cent for `v8-ls` at its 690 rpm idle hold.
- `v12-merlin` is healthy in every load hold. It does not start from the starter at speed control 0, but it runs at about 878 rpm when the dynamometer spins it to 600 rpm and lets it go.
- `v12-412` is not healthy below 11000 rpm. Its firings vary a lot from cycle to cycle (peak CV 0.20 to 0.43 at load), it misfires at idle (29 per cent of the firings in the free-running idle), and its 5000 Hz simulation is too coarse at high RPM (the runner peak rises by 17 per cent at 10000 Hz).
- `i4-vtec` turns the other way (starter speed -500 rpm). It fires 1-3-4-2, not the 1-2-4-3 that its spark angles give. A tool that reads the firing order from the dump spark angles gets this engine wrong.

The closed-throttle holds of `v8-ls`, `v12-412` and `v12-merlin` fail the phase check in some holds. The check cannot tell a real fault from a pulse that it reads on another wave in the runner. Do not use those holds for firing analysis.

## Engines

| Engine | Upstream script | Wrapper | Cylinders | Simulation (Hz) | Redline (rpm) | Rev limit (rpm) | Dump idle (rpm) | Firing order | Intervals (deg) | Exhaust systems |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `i4-vtec` | `atg-video-1/05_honda_vtec.mr` | none | 4, inline | 20000 | 8400 | 9400 | 771 | 1-3-4-2 | 180 even | 2, 12.33 m each |
| `v8-ls` | `atg-video-2/07_gm_ls.mr` | none | 8, V 90 | 10000 | 6500 | 6800 | 693 | 1-8-4-5-7-3-6-2 | 90 even | 2, 2.54 m and 4.37 m |
| `f4-ej25` | `atg-video-1/06_subaru_ej25.mr` | none | 4, flat | 20000 | 6500 | 6500 | 833 | 1-2-3-4 | 180 even | 2, 12.33 m each |
| `v12-412` | `atg-video-2/12_ferrari_412_t2.mr` | none | 12, V 75 | 5000 | 18000 | 18500 | 3815 | 1-7-5-11-3-9-6-12-2-8-4-10 | 75 and 45 | 2, 0.51 m and 1.42 m |
| `v12-merlin` | `atg-video-2/11_merlin_v12.mr` | none | 12, V 60 | 7000 | 3000 | 3500 | 0 (stalls) | 1-12-4-9-2-11-6-7-3-10-5-8 | 60 even | 2, 0.76 m and 1.78 m |

All five scripts define `public node main`, so none needed the wrapper of `tools/import/engine-sim-main.sh`. The firing order uses the cylinder numbers of the dump, from 1, which are not always the numbers of the maker.

## Method

`just ref-upstream [engine]` writes everything under `out/reference/upstream/<engine>/`: the dump `engine.json`, a WAV at `--volume 0.5` and a CSV of each scenario, the health report `health.md`, and the simulation-rate comparison `simrate-compare/`. `tools/fit --upstream-scenarios` writes the scenarios into `scenarios/upstream/<engine>/` from the dump (`scenarios/upstream/README.md`). The holds and the free runs have one CSV row per simulation step.

`tools/fit --health` reads the per-step CSVs. The crank angle is the sum of the RPM over the steps, so the check compares the cylinders with each other. Each cylinder must fire at its spark angle plus one offset that all cylinders share. The checks, with their pass limits:

| Check | Measure | Pass |
| --- | --- | --- |
| Phase | Largest offset of a cylinder from the shared offset | 10 deg or less |
| Interval | Largest error of the interval between two successive firings against the spark angles | 10 deg plus one simulation step or less |
| Missing | Expected firings with a peak below 0.3 of the median of their cylinder (misfires) | none |
| Weak | Firings with a peak below 0.75 of that median | 2 per cent of the firings or less |
| Peak CV | Firing-to-firing standard deviation of the peak runner pressure over its mean | 0.2 or less |
| Cylinder spread | Largest over smallest mean peak of a cylinder | 6 dB or less |
| Idle | Free run at throttle 0: the last second at 300 rpm or more, and an SD of at most 5 per cent of the mean | both |
| Limiter | Free run at full throttle: the largest RPM from 0.97 to 1.05 of the rev limit | in range |

The check leaves out the first 0.5 s of each hold and the first 1 s of each free run. engine-sim turns the crank in the direction of the starter, and its RPM and dynamometer use the magnitude of the crank speed. For an engine with a negative starter speed, the check therefore negates the spark angles. The per-hold tables are in `out/reference/upstream/<engine>/health.md`.

## Per engine

"Load" is the holds at throttle 0.5 and 1. "Closed" is the holds at throttle 0.

### i4-vtec

| Check | Result | Numbers |
| --- | --- | --- |
| Phase | pass | at most 0.4 deg in all 27 holds and the free idle |
| Interval | pass | at most 1.1 deg |
| Missing | pass | 0 in every hold and the free idle |
| Weak | pass | 0 |
| Peak CV | pass | load 0.014 to 0.030, closed 0.004 to 0.061, free idle 0.074 |
| Cylinder spread | pass | at most 0.47 dB |
| Idle | pass | 767 rpm, SD 23.7 rpm |
| Limiter | pass | largest 9467 rpm against 9400 rpm |
| Simulation rate | pass | see the simulation-rate table |

The crank turns the other way. With the spark angles of the dump as they are, every hold fails phase and interval by 360 degrees between cylinders 1 and 4 and cylinders 2 and 3. The runner peak at full throttle rises from 57 kPa at 5000 rpm to 152 kPa at 6000 rpm. This step is likely the VTEC cam change.

### v8-ls

| Check | Result | Numbers |
| --- | --- | --- |
| Phase | load pass, closed fail at 6000 and 6500 rpm | load at most 6.0 deg; closed 80.7 deg at 6000 rpm, 50.0 deg at 6500 rpm, at most 2.9 deg below |
| Interval | load pass, closed fail at 6000 and 6500 rpm | load at most 11.6 deg (limit 13.9 deg at 6500 rpm); closed 93.9 and 100.4 deg |
| Missing | pass | 0 |
| Weak | pass | 1 of 418 firings in the free idle, 0 in the holds |
| Peak CV | pass | load 0.008 to 0.110, closed 0.037 to 0.140 |
| Cylinder spread | pass | at most 2.25 dB |
| Idle | pass | 696 rpm, SD 9.3 rpm |
| Limiter | pass | largest 6839 rpm against 6800 rpm |
| Simulation rate | runner pass, audio changes | see the simulation-rate table |

At load the phase rises steadily with RPM, from 0.3 deg at 690 rpm to 6.0 deg at 6500 rpm. That is a time offset of 0.15 to 0.2 ms between cylinders, which can come from the different header primary lengths (0 to 0.17 m).

### f4-ej25

| Check | Result | Numbers |
| --- | --- | --- |
| Phase | pass | at most 0.3 deg |
| Interval | pass | at most 1.5 deg |
| Missing | pass | 0 |
| Weak | pass | 2 of 255 firings in the free idle, 0 in the holds |
| Peak CV | pass | load 0.003 to 0.033, closed 0.008 to 0.115 |
| Cylinder spread | pass | at most 0.42 dB |
| Idle | pass | 852 rpm, SD 25.3 rpm |
| Limiter | pass | largest 6638 rpm against 6500 rpm |
| Simulation rate | pass | see the simulation-rate table |

### v12-412

| Check | Result | Numbers |
| --- | --- | --- |
| Phase | load pass, closed fail at 5000, 6000, 10000, 15000 and 16000 rpm | load at most 0.4 deg; closed 67 to 201 deg in those holds |
| Interval | load pass, closed fail in the same holds | load at most 3.6 deg |
| Missing | fail below 6000 rpm | load 6 of 951 (throttle 0.5) and 2 of 951 (throttle 1) at 3810 rpm, 0 above; closed 265 of 952 at 3810 rpm, 346 of 1249 at 5000 rpm; free idle 976 of 3414 |
| Weak | fail from 3810 to 10000 rpm at load | 32 per cent of the firings at 3810 rpm, 29 per cent at 6000 rpm, 4.6 per cent at 10000 rpm, at most 0.9 per cent from 11000 rpm |
| Peak CV | fail from 3810 to 8000 rpm at load | load 0.43 at 3810 rpm, 0.22 at 8000 rpm, 0.08 to 0.14 from 11000 rpm; closed up to 0.66 |
| Cylinder spread | load pass | load at most 0.94 dB; closed up to 11.1 dB at 6000 rpm |
| Idle | pass | 3794 rpm, SD 66.6 rpm |
| Limiter | pass | largest 18568 rpm against 18500 rpm |
| Simulation rate | fail | see the simulation-rate table |

The script sets `burning_efficiency_randomness: 1.0`. The engine-sim default is 0.5 and the Merlin uses 0.1. This is the likely cause of the spread and of the misfires.

### v12-merlin

| Check | Result | Numbers |
| --- | --- | --- |
| Phase | load pass, closed fail at 600 rpm, free idle fail | load at most 2.1 deg; closed 96.4 deg at 600 rpm; free idle 82.7 deg |
| Interval | load pass, closed fail at 600 rpm, free idle fail | load at most 4.3 deg; closed 107.5 deg; free idle 114.2 deg |
| Missing | holds pass, free idle fail | 0 in the holds, 1 of 775 in the free idle |
| Weak | pass | 5 of 775 firings in the free idle, 0 in the holds |
| Peak CV | pass | at most 0.110 |
| Cylinder spread | pass | at most 2.89 dB |
| Idle | pass | 862 rpm, SD 27.0 rpm, from the 600 rpm hold |
| Limiter | pass | largest 3517 rpm against 3500 rpm |
| Simulation rate | runner pass, audio changes | see the simulation-rate table |

The dump idle is 0 rpm: from the starter at speed control 0, the engine does not start. The scenarios therefore use the fallback idle of `tools/import`, 600 rpm, which is below the 878 rpm at which the engine settles in the last second of the free run. Because of the rule that gives the hold steps, the holds are at 600, 2000 and 3000 rpm, with a 1400 rpm step from idle.

## Free-running idle

The dynamometer holds the scenario idle at throttle 0 for 1 s, then the engine runs free at throttle 0 for 10 s. The statistics are over the last 9 s. Wander is the largest peak of the RPM spectrum from 0.2 Hz up to a quarter of the idle firing frequency.

| Engine | Dump idle (rpm) | Mean (rpm) | SD (rpm) | Min (rpm) | Max (rpm) | Last 1 s (rpm) | Wander (Hz) | Wander amplitude (rpm) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `i4-vtec` | 771 | 767 | 23.7 | 692 | 832 | 770 | 0.59 | 15.5 |
| `v8-ls` | 693 | 696 | 9.3 | 674 | 718 | 704 | 0.22 | 5.4 |
| `f4-ej25` | 833 | 852 | 25.3 | 734 | 900 | 851 | 0.68 | 12.3 |
| `v12-412` | 3815 | 3794 | 66.6 | 3606 | 3948 | 3831 | 0.42 | 68.5 |
| `v12-merlin` | 0 | 862 | 27.0 | 759 | 879 | 878 | 0.22 | 4.2 |

A wander at 0.22 Hz is at the lowest frequency of the spectrum over 9 s, so for `v8-ls` and `v12-merlin` it is a slow drift and not a periodic hunt. For `v12-merlin` it is the rise from the 600 rpm hold to about 878 rpm.

The full-throttle free runs bounce off the limiter at 3.7 Hz (`v8-ls`), 6.4 Hz (`v12-412`), 7.5 Hz (`f4-ej25`), 10.3 Hz (`i4-vtec`) and 10.5 Hz (`v12-merlin`). The `v12-412` bounce is the largest, 15404 to 18568 rpm, from its 0.1 s limiter cut with no load.

## Simulation rate

The recipe renders the `simrate` hold (full throttle at the hold step nearest three quarters of the top) twice: as shipped, and from a temporary copy of the script with only the `simulation_frequency` literal doubled. The chance distance is the same comparison between the `simrate` render and the same 3 s hold cut from `holds-t100`, at the same rate. It shows how far two renders of one state differ by chance.

| Engine | Hold (rpm) | Rate (Hz) | Step at the hold (deg) | Runner peak 1x / 2x (kPa) | Loudness 1x / 2x (LUFS) | Third-octave distance 2x / chance (dB) | Order distance 2x / chance (dB) | Result |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `i4-vtec` | 6000 | 20000 | 1.8 | 152.5 / 155.4 | -11.77 / -11.76 | 0.24 / 0.45 | 0.26 / 0.14 | pass |
| `v8-ls` | 5000 | 10000 | 3.0 | 38.0 / 37.9 | -12.25 / -12.74 | 3.63 / 0.35 | 4.37 / 0.35 | runner pass, audio changes |
| `f4-ej25` | 5000 | 20000 | 1.5 | 138.5 / 140.6 | -10.85 / -10.84 | 0.19 / 0.32 | 0.25 / 0.28 | pass |
| `v12-412` | 13000 | 5000 | 15.6 | 60.6 / 70.7 | -12.78 / -12.69 | 1.08 / 0.36 | 1.29 / 0.41 | fail |
| `v12-merlin` | 2000 | 7000 | 1.7 | 31.3 / 31.1 | -12.34 / -12.53 | 1.66 / 0.20 | 5.41 / 0.46 | runner pass, audio changes |

The runner pressure is the simulation itself. It stays within 2 per cent at the doubled rate for four engines. For `v12-412` it rises by 17 per cent, so 5000 Hz does not resolve this engine at 13000 rpm. At 18000 rpm one step is 21.6 deg, and the 45 deg firing interval is about two steps.

The loudness hardly changes because the engine-sim leveller holds the output level. The audio test is not clean. engine-sim sets the exhaust delay filter for a 10 kHz step rate for every engine (`m_delayFilters[i].initialize(delay, 10000.0)`), so a doubled rate halves every exhaust delay. For `i4-vtec` and `f4-ej25`, whose two exhaust systems have the same length, the halving moves every path together, and the spectrum stays within chance. For `v8-ls`, `v12-merlin` and `v12-412`, whose exhaust systems differ in length, it changes the delay between the paths and therefore the spectrum. Their audio change at the doubled rate comes mostly from this defect. It does not show that their own rate is too low.

## Unhealthy

1. `v12-412` below 11000 rpm: 5 to 32 per cent weak firings and a peak CV of 0.20 to 0.43 at load, misfires at 3810 rpm, and about 28 per cent misfires at closed throttle up to 5000 rpm and in the free idle. The upstream `burning_efficiency_randomness: 1.0` is the likely cause. For a steady reference of this engine, measure at 11000 rpm or more, or average over many cycles.
2. `v12-412` simulation rate: 5000 Hz is too low at high RPM (runner peak +17 per cent at 10000 Hz, 15.6 deg per step at 13000 rpm).
3. `i4-vtec` rotation: the starter speed is -500 rpm, so the engine fires 1-3-4-2 in time, the reverse of its spark angles. `tools/import` takes the crank angle of each cylinder from the dump spark angle (`wrap720(c.SparkDeg - first)`) with no check of the starter sign, so the catalogue entry of this script likely has the reversed order. With two exhaust systems, the order decides which path each firing feeds.
4. `v12-merlin` start: the engine does not start from the starter at speed control 0, and its scenario idle (600 rpm) is below the 878 rpm at which it runs free.
5. Closed-throttle phase: in some closed-throttle holds of `v8-ls` (6000 and 6500 rpm), `v12-412` (5000, 6000, 10000, 15000, 16000 rpm) and `v12-merlin` (600 rpm and the free idle), the pulses of one or more cylinders are 50 to 210 deg from the others. Their runner peaks are 0.3 to 6.5 kPa, against 6.9 to 171 kPa at load. The firings stay regular (cylinder CV at most 0.2 except where the `v12-412` misfires), so the check likely reads the onset on another wave in the runner. The result is not conclusive.

## Limits of the check

- The crank angle has no fixed origin, so the check tests the cylinders against each other. An error that moves every cylinder by the same angle does not show.
- The free-segment scenarios (`idle-free`, `limiter-free`) use the `free` segment of the engine-sim renderer. `snarl-render` and `snarl-compare` do not read them.
- The chance distances come from one pair of renders per engine.

## Reproduce

```sh
nix develop -c just ref-upstream            # all five engines
nix develop -c just ref-upstream v12-412    # one engine
```

The chance comparison is not in the recipe. For `v12-412`, the 13000 rpm hold is the tenth hold of `holds-t100`, which starts at 31.5 s:

```sh
ffmpeg -nostdin -y -ss 31.5 -t 3 -i out/reference/upstream/v12-412/holds-t100.wav out/reference/upstream/v12-412/simrate-baseline.wav
go run ./cmd/snarl-compare --spectrograms=false --scenario scenarios/upstream/v12-412/simrate.scn \
  --out out/reference/upstream/v12-412/simrate-baseline-compare \
  out/reference/upstream/v12-412/simrate.wav out/reference/upstream/v12-412/simrate-baseline.wav
```

The other start times are 17.5 s (`i4-vtec`, 6000 rpm), 14 s (`v8-ls` and `f4-ej25`, 5000 rpm) and 3.5 s (`v12-merlin`, 2000 rpm).
