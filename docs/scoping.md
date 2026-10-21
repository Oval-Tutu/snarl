# snarl scoping

Date: 2026-10-17. Synthesis of the six phase 1 reports in `docs/research/`. Each claim names its report. Figures marked "estimate" are mine and are unmeasured.

## 1. Summary

snarl is a procedural engine sound generator in Go for FiveHorizons. The game writes RPM, throttle, acceleration, gear and event counters into an atomic snapshot each tick. snarl renders mono float32 on the audio goroutine with zero allocations. The model is a pulse train: a crank phasor, one shaped pulse per firing, per-bank delay lines and a crossover, a small exhaust body of delay lines and biquads, gated noise layers, and pops as events. engine-sim and EngineLab are references for the cheap stages only, not ports ([engine-sim.md](research/engine-sim.md), [enginelab.md](research/enginelab.md)). One model, tier 1, runs on every target from the Switch to a modern PC. A tier 2 for modern PCs is a later option. The quality bar is four profiles that sound different from each other and believable for their type. An offline prototype passes a go or no-go gate against reference renders. On no-go, a sample-based backend replaces it behind the same API and the game does not change.

## 2. Model design

Signal chain in order, one row for each layer and processing stage of the model. The origin is one of three values. "From engine-sim" and "from EngineLab" name the upstream element that snarl copies in form, not in code. "New (approved)" is a snarl element that neither upstream has and that the owner approved: the pop kinds and triggers, the idle character, the limiter bark, the jitter and the final gear waver, the output safety stages (soft clipper and peak limiter) and the overrun level. "New, not approved" is a snarl element that neither upstream has and that the owner did not approve. The list after the table repeats these elements. Sources: [prior-art.md](research/prior-art.md) (PA), [engine-sim.md](research/engine-sim.md) (ES), [enginelab.md](research/enginelab.md) (EL), [engine-profiles.md](research/engine-profiles.md) (EP), [static-analysis.md](research/static-analysis.md) (SA), [hiss-analysis.md](research/hiss-analysis.md) (HA).

