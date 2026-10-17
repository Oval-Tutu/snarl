# FiveHorizons: what snarl must fit

Research of the FiveHorizons code at `/home/martin/Development/Oval-Tutu/FiveHorizons` on 2026-10-17, for the engine sound integration. Every path below is relative to that repository unless it starts with `snarl/`. Line numbers are from the working tree at commit time of this note.

## Summary

- The game has no audio code. `docs/audio.md` sets the rules that snarl must meet: a per-tick snapshot through an atomic swap, mixing on the Oto goroutine, zero allocations, no locks, 48 kHz, one `audio.NewPlayerF32` stream owned by the game mixer.
- The handling model already exports `RPM` (1000 to 8000), `Revs` (0 to 1), `Gear`, `Wheelspin`, and `Upshift` in `handling.State`. Throttle is in `race.Race.Input`. No engine load, limiter, overrun, or clutch flag exists. Each needs a derivation or a small export from the handling model.
- The gear is a bool (`sim.Controls.HighGear`) in the simulation, the record format, the HUD, the driver AI, and the golden hashes. A third gear touches about 15 files and bumps the record format.
- The controller workaround is one filter, `ebiten.IsStandardGamepadLayoutAvailable`, in `internal/input/pads.go`. A copy of that file plus one constant stands alone.
- Both player cars are the same Roadsterossa body with a visual-only choice string. There is no engine data per car yet.

## 1. Controller enumeration

### The workaround

The problem and the fix are in `docs/devlog/041-gamepads.md` lines 3 to 21. The virtual pointer of the keyd key remapper has absolute axes, so the Linux backend of Ebitengine lists it as a gamepad, and it took gamepad ID 0 (line 7). The game once read gamepad 0 only. The fix reads every connected gamepad and follows the one with the standard layout that the player used last (line 8). A device without the standard layout is never read, even when it is the only device (line 12).

The filter is `ebiten.IsStandardGamepadLayoutAvailable(id)`:

| Item | File | Lines |
| --- | --- | --- |
| `padSource` interface, the seam for tests | `internal/input/pads.go` | 11 to 23 |
| `ebitenPads`, the real source. `standard` wraps `IsStandardGamepadLayoutAvailable`. `used` scans every standard button and both sticks past `StickDead` | `internal/input/pads.go` | 25 to 71 |
| `namedPads`, stand-ins for screenshots of the settings screen | `internal/input/pads.go` | 73 to 101 |
| `Pad` and `Match`: GUID, name, and index identify a pad across runs because the ID changes | `internal/input/pads.go` | 103 to 130 |
| `pads` struct, with the comment that names keyd as the reason | `internal/input/pads.go` | 138 to 158 |
| `pads.update`: lists IDs into a reused slice, skips non-standard pads, picks the pad used last, allocates only on plug or unplug | `internal/input/pads.go` | 169 to 204 |
| `pads.list`: builds `known` from the standard pads, counts duplicates by GUID and name | `internal/input/pads.go` | 214 to 231 |
| `Reader.Gamepad`, `Pads`, `Prefer`, `Chosen`: the public surface | `internal/input/pads.go` | 242 to 263 |
| Button and stick readers that return zero for no pad | `internal/input/pads.go` | 265 to 302 |
| `StickDead = 0.13` and `Stick` (radial dead zone, squared curve) | `internal/input/input.go` | 51 to 63 |
| Button constants: `padThrottle` is `FrontBottomRight`, `padBrake` is `FrontBottomLeft`, gear is `padA` or `padR1` | `internal/input/input.go` | 36 to 49, 120 |
| `-gamepads` dev flag that logs each pad, its SDL ID, and its standard layout | `cmd/fivehorizons/gamepads_dev.go` | 14 to 60 |
| Saved pad as `padGuid`, `padName`, `padIndex`, with size limits | `internal/settings/settings.go` | 104 to 109, 173 to 183 |

