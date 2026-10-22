# Overnight report: the crackle pop model

Status: done, apart from the blind test, which you must do. All work is on the branch `pop-model`, 20 commits on top of `main` (51f94ef), from c40b58f to 71f25e3. Nothing is pushed or merged. `go test ./...`, `go test -race -short ./...` and `go vet ./...` pass. `just lint` still fails on the known golangci-lint toolchain mismatch.

## 1. Listen first

The files are in `out/listen-pops-model/`. To build them again, run `nix develop --command just listen-pops-model`.

| File | What it plays |
| --- | --- |
| `clips-then-model.wav` | Three real clips, then three model crackles, at one level. `README.txt` gives the start times. |
| `v8-ls-liftoffs.wav`, `i4-vtec-liftoffs.wav`, `v12-412-liftoffs.wav` | Six lift-offs from 5000 rpm with the new pops, `race` character, at -19 LUFS. |
| `v8-ls-old-then-new.wav` | The same lift-offs with the old pops of `main`, then with the new pops, at -19 LUFS. |
| `blind/trial-01.wav` to `trial-20.wav` | Each trial plays item A, a short gap, then item B. One item is a real clip and the other is a model crackle. |

For the blind test, write A or B for the real recording in each trial. Open `blind/.key/answers.txt` only at the end. If you are correct in 14 or more of the 20 trials, you can hear the difference. If you are correct in 13 or fewer, the model passes. The real clip is item A in 10 trials and item B in 10 trials, so a listener who always answers A scores 10.

## 2. What changed

- **Measurement.** `tools/crackle` measures the 30 short exterior clips: the shocks, the rise, the crest factor, the energy in each band over time, the body, and the channels (`docs/research/crackle-measure.md`). The clips are one population of crackle trains. Each clip has about 40 shocks, a strong shock about every 32 ms, and a body near 52 Hz. No clip is a single bang.
- **The model.** `internal/crackle` is a generator from class-level statistics (`docs/research/crackle-model.md`). The shock times and levels come from the measured distributions. The shocks play from a bank of 64 grains, over a bed, a ring that follows the shock activity, and a 52 Hz noise body. The bank is made once per process from the parameters. No audio and no per-clip data is in the repository. The fit uses measurement and classic signal processing only, with no learned model.
- **The engine.** The decision layer of the pops is unchanged: lift-offs, coasts, requests, limiter bangs, characters, `FlagNoPops`, and the `Pops()` reports. Only the sound of each event changed. The pops now mix after the engine-sim runtime, before the resampler, so the 1900 Hz filter and the long impulse responses no longer remove the crack. The second runtime pass for the pops is gone.
- **The game API** in `api.go` is unchanged.

## 3. Results

The model against the clips, on 240 trains that no fit step used: 183 of 237 measures have their median inside the p25 to p75 of the clips. Nine measures cannot pass, because the model is mono and the clips have edit marks. The largest remaining misses are the band levels from 500 Hz to 1 kHz before and long after the strongest shock, 4.6 to 5.6 dB high.

The new pops in the engine, medians against the clips:

| | Above 2 kHz, dB | Below 125 Hz, dB | Body, Hz | Strong shock crest, dB |
| --- | --- | --- | --- | --- |
| Clips | -16.6 | -1.1 | 52 | 8.5 |
| Old pops | -24.3 to -27.4 | -5.4 to -13.8 | 86 to 421 | 6.3 to 7.0 |
| New `KindCrackle` | -15.6 | -1.0 | 49 | 7.9 |
| New `KindPop` | -12.8 | -1.8 | 50 | 10.0 |
| New `KindBoom` | -18.5 | -0.7 | 50 | 6.9 |
| New `KindBurble` | -12.2 | -1.9 | 51 | 11.6 |

With pops off, the engine output is byte-identical to `main` in 20 of 20 renders. The engine scorecard is unchanged, at 97.7 to 100.

## 4. Cost

| Engine | Phone slow core with pops, before | After | Render after |
| --- | --- | --- | --- |
| `i4-vtec` | 6.2% | 5.6% | 5.6% |
| `v8-ls` | 6.7% | 4.9% | 4.4% |
| `f4-ej25` | 6.4% | 5.7% | 5.7% |
| `v12-412` | 5.1 to 6.8% | 4.0% | 2.9% |
| `v12-merlin` | 6.1% | 4.3% | 3.3% |

The pops now add 0 to 1.2 points on the phone, against 1.4 to 3 points before. On this PC, `BenchmarkPops` fell from 0.97% to 0.78% of one core. One crackle voice costs about 0.05% of one core on this PC. The 4-cylinder engines are still over 5% on the phone, because of the engine itself and not the pops. The raw results are in `out/phone/pop-model-*.txt`.

## 5. Sources of the pop kinds

| Kind | Source |
| --- | --- |
| `KindCrackle` | Measured. Its length stays inside the measured range of 368 to 637 ms. |
| `KindBoom` | Partial. The body is measured. The length of 4 to 16 shocks is derived, because the clips have no short trains. |
| `KindPop` and the limiter bang | Derived. One strong shock of the measured shape. No clip holds an isolated pop. |
| `KindBurble` | Not measured. It plays one weak shock, the closest measured class. |

`internal/core/pops.sources.toml` records each of these.

## 6. Review

A code review found no defects, 3 risks and 9 maintainability findings. The fixes:

- A pop that took a busy slot stopped the old crackle in one sample, which made a click. The old crackle now fades out over 1.5 ms, with a test.
- A bad embedded model file made `New` panic. `New` now returns an error, with a test.
- Eight of the nine maintainability findings are fixed. The ninth adds an exported parser only for tests, so it is skipped.
- The dead pop keys `body_hz` and `jump_ms` and the unused `esynth.Unlevelled` are gone. `docs/scoping.md` section 2 and `docs/research/engine-cost.md` now describe the new mix point.

## 7. Decisions for you

1. **The blind test.** The model passes only after your test.
2. **Gear-up pops.** I sent you a recommendation this morning: a gear-up pop only at 0.75 of the redline or more, with the throttle at 0.8 or more, fired by snarl from `ShiftUp`. It has three open questions: the Merlin, the `stock` character, and a new `SourceShift` value. The evidence is in `out/research/upshift-pops-web.md` and `out/research/upshift-pops-code.md`. I changed no gear-up code.
3. **`KindBoom` and `KindBurble`.** Their sources are partial or not measured. Tell me if they stay.
4. **The catalogue.** `profiles/catalogue/README.md` and the 42 generated catalogue files still list the removed keys. They change when the catalogue is rebuilt with tables.

## 8. Rule breaches by the workers

- One no-op heredoc command, in the measurement phase.
- In the model phase: file edits with sed, two heredoc appends, and a temporary start file in `out/`. The worker also selected one parameter with the held-out trains. I had it select that parameter again with the fit trains, and check on new seeds.
- The model worker made a copy of the repository in `/tmp/snarl-before` and edited a file in it, and it did not report that. The copy had no Git data and was not part of any commit. I deleted it.
- In the fix phase: one inline shell loop to run 20 renders and compare them.

No generated script was committed, and every analysis tool is reviewed code in the repository.
