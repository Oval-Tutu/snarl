# snarl scoping

Date: 2026-10-17. Synthesis of the six phase 1 reports in `docs/research/`. Each claim names its report. Figures marked "estimate" are mine and are unmeasured.

## 1. Summary

snarl is a procedural engine sound generator in Go for FiveHorizons. The game writes RPM, throttle, acceleration, gear and event counters into an atomic snapshot each tick. snarl renders mono float32 on the audio goroutine with zero allocations. The model is a pulse train: a crank phasor, one shaped pulse per firing, per-bank delay lines and a crossover, a small exhaust body of delay lines and biquads, gated noise layers, and pops as events. engine-sim and EngineLab are references for the cheap stages only, not ports ([engine-sim.md](research/engine-sim.md), [enginelab.md](research/enginelab.md)). Three quality tiers scale the exhaust body from the Switch to a modern PC. The quality bar is four profiles that sound different from each other and believable for their type. An offline prototype passes a go or no-go gate against reference renders. On no-go, a sample-based backend replaces it behind the same API and the game does not change.

## 2. Model design

Signal chain in order. "Reused" names the element that snarl copies in form, not in code. Sources: [prior-art.md](research/prior-art.md) (PA), [engine-sim.md](research/engine-sim.md) (ES), [enginelab.md](research/enginelab.md) (EL), [engine-profiles.md](research/engine-profiles.md) (EP).

| Stage | Design | Reused from | Source |
| --- | --- | --- | --- |
| Crank phase | Phasor at `RPM/120` Hz, scalar crank state driven by game RPM. No rigid-body solver. | Replaces the engine-sim solver | PA 6, ES 4 |
| Firing schedule | Per-cylinder crank offset and bank id from the profile. The rev limiter starts a cut when the snapshot RPM reaches `limiter_rpm` with the throttle above 0.1, or on a `Limiter` event. A cut of `limiter_ms` skips firings with no smoothing: `fuel` skips all, `injector` fires one firing in eight with a subset that moves on with each cut, and `ignition` skips all with a chance of the pop density of one sharp bang at a random time in the cut. The pitch falls by `limiter_dip_rpm` over the cut and climbs back over `limiter_recovery_ms`, so the bounce is audible at a flat snapshot RPM. The next cut can start after the recovery, at 1000 / (`limiter_ms` + `limiter_recovery_ms`) cuts per second. No overrun air noise, overrun level or overrun pop plays during a cut or a recovery. | engine-sim ignition module, EngineLab `crank_offset_deg` and `bank_id` | ES 4, EL 4 |
| Per-cylinder pulse | Envelope `(1-exp(-a*phi))*exp(-b*phi)` with harmonic decay, random per-firing gain, slow jitter. Tier 0 reads a precomputed table. The amplitude rises with RPM. It also rises with load along a smooth load curve, from the overrun level at load 0, through `[pulse] part_load_db` at load 0.5, to full load at load 1. The overrun level, `[overrun] level_db` at the redline and `idle_level_db` at idle, is about 1 dB below full throttle at the same RPM. The wave steepening and the jet noise follow the pulse gain. The noise AM gives the timbre change with load. | PTR envelope, engine-sim `burning_efficiency_randomness`, Farnell jitter | PA 1 and 6, ES 4 |
| Low-RPM character | The `[idle]` settings: random per-firing gain (`variation`) and delay (`timing_deg`), a larger fixed per-cylinder gain spread (`imbalance`) for more half-order content, a slow RPM modulation (`hunt`) and a slow pulse amplitude modulation (`rock`) at `hunt_hz` with a slow random wander. Each has its full depth at idle and falls to zero at `fade_rpm`. All except `imbalance` also fall with load to zero at full load. Above `fade_rpm` the output does not change. | engine-sim `burning_efficiency_randomness` | ES 4 |
| Per-bank delay lines | One delay per cylinder from primary length at a hot-gas speed of sound of 550 to 580 m/s, summed per bank. Unequal lengths give rumble. | engine-sim `DelayFilter` and `primary_length`, EngineLab per-duct delays | ES 4, EL Implications 2 |
| Crossover | Profile parameter with three states: none, full merge, partial with a coupling factor. | EngineLab `ExhaustCrossoverScattering`, LS3 `crossover_coupling 0.30` | EP Implications 2, EL 6 |
| Exhaust body | Per path: collector delay, wall-loss shelf, optional expansion chamber, plane-mode cutoff, Levine-Schwinger outlet biquad. Tier 2 adds a resonator bank or a short impulse response (IR). | EngineLab `DuctWallLoss`, `ExpansionChamberMuffler`, `DuctModeCutoff`, `UnflangedPipeRadiation`. PTR bank combs plus a shared tail as the fallback form. | EL 4, PA 6, ES Implications 2 |
| Noise layers | Low-passed noise (2 kHz) as amplitude modulation on the pulses, jet noise scaled by flow, both gated by `max(load, 0.02)^0.7`. Air noise gated by overrun, through two one-pole low passes at 900 Hz, so that its energy sits below about 1.5 kHz and under the exhaust. It swells at a lift-off and decays over about 0.3 s to a quarter of the swell level. Intake roar for the inline-4 with a step at VTEC changeover. Fan tone for the flat-6. | engine-sim noise AM, EngineLab `ExhaustJetNoise`, PTR regime gates | ES 3, EL 4, PA 4, EP Implications 6 |
| Pops | Arm on throttle above 0.20 at or above a minimum RPM, fire on closed throttle above 1.65 times idle, intervals from a low-discrepancy sequence, a wall-heat integrator silences a cold engine. Each pop is a pressure jump into the path delay line plus a 4 to 8 kHz crack. Per-profile density. | EngineLab ECU gating and `ReactionCrackSynthesiser`, Baldan decaying probability | EL 5, PA 4, EP Implications 7 |
| Output | Load-dependent steepening, 10 Hz DC block, 0.45 fs low pass, a fixed calibration gain per profile, a static gain over RPM for all loads, and a look-ahead peak limiter into `tanh`. The RPM gain is `[voice] idle_gain_db` (plus `tier1_idle_gain_db` in tier 1) at and below `idle_gain_rpm` and falls along a smoothstep to 0 dB at the redline, so that the level does not fade as the RPM falls. No gain follows the signal except the peak limiter. The engine model sets the overrun level. Mono float32 into the caller's buffer. | EngineLab `NonlinearDuctAcoustics` and `AutoGainSoftClipper`, engine-sim DC block and leveller | EL 4, ES 3, [fivehorizons.md](research/fivehorizons.md) 7 |