The tests that pin the behaviour are `TestPadsSkipKeydPointer` and `TestPadsNoStandardGamepad` in `internal/input/input_test.go` lines 138 to 165. `fakePads` (lines 27 to 60) implements `padSource` with maps of standard flags, busy flags, names, buttons, and axes.

### Dependencies on the rest of the game

`pads.go` imports only `math`, `slices`, `ebiten`, and `inpututil`. It uses `StickDead` from `input.go`. The exported methods hang off `*Reader` (`input.go` lines 67 to 83), and `Reader` pulls in `ebitengine-input` and `sim.Ramp`, which the harness does not need.

### What a copy into snarl needs

Copy `internal/input/pads.go` whole into a snarl internal package, with a header comment that names the source file. Add `StickDead` from `input.go` line 52. Replace the `*Reader` receiver of `Gamepad`, `Pads`, `Prefer`, and `Chosen` with the harness's own reader type, or expose `pads` directly. Keep `vibrate` in the interface only if the harness wants rumble. Drop `namedPads` unless the harness takes screenshots. Copy `fakePads` and the two keyd tests from `input_test.go` so the filter stays tested. The harness then reads the throttle from `padThrottle` through `value`, the brake from `padBrake`, and gear changes from `justPressed`.

Confidence: high. The whole workaround is in one file and the devlog states the cause.

## 2. Vehicle model

### Where the numbers come from

| Item | File | Lines |
| --- | --- | --- |
| `sim.Controls`: `Throttle`, `Brake`, `Steer`, `HighGear bool` | `internal/sim/sim.go` | 89 to 95 |
| `sim.Quantise`: pedals in 255 steps, steering in 32767 | `internal/sim/sim.go` | 97 to 134 |
| `sim.Car`: `Speed` in m/s, `Air`, `Light`, `Crashed`, `NoThrottle` | `internal/sim/sim.go` | 29 to 51 |
| `sim.World`: `Car`, `Prev`, `Ticks`, `Step`, `Interpolated` | `internal/sim/sim.go` | 142 to 227 |
| Fixed tick: `TickRate = 120`, `Tick = 1/120` | `internal/race/race.go` | 25 to 28 |
| `sim.Clock`: frame time into whole ticks, `MaxFrame` 0.25 s | `internal/sim/clock.go` | 7 to 35 |
| Frame loop: `input.Read()` once a frame, `input.Tick` and `g.step` for each tick | `internal/game/game.go` | 663 to 666 |
| `Game.step`: `Race.Step`, then `rumbleTick`, then `afterTick` | `internal/game/rules.go` | 13 to 17 |
| `Race.Step`: countdown, driver AI, `Quantise`, `World.Step`, traffic, collisions, rules | `internal/race/race.go` | 340 to 388 |
| `Race` fields: `World`, `Model *handling.Model`, `Input sim.Controls`, `Upshift`, `Landed` | `internal/race/race.go` | 137 to 167 |
| Rev model constants: `LowTop` 190 km/h, `HighTop` 290 km/h, `IdleRPM` 1000, `MaxRPM` 8000, `RedZone` 0.95, `ShiftWindow` 0.15 s, `WheelspinMax` 0.6 s, `ratioSlide` over 0.25 s | `internal/handling/handling.go` | 31 to 59 |
| `revClimb` table from the original, 256 bytes | `internal/handling/handling.go` | 61 to 88 |
| `handling.State`: what other systems read | `internal/handling/handling.go` | 215 to 234 |
| `handling.Model` private fields: `ratio`, `window`, `upshift`, `shiftRevs`, `spin`, `carry` | `internal/handling/handling.go` | 236 to 264 |
| `Model.Step`: smooths inputs over 4 ticks, calls `shift` then `engine` | `internal/handling/handling.go` | 288 to 379 |
| `Model.shift`: gear change window and wheelspin | `internal/handling/handling.go` | 427 to 456 |
| `Model.engine`: ratio slide, throttle goal, climb, decay, brake, speed cap | `internal/handling/handling.go` | 458 to 536 |
| `Model.status`: `Revs = clamp(v/ratio, 0, 1)`, `RPM = IdleRPM + (MaxRPM-IdleRPM)*Revs` | `internal/handling/handling.go` | 798 to 828 |
| `average`: 4 tick smoothing of throttle, brake, steering | `internal/handling/handling.go` | 856 to 874 |

