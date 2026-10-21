# snarl scoping

Date: 2026-10-17. Synthesis of the six phase 1 reports in `docs/research/`. Each claim names its report. Figures marked "estimate" are mine and are unmeasured.

## 1. Summary

snarl is a procedural engine sound generator in Go for FiveHorizons. The game writes RPM, throttle, acceleration, gear and event counters into an atomic snapshot each tick. snarl renders mono float32 on the audio goroutine with zero allocations. The model is a pulse train: a crank phasor, one shaped pulse per firing, per-bank delay lines and a crossover, a small exhaust body of delay lines and biquads, gated noise layers, and pops as events. engine-sim and EngineLab are references for the cheap stages only, not ports ([engine-sim.md](research/engine-sim.md), [enginelab.md](research/enginelab.md)). One model, tier 1, runs on every target from the Switch to a modern PC. A tier 2 for modern PCs is a later option. The quality bar is four profiles that sound different from each other and believable for their type. An offline prototype passes a go or no-go gate against reference renders. On no-go, a sample-based backend replaces it behind the same API and the game does not change.

## 2. Engine design

The engine plays the synthesiser input of engine-sim from cycle tables into a real-time port of the engine-sim synthesiser, with the snarl game features around it. It replaces the pulse-train model of the earlier phases: the pulse tables, the formant and pop banks, the noise layers, the path stages, the snarl leveller, the output calibration, the peak limiter and the soft clipper are gone, with every profile key that only they used. Where sections 1, 3 and 8 name those stages, they describe the earlier model. The five embedded profiles are the engine-sim engines `i4-vtec`, `v8-ls`, `f4-ej25`, `v12-412` and `v12-merlin` (`profiles/README.md`). The EngineLab `f6-aircooled` is not embedded: EngineLab writes no synthesiser input, so it has no cycle tables.

Signal chain in order, one row for each stage. "From engine-sim" is an engine-sim element: ported code, or data measured from renders of the unchanged engine-sim scripts. "New (approved)" is a snarl element that engine-sim does not have and that the owner approved: the own pops, the pop requests and the pop report, the final gear hold, `FlagNoPops`, the pop character and the idle wander. "New, not approved" is a snarl element that the owner has not decided on. The list after the table repeats these elements. Sources: [cycle-tables.md](research/cycle-tables.md) (CT), [esynth-port.md](research/esynth-port.md) (EP), [esynth-runtime.md](research/esynth-runtime.md) (ER), [alignment-scorecard.md](research/alignment-scorecard.md) (AS), [reference-health.md](research/reference-health.md) (RH).

The engine runs in control blocks: one control block writes the simulation steps of one convolution block of 256 samples at 44.1 kHz (5.8 ms), when the synthesiser needs input. `Render` reads the snapshot once per call. `internal/core` holds the engine, `internal/cycles` the playback (`Live`) and `internal/esynth` the synthesiser (`Runtime`).

