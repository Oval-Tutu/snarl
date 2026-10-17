# engine-sim: audio internals, reference for snarl

Research date: 2026-10-17. Source: shallow clones of `ange-yaghi/engine-sim` (branch `master`, commit `85f7c3b`, 22 January 2023) and `Engine-Simulator/engine-sim-community-edition` (commit `4e5c20d`, 11 September 2025). File paths below are relative to the engine-sim repository root unless stated. Git submodules (`delta-studio`, `simple-2d-constraint-solver`, `csv-io`, `piranha`, `direct-to-video`) were not fetched, so claims about their contents are marked unverified.

## Summary

The sound comes from one scalar per exhaust system per simulation step: a scaled exhaust runner pressure, delayed by the pipe length, then summed per exhaust system. The `Synthesizer` class resamples that signal to 44.1 kHz and runs a short per-sample chain: jitter, DC removal, derivative mix, multiplicative noise, convolution with a recorded impulse response (IR), a low pass and a peak-following leveller. The synthesiser is small (about 330 lines plus six filter classes), separable, and easy to port. The convolution is the only expensive stage: up to 10,000 taps per exhaust channel per output sample in direct form. The physics that feeds the synthesiser is heavy (rigid-body solver plus eight gas substeps per step at 10 to 40 kHz) and is not needed in snarl. Nothing in the code models afterfire, backfire or overrun pops. The code is MIT. The impulse response WAV files carry no separate licence statement, so they fall under the repository MIT licence by default, but the recordings' provenance is not documented.

## 1. Signal chain

| Stage | Class, file | What it does |
| --- | --- | --- |
| Combustion | `CombustionChamber::ignite`, `src/combustion_chamber.cpp:169-224` | Starts a flame event when the ignition module fires and the equivalence ratio is between 0.5 and 1.9. Burn efficiency has a random term (`burning_efficiency_randomness`). |
| Cylinder gas flow | `CombustionChamber::flow`, `src/combustion_chamber.cpp:235-355` | Four `GasSystem::flow` calls per cylinder per fluid substep: plenum to intake runner, runner to cylinder, cylinder to exhaust runner (`m_exhaustRunnerAndPrimary`), exhaust runner to collector. |
| Exhaust collector | `ExhaustSystem::process`, `src/exhaust_system.cpp:60-81` | One `GasSystem::flow` from collector to a 1000 m3 atmosphere, then velocity decay. |
| Sound tap | `PistonEngineSimulator::writeToSynthesizer`, `src/piston_engine_simulator.cpp:370-413` | Per cylinder: `exhaustFlow = attenuation^3 * 1600 * (P_runner - 1 atm + 0.1 * dynamicPressure(+x) + 0.1 * dynamicPressure(-x))` (lines 394-398). `attenuation` is filtered RPM clamped to 40, so the engine fades in from standstill. The value passes through a per-cylinder `DelayFilter` (line 402-403) and is scaled by `sound_attenuation`, `audio_volume / cylinderCount` and `1 / exhaustLength^2` (lines 405-409), then summed into one slot per exhaust system. `synthesizer().writeInput(...)` is called once per simulation step (line 412). |
| Pipe delay | `DelayFilter`, `include/delay_filter.h:32-44` | Delay = (header primary length + exhaust length) / 343 m/s, created at a fixed 10,000 Hz rate (`src/piston_engine_simulator.cpp:219-226`). The rate is hard-coded and does not follow `simulation_frequency`, so the delay is wrong for engines that run at 20 to 40 kHz. |
| Resample and gate | `Synthesizer::writeInput`, `src/synthesizer.cpp:168-195` | Linear interpolation from the simulation rate to 44.1 kHz, then a fourth-order Butterworth low pass at 1900 Hz per input channel (`src/synthesizer.cpp:73`). |
| Per-sample synthesis | `Synthesizer::renderAudio(int)`, `src/synthesizer.cpp:281-331` | See section 3. |
| Output | `Synthesizer::readAudioOutput`, `src/synthesizer.cpp:141-159`, consumed by `EngineSimApplication::process`, `src/engine_sim_application.cpp:253-305` | int16 mono 44.1 kHz into a delta-studio `ysAudioSource` ring buffer. |

