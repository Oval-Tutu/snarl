# FiveHorizons changes for the engine sound

Date: 2026-10-17. Proposals for the game changes that snarl needs. Paths are relative to `/home/martin/Development/Oval-Tutu/FiveHorizons` unless stated. Line numbers are from the working tree at commit `a5f57bf6`. Sizes use the T-shirt scale (XS to XL). The agreed decisions in `docs/scoping.md` section 11 are the base: a backend-neutral API, load derived in snarl, a synthesised limiter and clutch, a car table outside `render`, one `audio.NewPlayerF32` owned by the game, and the `BufferSize` change in the fork.

None of the seven proposals blocks the snarl harness. The harness copies `handling.Model.engine` and `revClimb` and runs any gear count on its own.

## 1. Engine state per tick

**Change:** none to the handling model. The game reads `handling.State.RPM`, `Revs`, `Gear`, `Wheelspin` and `Upshift` (`internal/handling/handling.go` lines 215 to 234) and `race.Race.Input.Throttle` as they are.

**Reason:** linear RPM from `Revs` is enough for a believable note. `Model.engine` (lines 458 to 536) already gives the three behaviours that the ear expects. The ratio slide (lines 460 to 466) throws the revs up on a change down and drops them on a change up, as `docs/mechanics/handling.md` lines 156 to 159 require. The hold during the window and the wheelspin (lines 472 to 480) keeps the revs flat while the clutch is out. The decay branch (lines 502 to 511) gives the overrun. Two things are missing and both are audio only: the limiter bounce (the revs stop at the throttle goal, line 501, and never pass it) and a rev flare on wheelspin (the hold at line 475 keeps the revs flat). snarl synthesises both from the `Limiter` counter and the `Wheelspin` flag, so the simulation stays untouched and no replay changes.

An engine RPM in the handling model would change `Model.Step`, every golden hash (`internal/race/hash.go` lines 58 and 59 hash `Revs` and `RPM`) and every replay, and the sound gains little, because pitch and load are both in the snapshot.

**Optional later XS:** export `Clutch bool` from `m.window > 0` in `Model.status` (line 800). `Hash` does not read it, so the hashes stay. Not needed now, because snarl times `ShiftWindow` (line 40) from the shift counters.

**Records, hashes, replays:** none. **Size:** XS (none in the model, the read is part of proposal 5).

## 2. Event counters and flags

**Change:** a new file `internal/game/enginesnap.go` with a method `engineTick` that runs in `Game.step` after `g.Step` and before `g.rumbleTick` (`internal/game/rules.go` lines 13 to 17). It keeps the previous tick's gear, RPM, speed and red zone state, and fills the snarl snapshot. It follows the form of `rumble.tick` (`internal/game/rumble.go` lines 175 to 215), which already detects a change down by comparing `r.gear` with `st.Gear` (line 194).

| Field | Source |
| --- | --- |
| `Running` | `!car.Crashed && !car.Hidden && !car.NoThrottle`, as `Game.engine` does (`internal/game/exhaust.go` lines 66 to 75) plus `NoThrottle` (`internal/sim/sim.go` line 42) |
| `Airborne` | `car.Air > 0` (`handling.go` line 303) |
| `Wheelspin` | `st.Wheelspin` (line 804) |
| `Clutch` | Timed in snarl for `handling.ShiftWindow` after a shift counter changes |
| `RedZone` | `st.Revs >= handling.RedZone` (line 38) |
| `Overrun` | `Throttle < 0.05 && st.RPM < prevRPM` |
| `ShiftUp`, `ShiftDown` | `st.Gear` against the previous tick |
| `Limiter` | `RedZone` rose this tick with `Throttle > handling.WheelspinThrottle` (line 42) |
| `Launch` | `World.Prev.Speed == 0 && World.Car.Speed > 0 && Throttle >= 0.9` in low gear. `World.Prev` is at `sim.go` line 147 |

Counters are `uint8` and wrap. snarl detects a change by difference. Wheelspin stays a flag, because it lasts up to `WheelspinMax` 0.6 s (line 41).

**Reason:** the game has no limiter, overrun or clutch export (`docs/research/fivehorizons.md` section 2), and each derives from exported state in one tick, outside the simulation, so `docs/audio.md` line 15 holds.

**Records, hashes, replays:** none. Audio reads and never writes. **Size:** S.

## 3. Three gears

**Change:** add `Mid` between `Low` and `High` in `handling.Gear` (`handling.go` lines 15 to 22) with `MidTop` as a starting point of 240 km/h between `LowTop` 190 and `HighTop` 290 (lines 33 and 34). Replace the bool with the gear number end to end.