Not reused: the engine-sim solvers and 10,000-tap convolution (ES 3), the EngineLab finite-volume network, intake network and forced induction layer (EL 4).

## 3. Quality tiers

All costs are estimates until the phase 5 benchmarks run. No published source gives an ARM figure for a pulse model (prior-art.md Open questions). The tier is a construction parameter. The mapping follows enginelab.md Implications 8 and prior-art.md section 6.

| Tier | Target | Content | Cost estimate |
| --- | --- | --- | --- |
| 0 | Nintendo Switch, OnePlus 3T | Firing schedule, table pulse per cylinder, per-cylinder gain and timing imbalance, per-bank delay lines, crossover mix, one first-order low shelf and one outlet biquad per path, one gated noise source, pops, peak limiter with 0.5 ms look-ahead, soft clipper | A few multiply-adds per sample plus one delay read per cylinder, well under the 1060 cycles per frame in section 4 (estimate) |
| 1 | Mid-range PC and phones | Tier 0 plus live pulse shaping, wall-loss shelf, mode cutoff, expansion chamber, jet noise, steepening, intake resonator, mechanical layer | A few biquads and delay lines per path (estimate) |
| 2 | Modern PC | Tier 1 plus a resonator bank or 512 to 1024 tap IR per path, two-outlet stereo observer, optional structural modes | 45 to 90 million multiply-adds per second per channel for the IR form (engine-sim.md Implications 2) |

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
	Flags    uint8   // Running, Airborne, Wheelspin, Clutch, RedZone, Overrun

	// Event counters: the reader detects a change by difference.
	ShiftUp, ShiftDown, Limiter uint8
}

type Engine interface {
	Set(Snapshot)         // game goroutine, atomic store
	Render(dst []float32) // audio goroutine, mono, no allocation
}