The sound tap is pressure, not mass flow. `CombustionChamber::getLastTimestepExhaustFlow` exists (`include/combustion_chamber.h:76`) but only feeds `Simulator::getTotalExhaustFlow`, which the audio path does not use. Note the static `lastValveLift[8]` array at `src/piston_engine_simulator.cpp:379` and 400: engines with more than eight cylinders (V10, V12) write past the end of the array. The value is never read.

## 2. Step rate, feeding, buffering, latency

- Simulation rate: `Engine::Parameters::initialSimulationFrequency`, set per engine script by `simulation_frequency` (`es/objects/objects.mr:113`, default 10,000). Shipped profiles use 5,000 (Ferrari V12) to 40,000 (Honda single). `Simulator` default is 10,000 (`src/simulator.cpp:13`). The user can change it live between 400 and 400,000 Hz (`src/engine_sim_application.cpp:753-757`).
- Fluid substeps: 8 per simulation step (`src/engine.cpp:393`, `src/piston_engine_simulator.cpp:26`), so gas flow runs at 80 to 320 kHz.
- Frame loop: `startFrame(1 / avgFramerate)` computes `m_steps = round(dt * simSpeed / timestep)` then adjusts by plus or minus 10 per cent to keep the synthesiser input queue near the target latency (`src/simulator.cpp:64-93`). `simulateStep()` runs the solver, engine, vehicle, transmission, combustion and gas, then `writeToSynthesizer()` (`src/simulator.cpp:95-153`). `endFrame()` calls `Synthesizer::endInputBlock` which wakes the render thread (`src/simulator.cpp:164`, `src/synthesizer.cpp:197-213`).
- Synthesiser thread: `Synthesizer::renderAudio()` waits for input and renders until the output ring holds 2000 samples (45 ms) (`src/synthesizer.cpp:222-256`).
- Latency budget by design: target synthesiser input latency 0.1 s (`src/simulator.cpp:12`), output ring up to 45 ms, and the application keeps the DirectSound write pointer 0.1 s ahead of the play cursor (`src/engine_sim_application.cpp:257`, resync at 0.5 s lead, line 263-267). Control to sound is therefore about 200 to 250 ms plus the physical pipe delay (about 6 ms for 2 m). This is a consequence of the frame-driven producer and generous buffers, not of the synthesis. The per-sample `renderAudio(int)` has no block dependency and can run at any buffer size.
- Input and output ring buffers are 44,100 samples each (`src/simulator.cpp:204-212`). `RingBuffer<T>` (`include/ring_buffer.h`) is a plain array ring.

## 3. The synthesiser stage in detail

Per exhaust channel, per 44.1 kHz output sample (`src/synthesizer.cpp:281-331`):

1. `JitterFilter::fast_f` (`include/jitter_filter.h:22-47`): a 10-sample history. A uniform random offset in [0, 9], scaled by `inputSampleNoise` (default 0.5), is low-passed by a fourth-order Butterworth at `inputSampleNoiseFrequencyCutoff` (10 kHz) and used as a fractional read position with linear interpolation. This is random delay modulation, which adds high-frequency "fizz" and breaks periodicity.
2. DC removal: `f = f_in - LowPassFilter(f_in)` with a 10 Hz one-pole (`src/synthesizer.cpp:65`, `include/low_pass_filter.h:15-20`).
3. Derivative: `f_p = (f_in - previous) / dt` (`include/derivative_filter.h`, `src/derivative_filter.cpp:12-17`).
4. Air noise: white noise from `rand()`, Butterworth low pass at `airNoiseFrequencyCutoff` (2 kHz), mixed as `r_mixed = airNoise * r + (1 - airNoise)`. The filter object used is always `m_filters[0]` (`src/synthesizer.cpp:299-300`, uses `m_filters->` instead of `m_filters[i].`), so all channels share one noise filter state.
5. Mix: `v_in = f_p * dF_F_mix + f * r_mixed * (1 - dF_F_mix)` with `dF_F_mix` default 0.01 (`hf_gain` in scripts). The multiplication of the pulse by low-passed noise is amplitude modulation, so it spreads each pulse into a band of noise. Subnormals are flushed to zero.
6. Convolution: `v = convolution * ConvolutionFilter(v_in) + (1 - convolution) * v_in`. `ConvolutionFilter::f` (`src/convolution_filter.cpp:37-52`) is a direct-form FIR over a circular shift register. The IR is loaded from a 16-bit mono 44.1 kHz WAV, trimmed to the last sample with magnitude above 100, capped at 10,000 taps (227 ms), scaled by `volume / INT16_MAX` (`src/synthesizer.cpp:86-105`). All shipped IRs exceed 10,000 samples (file sizes 29 to 97 kB), so every engine runs with 10,000 taps per exhaust channel.

