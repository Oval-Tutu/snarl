# Pop level by loudness

Status: done, apart from the listening choice, which you must make. The kind gains now come from loudness over the engine (ITU-R BS.1770 K-weighting), and one offset, `popOffsetLU` in `internal/core/pops.go`, moves all kinds together. The default offset is -6 LU, the second-highest variant, until you choose.

## 1. Listen first

Run `nix develop --command just listen-pop-level`. It writes `out/listen-pop-level/pop-level-variants.wav` and `README.txt`. The file is 265 s long. It plays four variants of the pop level. Each variant plays two lift-offs of each of the five engines with the `race` pops. Each engine has one gain in all four variants, the gain that puts it with no pops at -19 LUFS, so only the pops change.

| Variant | Starts at | Pops against the level before this change | `popOffsetLU` | `[pops] gain` at the default |
| --- | ---: | ---: | ---: | ---: |
| A | 0 s | -2 LU | -2 | 1.585 |
| B (default) | 67 s | -6 LU | -6 | 1 |
| C | 134 s | -10 LU | -10 | 0.631 |
| D | 201 s | -14 LU | -14 | 0.398 |

Inside each variant, the engines start 13 s apart in the order v8-ls, f4-ej25, v12-merlin, i4-vtec, v12-412. `README.txt` gives each start time. A silence detection of the file (ffmpeg `silencedetect`) finds the 1 s and 3 s gaps at the times of `README.txt`. To apply a choice, set `popOffsetLU` to the offset of the variant. Then set the bands of `TestPopLevelOverEngine` in `pops_test.go` to overLU plus the new offset, plus and minus 3.5 LU.

## 2. Why the pops were loud

Brightness does not make the pops loud. K-weighting puts each pop kind 1.9 to 3.1 dB below its plain RMS level, because most of the pop energy is in the body near 50 Hz, and the K-weighting high-pass at 38 Hz and its shelf above 1.7 kHz favour the middle band. The engine on the lift-offs is within -0.4 to +0.7 dB of its RMS level. So the RMS targets were not louder by loudness than by RMS.

The pops were loud for two reasons:

- **They sit above the engine.** With the `race` character (+3 dB), the median pop was 2.4 to 4.0 LU over the engine on overrun by span loudness. The crackle trains fill the 400 ms momentary window, so the crackle was +2.5 LU over the engine on the momentary loudness, the highest of the kinds.
- **They reach full scale.** The engine and the pops together reached the pop limiter ceiling of 0.95 in 89 of the 192 lift-off pops. The limiter cut the pop gain by up to 12.6 dB.

The pop limiter does not duck the engine. `Engine.mix` adds `g * pop` to the engine sample, and the gain `g` scales only the pops. A render with pops less a render with no pops is the pops alone, to the last bit, at the 44.1 kHz synthesiser rate.

## 3. Measured levels

`just pop-level` measures the levels with `tools/crackle --pop-level`. For each engine and each kind, it renders the lift-offs with only that kind in use, with no pops (`FlagNoPops`, the same seed), and with the pops at a gain of 0.01. All three renders are at 44.1 kHz, where the engine has no resampler. The render with pops less the render with no pops is the pop after the limiter. The quiet render less the render with no pops, times 100, is the pop before the limiter, because the pop gain sets no random draw. The pop start comes from the pop report of the engine. A pop ends at the next pop or after 1 s.

- **RMS**: the plain RMS level of the pop over the span from its start that holds 90 per cent of its energy (at least 10 ms), against the engine with no pops over the same span. This is the old calibration measure.
- **Span**: the same span and ratio, with both signals K-weighted (`analysis.KWeighted`).
- **Momentary**: the K-weighted ratio over the 400 ms from the start of the pop. The window holds the later pops of a lift-off run, because they start 0.11 to 0.26 s apart.
- **Loss**: the level that the pop limiter takes from the pop.

The scenarios are `tools/crackle/lift.scn` (5000 rpm, for v8-ls, f4-ej25 and v12-merlin), `lift-i4-vtec.scn` (6500 rpm) and `lift-v12-412.scn` (14000 rpm), with the `race` character and the profile pop density. The values are the medians over the pops, then the mean over the five engines.

| Kind | RMS before | Span before | Momentary before | Span after | Momentary after | Pop K | Engine K |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| pop | +6.3 dB | +4.0 LU | -3.8 LU | -0.1 LU | -7.9 LU | -2.1 dB | -0.1 dB |
| boom | +6.5 dB | +3.5 LU | -0.8 LU | -2.1 LU | -6.0 LU | -3.1 dB | -0.1 dB |
| crackle | +4.6 dB | +2.4 LU | +2.5 LU | -3.2 LU | -2.6 LU | -2.4 dB | -0.1 dB |
| burble | -1.3 dB | -3.2 LU | -10.7 LU | -9.3 LU | -16.7 LU | -1.9 dB | -0.1 dB |