| Stage | Design | Origin | Source |
| --- | --- | --- | --- |
| Crank phase | Phasor at `RPM/120` Hz, scalar crank state driven by game RPM. No rigid-body solver. | From engine-sim: the crank angle, with a phasor in place of the engine-sim solver | PA 6, ES 4 |
| Firing schedule | Per-cylinder crank offset and bank id from the profile. | From engine-sim: the ignition module. EngineLab has the same data in `crank_offset_deg` and `bank_id` | ES 4, EL 4 |
| Rev limiter | The rev limiter starts a cut when the snapshot RPM reaches `limiter_rpm` with the throttle above 0.1, or on a `Limiter` event. A cut of `limiter_ms` stops the firing of every cylinder, as engine-sim cuts its ignition. In a cut, each cylinder pumps a pulse at its firing time at `limiter_pump_db` relative to a firing, with no combustion variation. The pumping level falls along a raised cosine to `limiter_notch_db` 35 ms after the cut starts and returns to `limiter_pump_db` 35 ms later. The pump and notch levels come from the engine-sim limiter references, whose leveller lifts the pumping close to the firing level after a short notch. The level shape updates once per control block and sets the gain of the levelled engine, which ramps over the block, so that the leveller does not undo it. As in engine-sim, the bounce comes from an internal RPM, so it is audible at a flat snapshot RPM. In a cut, the internal RPM falls at `limiter_fall_rpm_s`. After the cut, the engine fires and the internal RPM climbs at `limiter_rise_rpm_s` back to `limiter_rpm`, where the next cut starts. The dip is `limiter_fall_rpm_s` times `limiter_ms`, and one bounce takes `limiter_ms` plus the dip divided by `limiter_rise_rpm_s`. `limiter_load_scale` multiplies both rates for an engine in gear, which makes the dip larger at the same bounce rate. The rates and the cut length come from the free-running engine-sim limiter references, except the v8 cut length, which is set by ear to 140 ms with a jitter of 0.15. The internal RPM updates once per control block with one multiply-add. No overrun level or overrun pop plays during a cut or a climb. | From engine-sim: the ignition module cut and the leveller output of the limiter references | ES 4 |
| Limiter bark | After a cut, the levelled engine has `limiter_bark_db` of extra gain, which decays with a time constant of 10 ms. The level comes from the engine-sim limiter references. The flat12 sets 0 dB, because the leveller recovery after a cut gives it a bark of +2.1 dB against +1.0 dB in engine-sim. | New (approved) | ES 4 |
| Limiter jitter | `limiter_jitter` (default 0) multiplies each cut length by a random factor from 1 minus to 1 plus its value, drawn once per cut from the engine's random source, so that the bounce is less regular. With 0, no draw runs, and the output does not change. | New, not approved | ES 4 |
| Limiter handover | When the throttle closes, the gear changes or the snapshot RPM falls below the limiter, the pitch returns from the internal RPM to the snapshot RPM over about 50 ms. | New, not approved | ES 4 |
| Final gear hold | With `FlagFinalGear`, the engine runs out of power before the limiter, as a real car does in top gear: no cut starts, the `Limiter` event is ignored, and the snapshot RPM stops at `limiter_rpm`. A cut in progress ends at once with no bark, the pitch returns over about 50 ms, and the gain of the levelled engine rises from the pump level at 1 dB per 5 ms. | New, not approved | `FlagFinalGear` in `api.go` |
| Final gear waver | At `limiter_rpm` in the final gear with the throttle open, the pitch wavers below `limiter_rpm` by up to `final_gear_waver` (default 0.003, about 20 RPM at 7000 RPM), from two table sines at 0.37 Hz and 1.13 Hz, updated once per control block. | New (approved) | `FlagFinalGear` in `api.go` |
| Per-cylinder pulse | Envelope `(1-exp(-a*phi))*exp(-b*phi)` over `[pulse] width_deg` with harmonic decay, in place of the exhaust runner pressure of engine-sim. The envelope runs live per pulse, with the attack set once per control block. | From engine-sim: the exhaust runner pressure tap and the valve timing that sets the pulse width. The envelope form is the PTR envelope | PA 1 and 6, ES 4 |
| Cam switch | Above `cam_switch_rpm`, `[pulse] cam_attack` replaces the attack and `[pulse] cam_gain` adds pulse gain. The switch has a hysteresis of 100 RPM and a crossfade with a time constant of 15 ms. Only the inline4 sets it. | From engine-sim: the cam profile switch of the inline-4 script (`atg-video-1/05_honda_vtec.mr`) | ES 4 |
| Fractional pulse start | Each pulse starts at its fractional sample position: its first sample is the envelope at that position, two exponentials per firing. A two-sample polynomial band-limited ramp rounds the corner of the start, and the pulse subtracts its end value along its length, so that it ends at zero. The pulses then repeat from cycle to cycle, and the silent engine has a residual of -81 to -119 dB. | New, not approved | SA |
| Pulse level | The amplitude rises with RPM. It also rises with load along a smooth load curve, from the overrun level at load 0, through `[pulse] part_load_db` at load 0.5, to full load at load 1. The wave steepening and the jet noise follow the pulse gain. Above the knee of the leveller, the output level does not follow the pulse level. | From engine-sim: the intake charge mass that sets the pulse amplitude under throttle | ES 4 |
| Overrun level | The overrun level, `[overrun] level_db` at the redline and `idle_level_db` at idle, is about 1 dB below full throttle at the same RPM. It sets the output level only below the knee of the leveller, at closed-throttle idle. Above the knee, the leveller holds the overrun within about 2.5 dB of full throttle at the same RPM, as in engine-sim. | New (approved) | ES 4 |
| Combustion variation | Random per-firing gain spread, `[pulse] cycle_variation`. | From engine-sim: `burning_efficiency_randomness` | ES 4 |
| Cylinder gain spread | Fixed per-cylinder gain spread, `[pulse] gain_variation`. | From engine-sim: the per-cylinder `sound_attenuation` | ES 4 |
| Cylinder timing spread | Fixed per-cylinder firing angle spread, `[pulse] timing_variation_deg`. | New, not approved | PA 6 |
| Jitter | Slow random RPM and gain wander, `[pulse] jitter`, updated once per control block. | New (approved): Farnell jitter | PA 1 and 6 |
| Low-RPM character | The `[idle]` settings: random per-firing gain (`variation`) and delay (`timing_deg`), a larger fixed per-cylinder gain spread (`imbalance`) for more half-order content, a slow RPM modulation (`hunt`) and a slow pulse amplitude modulation (`rock`) at `hunt_hz` with a slow random wander. Each has its full depth at idle and falls to zero at `fade_rpm`. All except `imbalance` also fall with load to zero at full load. Above `fade_rpm` the output does not change. | New (approved). The random gain spread follows engine-sim `burning_efficiency_randomness` | ES 4 |
| Per-bank delay lines | One delay per cylinder from primary length at a hot-gas speed of sound of 550 to 580 m/s, summed per bank. Unequal lengths give rumble. | From engine-sim: `DelayFilter` and `primary_length`. EngineLab has per-duct delays | ES 4, EL Implications 2 |
| Collector overlap | `[exhaust] overlap` adds the square of each bank sum to it, before the crossover, so that overlapping pulses of one bank add a pressure peak and keep the per-bank firing pattern audible. | New, not approved | SA |
| Crossover | Profile parameter with three states: none, full merge, partial with a coupling factor. | From EngineLab: `ExhaustCrossoverScattering`, LS3 `crossover_coupling 0.30` | EP Implications 2, EL 6 |
| Collector | Per path: a delay of twice the collector length, with the `reflection` gain and a one-pole `damping_hz` low pass in its feedback. | From EngineLab: the duct delay lines and the junction reflection | EL 4 |
| Wall loss | Per path: a high shelf of `wall_loss_db` at `wall_loss_hz`. | From EngineLab: `DuctWallLoss` | EL 4 |
| Low shelf | Per path: a low shelf of `[exhaust] low_shelf_db` below `low_shelf_hz`. | From EngineLab: the user shelf of the master stage | EL 4 |
| Expansion chamber | Per path, optional: `chamber_m` and `chamber_area_ratio` set one delay line with two junctions. | From EngineLab: `ExpansionChamberMuffler` | EL 4 |
| Mode cutoff | Per path: a fourth-order Butterworth low pass at the plane-mode cutoff of `pipe_radius_m`. | From EngineLab: `DuctModeCutoff` | EL 4 |
| Wave steepening | Per path: the engine wave plus `[exhaust] steepening` times the wave and its clamped magnitude, scaled by the pulse gain. The pops pass it unchanged. | From EngineLab: `NonlinearDuctAcoustics` | EL 4 |
| Outlet | Per path: a Levine-Schwinger outlet biquad from `outlet_radius_m`. | From EngineLab: `UnflangedPipeRadiation` | EL 4 |
| Outlet direct | Per path: `[exhaust] outlet_direct` times the pressure before the outlet biquad passes next to it, which keeps low-frequency weight. | New, not approved | EL 4 |
| Path gain | Per path: `gain`, the level of the path against the other paths. | From engine-sim: `audio_volume` of the exhaust system | ES 4 |
| Listener distance | Per path, optional: a delay of `listener_m` through air at 343 m/s. | From EngineLab: the `r/c` delay of `FreeFieldObserver` | EL 4 |
| Pulse low pass | Per path, before the exhaust stages: a fourth-order Butterworth low pass at 1.9 kHz on the pulse sum, two biquads. It removes the harmonics above 2 to 3 kHz: at the 3000 RPM full-throttle hold, the 2.8 to 5.7 kHz octave is now -34 to -44 dB against -31 to -45 dB in engine-sim, and was -21 to -28 dB. | From engine-sim: the low pass of the synthesiser input | ES 3, engine-sim-target.md 3.1 |
| Derivative mix | Per path: `[noise] hf_gain` times the derivative of the low-passed pulse per second, plus 1 minus `hf_gain` times the pulse times the AM term. The input scales by 1/(1 - `hf_gain` + `hf_gain` times 2 pi 400 Hz), so that it keeps about the scale of the pulse. The tuned profiles set `hf_gain` 0: with the derivative, the 31 to 125 Hz octaves match engine-sim within 0.3 to 5 dB, but the exhaust path turns each derivative pulse into a short spike, with a crest factor of 10 to 17 dB against 6 to 8 dB in engine-sim. The engine-sim impulse response spreads the same pulse over 45 ms. | From engine-sim: `hf_gain` (`dF_F_mix`) | ES 3, engine-sim-target.md 3.1 |
| Noise AM | Per path, before the exhaust stages: the pulse times `am` times r plus 1 minus `am`, where r is uniform noise through a fourth-order Butterworth low pass at `[noise] am_cutoff_hz` (2 kHz), two biquads shared by the paths. At `am` 1, the AM term has no constant part. The noise follows the pulse, so it rises and falls over each cycle: the AM peak is 4.6 to 12.5 dB and the trough -7 to -24 dB at full throttle, against 2.8 to 11.2 dB and -4.1 to -18.3 dB in engine-sim. `am` sets the 2 to 6 kHz residual of the idle hold to the engine-sim value within 0.3 dB: 0.36 (flat12), 0.44 (flat6), 0.06 (inline4) and 0.25 (v8). | From engine-sim: the air noise of the synthesiser | ES 3, engine-sim-target.md 5 |
| Noise fall with RPM | With `hf_gain` 0, the noise gain of the AM term falls with the square root of the idle RPM over the RPM, once per control block. In engine-sim, the derivative weights each harmonic by its frequency, so that the share of the noise falls as the RPM rises. With `hf_gain` above 0, the fall is off. | New, not approved | engine-sim-target.md 3.2 |
| Jet noise | Band-passed noise at `[noise] jet_hz`, scaled by `[noise] jet`, the flow and the RPM. It is part of the engine before the leveller, so it follows the limiter levels with the engine. flat6 and v8 set `jet` to 0.003 and 0.015, which puts the 6 to 16 kHz residual of the top hold at the engine-sim value within 0.5 dB. | From EngineLab: `ExhaustJetNoise` | EL 4 |
| Pops | Two sources. Requested pops: the game increments `Snapshot.PopRequest` for each pop that it wants, for example a gear change under the right conditions, and sets `PopStrength` from 0 to 1. Each request plays one pop at the next audio block, with no random chance and no wall heat, at least 50 ms after the pop before. A request inside those 50 ms waits, and up to four requests wait. Below 1/3 the strength gives a pop or a burble, below 2/3 a pop or a boom, and from 2/3 a boom or a short crackle of at most 10 sub-events. The kind is random but never the kind of the last request, the level spreads over plus or minus 3 dB and the body frequency over plus or minus 8 per cent. When every one of the 4 slots is busy, it takes the slot of the oldest own pop, or of the oldest pop. Own pops: a lift-off (the throttle falls by more than 0.5 in about 150 ms, above 0.6 of the redline, on overrun, with a warm exhaust) starts a run with a chance of 0.8. The run has 1 to 3 pops, each next pop with 0.6 times the chance of the one before. The first is a crackle or a boom, the later ones a pop or a burble, 150 to 400 ms after the lift and then 110 to 264 ms apart. The run ends 1.5 to 2.5 s after the lift, and no lift-off run starts in the 2 s after that. A wind-down splits the RPM from the lift-off RPM down to 1.3 times idle into bands of about 1000 RPM. As the smoothed RPM on overrun falls into each band, one chance of 0.55 plays a pop or a burble, with a pop share of 0.8 in the top band down to 0.2 in the lowest. A band rolls again after the RPM rises 150 RPM above its top, or after the overrun ends, so a coast at a constant RPM plays no pop. Own pops keep to at most 6 in any 10 s, and at least 0.7 s apart when two triggers alternate. A lift-off that misses its chance still rests. 5 times `[pops] density` multiplies each own chance. With the profile densities, the sport character plays about 2.5 own pops on average from a lift at the redline down to idle. `[pops] character` (`stock`, `sport` by default, or `race`, or `Snapshot.PopCharacter` at run time) multiplies the own chances by 0.5, 1 or 1.8 and their gain by -6, 0 or +3 dB, lets a stock first pop and a race later pop be any kind, and changes the requested pops by -2, 0 or +1 dB. `FlagNoPops` stops the own pops. An ignition cut bang has a chance of twice the density, and plays with `FlagNoPops`. A wall-heat integrator silences the own pops of a cold engine. All triggers run once per control block. Each pop is a pressure jump into the path delay line plus a crack above 3.8 kHz. The sub-events of a crackle are clicks: a noise burst through a 1.5 kHz band pass that starts at once and decays to -20 dB in 9.5 ms, at log-normal gaps around 6.5 ms, from the clicks of the crackle pack (crackles.md section 11). Every pop, own, requested or a bang, scales by the engine level, so that it keeps one balance with the engine on every profile and at any throttle. The engine level is the RMS level of the levelled engine, with no DC, followed over 50 ms in the control blocks with no pop and held while a pop sounds. Each kind has a level over the engine: +4.8 dB for a pop, +4.5 dB for a boom, +1.5 dB for a crackle and -4.3 dB for a burble, as the RMS level over the span from the pop start that holds 90 per cent of its energy, against the engine with no pop over the same span. `New` sets the gain of each kind on each path from 7 rendered pops at half the sample rate, for own and for requested pops. A process-wide cache holds these gains for each set of path, exhaust and pop shape values and each sample rate, so that only the first `New` for a profile renders them. The wave steepening acts on the engine wave only: while the pop jump of a path sounds, to six body decays after its last sub-event, and for 100 ms after that, the linear stages, the outlet and the listener delay of that path run a second time for the pop alone. The pops join the engine after the leveller, so that a pop never moves the leveller gain. `[pops] gain` multiplies every pop over these levels. `Engine.Pops` reports the pop count and the kind, source and strength of the latest pop. | New (approved). The gating and the crack follow EngineLab ECU gating and `ReactionCrackSynthesiser`, the run chances follow the Baldan decaying probability, racing-game-pops.md section 5 and fivehorizons-exhaust.md | EL 5, PA 4, EP Implications 7 |
| DC block | 10 Hz DC block on the engine sum of the paths, before the leveller, and on the sum of the levelled engine and the pops. | From engine-sim: the DC removal of the synthesiser | ES 3 |
| Leveller | `[voice] leveller_db` of gain into the engine-sim peak leveller: the peak follows the input magnitude at once and decays by 0.999 per sample at 44.1 kHz (8.7 dB in 22.7 ms), the gain moves towards the target over the peak, at most 1.9, by a one-pole smoothing of 0.1 per sample at 44.1 kHz, both converted to 48 kHz. Above the knee, at the target over 1.9, the output peak stays at the target. About 9 operations per sample, with a division only on a sample that sets a new peak, and no call into the math package. The pops join after it. `leveller_db` puts the closed-throttle idle 2 to 7 dB below the knee, so that the idle character stays. Over the sweep-wot scenario, the 0.5 s peak stays in a band of 2.3 to 3.7 dB (engine-sim 1.3 to 2.3 dB, no leveller 3.6 to 5.6 dB) and the loudness in a band of 2.3 to 4.3 LU (engine-sim 1.4 to 3.0 LU, no leveller 2.9 to 5.8 LU). The loudness band is wider than in engine-sim, because the crest factor of the snarl exhaust path rises from about 6 dB to about 14 dB with RPM. | From engine-sim: `LevelingFilter` | ES 3, engine-sim-target.md 3.3 |
| Output low pass | Low pass at 0.45 times the sample rate. | From engine-sim: the output Butterworth at 0.45 fs | ES 3 |
| Calibration gain | `[voice] output_db`, a fixed gain per profile after the leveller and the pops that calibrates the sweep-wot scenario to -16 LUFS. It stays, because the crest factor, and so the loudness at the leveller target, differs by up to 2.5 LU between the profiles. | From engine-sim: the `volume` after the leveller | ES 3 |
| Peak limiter and soft clipper | A peak limiter with 0.5 ms look-ahead into a cubic soft clipper. Mono float32 into the caller's buffer. Both still act: on the pops of the overrun and limiter scenarios, the output reaches -0.22 dBFS, the soft clipper output for the limiter ceiling, and the soft clipper takes about 0.8 dB off the highest engine peaks of the sweep, at -2.6 dBFS. | New (approved). The soft clipper takes the place of the EngineLab `AutoGainSoftClipper` | EL 4 |