Then across channels: sum, fourth-order Butterworth at 0.45 x sample rate (`src/synthesizer.cpp:79`, 318), `LevelingFilter` (`src/leveling_filter.cpp:17-34`): peak tracker with decay 0.999 per sample, gain = clamp(target / peak, 1e-5, 1.9), smoothed by 0.9/0.1, target 30,000 (int16 scale). Then `volume`, round, clamp to int16.

All filters are `float` except the input antialiasing Butterworth, and `ButterworthLowPassFilter::setCutoffFrequency` (`include/butterworth_low_pass_filter.h:56-71`) gives the closed-form fourth-order coefficients.

CPU cost per output sample per channel: the convolution is 10,000 multiply-adds. Everything else is under about 60 floating-point operations, two `rand()` calls and one `std::uniform_real_distribution` draw. For two exhaust channels at 44.1 kHz the convolution is 882 million multiply-adds per second, which on its own exceeds snarl's 5 per cent of one mobile core. The rest of the chain is well under 1 per cent.

Portability verdict: the synthesiser is cheap and separable apart from the convolution length. It depends only on its own filter headers, `<random>`, `<thread>`, `<mutex>` and an unused `#include "../include/delta.h"` (`src/synthesizer.cpp:4`). Three filter headers use the MSVC keyword `__forceinline` (`include/low_pass_filter.h:15`, `include/jitter_filter.h:22`, `include/butterworth_low_pass_filter.h:28`).

## 4. Audible and inaudible physics, with cost

| Part | Audible effect | Cost per simulation step | Keep for snarl? |
| --- | --- | --- | --- |
| Rigid-body solver, `atg_scs::OptimizedNsvRigidBodySystem` + Gauss-Seidel (`src/simulator.cpp:28-34`, `src/piston_engine_simulator.cpp:39-186`) | Produces crank angle and speed, which set pulse timing, RPM flutter and load response. The sound tap does not read piston kinematics directly. | Bodies: 1 per crank + 2 per cylinder + vehicle mass. Constraints: 3 per cylinder + crank fixes + friction + dyno + starter + clutch. Solver internals unverified (submodule not fetched). | No. Replace with a scalar crank model: angle, speed, inertia, torque from game RPM and load. |
| Ignition module (`src/ignition_module.cpp:53-100`) | Firing order and timing. Rev limiter sets `m_revLimitTimer` for `limiter_duration` when speed exceeds `rev_limit` and skips all sparks during that time (lines 88-95). | Trivial. | Yes, as an event scheduler. |
| Combustion chamber flame model (`src/combustion_chamber.cpp:169-224`, 315-353) | Pressure rise shape, random burn efficiency gives pulse-to-pulse variation. | A few dozen operations per lit cylinder per substep. | Only the outcome: per-firing amplitude with a random term. |
| Gas system, `GasSystem::flow` (`src/gas_system.cpp:352-520`, `flowRate` 183-230) | The exhaust runner pressure is the sound. Choked flow, momentum and velocity decay shape the pulse edge and the resonances. | Each call: two `pow`, two or three `sqrt`, energy bookkeeping. Per step at 8 substeps for a V8: 8 x (4 x 8 cylinder flows + 2 exhaust + 1 intake) = 280 calls, 2.8 million per second at 10 kHz. | No. Replace with a per-cylinder pulse shape table indexed by load and RPM. |
| Intake plenum and runners (`src/intake.cpp`, `m_intakeRunnerAndManifold`) | Sets charge mass and so pulse amplitude under throttle. Intake noise is not tapped for audio. | Included in the above. | No. Map throttle and load to amplitude. |
| Exhaust pipe model (`ExhaustSystem`, `DelayFilter`, `1 / exhaustLength^2`, `primary_length` per cylinder) | Unequal header lengths produce the Subaru "rumble" by offsetting cylinder pulses (`02_subaru_ej25_uh.mr`). The collector `GasSystem` is a single lumped volume, so there are no standing-wave resonances in the physics; the IR supplies them. | One `GasSystem::flow` per exhaust per substep plus one delay per cylinder per step. | Yes, as per-cylinder delay lines and per-bank sum. Cheap. |
| Valvetrain and camshaft (`src/standard_valvetrain.cpp`, `src/camshaft.cpp`) | Sets exhaust port flow window via `exhaustFlowRate(lift)` (`src/cylinder_head.cpp:56-59`), so blowdown timing and pulse width. | `Function::sampleTriangle` lookups per cylinder per step. | Only as a pulse width parameter. |
| Vehicle, transmission, dyno, starter | Change RPM trajectory only. | Small. | No. The game supplies RPM. |

