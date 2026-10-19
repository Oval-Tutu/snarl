# FiveHorizons: the exhaust events

Research of the exhaust effect code in `/home/martin/Development/Oval-Tutu/FiveHorizons` on 2026-10-19, for the event-driven pops of snarl. Paths are relative to that repository unless they start with `snarl/`. Line numbers are from the working tree at the head commit of that date. The earlier report is [fivehorizons.md](fivehorizons.md).

## Summary

- The exhaust code is one file, `internal/game/exhaust.go` (250 lines). It defines three visual effects: a change-up pop, a crackle on a lift or a change down, and a haze at a standstill. It is wired and live: `Game.drawScene` calls it every frame, and `-exhaust=false` turns it off. What it does not have is an audio consumer. No sound code reads it.
- It is presentation only. It runs at frame rate with its own seeded random generator, outside the simulation, the record, and the golden hashes. snarl can read the same inputs without a determinism rule.
- Its events map onto the snarl snapshot today. `ShiftUp` and `ShiftDown` are already counters in `snarl/api.go`, and the lift is the rising edge of `FlagOverrun`. The gap is on the snarl side: `engine.Render` reads only the `Limiter` counter, and `model.Control` has no shift field, so the pop model has no way to hear a gear change.
- The design worth copying is the trigger discipline, not the flame: an edge trigger (a lift from at least 60 percent to at most 20 percent throttle), gates on revs and speed, a chance of three in four, one short burst of one to three pops, then a 2 s rest. That is what makes the effect occasional.

## 1. Where the code is and what it defines

| Item | File | Lines |
| --- | --- | --- |
| Package comment: the three effects, "for the eye only" | `internal/game/exhaust.go` | 11 to 15 |
| Tuning constants: pop, lift and crackle, haze, flame sizes | `internal/game/exhaust.go` | 16 to 46 |
| Flame tints | `internal/game/exhaust.go` | 48 to 52 |
| `engineState`: `High`, `Revs`, `Throttle`, `Speed`, `Running` | `internal/game/exhaust.go` | 57 to 63 |
| `Game.engine`: fills `engineState` from `Model.State`, `Input.Throttle`, and the car | `internal/game/exhaust.go` | 65 to 75 |
| `flame`, `exhaustPuff`, `exhaust` state, `newExhaust` with a PCG seeded `(1986, 0xe8a05)` | `internal/game/exhaust.go` | 77 to 105 |
| `exhaust.reset` for a restart | `internal/game/exhaust.go` | 107 to 111 |
| `exhaust.update`: the trigger switch and the pop timers | `internal/game/exhaust.go` | 113 to 159 |
| `crackle`, `pop`, `light`, `puff` | `internal/game/exhaust.go` | 161 to 206 |
| `Game.updateExhaust`: draws flames into the glow halos and emits puffs into the smoke | `internal/game/exhaust.go` | 208 to 244 |
| `render.CarExhausts` and `Scene.Exhausts`: the four pipe mouths in road space | `internal/render/exhaust.go` | 5 to 37 |
| `roadsterossaExhausts`: the pipe mouth points in body space, both cars | `internal/render/carmodel.go` | 28, 61, 85, 89 to 97 |
| `smoke.Puffs.Exhaust`: the grey puff | `internal/smoke/smoke.go` | 57 to 61, 387 to 407 |
| Tests: change-up pop, overrun crackle, haze, zero allocation | `internal/game/exhaust_test.go` | 24 to 124 |

### The events

`exhaust.update` (lines 115 to 159) reads the current `engineState` against the previous one and runs one `switch` with three cases. Only one case fires per update, in this order of priority.

| Event | Trigger (lines 131, 136, 138) | Gates | Chance | Rate limit | What it carries |
| --- | --- | --- | --- | --- | --- |
| Change-up pop | `e.High && !p.High`: the selected gear went low to high since the previous frame | `Speed > popFrom` (12 m/s), `Throttle > popThrottle` (0.3) | Always | None | One flame of strength 1 for `popLife` 0.1 s at both cowlings, one puff of `popPuff` 0.5 at `popJet` 3 m/s (lines 132 to 135) |
| Change-down crackle | `!e.High && p.High`: the gear went high to low | `Speed > overrunFrom` (20 m/s) | Always | Sets `rest` to `crackleRest` 2 s (line 163) | `crackle(2 + rng.IntN(2))`: 2 or 3 pops (line 137) |
| Lift crackle | `p.Throttle >= liftFrom` (0.6) and `e.Throttle <= liftTo` (0.2): the throttle fell through both thresholds between two consecutive frames | `Revs >= overrunRevs` (0.75, so 6250 RPM), `Speed > overrunFrom` (20 m/s), `rest <= 0` | `overrunOdds` 0.75 | `rest = crackleRest` 2 s, set on every qualifying lift, whether or not the chance passes (lines 139 to 142) | `crackle(1 + rng.IntN(3))`: 1 to 3 pops |
| Haze | Not in the switch. `Speed < hazeBelow` (0.3 m/s) | None | Always | `hazeGap` 0.5 s times 0.7 to 1.3 | One puff of `hazePuff` 0.3 at `hazeJet` 0.5 m/s, from the cowlings in turn (lines 152 to 158) |

