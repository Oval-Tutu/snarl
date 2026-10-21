# EngineLab offline audio exporter

`EngineLabOfflineAudioExporter` is the headless renderer of [EngineLab](https://github.com/zolaski333/EngineLab). It runs an engine from the EngineLab catalogue through a JSON scenario, with the same simulation and audio path as the EngineLab application, and writes WAV files faster than real time. Upstream builds it on Windows only. This directory builds it on Linux from the unchanged upstream source, for snarl reference renders.

## Build

```sh
nix build -f reference/enginelab
```

The build uses the nixpkgs revision in the repository `flake.lock`. It fetches EngineLab at commit `d345ba4` (18 October 2026, the same commit as `tools/import/enginelab.nix`). It also fetches the three dependencies at the tags that the upstream `CMakeLists.txt` declares: JUCE `8.0.13`, nlohmann/json `v3.12.0` and yaml-cpp `0.8.0`. CMake uses these sources through `FETCHCONTENT_SOURCE_DIR_*` with `FETCHCONTENT_FULLY_DISCONNECTED=ON`, so the build has no network access.

The build sets `ENGINELAB_BUILD_TESTS=OFF` and `ENGINELAB_ENABLE_WARNINGS_AS_ERRORS=OFF`, and builds only the `EngineLabOfflineAudioExporter` target. JUCE needs the ALSA, X11, FreeType and Fontconfig headers at configure time and at compile time. The exporter does not open an audio device or a window.

The output contains:

- `bin/EngineLabOfflineAudioExporter`
- `share/enginelab/`: the upstream `engines/`, `parts/`, `assets/` and `voicing/` directories. Pass this directory as `--catalog-root`.
- `share/doc/enginelab/`: `LICENSE.md` and `THIRD_PARTY_NOTICES.md`.

To expose the package in the flake, add this line to the `packages` attribute set in `flake.nix`, beside `engine-sim-render`:

```nix
enginelab-offline-audio-exporter = nixpkgs.legacyPackages.${system}.callPackage ./reference/enginelab/package.nix { };
```

## Upstream changes

There are no patches. The simulation, audio and tool sources are unchanged. The only build change is the compiler flag `-include cstdint`, because yaml-cpp 0.8.0 uses `uint16_t` without `<cstdint>`, and GCC 13 and later reject that.

The binary has the build directory compiled in as its default catalogue root. Always pass `--catalog-root`.

## Usage

```sh
R=$(nix build -f reference/enginelab --print-out-paths --no-link)
$R/bin/EngineLabOfflineAudioExporter --catalog-root $R/share/enginelab --list-engines
$R/bin/EngineLabOfflineAudioExporter --catalog-root $R/share/enginelab \
  --engine "Aircooled-like 3.6 Flat-6" --scenario reference/enginelab/scenarios/sweep-wot.json \
  --output /tmp/f6-sweep --sample-rate 48000 --format float32 --volume 0.5 --stems
```

| Option | Meaning |
| --- | --- |
| `--engine NAME` | A substring that matches exactly one catalogue engine name. |
| `--output DIR` | A new or empty directory. Add `--overwrite` to replace earlier files. |
| `--scenario FILE` | A scenario in the EngineLab JSON format, schema version 1. Without it, the exporter uses its showcase: crank, idle, rev-up, limiter and overrun. |
| `--sample-rate HZ` | 48000, 96000 or 192000. The default is 96000. |
| `--format pcm24\|float32` | The default is `pcm24`. The snarl WAV reader reads `float32` and 16-bit PCM only. |
| `--stems`, `--no-stems` | Write the stems and the engine order map, or the master only. The default is `--stems`. |
| `--volume GAIN` | The master gain, 0 to 2. At the catalogue default of 1, the flat-6 exceeds full scale and the output limiter acts on it. Use 0.5 for clean references. |

The exporter writes stereo files: `master.wav`, eight stems (`stem_combustion`, `stem_exhaust_dry`, `stem_exhaust_ir`, `stem_intake`, `stem_forced_induction`, `stem_mechanical`, `stem_exhaust_pressure_wave`, `stem_exhaust_jet`), `premaster.wav` and `master_processing_delta.wav`. `engine-order-map.csv` has the columns `time_s`, `rpm`, `order`, `frequency_hz` and `level_dbfs` for half orders 0.5 to 24 of the master, every 1/24 s, with the simulated RPM. `render-manifest.json` has the limiter counters and, for each stage, the requested RPM and the mean, minimum, maximum and settled RPM that the simulator reached. The exporter does not write a per-cylinder exhaust pressure. The closest signals are the exhaust stems and the order map.

### Scenario format

A scenario is a list of stages. Each stage has all of these fields: `name`, `duration_seconds`, `ignition`, `starter`, `governed`, `throttle_start`, `throttle_end`, `load_start`, `load_end`, `target_rpm_start` and `target_rpm_end`. The throttle, the load and the target RPM change linearly over the stage. A scenario is at most 120 s and 128 stages long.

With `governed: true`, a dynamometer follows the target RPM. It can only brake: it adds load when the engine is faster than the target, and it cannot drive the engine. With `governed: false`, the load fields apply directly, and a load of 0 lets the engine run free. The governor integrator carries over from one governed stage to the next, and an ungoverned stage resets it.

### Flat-6 references

`just ref-enginelab` renders each scenario in `scenarios/` for the Aircooled-like 3.6 Flat-6 (snarl catalogue id `el-f6-aircooled`) at 48 kHz, 32-bit float and `--volume 0.5`. It writes the exporter output to `out/reference/enginelab/f6-aircooled/<scenario>/`. Beside it, `<scenario>.wav` is the master without the stages whose names start with `preroll_`. That file matches the snarl scenario in `scenarios/upstream/f6-aircooled/<scenario>.scn`. The stems and the order map keep the pre-roll, so subtract the pre-roll from their times.

| Scenario | Pre-roll | Content |
| --- | --- | --- |
| `holds-t050` | 6.5 s | 3 s holds at 780, 2000, 3000, 4000, 5000, 6000 and 7000 rpm, throttle 0.5, with 0.5 s sweeps between them. |
| `holds-t100` | 6.5 s | The same holds at throttle 1. |
| `sweep-wot` | 6.5 s | A full-throttle sweep from 780 to 7000 rpm over 8 s. |
| `overrun` | 11 s | 2 s at 7000 rpm and throttle 1, then 6 s at throttle 0 with the engine running free. The engine reaches idle about 4 s after the throttle closes. |
| `idle-free` | 8.5 s | 6 s of free running at throttle 0, under the EngineLab idle control. |

Each pre-roll cranks the engine for 1.5 s and lets it idle. The holds and the sweep then hold 780 rpm at their throttle for 1.5 s, so that the governor settles. The overrun runs up from 780 to 7000 rpm at full throttle over 6 s. The idle waits 7 s, so that the after-start flare decays.

Measured from `render-manifest.json`, the holds settle within 25 rpm of their targets in their last quarter. The 0.5 s sweeps are faster than the engine can follow. In `holds-t100`, a hold starts up to 320 rpm below its target and overshoots it by up to 310 rpm before it settles. The free idle runs at 766 to 784 rpm. No render uses the output limiter or the hard clamp.

Some references are not possible with the unchanged exporter:

- Holds at throttle 0 above idle. The dynamometer cannot drive the engine, so a closed-throttle hold at 3000 rpm falls to idle (776 rpm settled).
- A run into the limiter. At full throttle with no load, the flat-6 reaches about 7240 rpm, below its 7400 rpm ignition limit. A limiter reference needs a copy of the engine file with a lower `rev_limit_rpm`.
- A top hold at the 7600 rpm redline, for the same reason. The holds and the sweep stop at 7000 rpm.

## Licences

- EngineLab is MIT, copyright 2026 EngineLab Contributors (`LICENSE.md` in the source).
- JUCE is AGPLv3, or the commercial JUCE 8 licence. The exporter links JUCE, so the binary is under the AGPLv3.
- nlohmann/json and yaml-cpp are MIT.
- Use this build for internal reference renders only. Do not ship the binary, or the EngineLab impulse responses in `assets/ir/`, in snarl or in the game.
- The Nix files, the scenarios and this README are part of snarl.