## 5. Engine definition format and shipped profiles

Scripts are Piranha `.mr` files. `assets/main.mr` imports `engine_sim.mr`, a theme, one engine file, then calls `main()`. The `es/` directory is the standard library (`es/objects/objects.mr` defines the nodes, `es/actions/actions.mr` the builder actions).

Parameters that define the sound:

| Node, file | Parameter | Effect |
| --- | --- | --- |
| `engine`, `es/objects/objects.mr:97-118` | `simulation_frequency`, `hf_gain`, `jitter`, `noise` | Step rate, `dF_F_mix`, `inputSampleNoise`, `airNoise` (`src/engine_sim_application.cpp:482-488`). |
| `exhaust_system`, `es/objects/objects.mr:545-578` | `length`, `collector_cross_section_area`, `outlet_flow_rate`, `primary_tube_length`, `primary_flow_rate`, `audio_volume`, `velocity_decay`, `impulse_response` | Pipe delay, collector volume, outlet restriction, per-system gain, and the IR. |
| `impulse_response`, `es/objects/objects.mr:539-543` | `filename`, `volume` | IR WAV and gain. Library in `es/sound-library/impulse_responses.mr`. |
| `add_cylinder`, `es/actions/actions.mr:86-108` | `sound_attenuation` (default 1.0), `primary_length` (default 0) | Per-cylinder gain and header primary length. |
| `cylinder_head_parameters`, `es/objects/objects.mr:362-373` | `exhaust_port_flow`, `exhaust_runner_volume`, `exhaust_runner_cross_section_area` | Blowdown rate and runner lumped volume. |
| `ignition_module`, `es/objects/objects.mr:592-597` | `rev_limit` (default 7000 rpm), `limiter_duration` (default 0.5 s), `timing_curve` | Limiter behaviour and firing order through wires. |
| `fuel`, `es/objects/objects.mr:51-64` | `burning_efficiency_randomness`, `max_burning_efficiency` | Pulse-to-pulse variation. |
| `rod_journal(angle)`, `cylinder_bank(angle)` | crank and bank geometry | Firing intervals, hence the fundamental pattern. |

Shipped profiles (`assets/engines/`):