A crackle (lines 162 to 172) is a queue of up to `crackleMax` 3 pops. The first fires `crackleFirst` 0.03 s after the trigger, and each next one `crackleGap` 0.07 to 0.14 s later. A pop (lines 175 to 189) is a flame of strength 0.45 to 0.8 for 0.04 to 0.07 s. It picks a side with `rng.IntN(4)`: 0 and 1 are one cowling, 2 and 3 are both. Only the first pop of a crackle puffs, at `cracklePuff` 0.25. `light` (lines 193 to 202) does not replace a stronger flame that still burns.

Two gating rules sit above the switch. `Running` is `!car.Crashed && !car.Hidden` (line 73). When it is false, or on the first update, `update` clears the queue and primes the previous state without triggering (lines 125 to 129), so a crash or a restart does not fire a pop. `dt <= 0` returns at once (line 116).

Numbers the devlog states in prose match the code: `docs/devlog/056-exhaust-effects.md` lines 7 to 11 and `docs/effects-and-feel.md` line 63.

## 2. What consumes it, and how it is wired

The exhaust is wired and runs in the shipped game.

| Step | File | Lines |
| --- | --- | --- |
| `Game.exhaust *exhaust` field | `internal/game/game.go` | 160 |
| `Options.NoExhaust` | `internal/game/game.go` | 116 |
| Halo pool sized `haloCapacity + exhaustHalos` | `internal/game/game.go` | 406 |
| `newExhaust()` and `drawOff = opt.NoExhaust` at load | `internal/game/game.go` | 433 to 434 |
| `g.updateExhaust(g.frameDT, car)` in `drawScene`, after `updateLamps` | `internal/game/game.go` | 987 |
| `g.exhaust.reset()` on a restart | `internal/game/restart.go` | 32 |
| `-exhaust` dev flag | `internal/devflags/devflags.go` | 91 to 93 |
| Flag documented | `docs/flags.md` | 34 |
| Field classified as `suiteLeaves` for the benchmark suite | `internal/game/snapshot_test.go` | 30 |

Consumers: the glow halos (`g.halos.Add`, exhaust.go lines 230 to 231) and the smoke pool (`g.puffs.Exhaust`, line 240). Nothing else reads the exhaust. The rumble has its own change-down nudge from `st.Gear` against the previous tick (`internal/game/rumble.go` line 194) and does not share code with the exhaust. The HUD does not read it. There is no audio code in the game (fivehorizons.md section 4).

Why the maintainer may think it is unused: the benchmark autopilot never changes gear, so the suite never shows the pop or the crackle (`docs/devlog/056-exhaust-effects.md` line 35). In play, the lift crackle needs a throttle fall from 0.6 to 0.2 between two frames (see section 5), which a keyboard gives and a pad trigger may not.

History. Four commits touch these files: `0bf98346` (2026-10-05, `feat(render): find the exhaust pipe mouths of the player car`), `5312f615` (2026-10-05, `feat(car): pop the exhausts on a change up, crackle on the overrun, and haze at rest`), `e19ff0b1` (2026-10-05, devlog and backlog), and `41878c8a` (2026-10-11, `refactor(render): share the body-to-road conversion of the lamps and exhausts`). The commit body of `5312f615` states the intent: flames from the glow halos and puffs from the smoke pool, no new layer, texture, or shader. Design notes: `docs/devlog/056-exhaust-effects.md` (the effect table, five rejected experiments, two open threads), `docs/effects-and-feel.md` line 63, `docs/backlog-done-archive.md` line 87. There is no decision record for the exhaust in `docs/decisions/`. The audio documents mention events only in general: the per-tick snapshot "holds the listener, the positions of the emitters, the revs, and the events" (`docs/audio.md` line 28, decision 0020 line 48).

Where audio attaches: not to the exhaust. `Game.engine` (exhaust.go lines 66 to 75) is the read boundary, and the audio snapshot write belongs in `Game.step` after `Race.Step`, next to `rumbleTick` (`internal/game/rules.go` lines 13 to 17, fivehorizons.md section 7). The exhaust runs at frame rate in `drawScene`, so it is the wrong place for a per-tick snapshot.