Not reused: the engine-sim solvers and 10,000-tap convolution (ES 3), the EngineLab finite-volume network, intake network and forced induction layer (EL 4).

Removed: the valve tick, the intake noise with its step at the cam switch, and the cooling fan tone. Neither upstream has these layers. The cam switch of the pulse stays, because the engine-sim inline-4 has it. Three more stages are removed. The gain over RPM (`[voice] idle_gain_db` and `idle_gain_rpm`) is redundant, because the leveller holds the level over RPM above its knee. The noise AM shaping (the load gate, `am_redline` and `am_idle`) is replaced by the engine-sim AM term and the noise fall with RPM. The jet noise gate of a limiter cut is redundant, because the limiter levels now act on the whole levelled engine.

New, not approved. These elements stay in the model until the owner decides on them:

- Limiter jitter (`limiter_jitter`), default 0. Only the v8 sets it.
- Limiter handover: the 50 ms return of the pitch to the snapshot RPM after a bounce.
- Final gear hold: no cut in the final gear, with the firing gain rise after a cut.
- Fractional pulse start: the band-limited corner and the end at zero of each pulse.
- Cylinder timing spread (`[pulse] timing_variation_deg`).
- Collector overlap (`[exhaust] overlap`).
- Outlet direct (`[exhaust] outlet_direct`).
- Noise fall with RPM: the square-root fall of the noise gain with `hf_gain` 0, in place of the engine-sim derivative weighting.