| File | Engine | Layout | Sim rate | IR |
| --- | --- | --- | --- | --- |
| `atg-video-1/01_honda_trx520.mr` | Honda TRX520 | single | 40000 | mild_exhaust_0_reverb |
| `atg-video-1/02_kohler_ch750.mr`, `kohler/kohler_ch750.mr` | Kohler CH750 | 90 deg V-twin | 30000 | default_0 |
| `atg-video-1/03_harley_davidson_shovelhead.mr` | Shovelhead | 45 deg V-twin | 35000 | minimal_muffling_01 |
| `atg-video-1/04_hayabusa.mr` | Suzuki Hayabusa | inline-4, 180 deg crank | 20000 | minimal_muffling_03 |
| `atg-video-1/05_honda_vtec.mr` | Honda B18C5 | inline-4, 180 deg crank, VTEC | 20000 | mild_exhaust_0 |
| `atg-video-1/06_subaru_ej25.mr`, `atg-video-2/01_subaru_ej25_eh.mr`, `02_subaru_ej25_uh.mr` | Subaru EJ25 | flat-4 (banks +90, -90) | 20000 | mild_exhaust_0, minimal_muffling_02 |
| `atg-video-1/07_audi_i5.mr`, `audi/i5.mr` | Audi inline-5 | inline-5 | 17000 | mild_exhaust_0, default_0 |
| `atg-video-1/08_radial_5.mr`, `atg-video-2/09_radial_9.mr` | radials | 5 and 9 cylinder | 12000, 7500 | minimal_muffling_01, mild_exhaust_0_reverb |
| `atg-video-2/03_2jz.mr`, `bmw/M52B28.mr` | Toyota 2JZ, BMW M52 | inline-6 | 10000 | default_0 |
| `atg-video-2/04_60_degree_v6.mr`, `05_odd_fire_v6.mr`, `06_even_fire_v6.mr` | generic V6 | 60 and 90 deg | not set (default 10000) | mild_exhaust_0_reverb |
| `atg-video-2/07_gm_ls.mr`, `chevrolet/chev_truck_454.mr`, `chevrolet/engine_03_for_e1.mr` | GM LS, Chevrolet 454 | 90 deg V8, cross-plane (journals 0, 270, 90, 180 at `07_gm_ls.mr:201-204`; 0, -90, -270, 180 at `chev_truck_454.mr:45-48`) | 10000 | default_0 |
| `atg-video-2/08_ferrari_f136_v8.mr` | Ferrari F136 | 90 deg V8, flat-plane (0, 180, 180, 0 at lines 198-201) | 10000 | mild_exhaust_0_reverb |
| `atg-video-2/10_lfa_v10.mr` | Lexus 1LR-GUE | V10 | 6500 | mild_exhaust_0_reverb |
| `atg-video-2/11_merlin_v12.mr` | Merlin V-1650 | 60 deg V12 | 7000 | minimal_muffling_01 |
| `atg-video-2/12_ferrari_412_t2.mr` | Ferrari Tipo 044/1 | 75 deg V12, 18500 rpm limiter | 5000 | minimal_muffling_01 |

Match to snarl's initial profiles:

- Inline-4: yes, `04_hayabusa.mr` and `05_honda_vtec.mr`.
- Flat-6: none. Derive from the EJ25 flat-4 (banks at +90 and -90 degrees) with three cylinders per bank and a 120 degree crank.
- Cross-plane V8: yes, `07_gm_ls.mr` and `chev_truck_454.mr`.
- Flat-12: none. Derive from `12_ferrari_412_t2.mr` or `11_merlin_v12.mr` with bank angles changed to +90 and -90 degrees. The Testarossa is a 180 degree flat-12 with a 60 degree-V-style crank, so the crank journals of the Merlin file are the closer start.

## 6. Afterfire, backfire, overrun pops