## 3. Determinism

Presentation only. Evidence:

- It runs in `drawScene` with `g.frameDT`, the animation seconds of the drawn frame (`internal/game/game.go` lines 229 and 987), not in the tick.
- It reads the interpolated car for `Speed` (`g.World.Interpolated(alpha)`, game.go line 970, passed at line 987) and the latest tick for the gear, revs, and throttle (exhaust.go lines 67 to 71).
- It owns a `math/rand/v2` PCG seeded with constants (exhaust.go line 104), outside the race state. The determinism rule for seeded generators (`docs/mechanics/determinism.md` line 15) applies to the simulation, which the exhaust never writes.
- `internal/race/hash.go` and `internal/record/` do not mention the exhaust, the halos, or the puffs.
- The benchmark suite leaves the field alone (`snapshot_test.go` line 30), and a restart resets it (`restart.go` line 32).

The fixed seed makes the effect repeatable from a restart for the same frame timing, and the `#nosec` comment says that is the aim (line 104). Two runs at different frame rates diverge, which is fine for a visual.

Consequence for snarl: the game may compute the same triggers and write them to the audio snapshot without a `dmath` or rounding rule, because the audio reads the simulation and never writes back (`docs/audio.md` line 15).

## 4. How it maps to snarl

### What snarl has today

| snarl item | File | Lines | Note |
| --- | --- | --- | --- |
| `Snapshot` with `ShiftUp`, `ShiftDown`, `Limiter` counters and `FlagOverrun` | `snarl/api.go` | 33 to 63 | The counters exist in the API |
| `engine.Set` packs the three counters | `snarl/engine.go` | 150 to 162 | Stored |
| `engine.Render` diffs only `Limiter` and passes `LimiterEvents` | `snarl/engine.go` | 198 to 213 | `ShiftUp` and `ShiftDown` are read back and dropped |
| `model.Control` has `LimiterEvents` and `Gear`, no shift events | `snarl/internal/model/synth.go` at `b762d34` | 42 to 53 | The pop model cannot see a shift |
| `Synth.control` sets `over` once per block from `Overrun`, the throttle and the limiter state. No lift edge is kept: the air swell that used one is gone | `snarl/internal/model/synth.go` at `b762d34` | 381 | The edge the game uses follows from `over` against the previous block |
| `pops.control`: arm on throttle, open a window on overrun, fire runs by density while the window stays open | `snarl/internal/model/pops.go` | 182 to 209, 244 to 263 | Continuous through the overrun, with a probability that halves over 2.5 s |

### The mapping

| Game event | snarl source | Fit | Missing |
| --- | --- | --- | --- |
| Change-up pop | `ShiftUp` counter. The game writes it from `State.Gear` against the previous tick (fivehorizons.md section 7), which is the same edge as exhaust.go line 131 | Good. The gates `Speed > 12` and `Throttle > 0.3` are both in the snapshot, so snarl can apply them, or the game can count only a gated change | A `ShiftUpEvents` field in `model.Control`, the diff in `engine.Render`, and a fire path from the event to `pops.fire` with a kind. The game pop is a single strong flame, so `kindPop` or `kindBoom` at a fixed gain, not a run |
| Change-down crackle | `ShiftDown` counter | Good. Gate on `Speed > 20` from the snapshot | Same plumbing as above. The game queues 2 or 3 pops 0.07 to 0.14 s apart. One `kindCrackle` event already is a train of 5 to 32 sub-events at 12.5 ms (pops.go line 53), so one event stands in for the game's three |
| Lift crackle | The rising edge of `FlagOverrun`, which snarl can detect from `over` in `Synth.control` (synth.go line 381 at `b762d34`). RPM in place of `Revs >= 0.75` (a profile `min_rpm` or 0.75 of the redline), `Speed > 20` from the snapshot | Good, but the trigger differs. The game needs a fall from 0.6 to 0.2 between two frames. snarl can hold the throttle of the last few blocks and test the same fall over about 150 ms, which also fixes the pad problem of section 5 | The chance (0.75), the rest (2 s), the count (1 to 3), and a rule that the burst replaces the continuous overrun run, or runs beside it at a lower density |
| Haze | None | No sound. An idle burble is a different thing and the pop model does not need it | Nothing |
| Run down from high to low revs | Not in the game. The game fires once at the lift and rests 2 s | snarl's overrun window (pops.go lines 182 to 209) already covers it, with a run probability that halves over 2.5 s | Lower density, or gate the continuous run on RPM falling and keep the lift burst as the main event |

The game writes the snapshot once per tick at 120 Hz and the exhaust reads once per frame, so the counters see a change at least as often as the exhaust does. A counter change of 2 in one block (two shifts in 8 ms) cannot happen, because `ShiftWindow` is 0.15 s (`internal/handling/handling.go` line 40).