The throttle sets a goal in revs. Revs chase the goal (line 471 to 511). Speed is revs times the ratio of the current gear, capped at `maxAccel` 80 m/s per second (line 517). Revs can exceed 1 after a change down at speed and then fall at `overRev` (lines 56, 507 to 508), but `State.Revs` clamps to 1 (line 801), so the exported RPM never passes 8000.

### What maps to the snarl inputs

| snarl input | Source today | Note |
| --- | --- | --- |
| RPM | `handling.State.RPM`, line 218 and 802 | Linear from `Revs`. 1000 at rest, 8000 at full. In the golden hash (`internal/race/hash.go` line 59). |
| Throttle | `race.Race.Input.Throttle`, `race.go` line 154 | Quantised to 1/255 steps after the countdown and the rules act. `Game.engine` (`internal/game/exhaust.go` lines 54 to 75) already packs `High`, `Revs`, `Throttle`, `Speed`, `Running` for the exhaust, which is the form that the audio snapshot can copy. The smoothed throttle `thr` inside the model is private. |
| Load | None | `State.Load` (line 225) is cornering load, not engine load. See the derivation below. |
| Gear | `handling.State.Gear`, line 219 | `Low` or `High`. |
| Gear change event | Change of `State.Gear` between ticks | The rumble does this for the change down at `internal/game/rumble.go` lines 194 and 213. `State.Upshift` (line 227) is true for one tick when a change up engages above 5 m/s after the 0.15 s window. `Race.Upshift` (`race.go` line 158) latches it until the smoke clears it (`internal/game/smoke.go` lines 70 to 72). |
| Rev limiter | None | Revs stop at the throttle goal, which is at most `target/ratio`, so there is no bounce. `RedZone` 0.95 is the only threshold. The HUD and the wheelspin rule both use it. |
| Overrun | None | Revs fall when the goal is below them (lines 502 to 511) at `decay` 0.2 per second, or 4 times that off the road. Derivable from throttle near zero and RPM falling. |
| Clutch or shift window | None exported | `m.window > 0 || m.spin > 0` (line 472) holds the revs. `State.Wheelspin` exports the spin half only. |
| Engine running | `!car.Crashed && !car.Hidden` | As `Game.engine` line 73. `Car.NoThrottle` (`sim.go` line 42) also holds the engine after a restart. |
| Airborne | `car.Air > 0` | Throttle does nothing in the air, revs hold (lines 303 to 307). |

### Engine load derivation

The model has no torque or load. Two derivations are possible without changing the handling model:

- From acceleration: `(World.Car.Speed - World.Prev.Speed) / race.Tick`, normalised by a per-gear peak. `World.Prev` exists for interpolation (`sim.go` line 147). Zero on the overrun, high on a launch, falls with speed in each gear, which matches how a real engine note darkens under load.
- From throttle against revs: load is the throttle while the revs are below the goal, and zero while they fall. Cheaper, but it steps with the 1/255 throttle quantisation and ignores the ratio.

Recommendation: let the game pass raw throttle, RPM, and the acceleration of the car, and let snarl shape load from them. The handling model stays untouched, and the harness can drive the same inputs from a pad. Flag: the audio is not part of the simulation (`docs/audio.md` line 30), so a derivation in the game is free of the determinism rules.

Confidence: high on what exists, medium on the load recommendation, which the sound design decides.

## 3. Gear model

### How the two gears work

The gear is a bool from the controls to the HUD:

| Step | File | Lines |
| --- | --- | --- |
| `Gear` type, `Low` and `High`, `String` gives `LO` and `HI` | `internal/handling/handling.go` | 15 to 29 |
| Lever toggles on Space, Shift, pad A, or pad R1: `r.high = !r.high` | `internal/input/input.go` | 25 to 31, 65 to 66, 118 to 123 |
| `Reset` puts the lever in low | `internal/input/input.go` | 108 to 113 |
| `shift`: a change opens the 0.15 s window, records `upshift = sel == High`, `shiftRevs = speed / LowTop`. Wheelspin on a change up in the red zone with throttle above 0.9 | `internal/handling/handling.go` | 430 to 456 |
| `engine`: `target := TopSpeed(m.Gear == High)`, ratio slides at `ratioSlide`, high gear has a climb floor of 75 percent of the peak, low gear has the fast launch | `internal/handling/handling.go` | 460 to 497 |
| `offThrottle`: low gear doubles the off-road throttle share | `internal/handling/handling.go` | 540 to 552 |
| `TopSpeed(high bool)` | `internal/handling/handling.go` | 770 to 776 |
| `Decel(b, high bool)` for the driver AI | `internal/handling/handling.go` | 762 to 764 |
| Launch smoke only in low gear | `internal/handling/handling.go` | 820 to 822 |
| The spec: two gears, curves cross at about 180 km/h, best change up at the red zone | `docs/mechanics/handling.md` | 45 to 60, 182 to 190 |

### Where a third gear changes code

The simulation and its consumers assume a bool. A third gear needs each of these:

| Area | File | Lines | Change |
| --- | --- | --- | --- |
| Controls | `internal/sim/sim.go` | 94, 114 | `HighGear bool` becomes a gear number. `Quantise` copies it. |
| Input | `internal/input/input.go` | 120 to 123 | The toggle becomes up and down. Today A and R1 both toggle, and Space and Shift both toggle, so the keymap and the pad map need a down button. |
| Record format | `internal/record/record.go`, `recorder.go` | 85, 88 to 93, 207, and 83 | `runHigh` is bit 3 of the flags byte. Two gear bits and `FormatVersion` 10. Old records stop loading unless the reader keeps the version 9 path. |
| Handling constants | `internal/handling/handling.go` | 33 to 34, 45, 439, 460, 484, 488, 494 | A `MidTop`. `ratioSlide` is one rate between two tops. `shiftRevs = speed / LowTop` assumes the change is from low. `upshift = sel == High` becomes `sel > m.Gear`. `gear := HighTop / m.ratio` still works. The climb floor and the fast launch need a rule for mid. |
| `TopSpeed`, `Decel`, `offThrottle` | `internal/handling/handling.go` | 540 to 552, 762 to 776 | Take a `Gear`, not a bool. |
| Autopilot | `internal/handling/autopilot.go` | 12 to 14, 48 to 58 | `autoShiftUp` 150 km/h and `autoShiftDown` 100 km/h become two thresholds each. The benchmark suite drives on the autopilot, so its results change. |
| Driver AI | `internal/driver/driver.go` | 75 to 77, 94 to 97, 698 to 720 | `gear` returns a bool, `BestShift` 176 km/h and `redLine` 187 km/h are one change up. `internal/driver/stig.go` line 948 sets `in.HighGear`. |
| Playtest | `internal/playtest/playtest.go` | 372, 548 | Uses `handling.High` and `driver.BestShift`. |
| HUD | `internal/hud/hud.go` | 64, 385 to 413 | `Gauges.HighGear bool` draws one chip with `HIGH` or `LOW`. `internal/game/rules.go` line 26 fills it. `cmd/hudmockup/main.go` lines 42 and 51 and `cmd/menumockup/main.go` line 126 set it. |
| Exhaust | `internal/game/exhaust.go` | 58, 69 | `engineState.High bool`. |
| Riders | `internal/game/riders.go` | 104 | `SetGear(bool)` for the lever hand. |
| Rumble | `internal/game/rumble.go` | 129, 194, 324 | The change down nudge and the launch rumble compare with `High` and `Low`. |
| Golden hashes | `internal/race/hash.go` | 60 | `h.int(int(st.Gear))`. Every golden hash test changes once the ratios change. |
| Motion log | `internal/game/motionlog.go` | 64 | `Gear.String()`. |
| Spec | `docs/mechanics/handling.md` | 45 to 60, 182 to 190 | The maintainer approved the two gear rule on 2026-10-10 (line 50). |