Not modelled. A search for `afterfire`, `backfire`, `pop`, `crackle` over `src/`, `include/` and `assets/` returns nothing. The exhaust `GasSystem` carries a fuel fraction in its `Mix`, but no code reacts fuel outside the cylinder: `GasSystem::react` is called only from `CombustionChamber::flow` (`src/combustion_chamber.cpp:339`). `ExhaustSystem::process` only vents to atmosphere. The rev limiter cuts spark (`src/ignition_module.cpp:56`, 88-95), so unburnt charge leaves through the exhaust and the pressure pulses drop. That gives the "bouncing off the limiter" stutter but no pops. Overrun with a closed throttle lowers charge mass and so pulse amplitude. `ignite` refuses lean mixtures below equivalence ratio 0.5 (`src/combustion_chamber.cpp:174-176`), which can give misfires, but there is no exhaust ignition. snarl must design crackles itself, for example as random short bursts triggered by the overrun and limiter events, convolved with the same IR.

## 7. Headless Linux build feasibility

Targets in `CMakeLists.txt`:

| Target | Lines | Depends on | Linux status |
| --- | --- | --- | --- |
| `engine-sim` static library (core physics and `Synthesizer`) | 47-144 | `simple-2d-constraint-solver`, `csv-io`, `delta-basic` | The only delta-studio use in the library sources is the unused `#include "../include/delta.h"` at `src/synthesizer.cpp:4`. Three headers use `__forceinline`. With that include removed and `#define __forceinline inline` for GCC or Clang, the library has no Windows code. `simple-2d-constraint-solver` and `csv-io` are plain C++ by reputation but unverified here. |
| `engine-sim-script-interpreter` | 146-177 | `csv-io`, `piranha` | No Windows includes in `scripting/`. `piranha` needs Boost, flex and bison (README). Piranha Linux build is unverified. |
| `engine-sim-app` | 206-305 | `WIN32` executable, `WinMain` + DirectX 11 (`src/main.cpp:5-16`), delta-studio window, renderer and `ysAudioDevice`, `ysWindowsAudioWaveFile` for IR loading (`src/engine_sim_application.cpp:493-497`), prebuilt `discord-rpc.lib` (line 293) | Windows only. Not needed for rendering. |
| `engine-sim-test` | 308-320 | gtest (FetchContent) + `engine-sim` | Shows that the core library already links without the UI. `test/synthesizer_tests.cpp` drives `Synthesizer::writeInput` and `renderAudio()` single-threaded. |

Minimum changes for a headless renderer:

1. Fetch submodules `simple-2d-constraint-solver`, `csv-io` and `piranha`. Skip `delta-studio` and `direct-to-video`. Set `DISCORD_ENABLED=OFF`, `DTV=OFF`.
2. Drop `delta-basic` from `target_link_libraries(engine-sim ...)` (line 137-140) and the include at `src/synthesizer.cpp:4`. Add a compatibility define for `__forceinline`.
3. Add a new executable target that links `engine-sim` and `engine-sim-script-interpreter`. Its `main()`: `es_script::Compiler::compile(path)` then `execute()` to get `Engine`, `Vehicle`, `Transmission` (`scripting/include/compiler.h:15-22`), `engine->createSimulator(vehicle, transmission)` (`src/engine.cpp:386-396`), set `simulationFrequency` from `engine->getSimulationFrequency()`, copy the audio parameters as the app does (`src/engine_sim_application.cpp:482-488`), load each exhaust IR with a small WAV reader (16-bit mono PCM, `file` confirms) replacing `ysWindowsAudioWaveFile`, then loop `startFrame(dt)`, `simulateStep()` until false, `endFrame()`, and call `synthesizer().renderAudio()` directly instead of starting the thread. Write `readAudioOutput` to a WAV and dump per-step internals (`chamber->m_exhaustRunnerAndPrimary.pressure()`, the staging buffer, RPM) to CSV.
4. Control: `engine->setThrottle(x)` (`include/engine.h:61`), `engine->getIgnitionModule()->m_enabled`, `m_starterMotor.m_enabled`, and `m_dyno.m_enabled`, `m_dyno.m_hold` with a hold speed to script RPM sweeps (`include/dynamometer.h:22-23`, `include/starter_motor.h:20`).
5. Paths: the compiler searches `../../es/`, `../es/` and `es/` relative to the working directory (`scripting/src/compiler.cpp:25-27`), and IR filenames resolve relative to `es/sound-library/impulse_responses.mr` (`scripting/include/impulse_response_node.h:28-33`). Run from the repository root or a `build/` child.