| Stage | Design | Origin | Source |
| --- | --- | --- | --- |
| RPM | The snapshot RPM, clamped from 0 to 20000, smoothed per control block with a time constant of 12 ms, and ramped linearly over the simulation steps of the block. In the final gear it stops at `limiter_rpm`. | New, not approved | |
| Idle wander | The crank speed times 1 plus `[idle] hunt` times 0.7 times a sine at `hunt_hz` plus 0.5 times a slow random wander with a standard deviation of 1, per control block. Full depth at idle, falling along a smoothstep to zero at `fade_rpm`, and with load to zero at half load. `hunt` and `hunt_hz` are the spread of the RPM averaged over one cycle and the main wander frequency of the free run at idle of each reference (`idle-free`). A dynamometer hold has no wander, so the scorecard of the holds turns it off. | New (approved). Measured from engine-sim | RH |
| Rev limiter | engine-sim cuts the ignition. At the limiter RPM with the throttle above 0.1, or on a `Limiter` event, a cut of `[limiter] cut_ms` turns the ignition off while an internal RPM falls at `fall_rpm_s`. After the cut, the internal RPM climbs at `rise_rpm_s` back to `limiter_rpm`, where the next cut starts. The cut and the climb run in time within the control block, so that the ignition turns off and on at the step where it would. `load_scale` multiplies both rates for an engine in gear. The rates and the cut are the median bounce of the free run at full throttle into the limiter of each reference (`limiter-free`). The engines bounce at 10.35, 3.67, 7.45, 6.42 and 10.55 Hz, against 10.3, 3.7, 7.5, 6.4 and 10.5 Hz in engine-sim (`TestLimiterBounce`). No own pop plays during a cut or a climb. | From engine-sim: the ignition module cut, measured | RH |
| Limiter handover | After a bounce or an event cut, the pitch returns from the internal RPM to the smoothed RPM with a time constant of 17 ms. | New, not approved | |
| Final gear hold | With `FlagFinalGear`, no cut starts, the `Limiter` event is ignored, a cut or a climb in progress ends, and the RPM stops at `limiter_rpm`. At the limiter with the throttle open, the pitch wavers below `limiter_rpm` by up to `[limiter] final_gear_waver` (0.003), from two table sines at 0.37 and 1.13 Hz. | New (approved) | `FlagFinalGear` in `api.go` |
| Crank phase | The phase advances per simulation step by the step time times the pitch RPM over 120, times the speed profile of the cycle: the crank speed over the cycle relative to its mean at 64 points, of the weighted table points. | From engine-sim: the crank angle and speed of the renders, measured | CT 4, 5 |
| Cycle tables | `tables` names `profiles/cycles/<id>.bin`: per grid point and exhaust channel, the average cycle of the synthesiser input as a Fourier series up to the Nyquist frequency of the simulation rate, on a grid of 250 rpm steps from idle to the top at throttle 0, 0.25, 0.5, 0.75 and 1, and ignition-off holds at throttle 0 and 1. Each channel reads its wave table, sampled 8 times above its harmonic count, at the emission phase (the crank phase less the channel delay at the RPM), with linear interpolation. The weights are linear in the throttle between the two nearest rows and linear in the RPM between the two nearest points of each row, and the edge outside the grid. A weight below 1e-6 goes to the first point. Played through the exact port, the tables score 98.2 to 100 on every hold and 100 on every sweep. | From engine-sim: the synthesiser input of the renders, measured | CT 3 to 5 |
| Intake lag | The throttle row weights move to their targets with a first-order lag of `[throttle] close_lag_s` when the throttle closes and `open_lag_s` when it opens, measured from the throttle steps of `grid/step.scn`. | From engine-sim, measured | CT 4 |
| Closing transient | After the throttle closes from 0.5 or more to 0.01 or less, engine-sim leaves the steady states for a second or more. The tables hold slices of the closing steps of `grid/close.scn` at the RPM of the ignition-off holds: each slice is a point measured over at least two whole cycles from 0.04 s after the close, with slices 1.5 times longer each time, up to 3 s. While the throttle stays closed, the slices take the weight of the closed-throttle row: linear in the time since the close between the two nearest slices, and in the RPM between the two nearest closing steps. After the last slice, the transient fades to the steady row over 1 s. A throttle above 0.01 ends it. With the slices, the closed-throttle window of the overrun scenario scores 100 on `i4-vtec` and `f4-ej25`, against 58.4 and 58.6 with the steady tables only. | From engine-sim, measured. The arming at 0.5 and the 1 s fade are new, not approved | CT 6.1 |
| Ignition | The ignition-off points take over from the ignition-on points, and back, linearly over one engine cycle, in which each cylinder misses or fires once. A switch at once clicked at each cut: the free-running limiter of `v12-merlin` scored 34.9 with it and 100 with the crossfade. | From engine-sim: the ignition-off holds, measured. The crossfade is new, not approved | CT 6.3 |
| Residual | Per channel and band: noise through the nine band filters of the tables, normalised to unit power, times the band envelope of the weighted points at the emission phase. The noise is the sum of four uniform values of one 64-bit xorshift draw, with the mean and the variance of a unit Gaussian. | From engine-sim, measured. The noise source is new | CT 4, 5 |
| Pops | The pop model of the earlier engine: requested pops (`PopRequest`, `PopStrength`), own pops on a lift-off and in the wind-down bands on overrun, the limits of the own pops, the character (`[pops] character` or `PopCharacter`), `FlagNoPops` and the pop report (`Pops`). A pop is a rounded pressure jump with a resonant body, and the sub-events of a boom, a crackle or a burble. It runs per simulation step on one exhaust channel in turn. Its pressure, over an engine of unit level, scales per control block by the RMS level of the engine before the leveller over the last 46 ms, so that it follows the engine that it sounds over. `New` calibrates the gain of each kind on each channel from 7 pops through the synthesiser without the leveller, to +4.8 dB for a pop, +4.5 dB for a boom, +1.5 dB for a crackle and -4.3 dB for a burble over the engine, and caches it per profile. In the engine, after the leveller, the medians are 4.4 to 6.5, 2.0 to 5.1, 0.6 to 2.8 and -4.5 to -1.0 dB (`TestPopLevelOverEngine`). The crack above 3.8 kHz and the clicks of the earlier engine are gone: the impulse response colours the pops. `[pops] limiter_bangs` is false, because the engine-sim cut has no afterfire. | New (approved) | crackles.md, racing-game-pops.md |
| Synthesiser input | Per channel, as `Synthesizer::writeInput`: linear interpolation from the simulation rate to 44.1 kHz, then the fourth-order 1900 Hz low pass in double. | From engine-sim: ported | EP, ER 1.2 |
| Synthesiser stages | Per sample and channel, as `Synthesizer::renderAudio`: the `rand()` draws, the jitter of 10 samples at 10 kHz, the 10 Hz DC removal, the derivative, and the mix of `dF_F_mix` times the derivative with the AC part times the air noise, with the audio parameters of the engine script. | From engine-sim: ported | EP, ER 1.2 |
| Pop side input | The pops enter as a second input of the synthesiser, which takes every stage of the engine input with the same random draws, so that the engine input plus the pops gives the same output as one input of both. It runs only while a pop sounds and until the impulse response has rung out. With the pops in the engine input, the leveller dropped its gain by 6 to 14 dB after each pop and stayed more than 1 dB down for 40 to 150 ms: the pops pumped the engine. Now the gain does not change (`TestPopLeveller`). | New, not approved: the fix of the pumping | ER |
| Convolution | The impulse response of each channel, the engine-sim file that the tables name, shipped unchanged in `profiles/ir/` and embedded: `mild_exhaust.wav` (1893 taps after the engine-sim trim), `smooth_39.wav` (6907) and `minimal_muffling_01.wav` (5979). Uniformly partitioned FFT convolution in float32, block 256. Channels with one impulse response share one convolution. | From engine-sim: the impulse responses. The FFT form is new | ER 1.2, 3 |
| Output low pass | Fourth-order Butterworth at 0.45 times 44.1 kHz, on the engine and on the pops. | From engine-sim: ported | EP |
| Leveller | `LevelingFilter` on the engine alone: the peak decays by 0.999 per sample, the gain is the target over the peak from 1e-5 to 1.9, smoothed by 0.1 per sample. The pops take the same gain. | From engine-sim: ported | EP, ER 1.2 |
| Pop limiter | The pops have a gain of their own that keeps the output below 0.95 of full scale: it falls at once to what a peak needs and recovers with a time constant of 50 ms. It never acts on the engine. | New, not approved | |
| Volume and clamp | `volume` after the leveller, then the engine-sim clamp to the 16-bit range, as float32 from -1 to 1. `volume` is 0.25, half the `--volume` of the references, so that the pops have 6 dB of room: at 0.5 they clipped, and the pop limiter took them 2 to 5 dB below their levels. | From engine-sim: ported. The value is a snarl default | |
| Resampler | Polyphase Kaiser-windowed sinc from 44.1 kHz to the output rate, 32 taps per phase, -6 dB at 20.9 kHz. | New | ER 1.3 |
| Start and stop | With `FlagRunning` clear, the output fades with a time constant of 20 ms, and at 120 dB down the engine is silent and does not run. It starts again with the same fade. The output is clamped from -1 to 1 after the resampler. | New, not approved | `FlagRunning` in `api.go` |