## 3. Quality tiers

The tier is a construction parameter. `Tier1` and `Tier2` are the only values, and `New` returns an error for any other value. `Tier2` renders the tier 1 model until a tier 2 prototype decides whether the parameter stays. An earlier tier 0, with a table pulse and the path without the tier 1 filters, sounded thin and is removed. Tier 1 costs 1.3 to 1.6 times what tier 0 cost, and at most 1.21 per cent of one OnePlus 3T fast core (measured).

| Tier | Target | Content | Cost |
| --- | --- | --- | --- |
| 1 | All targets: Nintendo Switch, OnePlus 3T, phones and PCs | Firing schedule, live pulse shaping per cylinder, per-cylinder gain and timing imbalance, per-bank delay lines, crossover mix, and per path a collector delay, wall-loss shelf, first-order low shelf, expansion chamber, mode cutoff, steepening and outlet biquad. Noise AM, jet noise, pops, peak limiter with 0.5 ms look-ahead, soft clipper | At most 1.21 per cent of one OnePlus 3T fast core (measured) |
| 2 | Modern PC | Tier 1 plus a resonator bank or 512 to 1024 tap IR per path, two-outlet stereo observer, optional structural modes | 45 to 90 million multiply-adds per second per channel for the IR form (engine-sim.md Implications 2, estimate) |

