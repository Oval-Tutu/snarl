# EngineLab: audio-relevant internals

Research notes for `snarl`. Source: https://github.com/zolaski333/EngineLab,
shallow clone at commit `6a1d50540ded9ae1fabe0f7ce16f6ff7780b5089` (2026-10-16).
File paths below are relative to that repository root. Read-only study, no
build or run. Claims without a path are marked unverified.

## Summary

EngineLab is a full engine simulator whose exhaust sound is the pressure that
a quasi-1-D finite-volume gas solver computes at the valves, carried by a
linear waveguide network to a radiation load and two microphones. The
physics thread, not the audio callback, is the cost (`docs/thermoacoustic-architecture.md`
section 18). The project's own measurements say that per-engine character
lives in the firing pattern and its envelope, not in the stationary spectrum
(section 13), and that the owner cannot recognise a specific engine by ear
(`docs/journal.md`, 2026-09-22). For `snarl`, the reusable parts are the small
self-contained audio elements and the afterfire triggering logic, not the
solver.

## 1. Audio architecture

### Signal chain

From `docs/thermoacoustic-architecture.md` ("Active chain") and
`docs/realtime-audio.md`:

1. 0-D combustion chamber (`GasCell`, `src/physics`) exchanges with the
   exhaust through a conservative Riemann flux at each valve, using a
   compressible isentropic nozzle law for the valve `CdA`
   (`docs/thermoacoustic-architecture.md` section 1).