"Pop K" and "Engine K" are the K-weighted level over the plain level of the pop and of the engine over the span. "After" is the default variant B. The pops fall by 4.1 to 6.1 LU, not by 6 LU in each case, because before the change the limiter took 1.3, 0.2 and 0.5 dB from the median pop, boom and crackle, and now it takes nothing from them.

| Engine | Engine K | Pops | Pops cut before | Least limiter gain before | Pops cut after | Least limiter gain after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| v8-ls | -0.1 dB | 52 | 27 | -11.4 dB | 5 | -5.5 dB |
| f4-ej25 | -0.4 dB | 32 | 21 | -12.6 dB | 7 | -6.7 dB |
| v12-merlin | -0.4 dB | 44 | 25 | -8.8 dB | 3 | -2.8 dB |
| i4-vtec | -0.2 dB | 36 | 10 | -4.9 dB | 0 | 0 dB |
| v12-412 | +0.7 dB | 28 | 6 | -10.4 dB | 2 | -4.4 dB |

A pop is cut when the limiter takes more than 0.1 dB from it. At the default, the limiter cuts 17 of the 192 pops, against 89 before.

`TestPopLevelOverEngine` checks the same span loudness on 41 own pops and 61 requested pops of each kind on each profile at 48 kHz. At the default, every median is within 1.8 LU of its target (-3, -4, -6.7 and -12.2 LU for pop, boom, crackle and burble), inside the 3.5 LU bands.

As a cross-check of the K-weighting, ffmpeg `ebur128` reads the v8-ls and v12-412 sections of variant D at -19.0 LUFS, and the whole file at -19.0 LUFS integrated. `analysis.IntegratedLoudness` set the engine gains to -19 LUFS with no pops, and the pops of variant D are 14 LU below the level before this change.

## 4. The calibration

`calibrate` in `internal/core/calibrate.go` renders 32 voices of each kind with no engine, as before. It now measures the K-weighted RMS level of each voice over the span that holds 90 per cent of its plain energy. The gain puts a voice of unit scale at `overLU + popOffsetLU` over the loudness of an engine of unit RMS level. The engine loudness is its RMS level plus `engineKDB`, -0.1 dB, the mean Engine K of the five engines in the table above.

`engineKDB` is a correction measured offline. The runtime keeps its plain RMS engine level, so the pops add no filter and no per-sample work. The correction is off by -0.3 to +0.8 dB on single engines. On v12-412, the pops are 0.8 LU below their target over the engine.

`overLU` keeps the relative levels of the RMS targets of commit 1d25f9a (pop +4.8, boom +4.5, crackle +1.5, burble -4.3 dB). Each value is the RMS target plus the K-weighted over the plain level of the calibration voices of the kind (pop -1.95, boom -2.57, crackle -2.27, burble -1.95 dB), less `engineKDB`: pop +3.0, boom +2.0, crackle -0.7 and burble -6.2 LU. At `popOffsetLU` 0, each kind gain is within 0.05 dB of its RMS gain (`TestKindLoudness`). The requested short crackle has its own calibration, so it now has the loudness target of the crackle and not its RMS target.

The measurement shows one possible defect, which this change does not fix. By span loudness, the crackle is the third of the four kinds. By momentary loudness, the crackle is the loudest kind by 3.3 LU, because its trains are 368 to 637 ms long and the other kinds are short. When you choose a variant, listen for a crackle that stands out from the pops and the booms. If it does, the crackle target can move down by itself.

## 5. Costs

The pops add no measurable cost. The calibration runs once per process, when the first engine starts, and adds one K-weighting pass over each of its 160 voices. `Render` runs the same code as before. `go test -bench '^Benchmark(Render|Pops)$' -count 8` with benchstat on the AMD Ryzen 9 7940HS of this PC:

| Benchmark | Before, per op | After, per op | Change |
| --- | ---: | ---: | --- |
| Pops, five profiles | 108.1 to 162.0 µs | 108.0 to 162.0 µs | no change (p = 0.10 to 0.96) |
| Render, five profiles | 91.9 to 167.0 µs | 92.5 to 168.8 µs | +1.0 to +1.8 per cent on three profiles |
| Geomean of all | 130.4 µs | 131.1 µs | +0.5 per cent |

`Render` runs no new code. The after run started straight after the renders of the variants, and I did not run the before benchmark again, so the Render change is not confirmed as noise. Both benchmarks allocate nothing, before and after.

With the pops off (`pops.density=0`), the renders of the five lift-off scenarios at 44.1 kHz are byte-identical to the renders of HEAD 3616fff.