## 4. Budgets

CPU: about 5 per cent of one core. On the Switch that is 51 million cycles per second, about 1060 cycles per stereo frame at 48 kHz ([platform.md](research/platform.md) section 5). Rules: scalar float32 in the per-sample loop, coefficients per block in float64, no `math.Sin`, `math.Exp` or division per sample, SIMD only as an experiment behind a build tag (platform.md section 5).

Latency: the 30 ms target is unreachable with stock Ebitengine. The `audio` package passes no device buffer size to oto, so every platform runs the driver default (43 to 128 ms) plus the player buffer. Expect 80 to 150 ms on desktop and 100 to 250 ms on phones (platform.md Conclusion). Options from platform.md section 7:

1. Fork change in `audio/context.go` to pass `BufferSize` to oto. Brings Linux and Windows to about 40 to 60 ms.
2. Fork change in oto's Oboe binding for Android, a C++ change.
3. Predict RPM 50 to 100 ms ahead from throttle and gear, so the perceived lag is the physics lag.
4. Revise the target to the measured figure.

platform.md recommends option 1 first, then measurement. Caveat: `docs/audio.md` line 17 and decision 0020 in FiveHorizons require no change to the Ebitengine fork (fivehorizons.md section 4). See section 11.

## 5. Public API sketch