| Area | Files and lines | Change |
| --- | --- | --- |
| Controls | `internal/sim/sim.go` lines 94 and 114 | `HighGear bool` becomes `Gear uint8`. `Quantise` copies it |
| Input | `internal/input/input.go` lines 25 to 31, 65 to 66, 108 to 123 | Today Space, Shift, pad A and pad R1 all toggle (line 120). Up: Space, pad A, pad R1. Down: Shift, pad B, pad L1. `Reset` puts the lever in low |
| Handling | `handling.go` lines 430 to 440, 460, 471, 484, 488, 494, 540 to 552, 762 to 776, 820 | `sel` comes from the controls. `upshift = sel > m.Gear`. `shiftRevs = speed / TopSpeed(m.Gear before the change)`. `TopSpeed`, `Decel` and `offThrottle` take a `Gear`. The `highPull` floor applies to mid and high. The fast launch and the launch smoke stay low only |
| Record | `internal/record/record.go` lines 85 to 93 and 207, `recorder.go` lines 82 to 84 | `runHigh` bit 3 becomes two gear bits, 3 and 4. `FormatVersion` 10. The reader keeps the version 9 path and maps bit 3 to `High` |
| Hash | `internal/race/hash.go` line 60 | No code change, but `High` becomes 2, so every golden hash changes |
| Autopilot | `internal/handling/autopilot.go` lines 13 to 14, 38, 49 to 58 | First change: keep low and high only, so the benchmark drives the same line |
| Driver AI | `internal/driver/driver.go` lines 75 to 77, 97, 698 to 720, `stig.go` line 948, `internal/playtest/playtest.go` lines 372 and 548 | First change: low and high only. Mid for the AI is a later tuning change |
| HUD | `internal/hud/hud.go` lines 64, 386, 409 to 412, `internal/game/rules.go` line 26, `cmd/hudmockup/main.go` lines 42 and 51, `cmd/menumockup/main.go` line 126 | `Gauges.Gear handling.Gear`, chip text `LOW`, `MID`, `HIGH` |
| Game | `internal/game/exhaust.go` lines 58 and 69, `riders.go` line 104, `rumble.go` lines 129, 194 and 324, `motionlog.go` line 64 | Compare with `Low` or pass the gear. The change down nudge fires on `st.Gear < r.gear` |
| Spec | `docs/mechanics/handling.md` lines 45 to 60 and 182 to 190 | Three tops, two crossing points, the approval note updated |

**Arcade feel:** keep one `ShiftWindow` of 0.15 s (line 40), one `ratioSlide` rate (line 45) so a low to high change still takes 0.25 s, and the wheelspin rule for any change up in the red zone. The best change stays at the red zone in each gear. The player who ignores mid drives the current two-gear game, because low to high skips mid when the player presses up twice inside the window.

**Reason:** a third gear gives snarl a second change up and a longer mid-range pull, where the note is richest. It is a simulation change and must be separate from the audio.

**Records, hashes, replays:** `FormatVersion` 10, every golden hash and every expectation of the benchmark change. Old records replay through the version 9 path. **Size:** L, after an S spike in the snarl harness that fixes `MidTop` and the climb rules by ear.

## 4. Car table and engine profile

**Change:** a new package `internal/cars` with:

```go
type Car struct {
	Model  *render.CarModel
	Engine string // snarl profile name, for example "flat12".
}

func Player(choice string) (*Car, error)
```

keyed by `render.OpenTopCar` and `render.ClosedTopCar` (`internal/render/carmodel.go` lines 35 to 39). `game.go` line 299 calls `cars.Player(opt.Car)` instead of `render.PlayerCarModel`. `PlayerCarModel` and `PlayerCarFiles` (lines 105 to 120) stay for the tests and the build. Both cars name `flat12`, because both are the Roadsterossa (lines 41 to 97).

**Reason:** `render` is visuals only (lines 12 to 14), and `Options.Car` never touches the simulation (`internal/game/game.go` line 86). The profile is audio and never enters `race.Config`.

**Records, hashes, replays:** none. **Size:** S.

## 5. Audio integration

**Change:** a new package `internal/audio`:

- `Mixer` owns `audio.NewContext(48000)` and one `audio.NewPlayerF32` on an `io.Reader` that renders one block per `Read`. The context is made before the title screen (`docs/audio.md` line 66). `Player.SetBufferSize` is set to the measured figure of proposal 7.
- `Engine` wraps `snarl.Engine`, made from the profile of proposal 4 before the stage starts.
- The snapshot type is `snarl.Snapshot` as in `docs/scoping.md` section 5, with the `Launch` counter of proposal 2 added. snarl owns the atomic store inside `Set`, so the game holds no atomics.

Lifecycle, following `rumbleFrame` (`rumble.go` lines 292 to 301):