The scorecard of the five engines against engine-sim (`go run ./tools/scorecard --dyno --no-pops`, the dynamometer scenarios with the idle wander and the own pops off, because engine-sim has neither) is 97.7 to 100 on every scenario: the holds at throttle 0, 0.5 and 1, the full-throttle sweep, the overrun with its closed-throttle sweep, the simulation-rate hold and the free-running limiter. The lowest window is 90.0: the first full-throttle hold of the `v12-412` overrun, where engine-sim differs from itself (CT 6.2), and the `i4-vtec` closed-throttle idle. The free run at idle does not reach 90: with the idle wander, the frame-wise measures of the scorecard compare two independent wanders (43 to 79), and with the RPM of the reference as the snapshot RPM, `v8-ls` and `f4-ej25` score 100 and 99.1, `i4-vtec` 90.4, `v12-412` 88.4 and `v12-merlin` 77.3. The tables are dynamometer holds, so a free-running engine is outside what they measure.

Cost: 0.51 to 1.14 per cent of one core of an AMD Ryzen 9 7940HS at 48 kHz in 256-frame blocks (`BenchmarkRender`, `BenchmarkStates`), of which the synthesiser is about 0.4 per cent (ER 4) and the rest the playback per simulation step, which rises with the simulation rate: 20 kHz for `i4-vtec` and `f4-ej25`. The tables and the impulse responses decode once per process and the engines share them.