Backend neutral. The game owns the one `audio.NewPlayerF32` stream and the mixer (fivehorizons.md section 7), so snarl exposes a voice, not a player. The harness wraps the voice in its own `io.Reader` that returns one 256-frame block per `Read` (platform.md Implications).

```go
// Snapshot is written by the game once per tick and read once per block.
type Snapshot struct {
	RPM      float32 // handling.State.RPM, 1000 to 8000
	Throttle float32 // 0 to 1
	Accel    float32 // m/s per second, snarl derives load from it
	Speed    float32 // m/s, for wind and road layers later
	Gear     uint8   // any count, 0 is lowest
	Flags    uint8   // Running, Airborne, Wheelspin, Clutch, RedZone, Overrun, FinalGear, NoPops

	// Event counters: the reader detects a change by difference.
	ShiftUp, ShiftDown, Limiter uint8

	PopRequest   uint8        // counter: one pop per increment
	PopStrength  float32      // 0 to 1, the strength of the latest request
	PopCharacter PopCharacter // 0 keeps the profile character, then Stock, Sport, Race
}

type Engine interface {
	Set(Snapshot)         // game goroutine, atomic store
	Render(dst []float32) // audio goroutine, mono, no allocation
	Pops() PopReport      // any goroutine, one atomic load, never blocks Render
}

// PopReport is the running count of pops and the latest pop.
type PopReport struct {
	Count    uint32
	Kind     PopKind   // KindPop, KindBoom, KindCrackle, KindBurble
	Source   PopSource // SourceRequest, SourceOwn, SourceLimiter
	Strength float32
}

func New(p Profile, sampleRate int, tier Tier) (Engine, error)
```

The game-side mapping of each field is in fivehorizons.md section 7. Load: the handling model has none, so snarl derives `load = clamp(max(Throttle, Accel / peakAccel[Gear]), 0, 1)`, smoothed, and forces it to zero while `Overrun` is set. The throttle is the lower limit, so a steady part throttle keeps its load, and acceleration only raises it. The game sets `Overrun` when `Throttle < 0.05` and RPM falls. snarl detects the limiter from the RPM and synthesises the cut and the bounce, because the handling model never bounces (fivehorizons.md section 2). The `Limiter` counter is an optional trigger with the same effect. The game decides when a pop is part of the feedback: it increments `PopRequest` on a gear change under its own conditions, and snarl plays that pop every time. snarl adds rare pops of its own on a lift-off and on a wind-down through the RPM bands on overrun, which the game can stop with `FlagNoPops`. The run-time character rides in the snapshot, so that it uses the same sequence lock as the other fields and needs no extra method. `Pops` reads one atomic word that `Render` writes once per block, so the game can show or react to each pop. The sample backend implements the same interface with a loop crossfade every 500 RPM and on-load and off-load layers (prior-art.md section 2).

## 6. Engine profiles

Format proposal: one TOML file per profile under `profiles/`, embedded and parsed at construction, outside the audio path. TOML matches the game catalogue (fivehorizons.md section 5). EngineLab's three-layer merge of default, family and engine is the model for defaults (enginelab.md section 4). Fields: identity, idle and redline, limiter type and RPM, cylinders (crank offset, bank), banks (primary lengths, collector length, outlet radius), crossover state and coupling, pulse shape, noise gains, pop density, voicing gains. Identifiers avoid manufacturer names, which the game forbids (fivehorizons.md Implications): `flat12`, `inline4`, `flat6`, `v8`.

Key parameters from engine-profiles.md:

| Profile | Engine | Per-bank interval | Dominant order | Idle | Redline and limiter | Exhaust and crossover | Pop density |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `flat12` | Ferrari Testarossa F113 | 120 even, banks 60 apart | 6th | about 1000, unverified | 6800, limiter unverified | Two 6-into-1 bank pipes into one silencer, four tailpipes. Crossover unknown, start with none | 0.2 |
| `inline4` | Honda B18C5 | 180 even, one bank | 2nd | 800 | 8400, fuel cut 8500 | 4-2-1 header, one silencer, one tailpipe. VTEC step at 5700 | 0.17 |
| `flat6` | Porsche M64 (993) | 240 even per bank, 120 overall | 3rd | 800 | 6800, governed 6700 | Per-bank 3-into-1 into a centre mixing chamber, two silencers. Full merge | 0.2 |
| `v8` | Ford 5.0 HO | 90-180-270-180 per bank | 4th with 0.5 and 1.5 sidebands | 675 | about 6000, fuel cut 6250 one injector at a time | Shorty headers, H-pipe, two silencers. Partial crossover | 0.2 |

The pop density is the `[pops] density` value of each profile. The starting points from engine-profiles.md were 0.4 (0.8 decat) for `flat12`, 0.05 for `inline4`, 0.1 for `flat6` and 0.25 for `v8`. They are tuning values, not measurements (engine-profiles.md Implications 7). The `inline4`, `flat6` and `v8` values rose with the wind-down trigger. Gaps (engine-profiles.md Open questions): the Testarossa firing order is the 512 BB value, its idle, limiter and valve timing are unverified, and its silencer internals are unknown. Pipe diameters and lengths are unknown for all but the Fox 2.25 in pipe. The 993 fan ratio comes from the 1976 fan.

## 7. References and comparison tool

References on Linux, in preference order:

1. Headless engine-sim through the Nix flake: a new executable that links the core library and script interpreter, runs a `.mr` script and writes a WAV plus a CSV of per-cylinder runner pressure and RPM (engine-sim.md section 7). Inline-4 and cross-plane V8 scripts exist. Derive the flat-6 from the EJ25 and the flat-12 from the Merlin with banks at plus and minus 90 degrees (engine-sim.md section 5). The Linux build is unverified.
2. `EngineLabOfflineAudioExporter` with warnings as errors off, tests off and an 8 MB or larger stack. Renders K20, LS3 and the flat-6 with stems. No flat-12. JUCE on Linux is unverified (enginelab.md section 7).
3. Wine with the Windows binaries. No CSV internals.
4. CC0 recordings: an air-cooled 911 idle and a 1985 Mustang idle. None for a flat-12 or a B-series (engine-profiles.md Reference recordings).

Comparison tool (prior-art.md section 5): drive both renders with the same RPM and throttle script, normalise with `ebur128`, then per RPM and throttle cell compute the order spectrum distance for orders 0.5 to 20, third-octave log-spectral distance, multi-resolution short-time Fourier transform (STFT) distance, crest factor and spectral flux. Report each cell and the worst cell with a spectrogram pair. ffmpeg supplies `ebur128`, `astats`, `aspectralstats` and `showspectrumpic`. The order analysis is Go code.

Gate criteria, proposed:

- The user tells the four profiles apart in a blind listen and judges each believable for its type.
- The order spectrum of each profile shows the expected dominant order and per-bank sidebands from section 6.
- Measured CPU per block on amd64 and arm64 projects under 5 per cent on the Switch.
- `Render` allocates nothing.
- Metric thresholds are set after the first reference renders, because no published benchmark exists for a pulse model (prior-art.md Open questions).

## 8. Repository layout and tooling

| Path | Content |
| --- | --- |
| root package `snarl` | Public API: `Snapshot`, `Profile`, `Engine`, `New` |
| `internal/dsp`, `internal/model` | Delay lines, biquads, noise, soft clipper. Crank, firing schedule, pulse, exhaust path, pops |
| `internal/sample` | Sample backend for the gate fallback |
| `profiles/` | TOML profiles |
| `internal/pads`, `internal/revmodel` | Copies of FiveHorizons `internal/input/pads.go` with `StickDead` and the keyd tests, and of `handling.Model.engine` with `revClimb` for any gear count. Header comments name the source files (fivehorizons.md sections 1 and Implications) |
| `cmd/snarl-harness` | Ebitengine app: pad input, on-screen display of RPM, throttle, load and gear, `io.Reader` adapter, underrun counter |
| `cmd/snarl-render`, `cmd/snarl-compare` | Offline WAV from an RPM and throttle script. Metrics and spectrograms |
| `nix/engine-sim.nix` | Headless reference renderer package |

`justfile` recipes: `build`, `test` (`go vet`, `go test -cover -covermode=atomic`), `lint` (`go mod tidy -diff`, `golangci-lint fmt --diff`, `golangci-lint run`, `govulncheck`, `gocyclo`), `bench`, `harness`, `render <profile> <script>`, `compare <a> <b>`, `ref-engine-sim <profile>`, `ref-enginelab <engine>`. The test and lint commands match `just/tailor.just` in the game (fivehorizons.md section 6).