2. Non-linear quasi-1-D gas network (`src/gas-dynamics/*/FiniteVolumeDuct.*`,
   `ExhaustNetworkLayout.*`, `ExhaustGasNetwork.*`): HLLC flux, TVD
   reconstruction, SSP-RK2, CFL step. Species, momentum and energy per cell,
   finite-capacity walls. It resolves mean flow, back-pressure and firing
   fundamentals "up to about 330-360 Hz in hot gas" (section 1, "Real-time
   scale separation").
3. `EngineSimulator::step` (`src/simulation/src/EngineSimulator.cpp`) publishes
   per mechanical sub-step a `CylinderPressureSample`
   (`src/events/include/enginelab/events/CylinderPressureSample.hpp`): chamber
   pressure, runner pressure, signed valve mass flow, port density, speed of
   sound, valve conductance, path index, validity flag, and
   `mechanicalSamplingFrequencyHz`.
4. `RealtimeEngineAudio` (`src/audio/include/enginelab/audio/RealtimeEngineAudio.hpp`,
   `src/audio/src/RealtimeEngineAudio.cpp`, 3,051 lines) removes a slow mean,
   forms characteristics `p+ = (p' + Zc U')/2`, `p- = (p' - Zc U')/2` with
   `Zc = rho c / A`, and feeds `AcousticExhaustNetwork`
   (`src/audio/src/AcousticExhaustNetwork.cpp`, 1,699 lines). That network is
   compiled from the same `ExhaustGraph` DAG: bidirectional delay lines per
   duct, N-port admittance scattering at junctions, `DuctWallLoss` shelf,
   `DuctModeCutoff` Butterworth at the plane-mode cutoff,
   `NonlinearDuctAcoustics` steepening warp, `ExpansionChamberMuffler`,
   `PorousLinerLoss`, `ExhaustCrossoverScattering`, `ValvePortTermination`
   (all headers in `src/audio/include/enginelab/audio/`).
5. Each outlet ends in `UnflangedPipeRadiation`
   (`src/audio/include/enginelab/audio/PipeRadiationModel.hpp`), a Levine-Schwinger
   load (Silva Pade (1,2) fit). The mouth volume acceleration gives a monopole;
   `ExhaustJetNoise` adds a `U^8` Strouhal 0.2 broadband source.
6. `FreeFieldObserver` (`src/audio/include/enginelab/audio/FreeFieldObserver.hpp`)
   propagates each outlet to left and right microphones with exact `r/c` delay,
   `1/r` decay and `ka` directivity. Stereo comes from path differences, not
   panning.
7. Optional convolution through `RealtimeConvolutionBank`
   (`juce::dsp::Convolution`, up to 8 partitioned IRs) only when a path declares
   `impulse_response` (`docs/realtime-audio.md`, "Impulse responses").
8. Master: user shelf, DC blocker, reconstruction filter, slow safety leveler,
   oversampled soft limiter (`juce::dsp::Oversampling`), then pascals to dBFS
   through `AcousticMonitorCalibration` (140 dB SPL RMS = 0 dBFS by default).

### Layers

Six pre-master stereo stems (`RealtimeEngineAudio::renderWithStems`,
`docs/realtime-audio.md`, "Diagnostic stems"): combustion, dry exhaust, exhaust
IR return, intake, forced induction, mechanical/structure.

| Layer | Class | Driver |
|---|---|---|
| Exhaust | `AcousticExhaustNetwork` | SI valve boundary per sub-step |
| Intake | `AcousticIntakeNetwork` (`src/audio/src/AcousticIntakeNetwork.cpp`) | signed intake valve flow; runners, WDF plenum compliance, throttle admittance, airbox, inlet duct, horn mouth |
| Structure | `StructuralModalRadiator` (`src/audio/src/StructuralModalRadiator.cpp`) | `StructuralExcitationSample`: gas force, inertia force, bearing reactions, side thrust, crank torque per sub-step |
| Forced induction | `ForcedInductionAcoustics` | shaft speed, blade or lobe orders, compressor and turbine power, wastegate and dump-valve flow |
| Starter | fixed-level voice inside `RealtimeEngineAudio` | `RealtimeAudioState::starter` atomic |
| Afterfire | `ThermoacousticHeatReleaseSource` + `ReactionCrackSynthesiser` | `ExhaustReactionEvent` from the gas solver |

The mechanical layer is off (`mechanical_gain: 0.0`) in every shipped voicing
(`voicing/default.yaml`, `docs/audio-voicing.md`); the owner found that it
only added an unpleasant hum (`docs/journal.md`, 2026-10-12).

### Threads and queues

- `EngineRuntime::run` (`src/runtime/src/EngineRuntime.cpp` line 1122) is a
  `std::jthread` at a fixed 240 Hz frame (`1.0 / 240.0`, 4.167 ms). Inside a
  frame `EngineSimulator::step` sub-steps the mechanics.
- `CylinderWorkerPool` (`src/simulation/include/enginelab/simulation/CylinderWorkerPool.hpp`)
  runs per-runner intakes and per-duct RK shares for engines with 3 or more
  cylinders; coupling stays serial so results are bit-identical
  (`docs/architecture.md`, "State ownership and threads").
- `SpscQueue<T, Capacity>` (`src/foundation/include/enginelab/foundation/SpscQueue.hpp`):
  power-of-two ring, trivially copyable payloads, acquire/release atomics, 64-byte
  aligned indices. Three instances in `src/runtime/include/enginelab/runtime/EngineRuntime.hpp`:
  `FiringEventQueue = SpscQueue<FiringEvent, 2048>`,
  `CylinderPressureQueue = SpscQueue<CylinderPressureSample, 8192>` (sized for
  up to 96 kHz of telemetry, more than 85 ms of look-ahead),
  `ExhaustAcousticQueue = SpscQueue<ExhaustAcousticSample, 2048>` (per-duct
  medium, outlet flow, reaction events).
- Slow telemetry crosses as `std::atomic<float>` fields in `RealtimeAudioState`
  (same header), with `static_assert` on lock-free atomics.
- The callback does no allocation, I/O, logging or locking; all buffers are
  sized in `prepare()` (`docs/realtime-audio.md`, "Callback contract"). A slow
  software PLL aligns simulation time to the sound card clock.
- Network replacement (live exhaust edit) is a pointer handoff: the new
  network fills silently for 150 ms, then a 30 ms crossfade; the old one is
  freed on the message thread (`docs/architecture.md`, "Live exhaust changes").

## 2. What carries the audible character

Measured findings, each with its source:

- Per-engine character is mostly in the firing pattern (envelope), not in the
  stationary spectral shape. Envelope modulation peaks at idle differ per
  engine (K20 62.5 Hz, LS3 196 Hz, Hayabusa 82 Hz, Big Twin 20.5 Hz). The
  cross-plane V8 has sub-firing modulation energy of 0.09 relative to firing
  against 0.00 for even I4s, from the 180/270/180/90 degree intervals per bank
  (`docs/thermoacoustic-architecture.md` section 13).
- Third-octave spread between engines is 15 to 30 dB per band; the shared
  trend is only the roll-off towards the treble. The team concluded that
  there was no homogenisation bug, only muffling (section 13).
- The physical band is bounded by the solver coupling rate at about 1.1 to
  2 kHz, later raised by a 125 us cap (LS3 Nyquist 5,640 Hz, Merlin 3,480 Hz)
  (sections 11 and 14). Front steepening (`NonlinearDuctAcoustics`) fills 1.5
  to 4 kHz: Hayabusa 10.9 to 34.2 %, K20 2.8 to 37.8 % of content at the
  limiter (section 15).
- Without a transmission-loss element every engine was a straight pipe and
  road engines converged on one timbre; the engines judged good by ear
  (Merlin, flat-6, radial) are the ones whose real exhaust is near-open
  (section 17). The expansion chamber (`ExpansionChamberMuffler`, Munjal
  closed form) fixed this; invented broadband absorption in the collector
  loop erased the fundamental and was removed (section 19).
- Wall loss, collector trunk length and a per-duct gas medium each changed
  timbre measurably: the old single-pole wall loss over-attenuated by 4.2 dB
  at 4 kHz and 11.1 dB at 8 kHz and flattened differences between engines; the
  hot-gas speed of sound differs 3 % per duct (section 25).
- The default 2 L collector volume made every engine muffled (CP2 +10 to
  +19 dB over 0.5 to 5 kHz when set to swept volume) (`docs/journal.md`,
  2026-10-15, "The default collector was a 2 L chamber").
- Header and silencer length interact non-additively below 1 kHz (+13 to
  +18 dB at 250 Hz when both long); header length matters more than the
  mid-pipe (`docs/journal.md`, 2026-10-12, "CP2 exhaust lengths").
- Cam duration and lift reach the sound as much as a modest exhaust change
  (shape 4.07 dB, worst band -11.8 dB at 315 Hz for -40 degree cams)
  (`docs/journal.md`, 2026-10-13, "Do the cams change the sound?").
- The low crest factor of the simulator the owner prefers (engine-sim) can
  come from its output stage alone: a level-normalising gain into `tanh`
  took crest from 11 to 14 dB down to 6 to 7 dB with almost no change in
  periodicity or tilt (`docs/journal.md`, 2026-10-13, "How much character an
  output stage alone gives"; `src/audio/include/enginelab/audio/AutoGainSoftClipper.hpp`).
- Radiating into ambient air density instead of hot duct gas raises level by
  +2.5 to +11.7 dB and changes only the balance against jet noise
  (`docs/journal.md`, 2026-10-13, two entries on far-field density).
- Mechanical layer sat 20 to 47 dB over the exhaust above 1 kHz; switched off
  (`docs/journal.md`, 2026-10-12).
- Blind listening: no engine is recognisable beyond its cylinder count; the
  rhythm (firing order) is right; the afterfire does not sound like a
  backfire (`docs/journal.md`, 2026-09-22; `VISION.md`). Valve turbulence
  noise: no preference at idle, control preferred on the rev; stays off
  (`docs/journal.md`, 2026-10-14).

## 3. Cost

- Frame: 240 Hz, 4.167 ms budget (`src/runtime/src/EngineRuntime.cpp` line 1122).
- Mechanical sub-steps: `SolverConfig` defaults `mechanicalFrequencyHz 2000`,
  `maximumMechanicalFrequencyHz 60000`, `maximumCrankDegreesPerStep 2.0`
  (`src/foundation/include/enginelab/foundation/EngineTypes.hpp` line 744).
  Measured sub-steps per frame: I2 31.1, LS3 31.5, Merlin 60.7
  (`docs/thermoacoustic-architecture.md` section 18).
- Exhaust coupling cap 125 us, at least 16 couplings per firing period
  (section 14, section 1). Intake coupling interval 330 us (section 22).
- Mesh: four-stroke production exhaust cells 360 mm, two-stroke 100 mm
  (`EngineSimulator::productionExhaustCellLengthM`, `docs/architecture.md`,
  "Gas field on the ducts"). Doc section 1 says "maximum cell length of
  300 mm"; `EngineSimulator.cpp` line 809 mentions a 95 mm target with at
  least three cells. The two figures conflict; the 360 mm figure is the
  most recent (journal 2026-10-05). Intake runners: 30 mm reference, 12-cell
  cap in tests (section 22).
- Realtime factor (simulated seconds per wall second, i5-10600 reference
  machine, `--free-run`): CP2 twin 3.28, K20A I4 1.92, LS3 V8 1.31 at 360 mm;
  at 180 mm 2.85 / 1.67 / 1.05; at 90 mm 2.39 / 1.32 / 0.73
  (`docs/journal.md`, 2026-10-05, "Wave resolution of the exhaust mesh"). LS3
  with audio 1.07 to 1.12, Merlin V12 1.40 to 1.47 after the worker pool
  (2026-10-11). Merlin V12 did not sustain real time before its exhaust was
  shortened (section 10).
- Where the frame goes: I2 cylinders 53 %, FV network 16 %, rest 31 %; LS3
  41 / 29 / 30 %; Merlin 20 / 52 / 28 % (section 18). The physics thread is
  the bottleneck, never the callback.
- Audio callback at 48 kHz, 256-sample block (5,333 us budget), p95: K20
  804 us (15 %), 2JZ 877 us (16 %), LS3 1,543 us (29 %), Merlin 1,755 us
  (33 %) (section 18). That callback is the waveguide network plus layers,
  without the physics.
- What scales cost: cylinder count (per-cylinder physics, parallelised);
  duct cell count and the shortest element, which sets the CFL step for the
  whole network (`.claude/CLAUDE.md`, "A single short exhaust element sets the
  time step of the whole network"); coupling rate (fixed work per coupling,
  section 11); mid-pipe length (3.73 m cost 8 % on the LS3, journal
  2026-10-07); a 2 L collector collapsing the CFL from 64.8 to 9.3 mm (journal
  2026-10-15). Limits: 8 audio exhaust paths, 32 cylinders, 64 ducts, 32
  outlets, 64 structural modes (`ExhaustAcousticSample.hpp`,
  `docs/realtime-audio.md`).
- Memory: `EngineRuntime` is about 7.4 MB, mostly
  `SpscQueue<CylinderPressureSample, 8192>` (`tools/CMakeLists.txt` comment).

## 4. Cheap, separable techniques worth reusing

Recommended for `snarl`, in priority order.

1. Firing pattern as the carrier of character. Per-cylinder crank offsets and
   bank assignment are the data (`crank_offset_deg`, `bank_id`,
   `firing_order` in `engines/*.yaml`). Sub-firing envelope modulation is what
   makes the cross-plane V8 differ from an I4 (section 13). This costs nothing.
2. `UnflangedPipeRadiation` (`PipeRadiationModel.hpp`): one biquad per outlet
   from `R(s) = -(1 + 0.167 (a/c) s) / (1 + 1.393 (a/c) s + 0.457 (a/c)^2 s^2)`
   via bilinear transform, radiated pressure from the volume acceleration.
   Gives the open-pipe reflection and the low-pass of a tailpipe for the
   price of a biquad.
3. `ExpansionChamberMuffler` (`ExpansionChamberMuffler.hpp`): two
   Kelly-Lochbaum junctions and one delay line; transmission loss follows
   Munjal `TL = 10 log10 [1 + 1/4 (m - 1/m)^2 sin^2(kL)]`. Keep it reactive
   (no broadband loss in the loop, section 19).
4. `DuctWallLoss` one-pole one-zero shelf fitted at two frequencies
   (closed form, section 25.1) and `DuctModeCutoff` 4th-order Butterworth at
   `f_c = 1.8412 c / (2 pi a)` (section 25.2). Both are set by radius and
   speed of sound only, so they differentiate a 42 mm motorcycle primary from
   a 142 mm V8 chamber with no tuning.
5. `NonlinearDuctAcoustics` (`NonlinearDuctAcoustics.hpp`): amplitude-modulated
   delay read, `delayScale = 1 / (1 + beta p' / (rho c^2))`, `beta = (gamma+1)/2`.
   Passive, zero cost when `p' = 0`, supplies the "bark" above the solver
   band (section 15). In a procedural model this is a cheap way to get
   load-dependent brightness.
6. `FreeFieldObserver` (`FreeFieldObserver.hpp`): two fractional delay lines,
   `1/r`, a one-pole `ka` directivity split, glide with a 0.02 sample per
   sample delay rate cap (2 % Doppler maximum). Stereo width from geometry.
   Sources clamp to 0.25 to 40 m.
7. `ExhaustJetNoise` (`ExhaustJetNoise.hpp`): one noise generator per outlet,
   power `K rho A U^8 / c^5`, centre at `St = 0.2`, `K = 1e-4 x 100`. Block-rate
   `configure()`, allocation-free `process()`. A natural "load hiss" layer.
8. `StructuralModalRadiator` estimated-family modes
   (`src/audio/src/StructuralModalRadiator.cpp` lines 119 to 251): block as a
   hollow aluminium box beam (3 vertical and 3 lateral bending modes, damping
   0.035 and 0.040), heads as simply supported plates (damping 0.045), two
   torsional shell modes (damping 0.030), all sized from mean bore, stroke,
   rod length and cylinders per bank. Exact analytical step of a damped
   oscillator per sample. 8 to 24 oscillators. The owner switched the layer
   off in every voicing, so treat it as a candidate, not a proven win.
9. Afterfire trigger logic (section 5 below) and the `ReactionCrackSynthesiser`
   (`ReactionCrackSynthesiser.hpp`): white noise through an LR4 high-pass at
   the crossover, -6 dB per octave tilt, envelope 0.1 ms attack and 0.5 ms
   release, RMS equal to the compact pressure jump. Deterministic xorshift
   per voice.
10. `AutoGainSoftClipper` (`AutoGainSoftClipper.hpp`): 5 Hz envelope follower,
    4x attack, 1 s hold, gain capped at 10, into `tanh`. The journal shows
    that it alone reproduces the crest factor of engine-sim. Cheap and
    probably what players expect from arcade engine audio.
11. Voicing data model (`voicing/default.yaml`, `voicing/engines/*.yaml`,
    `docs/audio-voicing.md`): three-layer merge (default, family, engine),
    strict schema, 14 scalar fields, hot-reload polled on the UI thread,
    audio thread reads atomics. Shipped per-engine profiles are small: the
    LS3 sets `low_frequency_gain 1.20`, `stereo_width 1.28`,
    `saturation_drive 0.34`; the flat-6 sets `high_frequency_gain 1.12`,
    `stereo_width 1.16`, `saturation_drive 0.14`
    (`voicing/engines/03_gm_ls3_like_engine.yaml`,
    `voicing/engines/09_porsche_flat6_like_engine.yaml`).
12. Calibration contract: keep the signal in pascals and convert once,
    `282.843 Pa peak = 200 Pa RMS = 140 dB SPL = 0 dBFS`
    (`docs/realtime-audio.md`). This makes layer balance a physical choice.

Not worth porting: the finite-volume solver, the intake network, the
forced-induction layer (semi-empirical and turbo-specific), the IR convolution
(no catalogue engine uses one, and the owner heard no difference at the
default mix, `docs/journal.md` 2026-10-14).

## 5. Afterfire model in detail

Sources: `docs/physical-afterfire.md`, `src/ecu/src/SimpleEcuModel.cpp` lines
505 to 680, `src/foundation/include/enginelab/foundation/EngineTypes.hpp` lines
444 to 530, `src/gas-dynamics/include/enginelab/gasdynamics/ExhaustGasNetwork.hpp`
lines 213 to 365, `src/audio/include/enginelab/audio/ThermoacousticHeatReleaseSource.hpp`,
`ReactionCrackSynthesiser.hpp`.

### Strategies and calibration

`ExhaustAfterfireStrategy`: `cleanDfco` (no fuel), `continuousAntiLag`
(continuous fuel, steady roar), `discreteAfterfire` (chopped slugs, pops).
Defaults in `ExhaustAfterfireConfig`: `ignitionTemperatureK 900`,
`reactionTimeConstantSeconds 0.010`, `reactionEfficiency 0.95`,
`overrunFuelFraction 0`, `overrunMinimumRpm 2500`, `overrunMaximumThrottle 0.02`,
`overrunPulseHz 0`, `overrunPulseDutyCycle 0.35`, `overrunPulseTimingVariation 0`,
`inductionTimeSeconds 0.004`, `inductionDecayTimeSeconds 0.008`,
`minimumEquivalenceRatio 0.45`, `maximumEquivalenceRatio 1.80`,
`quenchTemperatureK 520`. Only `engines/16_audio_physics_lab_689_twin.engine.yaml`
enables it (`ignition_temperature_k: 800`). The documented "audible demo"
profile: 2 ms reaction, 2 Hz nominal, 8 % duty, 40 % timing variation, 8 %
mean fuel; each interval then lies between 300 and 700 ms.

### ECU trigger (`SimpleEcuModel.cpp`)

Arming: ignition on, starter off, a fuel-retaining strategy with
`overrunFuelFraction > 0`, throttle above 0.20 and rpm at or above
`overrunMinimumRpm` arms the next lift-off (lines 513 to 531). Arming also
lifts the after-start DFCO inhibit.

Active window (`overrunBaseActive`, lines 617 to 624): petrol fuel, retaining
strategy, `overrunFuelFraction > 0`, deceleration fuel cut active (closed
throttle below 0.02 and rpm above 1.65 x idle, lines 557 to 559), rev
limiter not active, ignition on, starter off, rpm at or above the minimum,
throttle at or below `overrunMaximumThrottle`. On the rising edge the pulse
epoch latches to the lift-off time (line 627).

Pulse chopping (lines 634 to 650): for the discrete strategy,
`afterfirePulseOpen(elapsed, pulseHz, duty, timingVariation)` returns whether
the current slug is open. Interval boundaries come from a deterministic
low-discrepancy sequence (`afterfirePulseBoundarySeconds`, lines 53 to 68), so
the cadence is not a metronome but is reproducible. The fuel retained in an
open window is `overrunFuelFraction / duty` of a normal charge, capped at 1
(`afterfireRetainedChargeEquivalenceRatio`). Spark is cut.

Blockers are published as a bit mask (`AfterfireBlocker`: `notAuthored`,
`ignitionOff`, `cranking`, `throttleOpen`, `belowMinimumRpm`,
`fuelCutInactive`, `revLimiterActive`, `pulseChopClosed`) so the UI can say
why nothing fired.

### Chemistry in the gas network (`ExhaustGasNetwork::reactUnburnedFuel`)

Each cell and junction has a persistent `ReactionSiteState` with a
Livengood-Wu induction integral. A site burns only if: fuel and oxygen
coexist; local equivalence ratio is within 0.45 to 1.80; gas or wall exceeds
the ignition temperature; the mixture stays active for the induction delay
(`dt / tau(T, p, phi)` Arrhenius integral, schema 8); an established flame is
not below the quench temperature. Reaction consumes species and adds
`fuel_mass x LHV` to energy. The wall is the ignition source
(`peakWallTemperatureK`): 1.5 mm steel is about 5.9 kJ/m2K against a film of
a few hundred W/m2K, a time constant of tens of seconds, so a cold engine
cannot pop. Measured: on the CP2 the discrete profile burns 0 % at 800 K
(wall 390 to 424 degC), 18 % at 650 K (`docs/journal.md`, 2026-09-23).
Continuous 18 % fuel burns 0 % (phi 0.18 below the lean limit); the same
mass in slugs burns 89 % on the 2JZ.

### To the audio

The solver copies up to 32 `ExhaustReactionEvent`s per sample (node, axial
position, energy, duration, density, speed of sound, area) into
`ExhaustAcousticSample` (`EngineSimulator.cpp` lines 5354 to 5424), the
runtime timestamps them onto the audio timeline (`EngineRuntime.cpp` lines
1467 to 1479) and `RealtimeEngineAudio` holds 64 `ThermoacousticReactionVoice`s.
Each voice: `Delta p = (gamma - 1) Qdot / (A c)` with `gamma - 1 = 0.34`,
split equally into both travelling waves at the reaction's node and position,
reconstructed with the LR8 anti-imaging filter and a 25 Hz two-pole DC
blocker (`ThermoacousticHeatReleaseSource.hpp`). Power is held for exactly
`energy / duration`. A 100 kPa last-resort bound protects the linear network.
`ReactionCrackSynthesiser` adds the band above about 3.8 kHz with
`defaultCrackRatio = 1.0`, uncalibrated (bracket 0.8 to 2.5).

Measured pop: front 0.2 to 0.8 ms, duration 2 to 4 ms, energy centroid 450
to 790 Hz, 0.2 to 0.5 % between 4 and 8 kHz, ON/OFF peak -3 to +10 dB over
the engine (`docs/journal.md`, 2026-09-23). Energy split on the lab twin:
15 % at 20 to 120 Hz, 65 % at 120 to 500 Hz, 20 % at 500 Hz to 2 kHz, 0.4 % at
2 to 8 kHz (`docs/physical-afterfire.md`). The owner's verdict: it "does not
sound like a backfire, despite a dozen commits" (`docs/journal.md`,
2026-09-22).

## 6. Engine catalogue

Format: one YAML per engine under `engines/` (`schema_version` 5 to 9,
`family`, then `engine:` with `name`, `layout` (inline, v, flat, radial),
`bank_angle_deg`, `cylinder_count`, `firing_order`, `idle_rpm`, `redline_rpm`,
inertia, `intake`, `ignition` timing curve, `uses` (injection, camshafts,
exhaust, transmission, vehicle presets from `parts/*.yaml`), `exhaust_paths`
with either a scalar preset or a component `graph` (`pipe`, `merge`,
`splitter`, `resonator`, `muffler`, `catalyst`, `outlet`, `crossover`,
`src/foundation/include/enginelab/foundation/EngineTypes.hpp` line 30),
`acoustic_observer` microphone positions, and either `cylinder_template` or
explicit `cylinders` with `crank_offset_deg`, `bank_id`, `crank_journal_id`).
A matching voicing override sits in `voicing/engines/<key>_engine.yaml`.

| File | Name | Layout |
|---|---|---|
| `01_honda_k20a_like` | K20A-like 2.0 I4 VTEC | inline 4, NA |
| `02_toyota_2jz_gte_like` | 2JZ-GTE-like 3.0 I6 Turbo | inline 6, turbo |
| `03_gm_ls3_like` | LS3-like 6.2 Crossplane V8 | 90 degree V8, firing 1-8-7-2-6-5-4-3, X-pipe, dual mufflers |
| `04_subaru_ej25_like` | EJ25-like 2.5 Flat-4 Turbo | flat 4, turbo |
| `05_audi_i5_like` | Audi I5-like 2.5 Turbo | inline 5, turbo |
| `06_hayabusa_i4_like` | Hayabusa-like 1.3 I4 | inline 4 motorcycle |
| `07_harley_v_twin_like` | Big Twin-like 1.9 V2 | 45 degree V-twin, shared journal |
| `08_merlin_v12_like` | Merlin-like 19.8 V12 Scaled | 60 degree V12, supercharged, short stacks |
| `09_porsche_flat6_like` | Aircooled-like 3.6 Flat-6 | flat 6, firing 1-6-2-4-3-5, two 3-into-1 paths, no silencer |
| `10_radial5_like` | Radial-like 6.5 R5 | 5-cylinder radial |
| `11_yamaha_cp2_mt07_like` | Yamaha CP2 MT-07-like 689 Twin | parallel twin, 270 degree crank |
| `12_yamaha_cp3_mt09_like` | Yamaha CP3 MT-09-like 890 Triple | inline 3 |
| `13_yamaha_cp4_mt10_like` | Yamaha CP4 MT-10-like 998 Crossplane I4 | inline 4, cross-plane |
| `14_vw_2_0_tdi_like` | VW EA288-like 2.0 TDI I4 | inline 4 diesel, turbo |
| `15_cp2_full_system_like` | MT-07-like 689 Twin Full System | parallel twin, aftermarket exhaust |
| `16_audio_physics_lab_689_twin` | Audio Physics Lab 689 Twin | parallel twin, afterfire authored |
| `17_yamaha_rd350lc_like` | Yamaha RD350LC-like 347 Two-Stroke Twin | two-stroke twin, expansion chambers |

Matches for the `snarl` profiles:

- Inline-4: `01_honda_k20a_like` (car, 4-2-1 style with mid pipe) and
  `06_hayabusa_i4_like` (motorcycle). `13` is a cross-plane I4 with uneven
  intervals, a useful contrast.
- Flat-6: `09_porsche_flat6_like`. Its exhaust has no silencer, which
  produces spikes that drive the limiter at 7,030 rpm (`docs/journal.md`,
  2026-10-16, "Aircooled soft limiting"). Note this if you copy its geometry.
- Cross-plane V8: `03_gm_ls3_like`, with the complete graph shown above
  (690 mm primaries at 48 mm, two 3.0 L collectors, 80 mm crossover with
  `crossover_coupling 0.30`, two 6.3 L packed mufflers, 82 mm outlets at
  x = -0.45 and +0.45 m).
- Flat-12: no match. Nearest references are the flat-6 (bank geometry,
  180 degree) and the Merlin V12 (12-cylinder firing cadence, 60 degree V,
  one short stack per cylinder). A Testarossa-like flat-12 (180 degree V12,
  two 6-into-1 or 6-into-2 banks) would be a new engine file. The journal
  shows that 12 runners plus a wide collector doubled the FV sub-steps
  (section 18), which is irrelevant to a procedural model but shows that
  EngineLab cannot be used to render a reference flat-12 in real time on the
  reference machine without care.

## 7. Linux build feasibility of a headless renderer

Status: plausible but unverified. No Linux build exists
(`README.md`, "Windows only for now"; CI is `windows-2022` only,
`.github/workflows/ci.yml`).

Dependencies (`CMakeLists.txt`): CMake 3.24, C++20, JUCE 8.0.13, nlohmann/json
3.12.0, yaml-cpp 0.8.0, all through `FetchContent` at pinned tags. The
project already declares `LANGUAGES CXX C` for JUCE under Ninja and sets
`CMAKE_POLICY_VERSION_MINIMUM 3.5` for yaml-cpp. Warnings as errors is on in
the presets; GCC and Clang get `-Wall -Wextra -Wpedantic -Wconversion -Wshadow`
(`cmake/CompilerWarnings.cmake`). Expect new warnings on GCC that MSVC did
not raise; configure with `-DENGINELAB_ENABLE_WARNINGS_AS_ERRORS=OFF`.

Windows-specific code outside `src/app/`:

- `src/runtime/src/EngineRuntime.cpp` lines 7 to 11 and 1123 to 1125:
  `#include <Windows.h>` and `SetThreadPriority` under `#if defined(_WIN32)`.
  Guarded, compiles on Linux without it.
- `src/simulation/include/enginelab/simulation/CylinderWorkerPool.hpp` line 133:
  `_mm_pause` under `_MSC_VER`, with `__builtin_ia32_pause` and aarch64
  `yield` branches already present.
- `tools/WasapiLoopbackRecorder.cpp`: standalone, not in the build.
- `tests/AudioWorkshopTests.cpp` links `juce_gui_extra`; skip tests.
- `src/catalog/src/SavedEngines.cpp` uses `%APPDATA%` semantics through the
  app, not needed for harnesses.

JUCE footprint of the audio library (`src/audio/CMakeLists.txt`):
`juce_audio_basics`, `juce_audio_devices`, `juce_audio_formats`, `juce_dsp`,
with `JUCE_WEB_BROWSER=0 JUCE_USE_CURL=0`. Actual symbols used
(`src/audio/src/*.cpp`): `juce::AudioBuffer<float>`, `juce::dsp::Convolution`,
`juce::dsp::Oversampling`, `juce::ScopedNoDenormals`, `juce::AudioFormatManager`
for IR WAV decoding, `juce::File`. `juce_audio_devices` is linked but no
device code was found in the audio library; on Linux that module needs ALSA
headers at compile time (JUCE's documented Linux requirement, not verified
here). Dropping it from the link list is probably safe for the harnesses.

Headless targets (`tools/CMakeLists.txt`): `EngineLabAudioRenderHarness`
(links `EngineLab::Audio`, `Simulation`, `Catalog`; uses `juce::AudioBuffer`
and `juce::dsp::FFT`; options include `--catalogue-filter`, `--stems`,
`--output`, `--ir`, `--runtime-filter`, `--runtime-blip`),
`EngineLabOfflineAudioExporter` (links `EngineLab::Audio`, `Catalog`; WAV at
48/96/192 kHz, 24-bit or float, JSON scenario, stems; `docs/offline-hq-rendering.md`),
`EngineLabAbClipRenderer` (adds `juce_cryptography`), `EngineLabAfterfireHarness`.
All use `ENGINELAB_CATALOG_ROOT` compile definition to find `engines/` and
`voicing/`.

Blockers and risks:

1. Stack size: `EngineRuntime` is about 7.4 MB and several harnesses link
   with `/STACK:8388608` on MSVC only. Linux main thread default is 8 MB
   (`ulimit -s`), so a harness that places two runtimes on the stack may
   overflow. Mitigation: `ulimit -s unlimited` or run only harnesses that
   heap-allocate. `AudioRenderHarness` and `OfflineAudioExporter` carry the
   MSVC stack flag, so assume they need it.
2. JUCE Linux build dependencies for `juce_audio_devices` and `juce_core`
   (ALSA, possibly libcurl despite the flag, freetype is GUI only). Unverified.
3. Build time and memory: the owner caps Ninja at `-j 2` on 16 GB because of
   MSVC heap errors (`.claude/CLAUDE.md`); GCC behaviour unknown.
4. GCC warnings under `-Wconversion -Wshadow -Werror` on code compiled only
   with MSVC `/W4`. Likely many small fixes.
5. Determinism claims (bit-identical WAVs) hold per binary; a GCC build will
   not reproduce the MSVC checksums (`docs/audio-voicing.md` SHA-256), which
   matters only if you want to compare against their published figures.
6. The `README.md` asks for a Linux or macOS build as "help wanted"; the
   `ninja-dev` and `ninja-release` presets assume `scripts/vsenv.ps1`.

A minimal first attempt: configure with Ninja, `ENGINELAB_BUILD_TESTS=OFF`,
`ENGINELAB_ENABLE_WARNINGS_AS_ERRORS=OFF`, build only
`EngineLabOfflineAudioExporter`, then render `--engine LS3`, `--engine K20`,
`--engine Aircooled` with `--stems`. This is a compile-and-fix task, not a
port, as long as JUCE configures. Confidence: medium.

## 8. Licence of code and assets

- Source: MIT, "Copyright (c) 2026 EngineLab Contributors" (`LICENSE.md`).
- Libraries (`THIRD_PARTY_NOTICES.md`): JUCE 8.0.13 is AGPLv3 or the
  commercial JUCE 8 licence; nlohmann/json 3.12.0 MIT; yaml-cpp 0.8.0 MIT. The
  MIT grant covers EngineLab's own code only. A Go port that does not link
  JUCE is not affected by the AGPL.
- Impulse responses in `assets/ir/` (`exhaust_default.wav`, `exhaust_street.wav`,
  `exhaust_open.wav`, `exhaust_turbo.wav`, `exhaust_longtube.wav`,
  `exhaust_moto.wav`): from Engine Sim 2D by Ange Yaghi, "copyright (c) 2022
  Ange Yaghi", MIT, licence text in `assets/ir/LICENSE-es2d.txt`, per-file
  provenance in `assets/ir/README.md` (for example `exhaust_street.wav` is
  es2d `smooth/smooth_39.wav`). Redistribution needs that copyright notice
  and licence text.
- Fonts: Inter and JetBrains Mono under SIL OFL 1.1 (GUI only).
- Reference recordings: not in the repository; CC0 1.0 freesound clips
  listed with SHA-256 in `references/real-engine-audio/manifest.json`. All
  have unknown rpm and are proxies (`docs/journal.md`, 2026-09-22).
- Engine and voicing YAML, `parts/*.yaml`, docs: covered by the repository MIT
  licence. Engines are "-like" approximations, not manufacturer data
  (`README.md`).

## Implications for snarl

1. Do not port the solver. EngineLab's own data says the physics thread is
   the whole cost and the audible gain over a procedural model is not
   established by listening. The 5 % of one core budget rules out any
   finite-volume network.
2. Build the engine identity from the firing schedule. Per-cylinder crank
   offsets, bank membership and the exhaust path each cylinder feeds give
   the sub-firing envelope that measured as the main differentiator. Model
   each cylinder pulse as a short blowdown waveform with cycle-to-cycle
   variation, then sum per exhaust path with per-cylinder delays from
   primary length and a hot-gas speed of sound (about 550 to 580 m/s,
   section 25.4).
3. Use the small physical elements as the exhaust "voice": per path, a
   runner delay with `DuctWallLoss` shelf, an optional expansion chamber, a
   collector trunk delay, `DuctModeCutoff`, the Levine-Schwinger outlet
   biquad, and a `FreeFieldObserver` per outlet. This is a handful of
   biquads and delay lines per path, well under budget, and it scales from
   the inline-4 to a flat-12 by data alone.
4. Add `NonlinearDuctAcoustics` as the load-dependent brightness control,
   driven by a pressure estimate proportional to load, and `ExhaustJetNoise`
   as the flow hiss. Both have closed forms and no tuning tables.
5. Finish with an `AutoGainSoftClipper` stage. The journal shows that it is
   where the preferred simulator gets its crest factor.
6. Afterfire: reuse the ECU gating and the slug scheduler exactly (arm on
   throttle above 0.20 at or above a minimum rpm, fire on closed throttle
   above 1.65 x idle, deterministic low-discrepancy intervals of 300 to
   700 ms, duty 8 %). Replace the chemistry with a wall-heat state (a
   first-order integrator with a time constant of tens of seconds, fed by
   load) that gates pops. Replace the acoustic source with a compact pressure
   jump injected into the path delay line plus the `ReactionCrackSynthesiser`
   band. Keep the measured pop shape as the target: 0.2 to 0.8 ms front,
   2 to 4 ms duration, centroid 450 to 790 Hz, plus a 4 to 8 kHz crack.
   Expect to tune beyond EngineLab's result, which the owner did not accept.
7. Voicing as data: adopt the three-layer YAML merge and the field list, and
   keep the signal in pascals with one calibration constant.
8. Quality tiers map naturally: tier 0 is firing schedule plus one delay and
   one outlet biquad per path; tier 1 adds wall loss, mode cutoff, chamber
   and jet noise; tier 2 adds steepening, per-outlet stereo observer and
   structural modes.
9. Reference renders: if Linux builds succeed, use
   `EngineLabOfflineAudioExporter --stems` for K20, LS3 and Aircooled. If not,
   the catalogue YAML (geometry, firing order, crank offsets) is still the
   usable reference, together with the measured figures above.

## Open questions

1. Does `EngineLabOfflineAudioExporter` build on Linux with GCC 13 or Clang
   17? Unverified. The `juce_audio_devices` link and the 8 MB stack are the
   two likely failures.
2. Doc inconsistency on the production exhaust cell length: 300 mm
   (`docs/thermoacoustic-architecture.md` section 1), 360 mm
   (`docs/architecture.md`, journal 2026-10-05), 95 mm target with three
   cells in `EngineSimulator.cpp` line 809. Not resolved from the sources
   read.
3. The structural NVH layer is implemented but switched off by the owner's
   ear. Whether a cheap block-mode layer helps an arcade mix is untested.
4. No flat-12 or any 180 degree V12 exists in the catalogue. The firing
   order and bank split for a Testarossa-like engine would need another
   source.
5. The afterfire acoustic result was rejected by the owner. The gating logic
   is sound; the sound of the pop itself still needs a reference recording.
6. The pop crack ratio (`defaultCrackRatio = 1.0`, bracket 0.8 to 2.5) is
   uncalibrated.
7. Realtime figures were taken on an i5-10600 and a Ryzen 7 8840U; nothing
   on mobile or Switch-class CPUs. Only the audio callback figures (15 to
   33 % of a 5.3 ms block for the full waveguide network) hint at the cost of
   a waveguide-only approach, and those include eight-path networks far
   larger than `snarl` needs.