The `Gear` type is already a `uint8` with `iota` (lines 16 to 22), so `Mid` slots in between `Low` and `High` without a type change. Everything else compares with `== High` or passes a bool.

Caveat: a third gear is a simulation change. It changes replays, records, the golden hashes, the autopilot benchmark times, and the driver AI difficulty. Plan it as its own FiveHorizons change, separate from the audio.

Confidence: high.

## 4. Audio today

There is no audio code. No Go file imports `ebiten/v2/audio`, and `assets/audio/` holds only a README that says the folder stays empty until the maintainer chooses the sounds (`assets/audio/README.md` lines 1 to 5). The engine sound is parked (`docs/backlog.md` lines 16 and 114).

The design is settled in three documents:

| Rule | Source |
| --- | --- |
| The frame never waits for audio. No shared lock on the hot path | `docs/audio.md` line 11 |
| Zero allocations in the render path, on voice start and stop, and on a parameter change. A `TestFrameAllocations` style test checks it | `docs/audio.md` line 12 |
| Audio reads the simulation and never writes to it. The simulation runs on one goroutine | `docs/audio.md` line 15, `docs/mechanics/determinism.md` line 17 |
| A missing audio device gives silence, never an error | `docs/audio.md` line 16 |
| Pure Go, no cgo, no change to the Ebitengine fork | `docs/audio.md` line 17, decision 0020 line 20 |
| One mixer stream and one `audio.NewPlayerF32` player. The mixer renders on the Oto goroutine | `docs/audio.md` lines 22 to 23, decision 0021 lines 74 to 75 |
| The simulation publishes one snapshot for each tick through an atomic swap: listener, emitters, revs, events. The mixer ramps towards it | `docs/audio.md` lines 28 to 29 |
| Engine voice: crossfade between RPM samples with equal-power gains and a small playback rate change | `docs/audio.md` line 40, decision 0020 line 13 |
| Format: Ogg Vorbis at 48 kHz, mono for positioned sounds, never resample at play time | `docs/audio.md` lines 56 to 60, decision 0020 line 41 |
| Player buffer about 50 ms. Device buffer is the driver default, 42 to 128 ms | `docs/audio.md` lines 64 to 65 |
| Web has one thread, and audio starts after the first input | `docs/audio.md` lines 24 and 67 |
| Engine note: pitch follows the rev level, has an idle floor, a change down raises the note at once | `docs/mechanics/handling.md` lines 154 to 160 |
| Start cues `CueRed`, `CueYellow`, `CueGreen` reserved for beeps | `internal/rules/start.go` lines 14 to 21 |
| `OvertakeFunc` is "for the graphics and the sound" | `internal/race/race.go` lines 131 to 134 |

Sample rate: no `audio.Context` exists yet, so there is no code value. The documents fix 48 kHz. Decision 0020 line 41 cites Ebitengine issues 2509 and 3421 for the hitches that a resample at run time causes.

Divergence to flag: the documents assume a sampled engine with a crossfade. snarl is procedural. That removes the sample assets and the crossfade voice but keeps every rule above. `docs/audio.md` line 40 and decision 0020 line 13 need a note when snarl lands.

The frame rate rules in `AGENTS.md` lines 29 to 54 apply to the mixer: 60 Hz floor, zero allocations in the frame loop, measure worst frame times.

Confidence: high.

## 5. Car definitions

The player car is a visual table, not a simulation entity:

| Item | File | Lines |
| --- | --- | --- |
| `render.CarModel`: node names, material roles, exhaust mouths, door points, paints | `internal/render/carmodel.go` | 12 to 31 |
| Choice strings `OpenTopCar`, `ClosedTopCar`, `DefaultCar` | `internal/render/carmodel.go` | 33 to 39 |
| `OpenTop` and `ClosedTop` tables, both Roadsterossa, same exhausts | `internal/render/carmodel.go` | 41 to 97 |
| `playerCars` map and `PlayerCarModel(choice)` | `internal/render/carmodel.go` | 99 to 114 |
| `PlayerCarFiles` so the build embeds every car | `internal/render/carmodel.go` | 116 to 120 |
| `Options.Car`: "changes the model and the people's motion only, never the simulation. A menu can set it later" | `internal/game/game.go` | 86 |
| The game loads the model from the option | `internal/game/game.go` | 299 |
| Catalogue entries with `kind = "player-car"` | `assets/catalogue/roadsterossa.toml` | 37 to 56 |
| `catalogue.Model` struct, generated from the TOML by `cmd/catalogue` | `internal/catalogue/catalogue.go` | 1 to 42 |

Both cars are one body and would share one engine. The choice string is the only key.

Where an engine profile reference fits, in order of preference:

1. A new player-car table outside `render`, keyed by the same choice string, with `Model *render.CarModel` and `Engine` naming a snarl profile. `render` is visuals only (package comment at `carmodel.go` lines 12 to 14), and the game already keeps the car choice out of the simulation. The game resolves the choice once at load (`game.go` line 299) and hands the profile to the mixer before the stage starts, which meets the "nothing loads during a race" rule.
2. An `engine = "..."` key on each `[[model]]` with `kind = "player-car"` in the catalogue TOML, carried into `catalogue_gen.go`. This keeps data with the asset but needs a change to `cmd/catalogue` and `catalogue.Model`.
3. A field on `render.CarModel`. Smallest change, wrong package.

Caveat: an engine profile is audio only, so it never enters `race.Config`. If a car later gets its own gear ratios or top speeds, that is simulation and belongs in `race.Config` (`race.go` lines 67 to 87) and the record.

Confidence: high on what exists, medium on the placement, which is a design choice for the maintainer.

## 6. Build tooling

| Item | Value | Source |
| --- | --- | --- |
| Module | `github.com/oval-tutu/fivehorizons` | `go.mod` line 1 |
| Go | `go 1.26.9`, `toolchain go1.27.2` | `go.mod` lines 3 and 5 |
| Ebitengine | `v2.10.4`, replaced by `github.com/flexiondotorg/ebiten/v2` fork | `go.mod` lines 10 and 35 |
| Tetra3D | replaced by `github.com/flexiondotorg/tetra3d` fork | `go.mod` line 33 |
| Input library | `github.com/quasilyte/ebitengine-input v0.9.1`, keyboard and mouse only | `go.mod` line 12, `internal/input/input.go` lines 33 to 35 |
| Nix Go | `pkgs.go_1_27`, with a staticcheck override to 2026.2.2 for Go 1.27 export data | `flake.nix` lines 270 to 286 |
| Nix runtime libraries | alsa-lib, libGL, libX11, libXcursor, libXext, libXi, libXinerama, libXrandr, libXrender, prepended to `LD_LIBRARY_PATH` | `flake.nix` lines 289 to 309 |
| Nix Go tools | go, golangci-lint, govulncheck, goreleaser, gocyclo, goperf | `nix/go.nix` lines 4 to 11 |
| Dev shell extras | actionlint, brotli, gh, ffmpeg-headless, imagemagick, just, vorbis-tools, zip, tailor | `flake.nix` lines 310 to 345 |
| `just test` | `go vet ./...` then `go test -cover -covermode=atomic ./...`, then `just/project/test.sh` if present | `just/tailor.just` lines 106 to 126 |
| `just lint` | actionlint when workflows exist, `go mod tidy -diff`, `golangci-lint fmt --diff`, `golangci-lint run`, `govulncheck -show verbose`, `gocyclo -top 20 -avg`. `just lint correct` tidies and formats | `just/tailor.just` lines 129 to 172 |
| Lint config | golangci v2, gofumpt formatter, linters bodyclose, copyloopvar, dupword, errorlint, gocritic, gosec (G306 excluded), govet all except fieldalignment and shadow, ineffassign, intrange, misspell, noctx, revive rule list, staticcheck, unconvert, unparam, usestdlibvars, wastedassign | `.golangci.yml` lines 1 to 67 |
| Tooling owner | tailor fitted `flake.nix`, `justfile`, and manages files in `just/` and `nix/` that start with "Managed by Tailor". Do not edit those by hand | `AGENTS.md` lines 130 to 131, `justfile` lines 1 to 7 |
| CI | None. `.github/` has FUNDING, dependabot, and issue templates only. Local `just lint` and `just test` are the gate | `AGENTS.md` line 137 |
| Commits | Conventional Commits | `CONTRIBUTING.md` line 7 |
| Version | `just/project/version.sh` gives `YY.DDD+hash`, set with `-X main.version` | `justfile` lines 9 to 21 |
| Frame loop rule | Zero allocations, fixed tick, interpolate on draw | `AGENTS.md` lines 50 to 52 |
| Determinism rule | `dmath` in place of `math` in the simulation, no goroutines, rounded products | `AGENTS.md` line 64 |