One difference to decide: the game fires on the lever, when `State.Gear` changes at the start of the 0.15 s window (handling.go lines 435 to 439). `State.Upshift` (line 445) is true one tick later, at engagement above 5 m/s. For a sound, the pop belongs at the lever, when the throttle cuts, so the counter as proposed is right.

## 5. Assessment for a supercar arcade racer

Keep:

- The edge trigger with a rest. One burst per lift, then 2 s of quiet, is what makes a pop an event and not a texture. snarl's continuous overrun run is the cause of "too frequent". Confidence: high.
- The gates. Revs at or above 0.75 and speed above 20 m/s stop pops in town speeds and at low revs, where a real engine has too little unburnt fuel to pop. Speed above 12 m/s with the throttle above 0.3 on a change up means a hard shift, where a flat-shift pop is believable. Confidence: high.
- Change down always crackles. A supercar with an automatic blip crackles on the downshift. This is the one event the player causes on purpose, so a certain response rewards the input. Confidence: medium.
- Short bursts, uneven gaps, random side. One to three pops, 0.07 to 0.14 s apart, from one pipe or both, is the right scale. snarl's path target (`p.target`, pops.go lines 297 to 300) gives the side.
- Chance of three in four on a lift. Enough that the player learns the cause, not so certain that it becomes a metronome.
- The read boundary. `Game.engine` is the one place that reads the gear model for this purpose (`docs/devlog/056-exhaust-effects.md` line 16). The audio snapshot writer should be the same kind of thing.
- Zero allocation and a fixed seed (`TestExhaustUpdateDoesNotAllocate`, exhaust_test.go lines 113 to 124). Matches `docs/audio.md` line 12.

Weak points:

- The lift detector is one frame wide. It compares the throttle of the current frame with the previous frame (exhaust.go line 138). A keyboard release goes 1 to 0 in one tick and passes. A pad trigger released over 50 to 150 ms passes through 0.5, 0.4, 0.3 on the way and never shows 0.6 and 0.2 on two consecutive frames. I did not measure a pad. If this is right, pad players rarely see the crackle. For snarl, test the fall over a window of about 150 ms, or use the overrun flag edge with the revs gate. Confidence: medium.
- The burst is too short for a sound. Three flames in 0.3 s reads as a crackle on screen. In sound, a supercar overrun crackle is a train of cracks over 0.3 to 1.5 s (snarl `docs/research/crackles.md`). One snarl `kindCrackle` or `kindBoom` event per game event is the better unit.
- No heat. A cold engine pops less. snarl already has a heat state (pops.go lines 89 to 91, `heatFactor` line 211) and should keep it as a gain on the event pops, not drop it.
- Nothing during a long run down. A coast from 8000 to 2000 RPM gets one burst at the lift and then silence for 2 s, then nothing, because the throttle does not lift again. The user asks for pops on the run down. The game design has no answer. snarl's decaying run probability does, at a lower density than today.
- `Speed` is interpolated and `Revs` is from the tick (exhaust.go lines 67 to 72), so the two can be from slightly different moments. Harmless for a visual, and the snapshot avoids it by writing both from the same tick.
- Frame-rate sampling. The exhaust reads the tick state once per frame, so a throttle value that lasts one tick at 120 Hz can be missed at 60 Hz frames. The gear change is held for a full window, so it is never missed. The per-tick snapshot avoids this.
- The change-up pop has no revs gate. In low gear above 12 m/s the revs are above about 0.25, so the speed gate implies a revs floor, but a shift at 0.3 revs still pops. For sound, add a revs or load gate, or scale the gain by revs.

Recommendation for snarl: add `ShiftUpEvents` and `ShiftDownEvents` to `model.Control`, diff the counters in `engine.Render` as `Limiter` is diffed, and fire one gated event pop per counter change. Replace the continuous overrun scheduler with the game's burst and rest rule (one burst of one to three pops per lift, 2 s rest, three in four), and keep a sparse run during a long overrun at a fraction of today's density. Put the gates in snarl, not the game, so that the harness and the game behave the same from the same snapshot.

## Open questions

1. Where the gates live: in snarl from the snapshot (recommended), or in the game before it counts an event.
2. Whether the lift burst replaces the continuous overrun run or runs beside it at a lower density.
3. Whether a pad trigger lift crosses 0.6 to 0.2 in one frame in the game. A measurement with a real pad decides whether the game's visual trigger also needs a wider window.
4. Which snarl kind each game event uses: the change-up pop as `kindPop` or `kindBoom`, the change-down and lift crackles as `kindCrackle`, with the profile weights for the sparse run only.