Prior art: PR `ange-yaghi/engine-sim#346` "SDL build" (October 2022, still open) ported the full application to Linux and was blocked on a delta-studio PR. PR #112 "Linux build fixes" was closed in its favour. A headless target avoids delta-studio entirely and so avoids that blocker. Unverified: whether `piranha` and `simple-2d-constraint-solver` compile cleanly with GCC 13 or newer.

Community edition: the repository `Engine-Simulator/engine-sim-community-edition` contains no source code and no build files. It holds `README.md`, `docs/`, `tutorial/` and `.github/FUNDING.yml`. The README states that the project "is no longer actively maintained", that the distributed v0.1.14a is "almost identical to the open source version" in core functionality, and that the download is a Windows `.zip` with `bin/engine-sim-app.exe`. It has no licence file (GitHub reports `licenseInfo: null`). It therefore gives no Linux path and no additional code to read. Use `ange-yaghi/engine-sim` as the only code source.

## 8. Licences

- Code: MIT, `LICENSE`, copyright 2022 Ange Yaghi. GitHub reports the repository licence as MIT, last push 17 June 2024 (the `master` branch head is from January 2023, so the later push is on another branch).
- Impulse responses: `es/sound-library/*.wav` and `assets/sound-library/*.wav` have no separate licence, README or attribution. No file in `assets/` or `es/` mentions a licence. Under the repository's single `LICENSE` they are MIT by default, but the recordings' origin (own recordings or third-party) is undocumented. Treat redistribution in a commercial game as a legal question to confirm with the author, or record or synthesise snarl's own IRs.
- Engine scripts in `assets/engines/`: same MIT default, no separate statement.
- Community edition: no licence, no code.

## Implications for snarl

1. Port the synthesiser shape, not the code: per channel jitter (random fractional delay up to 10 samples), 10 Hz DC block, derivative mix at about 0.01, low-passed noise AM at 2 kHz, IR convolution, 0.45 fs low pass, peak leveller. All of these are a few lines each in Go and allocation-free.
2. Replace the direct 10,000-tap convolution. Options: partitioned FFT convolution (cheap enough for the Switch at 44.1 kHz with 256-sample partitions, more code), a 512 to 1024 tap truncated IR (direct form, about 45 to 90 million multiply-adds per second per channel), or a bank of 4 to 8 two-pole resonators fitted to the IR spectrum (cheapest, the natural low quality tier). The reference renders will show how much of the IR tail matters.
3. Replace the physics with a pulse generator: firing schedule from crank geometry and the firing order, per-cylinder pulse shape and amplitude from a small table indexed by RPM and load, per-cylinder delay lines for header lengths, per-bank sum. This keeps the audible structure (firing intervals, unequal headers, pulse-to-pulse randomness) without the solver.
4. The reference renderer must capture `m_exhaustRunnerAndPrimary.pressure()` per cylinder and the summed staging buffer per step, so snarl's pulse table can be fitted to real curves across RPM and load.
5. Design overrun crackles as a new event layer. The reference offers nothing to copy here.
6. Keep latency independent of frame rate: feed the synthesiser from the audio callback, not from a frame loop with a 100 ms target queue.
7. Check IR licensing before shipping any engine-sim WAV. Plan to produce snarl's own IRs.

## Open questions

- Do `piranha`, `simple-2d-constraint-solver` and `csv-io` build with a current GCC on Linux? Not checked (submodules not fetched, no build run).
- How much CPU does the physics take per engine at the scripted rates? The application shows it live, but no numbers were measured here.
- Which branch received the June 2024 push on `ange-yaghi/engine-sim`, and does it change the synthesiser? The shallow clone only read `master`.
- Who recorded the impulse responses and under what terms?
- How audible is the delay-filter rate bug (`src/piston_engine_simulator.cpp:226` fixed at 10,000 Hz) for 20 to 40 kHz profiles? It changes header delays by a factor of 2 to 4 and so the unequal-header effect.