snarl state at `/home/martin/Development/Oval-Tutu/snarl`: `flake.nix` already has `go_1_27`, gopls, just, and the same X11 and ALSA libraries plus `libXxf86vm` and `xvfb-run`. `.golangci.yml` is byte for byte the FiveHorizons file. There is no `go.mod`, no `justfile`, and no golangci-lint, govulncheck, or gocyclo in the shell.

To match conventions, snarl needs: a `go.mod` with `go 1.26.9` and `toolchain go1.27.2` on the same Ebitengine version (the fork `replace` is not needed unless snarl imports the game), a `justfile` with `test` and `lint` recipes that run the same commands as `just/tailor.just` lines 110 to 111 and 153 to 157, and golangci-lint, govulncheck, and gocyclo in the dev shell. Decision: whether to let tailor fit snarl, which keeps the two repositories in step, or write a short `justfile` by hand. Recommendation: tailor, because `AGENTS.md` line 130 names it as the owner of the game's tooling.

Code style in the game: full sentence comments that state the why and name the cross reference ("see X"), exported constants with unit comments, zero allocation tests named `Test...Allocations`.

Confidence: high.

## 7. Proposed integration interface

Based on what the game exports after each `Race.Step`:

```go
// Snapshot is what the game writes after each simulation tick, and what the
// audio goroutine reads. Plain values, so an atomic swap or packed atomics
// carry it with no lock and no allocation.
type Snapshot struct {
	RPM      float32 // handling.State.RPM, 1000 to 8000.
	Throttle float32 // race.Race.Input.Throttle, 0 to 1.
	Accel    float32 // (World.Car.Speed - World.Prev.Speed) / race.Tick, m/s per second. snarl shapes load from it.
	Speed    float32 // World.Car.Speed, m/s, for wind and road layers later.
	Gear     uint8   // handling.State.Gear: 0 low, 1 mid, 2 high.
	Flags    uint8   // Running, Airborne, Wheelspin, Clutch, RedZone, Overrun.

	// Events as counters, so the reader finds a change by a difference and
	// never needs a queue. A counter wraps, which is fine.
	ShiftUp   uint8
	ShiftDown uint8
	Limiter   uint8 // Entered the red zone with the throttle open.
}
```

Mapping from the code:

| Field | Game source |
| --- | --- |
| `RPM` | `g.Model.State.RPM` |
| `Throttle` | `g.Race.Input.Throttle` after `Race.Step` |
| `Accel` | `(g.World.Car.Speed - g.World.Prev.Speed) / race.Tick` |
| `Gear` | `g.Model.State.Gear` |
| `Running` | `!car.Crashed && !car.Hidden && !car.NoThrottle` |
| `Airborne` | `car.Air > 0` |
| `Wheelspin` | `g.Model.State.Wheelspin` |
| `Clutch` | Needs an export: `m.window > 0` (handling.go line 472). Until then, hold the note for `ShiftWindow` seconds after a shift counter changes |
| `RedZone` | `g.Model.State.Revs >= handling.RedZone` |
| `Overrun` | `Throttle < 0.05 && RPM < previous RPM`, computed in the game from the previous snapshot |
| `ShiftUp`, `ShiftDown` | `State.Gear` compared with the previous tick, as `rumble.tick` does |
| `Limiter` | `RedZone` rose this tick while `Throttle > 0.9`. The model has no bounce, so snarl synthesises the cut |

Write side: once per tick in `Game.step` after `g.Step` (`internal/game/rules.go` line 14), next to `rumbleTick`. Read side: the mixer reads the latest snapshot at the start of each buffer and ramps towards it, which `docs/audio.md` line 29 requires.

Render side: the game owns the one `audio.NewPlayerF32` stream and mixer (decision 0021). snarl should expose a voice that renders mono float32 into a caller's buffer at a given sample rate, not own a player. The harness owns its own player.

Profile side: a `Profile` struct per car, chosen before the stage starts, with the note shape and the idle and limiter RPM.

Confidence: medium. The field list follows the code, but the sound design decides which flags matter.

## Implications for snarl

- Mirror the game's constraints in the public API: no allocation after construction, no lock, float32 mono render into a caller buffer, 48 kHz, silence when no device.
- Take raw inputs (RPM, throttle, acceleration, gear, flags, event counters). Derive load inside snarl, so the game and the harness pass the same minimal set.
- The harness cannot import `internal/handling`. It needs its own rev model to turn a pad into RPM. Options: copy `Model.engine` and `revClimb` (handling.go lines 61 to 88 and 458 to 536) into the harness with a source comment, or a simple first order model. Recommendation: copy, so the harness feels like the game and the three gear design can be tried in the harness first.
- Copy `internal/input/pads.go` and `StickDead` as section 1 describes, with the keyd tests.
- Match the lint config now (already done) and the `test` and `lint` recipes.
- Use the `Gear uint8` from the start. The game's `handling.Gear` is already a `uint8` with `iota`, so a mid gear slots in.
- Keep the hard rule of `AGENTS.md` lines 9 to 11: no Sega, Ferrari, or Out Run names in identifiers, file names, or text.

## Open questions

1. Load: acceleration based or throttle based, and derived in snarl or in the game. The proposal above says acceleration, in snarl.
2. The third gear: `MidTop`, the ratio slide between three tops, the climb floor and launch rules for mid, and the up and down buttons. This is a FiveHorizons simulation change that bumps the record format and every golden hash. Who schedules it, and does the harness prototype it first.
3. Clutch flag: export `window > 0` from `handling.Model`, or let snarl time `ShiftWindow` from the shift counter.
4. Rev limiter: the model never bounces off a limiter. Does snarl fake the cut from `RedZone`, or does the maintainer want a limiter in the handling model (a simulation change).
5. Event transport: counters in the snapshot (proposed) or a lock free ring. Counters are enough for one engine voice.
6. Per-car profile home: a game side car table (proposed), the catalogue TOML, or `render.CarModel`.
7. Sample rate: fixed at 48 kHz, or does snarl take the rate at construction. Taking it costs nothing and lets the harness run on any device default.
8. `docs/audio.md` line 40 and decision 0020 line 13 assume sampled RPM crossfades. A procedural engine needs a decision record or an amendment.
9. Tooling: let tailor fit snarl, or hand write the `justfile`.
