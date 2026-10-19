# engine-sim headless reference renderer

`engine-sim-render` runs an [engine-sim](https://github.com/ange-yaghi/engine-sim) engine script offline on Linux. It holds or sweeps the engine speed with the engine-sim dynamometer, sets the throttle, and writes the synthesiser output as a mono 16-bit WAV file. It does not use an audio device or a window, and it runs faster than real time for most engines. The output is reference audio for the snarl model.

## Build

```sh
nix build -f reference/engine-sim
```

The build uses the nixpkgs revision in the repository `flake.lock`. It fetches engine-sim at commit `85f7c3b` (22 January 2023) and three of its submodules (`simple-2d-constraint-solver`, `csv-io`, `piranha`) at their pinned commits. It skips `delta-studio` and `direct-to-video`, because the renderer does not use them.

The output contains:

- `bin/engine-sim-render`
- `share/engine-sim/es/`: the engine-sim script library and impulse responses. The binary uses this directory by default.
- `share/engine-sim/engines/`: the engine scripts from upstream `assets/engines/`.

## Usage

```sh
R=$(nix build -f reference/engine-sim --print-out-paths --no-link)
E=$R/share/engine-sim/engines

# Cross-plane V8 at 3000 rpm, half throttle, 3 seconds.
$R/bin/engine-sim-render --script $E/atg-video-2/07_gm_ls.mr \
  --hold 3000:0.5:3 --volume 0.5 --out v8_3000.wav

# Full-throttle sweep from 1500 to 5500 rpm over 8 seconds, then 2 seconds closed throttle at 5500 rpm,
# with a per-step CSV of internal signals (every 10th step).
$R/bin/engine-sim-render --script $E/atg-video-2/07_gm_ls.mr \
  --sweep 1500:5500:1:8 --hold 5500:0:2 --volume 0.5 \
  --out v8_sweep.wav --csv v8_sweep.csv --csv-every 10
```

### Scenario format

A scenario is a list of segments on the command line. The segments run in the given order.

| Flag | Meaning |
| --- | --- |
| `--hold RPM:THROTTLE:SECONDS` | The dynamometer holds the engine at `RPM`. |
| `--sweep RPM0:RPM1:THROTTLE:SECONDS` | The dynamometer target moves linearly from `RPM0` to `RPM1`. |
| `--free THROTTLE:SECONDS` | The dynamometer is off and the engine runs free, in neutral. |

`THROTTLE` is the engine-sim speed control value in 0 to 1. This is the value that the app sets with the Q, W, E and R keys (0.01, 0.1, 0.2, 1.0). Engines with a governor interpret it as a speed request, other engines as a throttle position.

| Option | Default | Meaning |
| --- | --- | --- |
| `--settle SECONDS` | 3 | Pre-roll that is not written to the WAV. The starter cranks for the first 0.5 seconds, then the first segment's control applies. |
| `--volume GAIN` | 1 | Synthesiser gain after its leveller. At 1, as in the app, engine-sim clips a few per cent of samples. Use 0.5 for clean references. |
| `--rate HZ` | 48000 | Output sample rate. The synthesiser runs at 44100 Hz. The renderer converts with a windowed-sinc filter. Use 44100 for the unconverted output. |
| `--csv PATH` | none | Per-step CSV: `time_s` (0 at the end of the pre-roll), `rpm`, `throttle`, `dyno_torque_nm`, then `runner_pressure_pa_N` (the exhaust runner pressure that feeds the sound) and `exhaust_flow_mol_N` for each cylinder. |
| `--csv-every N` | 1 | Write every Nth simulation step to the CSV. |
| `--data-dir DIR` | build output | Directory that contains `es/`. |

### Engine dump

`--dump-json PATH` writes the engine that the script builds as JSON instead of a WAV: the redline, rev limit and limiter duration, the simulation frequency, the crankshafts with their throws and journal angles, the banks with their angles and bores, the intakes with their runner lengths, the exhaust systems with their lengths, primary tube lengths, collector areas, audio volumes and impulse responses, and for each cylinder its bank, journal angle, spark plug angle in the 720 degree cycle, bore, stroke, rod length, compression ratio, exhaust system, intake, header primary length and sound attenuation. Lengths are in metres, angles in degrees and speeds in rpm.

The dump also holds an idle rpm. The engine runs free at speed control 0 for 8 s (`--idle-seconds`), with the starter on until the engine runs at twice the starter speed, for at most 3 s. The idle rpm is the mean rpm of the last 2 s. It is near 0 for an engine that stalls. `--dump-json` takes no `--out`, `--csv` or segments. `just import-profiles` uses it for the profile catalogue (`profiles/catalogue/README.md`).

### Limiter reference

`just ref-limiter [profile]` renders the limiter reference for one profile, or for all four profiles, into `out/reference/engine-sim/<profile>/limiter.wav` with a CSV beside it (every 10th step). It reads the start rpm and the limiter rpm from the first sweep in `scenarios/<profile>/limiter.scn`. The dynamometer holds the start rpm at full throttle for 0.5 s (`--hold START:1:0.5`). Then the engine runs free at full throttle for 5 s (`--free 1:5`). The engine reaches its limiter about 0.1 s into the free run and bounces off it for about 4.7 s.

The upstream inline4 script `atg-video-1/05_honda_vtec.mr` cuts at 9400 rpm. The snarl `inline4` profile cuts at 8500 rpm. The recipe renders the inline4 reference from a temporary copy of the upstream script with `rev_limit` changed to 8500 rpm. The rev limit is a literal inside the engine node, so a local wrapper script cannot override it. `just ref-set` still uses the unchanged upstream script, because its holds stay below 8500 rpm.

In a free run the engine has no vehicle load, so the RPM dip at each cut is larger than in gear.

The script must be an engine-sim engine file that defines `public node main` and calls `set_engine`, `set_vehicle` and `set_transmission`, as the upstream files in `engines/` do. The renderer imports it by absolute path, so a local copy (for example a flat-6 derived from the EJ25) works.

Closest upstream scripts for the snarl profiles:

| snarl profile | Script |
| --- | --- |
| Inline-4 | `atg-video-1/05_honda_vtec.mr`, `atg-video-1/04_hayabusa.mr` |
| Cross-plane V8 | `atg-video-2/07_gm_ls.mr`, `chevrolet/chev_truck_454.mr` |
| Flat-4 (EJ25) | `atg-video-2/01_subaru_ej25_eh.mr`, `atg-video-2/02_subaru_ej25_uh.mr` |
| V12 | `atg-video-2/12_ferrari_412_t2.mr`, `atg-video-2/11_merlin_v12.mr` |

## Upstream changes

`CMakeLists.txt` in this directory replaces the upstream build. The upstream build needs delta-studio, a Windows-only Discord library and network access for googletest. The patches in `patches/` fix Windows and MSVC dependencies in the upstream sources:

- `0001`: removes the unused delta-studio include from the synthesiser, removes backslash include paths, makes the header constants `inline constexpr` (they were multiply defined), moves `GasSystem::Mix` out of the class for Clang, fixes a typo in `RingBuffer::overwrite` and a shadowed template parameter, adds missing standard includes. It also removes the unused `lastValveLift[8]` array in `PistonEngineSimulator::writeToSynthesizer`, which engines with more than eight cylinders wrote past.
- `0002` (piranha): replaces `__int64`, and replaces `is_complete` that Boost removed.
- `0003` (simple-2d-constraint-solver): adds missing includes.
- `0004`: adds read accessors for the spark plug angles, the rev limit, the limiter duration and the idle throttle plate position, for `--dump-json`.
- `0005`: sets the synthesiser latency to 0 in its constructor. Upstream reads it uninitialised in the first frame, so the first frame had a random step count, and an engine near a stall at idle could stall or run depending on the memory layout.

The build uses Clang. Upstream uses explicit template specialisations inside class scope, which GCC rejects.

The renderer keeps engine-sim behaviour otherwise, including its known defects. For example, the exhaust delay filter assumes a 10 kHz step rate for all engines.

## Licences

- engine-sim, simple-2d-constraint-solver and csv-io are MIT, copyright Ange Yaghi.
- piranha (Ange Yaghi) has no licence file at the pinned commit. The build fetches it from GitHub and does not redistribute it.
- The impulse responses in `es/sound-library/` have no separate licence statement, and their source is not documented. Use them for internal reference renders only. Do not ship them, or audio rendered with them, in snarl or the game.
- The driver and build files in this directory are part of snarl.
