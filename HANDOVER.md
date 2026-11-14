# Handover: the `pop-model` branch

Status on 14 November 2026: the pop work is complete, and the user approved the sound. The branch `pop-model` is not merged and not pushed. The repository has no Git remote. Three items stand before the merge to `main`: two user answers and the final checks.

## 1. Where the work is

- Branch `pop-model`, 51 commits on top of `main` (51f94ef), from c40b58f to the commit of this file.
- The tree is clean. `go vet ./...`, `go test ./...` and `go test -race -short ./...` pass.
- Run all tools through the dev shell: `nix develop --command just <recipe>` or `nix develop --command go ...`.
- `just lint` fails on a known mismatch between golangci-lint (go1.26) and Go 1.27.2. The user said to ignore it for now.

## 2. What the branch holds

| Work | Result | Report |
| --- | --- | --- |
| Engine tone across the full RPM range | 867 of 919 holds pass, against 814 before. The drive scorecard misses 20 windows, against 24. The Merlin click after a lift is fixed. | `docs/research/fullrange-scorecard.md`, `docs/research/drive-scorecard.md` |
| The v12-412 miss at 12500 rpm | engine-sim's own variation. engine-sim scores 66.9 against itself on another path, and snarl scores 64.6. Nothing changed. | `docs/research/fullrange-scorecard.md` section 14 |
| A new crackle model | Code made from class-level statistics of the 30 short exterior clips of the Skril Studio muffler crackle pack. No audio and no per-clip data is in the repository. | `docs/research/crackle-measure.md`, `docs/research/crackle-model.md`, `docs/research/pop-model-overnight.md` |
| The pop level | The calibration uses loudness (ITU-R BS.1770). The user chose variant C: `popOffsetLU = -10` in `internal/core/pops.go`. | `docs/research/pop-level.md` |
| The pop tail | Pops hold about 111 ms after the last strong shock, with weak trailing shocks. Each boom has one shock at the top level. `hold_tail_db` shortens the 27 Hz tail of pops and booms. | `docs/research/pop-tail.md` sections 8 and 9 |

One known miss remains in the tail: below 200 Hz, a pop takes 212 ms to fall 40 dB, and the clips take 161 ms (p75 177 ms). The full fit removes the 27 Hz thump that the clips have, so the thump stays. The user listened and said the result "sounds fine".

## 3. Open questions for the user

Restate each question in full when you ask it. The user does not accept a reference to a question without its text.

1. **The hiss.** On 2 November, the user received the engine-sim against snarl files in `out/listen-ab/`, drive files first. The user heard an airy, hissy high end when the engine revs. Ask the user if the hiss is still there, and if so, on which engine and at about which second.
2. **Gear-up pops.** The recommendation: a gear-up pop plays only at 0.75 of the redline or more, with the throttle at 0.8 or more, fired by snarl from `ShiftUp`. The RPM gates are i4-vtec 6300, v8-ls 4875, f4-ej25 4875, v12-412 13500, v12-merlin 2250. No gear-up code is changed yet. Three questions:
   1. The Merlin has no gearbox. Should it have no gear-up pops, or the same rule if the game calls `ShiftUp`?
   2. Should the `stock` character of the 4-cylinder engines have no gear-up pops, or should every character use the same rule?
   3. Should the pop report give a gear-up pop a new source value, `SourceShift`, or report it as the engine's own pop, `SourceOwn`?

## 4. Next steps, in order

1. Get the answers of section 3. If the user hears the hiss, investigate it with the full-range and drive scorecards before any listening file.
2. Make the gear-up pops from the answers.
3. Rebuild the 20 blind-test trials from the current model (`just listen-pops-model`), if the user still wants the test. The old trials are from the model before the level and tail changes.
4. Measure the cost on the phone again (section 6).
5. Bring `docs/research/pop-model-overnight.md` up to date, delete this file, and prepare the merge of `pop-model` to `main`.

After the merge: the 4-cylinder phone cost, which is above 5% (the engine, not the pops), the catalogue rebuild with tables (`profiles/catalogue/README.md` still lists the removed keys `body_hz` and `jump_ms`), the flat-6, the lint toolchain, GO-2026-6809, and the FiveHorizons integration.

## 5. Rules from the user

- The goal: at least 90% of engine-sim quality at about 5% of one core on the weakest target. Upstream profiles only. Nothing fabricated, and nothing tuned by ear.
- Run the full-range and drive scorecards, and tune as close as performance allows, before you offer the user an A/B test.
- Stay on `pop-model` for this work. Do not make a new branch.
- No pushes, no history rewrites without consent, and no agent attribution in commits.
- No ad-hoc or generated scripts, no sed or heredoc edits, and no inline shell loops. Use the file edit tools. Every analysis tool is reviewed Go code in the repository.
- The pack WAVs never enter the repository, only class-level statistics. No machine learning on the pack. The model trains on the 30 short exterior clips only.
- Do not raise licences.
- Write to the user in plain language, with no internal jargon. Restate any question that you refer to, and give correct dates.

## 6. Files that Git does not carry

`out/` is ignored by Git. Copy what you need, or build it again:

| Path | Rebuild |
| --- | --- |
| `out/pack/muffler/` | Extract the pack from `/home/martin/Development/Oval-Tutu/FH-audio/muffler-crackle-sounds-slaczky/`. `just crackle-measure`, `crackle-fit` and `crackle-fit-tail` need it. The engine does not. |
| `out/listen-ab/` | `just ref-listen` and `just listen-ab` |
| `out/listen-pop-level/` | `just listen-pop-level` |
| `out/listen-pop-tail-2/` | `just listen-pop-tail`, as described in `docs/research/pop-tail.md` section 9 |
| `out/phone/` | The phone results and the `adb` commands. The method is in `docs/research/engine-cost.md` sections 6 and 8. |

The phone is a OnePlus 3T. The `adb` client in an agent session cannot reach USB, so the user starts the `adb` server in a normal terminal, and the agent connects on localhost:5037. The agent may push, run and remove the test binary in `/data/local/tmp`, read the CPU frequency files, and wake the screen with `adb shell input keyevent KEYCODE_WAKEUP`. Install nothing, change no settings, and do not reboot.