Dev shell additions to `flake.nix`: golangci-lint, govulncheck, gocyclo, ffmpeg-headless, and cmake, ninja, boost, flex and bison for the reference build. `.golangci.yml` already matches the game. `go.mod` needs `go 1.26.9` and `toolchain go1.27.2` (fivehorizons.md section 6).

Run tailor only from its full path, `/home/martin/Development/wimpysworld/tailor/tailor`. The dev shell does not include tailor, and the flake has no input for it.

## 9. Revised phases

| Phase | Deliverable | Exit criterion |
| --- | --- | --- |
| 1 Research | Six reports and this document | Done |
| 2 Scaffold | `go.mod`, `justfile`, dev shell, public API types, `snarl-render` that writes a silent WAV | `just lint` and `just test` pass |
| 3 References | Headless engine-sim in the flake, EngineLab exporter attempt, CC0 clips, `snarl-compare` | A reference WAV and RPM track per profile (flat-12 derived), and a metrics report between two reference renders |
| 4 Offline prototype | Tier 0 and 1 model, four profiles, renders of idle, sweep, overrun and limiter | Gate review: go, or no-go to the sample backend |
| 5 Real-time harness | `snarl-harness`, pads and rev model copies, allocation test, benchmarks on amd64 and arm64, latency by the microphone method | Zero allocations, no underruns, measured CPU and latency per available platform |
| 6 Integration | Snapshot write in `Game.step`, mixer voice, car table with an engine profile, `docs/audio.md` and decision 0020 amended, fork `BufferSize` change if agreed | The game plays the engine with the chosen profile |
| 7 Polish | Pop tuning, tier 2, OnePlus 3T measurements, Switch when hardware exists | User sign-off per profile |

Phase 4b runs only on no-go: the sample backend behind the same API. It needs recordings that are not in hand for the flat-12 (prior-art.md Implications).

## 10. Risks

- Latency: stock Ebitengine cannot meet 30 ms, and the game's documents forbid fork changes (platform.md, fivehorizons.md section 4).
- CPU: tier 1 costs at most 1.21 per cent of one OnePlus 3T fast core (measured). The tier 2 cost is an estimate, and the Switch is not measured.
- Reference builds: engine-sim's piranha and EngineLab's JUCE are unverified on Linux (engine-sim.md section 7, enginelab.md section 7).
- Flat-12: no reference in either simulator, no free recording for the prototype or the sample fallback, and the firing order comes from the 512 BB (engine-profiles.md).
- Pops: EngineLab's owner rejected their own pop sound, engine-sim has none, and no recording was found (enginelab.md section 5, engine-sim.md section 6).
- Quality bar: no published listening benchmark exists for a pulse model (prior-art.md).
- Switch: buffer size and thread model are inside the closed `uwagaki` package, and there is no hardware (platform.md section 2).
- Game model: no load, no limiter, no clutch flag. snarl must synthesise each (fivehorizons.md section 2).

## 11. Open decisions

1. Latency target and fork change. Recommend: make fork change 1 (`BufferSize` option in `audio/context.go`), measure on Linux with the microphone method, set the target at about 50 ms on desktop with a documented phone figure, and amend `docs/audio.md` line 17 and decision 0020. Defer the Oboe change until the OnePlus 3T is measured.
2. Impulse response licence. enginelab.md section 8 reports the engine-sim IRs as MIT with a notice file. engine-sim.md section 8 finds no separate licence or provenance for the recordings. Recommend: ship no engine-sim IR. Tier 1 uses biquads and delay lines, tier 2 uses snarl's own IRs or a resonator bank. Use the engine-sim IRs only in internal reference renders.
3. Missing pop reference. Recommend: design to the EngineLab measured shape (0.2 to 0.8 ms front, 2 to 4 ms duration, centroid 450 to 790 Hz, plus a 4 to 8 kHz crack), tune by ear in the harness, and keep a one-shot sample layer behind the same event API. Ask the user for any recording they can source.
4. Testarossa firing order and manual gaps. Recommend: proceed with the 512 BB order. The per-bank 120-degree pattern does not depend on the numbering, so the sound risk is low. The user fetches the ManualsLib "Timing Data" pages for idle, valve timing and limiter. Keep the flat-12 crossover at none until the silencer internals are known.
5. Profile data format. Recommend TOML, embedded, parsed at construction.
6. Load derivation. Recommend acceleration based, derived in snarl.
7. Rev limiter and clutch. Recommend that snarl synthesises the cut from the `Limiter` counter and times `ShiftWindow` from the shift counters. No handling model change.
8. Third gear. Recommend any gear count in the harness now and a separate game change later (fivehorizons.md section 3).
9. Sample rate. Recommend a construction parameter, default 48 kHz.
10. Profile home in the game. Recommend a car table outside `render`, keyed by the car choice string (fivehorizons.md section 5).
11. Tooling owner. Recommend that tailor fits snarl, because the game names tailor as its tooling owner (fivehorizons.md section 6).
