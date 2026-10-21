# Overnight report: snarl against engine-sim

Status: done. All work is on the branch `overnight/engine-sim-match`. `main` is unchanged at `7ed553f`, and nothing is pushed. `just test`, `just lint`, `just test-harness` and `just lint-harness` pass.

## Listen and look first

1. Play the four files in `out/listen-overnight/`, one for each tuned engine. Each file plays the full-throttle sweep three times, at the same loudness: before (`main`), after (this branch) at 9 s, and engine-sim at 18 s. `README.txt` lists the start times. A spectrogram of each file is beside it as a PNG.
2. Run the harness: `just run-harness`. The new display shows a level meter and a spectrum of what you hear, with the firing frequency marked. The S key hides it. Keys 1 to 4 select the tuned engines, and left and right step through all 46.

To rebuild the listening files: `just listen-overnight <path to a snarl-render built from main>`.

## What changed

- **Invented layers removed.** The valve tick, the intake noise and the cooling fan tone are gone. Neither upstream project has them. This also cut the CPU cost by about 8 per cent.
- **The signal chain now follows engine-sim.** I measured what engine-sim does from its runner pressure to its WAV output, and copied each step that matters:
  - the engine-sim peak leveller, which removes the level dip as the RPM rises
  - the derivative of the pressure, cut off at 1.9 kHz
  - the noise modulation in the engine-sim form, applied to the pulse before the exhaust response
  - the engine-sim input jitter, which gives the high-frequency noise its engine-sim level
- **Exhaust response fitted to engine-sim.** A bank of 4 to 7 filters for each engine, fitted to the engine-sim impulse response, replaces five EngineLab filters. The engine-sim impulse responses cannot ship because of their licence, so the bank stores fitted parameters, not copied data.
- **Pulse shape measured from engine-sim.** Each firing is now the engine-sim runner pressure pulse, stored as a small table for each engine. The variation from firing to firing is also measured from engine-sim.
- **Static fixed.** Each pulse starts at its exact position. The flat-6 idle timing spread, which turned harmonics into noise, is gone.
- **Review.** A code review found 16 issues: 6 defects and 10 smaller items. Fifteen are fixed, and each defect has a test that failed before the fix.

Every stage of the model now has an origin in the table in `docs/scoping.md` section 2: from engine-sim, from EngineLab, or new.

## Results against engine-sim

| Measure | Before | After | engine-sim |
| --- | --- | --- | --- |
| Energy above 2.8 kHz at full throttle | Up to 16 to 29 dB too high | Within about 2 dB on most holds | Target |
| Strongest engine order at the top hold | 0.7 to 2 kHz | 330 to 560 Hz | 100 to 560 Hz |
| Peak level spread over the sweep | 3.6 to 5.6 dB | 1.5 to 2.4 dB | 1.3 to 2.3 dB |
| Noise above 2 kHz at idle | Steady static | At most 1 dB above engine-sim | Target |

The spectrograms show the same change. Before, the flat-12 had a bright comb of harmonics from 1 to 6 kHz and noise up to 15 kHz. After, its energy sits below about 600 Hz with a clean firing line, close to engine-sim. The inline-4 is now very close to engine-sim.

The full tables for every engine are in `docs/research/snarl-gap.md`, part "Final (phase 4e)".

## CPU

The worst case on this PC is the flat-12 at the limiter, at 0.28 per cent of one core. Every other state is 0.21 to 0.25 per cent. `Render` still makes no allocations.

I could not run the phone benchmark: the sandbox now hides the USB device nodes, so `adb` sees no phone. By scaling the earlier phone results, I estimate about 2.0 per cent of the OnePlus 3T slow core and 3.1 to 4.1 per cent of a Switch core for the worst case. This is an estimate, not a measurement. To measure, allow `adb` outside the sandbox, or run the commands in `bench_layers_test.go` on the phone yourself.

## Decisions for you

1. **The idle hunt.** The slow idle speed wander that you asked for measures as noise against engine-sim, because engine-sim holds the crank speed fixed in its references. It is not static: the noise above 2 kHz at idle matches engine-sim. I kept it.
2. **flat-6 level spread.** Its loudness range over the sweep is 5.2 LU, against 3.0 LU in engine-sim. Closing it needs a change to the overrun level that you approved, so I kept your level.
3. **Part throttle.** Above the leveller knee, part throttle and full throttle now have almost the same level, as in engine-sim. Tell me if you want more difference.
4. **Brightness at high RPM.** The V8 and the flat-12 are still up to 400 to 500 Hz brighter than engine-sim at 4000 to 5000 RPM.
5. **The gap between firings at idle** is 4 to 12 dB deeper than in engine-sim, because engine-sim's impulse response is much longer than the fitted bank. Copying it needs a new and costly stage, so I did not add it.
6. **Stages marked "new, not approved"** in `docs/scoping.md` section 2:
   - **Collector overlap:** a stage that squares each bank sum. It is a nonlinear stage, so it can add harmonics of its own. I recommend that you judge it first. The inline-4 already has it off.
   - **The others:** the limiter jitter, the limiter handover, the final gear hold, the fractional pulse start and the cylinder timing spread. You heard and accepted these in the harness, so I recommend that you mark them approved.
7. **Imported engines.** They use the pulse table of the closest tuned engine, because the engine-sim catalogue references were rendered with too few CSV rows to measure their pulses.
8. **One simplification is left:** two tools print numbers in different formats. Merging them changes the output of one tool.

## Housekeeping

- `govulncheck` reports GO-2026-6809 in the Go 1.27.2 standard library, fixed in 1.27.3. The code does not call the affected function.
- **Rule breaches by the workers.** Some workers broke the rule against ad-hoc commands overnight: a few read-only shell loops and `awk` lines to read results, two empty heredocs, and some `sed` edits. No generated code ran, and every analysis tool is a reviewed command or test in the repository.
- **The branch:** 23 commits on top of `main`. Review them with `git log main..overnight/engine-sim-match`. When you are happy, merge the branch into `main`.