func New(p Profile, sampleRate int, tier Tier) (Engine, error)
```

The game-side mapping of each field is in fivehorizons.md section 7. Load: the handling model has none, so snarl derives `load = clamp(max(Throttle, Accel / peakAccel[Gear]), 0, 1)`, smoothed, and forces it to zero while `Overrun` is set. The throttle is the lower limit, so a steady part throttle keeps its load, and acceleration only raises it. The game sets `Overrun` when `Throttle < 0.05` and RPM falls. snarl detects the limiter from the RPM and synthesises the cut and the bounce, because the handling model never bounces (fivehorizons.md section 2). The `Limiter` counter is an optional trigger with the same effect. The sample backend implements the same interface with a loop crossfade every 500 RPM and on-load and off-load layers (prior-art.md section 2).

## 6. Engine profiles

Format proposal: one TOML file per profile under `profiles/`, embedded and parsed at construction, outside the audio path. TOML matches the game catalogue (fivehorizons.md section 5). EngineLab's three-layer merge of default, family and engine is the model for defaults (enginelab.md section 4). Fields: identity, idle and redline, limiter type and RPM, cylinders (crank offset, bank), banks (primary lengths, collector length, outlet radius), crossover state and coupling, pulse shape, noise gains, pop density, mechanical layer flags, voicing gains. Identifiers avoid manufacturer names, which the game forbids (fivehorizons.md Implications): `flat12`, `inline4`, `flat6`, `v8`.

Key parameters from engine-profiles.md:

| Profile | Engine | Per-bank interval | Dominant order | Idle | Redline and limiter | Exhaust and crossover | Pop density |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `flat12` | Ferrari Testarossa F113 | 120 even, banks 60 apart | 6th | about 1000, unverified | 6800, limiter unverified | Two 6-into-1 bank pipes into one silencer, four tailpipes. Crossover unknown, start with none | 0.4 (0.8 decat) |
| `inline4` | Honda B18C5 | 180 even, one bank | 2nd | 800 | 8400, fuel cut 8500 | 4-2-1 header, one silencer, one tailpipe. VTEC step at 5700 | 0.05 |
| `flat6` | Porsche M64 (993) | 240 even per bank, 120 overall | 3rd | 800 | 6800, governed 6700 | Per-bank 3-into-1 into a centre mixing chamber, two silencers. Full merge. Fan tone | 0.1 |
| `v8` | Ford 5.0 HO | 90-180-270-180 per bank | 4th with 0.5 and 1.5 sidebands | 675 | about 6000, fuel cut 6250 one injector at a time | Shorty headers, H-pipe, two silencers. Partial crossover | 0.25 |

Pop densities are tuning starting points, not measurements (engine-profiles.md Implications 7). Gaps (engine-profiles.md Open questions): the Testarossa firing order is the 512 BB value, its idle, limiter and valve timing are unverified, and its silencer internals are unknown. Pipe diameters and lengths are unknown for all but the Fox 2.25 in pipe. The 993 fan ratio comes from the 1976 fan.

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
- CPU: every tier cost is an estimate. No ARM measurement exists for a pulse model (prior-art.md).
- Reference builds: engine-sim's piranha and EngineLab's JUCE are unverified on Linux (engine-sim.md section 7, enginelab.md section 7).
- Flat-12: no reference in either simulator, no free recording for the prototype or the sample fallback, and the firing order comes from the 512 BB (engine-profiles.md).
- Pops: EngineLab's owner rejected their own pop sound, engine-sim has none, and no recording was found (enginelab.md section 5, engine-sim.md section 6).
- Quality bar: no published listening benchmark exists for a pulse model (prior-art.md).
- Switch: buffer size and thread model are inside the closed `uwagaki` package, and there is no hardware (platform.md section 2).
- Game model: no load, no limiter, no clutch flag. snarl must synthesise each (fivehorizons.md section 2).

## 11. Open decisions

1. Latency target and fork change. Recommend: make fork change 1 (`BufferSize` option in `audio/context.go`), measure on Linux with the microphone method, set the target at about 50 ms on desktop with a documented phone figure, and amend `docs/audio.md` line 17 and decision 0020. Defer the Oboe change until the OnePlus 3T is measured.
2. Impulse response licence. enginelab.md section 8 reports the engine-sim IRs as MIT with a notice file. engine-sim.md section 8 finds no separate licence or provenance for the recordings. Recommend: ship no engine-sim IR. Tiers 0 and 1 use biquads and delay lines, tier 2 uses snarl's own IRs or a resonator bank. Use the engine-sim IRs only in internal reference renders.
3. Missing pop reference. Recommend: design to the EngineLab measured shape (0.2 to 0.8 ms front, 2 to 4 ms duration, centroid 450 to 790 Hz, plus a 4 to 8 kHz crack), tune by ear in the harness, and keep a one-shot sample layer behind the same event API. Ask the user for any recording they can source.
4. Testarossa firing order and manual gaps. Recommend: proceed with the 512 BB order. The per-bank 120-degree pattern does not depend on the numbering, so the sound risk is low. The user fetches the ManualsLib "Timing Data" pages for idle, valve timing and limiter. Keep the flat-12 crossover at none until the silencer internals are known.
5. Profile data format. Recommend TOML, embedded, parsed at construction.
6. Load derivation. Recommend acceleration based, derived in snarl.
7. Rev limiter and clutch. Recommend that snarl synthesises the cut from the `Limiter` counter and times `ShiftWindow` from the shift counters. No handling model change.
8. Third gear. Recommend any gear count in the harness now and a separate game change later (fivehorizons.md section 3).
9. Sample rate. Recommend a construction parameter, default 48 kHz.
10. Profile home in the game. Recommend a car table outside `render`, keyed by the car choice string (fivehorizons.md section 5).
11. Tooling owner. Recommend that tailor fits snarl, because the game names tailor as its tooling owner (fivehorizons.md section 6).