| Event | Where | Action |
| --- | --- | --- |
| Tick | `rules.go` line 14, after `g.Step` | `engineTick` writes the snapshot |
| Menu or pause | `game.go` lines 647 to 668, `hold` | A new `audioFrame(hold, dt)` beside `rumbleFrame` ramps the engine gain to zero. No tick runs, so the snapshot is stale and must not sound |
| Benchmark suite | `g.suite != nil` (line 294 of `rumble.go`) | Silent, so the audio cost never enters the frame figures |
| Race end | `g.Done()` | The snapshot keeps `Running` true and the engine idles at rest |
| Restart | `internal/game/restart.go` lines 14 to 28, next to `g.rumble.reset()` | `Engine.Reset` returns the voice to idle |
| No device | `audio.NewContext` | Oto reports through the ready channel. The mixer renders into silence and never errors (`docs/audio.md` line 16) |

Volume: `settings.Settings` has no audio field (`internal/settings/settings.go` lines 95 to 113). First change: a `-engine-volume` dev flag. Later: `Master` and `Effects` fields in `Settings` and a row on the settings screen, as `Rumble` is (line 102).

Tests: `TestFrameAllocations` form for `Render` and for `Set`, as `docs/audio.md` line 12 requires.

**Records, hashes, replays:** none. **Size:** M.

## 6. Document amendments

Proposed text, to replace in place.

`docs/audio.md` line 17:

> **Pure Go on every target.** The mixer adds no cgo. The Ebitengine fork carries one audio change: it passes the device buffer size to Oto, see decision 0020.

`docs/audio.md` line 40, the Engine row:

> Engine | A procedural voice from the snarl library: a pulse train driven by RPM, throttle, acceleration, gear, and events from the tick snapshot. No RPM samples.

`docs/audio.md` lines 64 to 65:

> - **Player buffer:** about 25 ms (a starting point).
> - **Device buffer:** about 25 ms through the fork, so that the engine note answers the throttle within about 50 ms on desktop. Phones keep the driver default until the OnePlus 3T is measured.

Decision 0020 (`docs/decisions/0020-ebitengine-audio.md`): line 7 adds "and by decision 0023". Line 13 and the crossfade clause of line 19 get a note: "Superseded by decision 0023: the engine note is procedural." Line 20 becomes: "The Ebitengine fork carries one audio change, the device buffer size, see decision 0023." Line 45 gains: "Decision 0023 made the fork change."

Decision 0021 (`docs/decisions/0021-the-game-writes-its-own-audio-mixer.md`): line 10 drops "a crossfade between engine RPM samples". Line 20 becomes: "The Ebitengine fork changes only the device buffer size, see decision 0023."

New decision 0023, "The engine note is procedural and the fork sets the device buffer": status proposed, the snarl API, the 50 ms desktop target, the measurement method, and the deferred Oboe change.

**Size:** XS.

## 7. The fork change for `BufferSize`

The fork is at `/home/martin/Development/flexiondotorg/ebiten`. The game pins branch `fivehorizons` at `5d80ed875`, tag `fivehorizons-5` (`docs/ebitengine-upstream.md` line 9). The local checkout is on `main`, not `fivehorizons`. Both branches have the same `audio/context.go`: `newDriver` calls `oto.NewContext` with `SampleRate`, `ChannelCount` and `Format` only (lines 36 to 40). Oto v3.5.1 accepts `NewContextOptions.BufferSize time.Duration` (`context.go` lines 70 to 76 in the module cache) and uses it at lines 99 to 103.

**Change:** one fork commit, R40, fork-only tier at first. Add a package variable in `audio` set before `NewContext`:

```go
// DeviceBufferSize is the buffer size that NewContext asks of the device, or 0 for the driver default.
var DeviceBufferSize time.Duration
```

and pass it as `BufferSize` at line 36. Add a test that the value reaches the options. Record R40 in `docs/ebitengine-upstream.md` lines 20 to 54 and in `AGENTS.md` line 103, which lists what the fork carries.

**Reason:** stock Ebitengine runs the driver default of 42 to 128 ms (decision 0020 line 45). An exported option is the smallest surface. Upstream may take it as an `audio.NewContextWithOptions`, which is an issue first.

**Revert:** the user reserves the right to revert this patch. So the change must stay easy to remove. Keep it as one fork commit. Keep the default at 0, which is the driver default. Set `DeviceBufferSize` in one place in the game. snarl and the harness must work on stock Ebitengine, so that a revert changes only the latency, not the behaviour.

**Records, hashes, replays:** none. **Size:** S for the fork, XS for the game pin and documents.

## Order

| Step | Proposal | Size | Blocks the harness |
| --- | --- | --- | --- |
| 1 | 6 documents and decision 0023 | XS | No |
| 2 | 7 fork `BufferSize`, pin, measure on Linux | S | No, the harness measures with its own player |
| 3 | 4 car table | S | No |
| 4 | 2 event counters and 5 audio integration, one change | M | No |
| 5 | 1 optional `Clutch` export | XS | No |
| 6 | 3 three gears, after the harness spike | L | No |

Steps 1 to 3 can land before snarl passes its gate. Step 4 waits for the snarl API. Step 6 is a separate FiveHorizons change with its own record version and golden hash update.