Not reused: the engine-sim physics (the cycle tables take its place), the exact port of the synthesiser with its direct convolution (the reference for the runtime, EP), and every EngineLab stage.

New, not approved. These elements stay until the owner decides on them:

- The RPM smoothing and ramp, and the limiter handover.
- The arming and the fade of the closing transient.
- The ignition crossfade over one engine cycle.
- The pop side input, the pop limiter and the volume of 0.25.
- The start and stop fade.

Removed with the earlier model, with their profile keys: the pulse table and its variations (`[pulse]`), the cam switch (`cam_switch_rpm`), the firing schedule and the delay lines (`[[cylinder]]`, `[exhaust]`), the paths with the collector and the listener distance (`[[path]]`), the noise layers (`[noise]`), the formant bank (`[[formant]]`), the pop bank (`[[pop_formant]]`), the crack (`[pops] crack`, `crack_ms`), the limiter pump, notch and bark levels and the limiter kind (`limiter_pump_db`, `limiter_notch_db`, `limiter_bark_db`, `limiter_kind`), the calibration and the leveller gain (`[voice]`), the peak limiter and the soft clipper. A profile that sets one of these keys fails to load. The catalogue in `profiles/catalogue/` keeps the earlier format until it has cycle tables, so snarl cannot load it (`snarl.ErrNoTables`).

## 3. Quality tiers

The tier is a construction parameter. `Tier1` and `Tier2` are the only values, and `New` returns an error for any other value. `Tier2` renders the tier 1 model until a tier 2 prototype decides whether the parameter stays. An earlier tier 0, with a table pulse and the path without the tier 1 filters, sounded thin and is removed. Tier 1 costs 1.3 to 1.6 times what tier 0 cost, and at most 1.21 per cent of one OnePlus 3T fast core (measured).

| Tier | Target | Content | Cost |
| --- | --- | --- | --- |
| 1 | All targets: Nintendo Switch, OnePlus 3T, phones and PCs | Firing schedule, the measured pulse table per cylinder, per-cylinder gain and timing imbalance, per-bank delay lines, crossover mix, and per path the pulse low pass, the input jitter, the derivative mix, the noise AM and a collector delay. The formant bank on the sum of the paths, the leveller, pops with their pop bank, peak limiter with 0.5 ms look-ahead, soft clipper | At most 1.21 per cent of one OnePlus 3T fast core (measured) |
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
