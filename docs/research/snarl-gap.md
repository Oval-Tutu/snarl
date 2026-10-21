# snarl against the engine-sim targets

Measured at `195ba4e` (tier 1, 48 kHz, the profiles as committed), before any change of the parallel model work. The targets are those of [engine-sim-target.md](engine-sim-target.md). The fitted formant bank and the path assessment are in [formant-fit.md](formant-fit.md).

## Answer

snarl misses every engine-sim target. The gaps, largest first:

1. Too much energy above 2.84 kHz, and harmonics up to 4 to 6 kHz. At full throttle, the power above 2.84 kHz is up to 16 dB (flat12, v8), 23 dB (flat6) and 29 dB (inline4) higher than in engine-sim. In the 2 to 6 kHz band, the residual share falls to -5 to -12 dB at high RPM, so harmonics are still there. In engine-sim it is 0 to -2.6 dB (noise only). The spectral centroid is 0.9 to 2.5 kHz and rises with the RPM, against 0.3 to 1.7 kHz in engine-sim. This is the likely cause of the synthetic sound at high RPM. Fix: the 1.9 kHz input low pass and the formant bank.
2. No fixed formant. The strongest order of snarl follows the firing frequency at low RPM (inline4 order 2 at 27 to 67 Hz, flat6 order 3 at 40 to 200 Hz, v8 order 4 at 45 to 317 Hz), and at high RPM it moves to 0.7 to 2 kHz. In engine-sim it stays near the formants, at 100 to 560 Hz. Fix: the formant bank in place of the chamber and outlet stages.
3. The level moves with the RPM. Over the full-throttle sweep, the loudness spread is 3.1 to 5.8 LU against 1.3 to 2.3 LU for flat12, inline4 and v8, and the peak spread 3.6 to 5.6 dB against 1.3 to 2.3 dB for all four. flat6 has the same loudness spread as engine-sim (2.9 against 3.0 LU), but it falls above 3000 RPM where engine-sim rises. The crest factor is up to 5 to 8 dB higher at full throttle. Fix: the engine-sim leveller.
4. The noise is wrong in level, trend and modulation. At full throttle the residual of snarl rises with the RPM (v8 from -18 to -12 dB) where engine-sim falls (-17 to -30 dB). The AM troughs are up to 12 to 14 dB deeper than in engine-sim, so the noise comes in bursts. At the closed-throttle idle, the 6 to 16 kHz noise is 8 to 25 dB below engine-sim. Fix: the noise AM before the formant filters, with the depth of `engine-sim-target.md` section 5.

## 1. Method

- Renders: `snarl-render` built at `195ba4e` with a clean tree, for `sweep-wot`, `holds-t000`, `holds-t050` and `holds-t100` of each profile: `out/analysis/snarl/<profile>/render/<scenario>.wav`.
- `just analyse` on each render: `out/analysis/snarl/<profile>/<scenario>/` with `spectrogram.png`, `waveform.png`, `waveform-log.png`, `histogram.png`, the ffmpeg logs, `describe.md` and `residual.md`.
- `snarl-compare --gap --scenario S.scn REF.wav B.wav` (new) puts the describe and residual measures of the stored engine-sim reference and of the snarl render side by side: `out/analysis/snarl/<profile>/<scenario>/gap.md`. A is engine-sim, B is snarl.
- snarl calibrates its sweep to -16 LUFS and engine-sim runs at `--volume 0.5`, so compare spreads and shapes, not absolute levels.

## 2. Gap table

Ranges over the windows of the full-throttle holds (`holds-t100`) unless the row names another scenario.

### flat12

| Target | engine-sim | snarl | Gap |
| --- | --- | --- | --- |
| Loudness spread, `sweep-wot` | 1.3 LU | 5.8 LU | 4.5 LU more |
| Peak spread, `sweep-wot` | 1.3 dB | 5.6 dB | 4.3 dB more |
| Crest factor | 6.6 to 8.3 dB | 10.0 to 14.6 dB | 2 to 7 dB higher |
| Centroid | 1251 to 1745 Hz | 1284 to 1991 Hz | rises to 2 kHz at the top |
| Power above 2.84 kHz | -22 to -30 dB | -13 to -25 dB | 2 to 16 dB more |
| Residual share 2-6 kHz | 0.0 to -0.1 dB | -0.1 to -11.7 dB | harmonics above 2 kHz |
| Residual | -6.0 to -12.8 dB | -11.3 to -18.1 dB | 1 to 12 dB less noise |
| Residual 6-16 kHz | -29 to -34 dB | -30 to -36 dB | close |
| AM peak / trough | +3.0 to +4.7 / -5.8 to -12.5 dB | +6.4 to +10.0 / -9.2 to -15.3 dB | peaks 2 to 7 dB higher, troughs up to 9 dB deeper (shallower at 3000 RPM) |
| Strongest order | 6 (100 to 475 Hz), then 2 and 4 (167 to 400 Hz) | 6 and 3 to 3000 RPM, then 9, 11, 6, 3 (300 to 750 Hz) | moves with RPM |
| Idle, closed throttle (1000 RPM, `holds-t000`) | residual -3.2 dB, 6-16 kHz -30.5 dB | -3.1 dB, -55.5 dB | 25 dB too little high noise |

### flat6

| Target | engine-sim | snarl | Gap |
| --- | --- | --- | --- |
| Loudness spread, `sweep-wot` | 3.0 LU (rises with RPM) | 2.9 LU (falls above 3000 RPM) | wrong trend |
| Peak spread, `sweep-wot` | 2.3 dB | 3.6 dB | 1.3 dB more |
| Crest factor | 3.6 to 10.0 dB | 6.7 to 10.6 dB | up to 7 dB higher at the top |
| Centroid | 765 to 1168 Hz | 1054 to 2253 Hz | up to 1.3 kHz higher |
| Power above 2.84 kHz | -27 to -42 dB | -15 to -28 dB | up to 23 dB more |
| Residual share 2-6 kHz | 0.0 to -0.8 dB | -0.1 to -10.6 dB | harmonics above 2 kHz |
| Residual | -16.7 to -32.4 dB | -14.8 to -22.0 dB | up to 18 dB more noise at high RPM |
| Residual 6-16 kHz | -33 to -46 dB | -32 to -40 dB | up to 12 dB more |
| AM peak / trough | +2.5 to +5.4 / -2.8 to -6.5 dB | +4.9 to +7.6 / -7.8 to -19.7 dB | troughs 4 to 13 dB deeper |
| Strongest order | 18, 9, 6, 3 (240 to 400 Hz) | 3 (40 to 300 Hz), 10 (1.1 kHz) at the top | no formant |
| Idle, closed throttle (800 RPM) | residual -16.1 dB, 6-16 kHz -37.8 dB | -11.7 dB, -60.4 dB | 23 dB too little high noise |

### inline4

| Target | engine-sim | snarl | Gap |
| --- | --- | --- | --- |
| Loudness spread, `sweep-wot` | 2.3 LU | 3.1 LU | 0.8 LU more |
| Peak spread, `sweep-wot` | 2.3 dB | 4.1 dB | 1.8 dB more |
| Crest factor | 3.6 to 8.3 dB | 8.9 to 15.2 dB | 3 to 8 dB higher |
| Centroid | 337 to 1088 Hz | 945 to 2464 Hz | 0.6 to 1.4 kHz higher |
| Power above 2.84 kHz | -34 to -52 dB | -9 to -28 dB | 20 to 29 dB more |
| Residual share 2-6 kHz | -0.2 to -2.6 dB | 0.0 to -9.1 dB | harmonics above 2 kHz |
| Residual | -23.9 to -41.8 dB | -10.0 to -23.1 dB | 1 to 22 dB more noise |
| Residual 6-16 kHz | -37 to -55 dB | -29 to -42 dB | 6 to 16 dB more |
| AM peak / trough | +4.3 to +11.2 / -5.9 to -18.3 dB | +8.8 to +12.3 / -12.9 to -26.4 dB | troughs up to 14 dB deeper |
| Strongest order | 10, 6, 4, 2 (125 to 560 Hz) | 2 (27 to 267 Hz), 8 (0.9 to 1.1 kHz), 14 (1.9 kHz) | sub-bass at idle, 2 kHz at the top |
| Idle, closed throttle (800 RPM) | residual -23.0 dB, 6-16 kHz -52.9 dB | -30.8 dB, -60.5 dB | 8 dB too little high noise |

### v8

| Target | engine-sim | snarl | Gap |
| --- | --- | --- | --- |
| Loudness spread, `sweep-wot` | 1.7 LU | 3.6 LU | 1.9 LU more |
| Peak spread, `sweep-wot` | 2.2 dB | 5.6 dB | 3.4 dB more |
| Crest factor | 5.6 to 8.5 dB | 7.4 to 12.3 dB | up to 5 dB higher |
| Centroid | 868 to 1049 Hz | 1246 to 1721 Hz | 0.2 to 0.8 kHz higher |
| Power above 2.84 kHz | -28 to -36 dB | -19 to -26 dB | 5 to 16 dB more |
| Residual share 2-6 kHz | 0.0 to -0.2 dB | 0.0 to -5.2 dB | harmonics above 2 kHz |
| Residual | -16.7 to -30.3 dB | -12.4 to -18.3 dB | up to 18 dB more noise at high RPM |
| Residual 6-16 kHz | -35 to -42 dB | -36 to -39 dB | close |
| AM peak / trough | +2.4 to +5.6 / -3.0 to -9.4 dB | +5.2 to +10.3 / -8.7 to -20.9 dB | troughs 3 to 12 dB deeper |
| Strongest order | 16, 12, 8, 5, 4 (180 to 417 Hz, mostly 400 Hz) | 4 (45 to 317 Hz), 12 and 8 (0.8 to 1 kHz) | no 400 Hz formant |
| Idle, closed throttle (675 RPM) | residual -11.3 dB, 6-16 kHz -35.3 dB | -5.4 dB, -60.1 dB | 25 dB too little high noise |

## 3. What the images show

- Spectrograms of `sweep-wot`: in every snarl file the harmonic lines go on to 4 to 6 kHz and fan out with the RPM. In engine-sim they stop at 1.5 to 2 kHz ([snarl inline4](../../out/analysis/snarl/inline4/sweep-wot/spectrogram.png) against [engine-sim inline4](../../out/analysis/engine-sim/inline4/sweep-wot/spectrogram.png)). The engine-sim v8 has a fixed bright band at 340 to 420 Hz. The snarl v8 has none, and its brightest lines move from 130 Hz to 800 Hz ([snarl v8](../../out/analysis/snarl/v8/sweep-wot/spectrogram.png) against [engine-sim v8](../../out/analysis/engine-sim/v8/sweep-wot/spectrogram.png)).
- Fixed horizontal lines from 2 kHz to 6 kHz show in every snarl file at every RPM ([snarl flat6](../../out/analysis/snarl/flat6/sweep-wot/spectrogram.png), [snarl v8 holds-t000](../../out/analysis/snarl/v8/holds-t000/spectrogram.png)). They are the peaks of the expansion chamber comb (every 807 Hz for the v8) with the outlet rise. engine-sim has no fixed lines above 2 kHz.
- At the closed-throttle idle, snarl is dark above 3 kHz ([snarl v8 holds-t000](../../out/analysis/snarl/v8/holds-t000/spectrogram.png), first 3 s). engine-sim has its modulated noise there.
- Waveforms: the peak envelope of snarl dips in the middle of the sweep and bulges from 4500 to 5500 RPM ([snarl v8](../../out/analysis/snarl/v8/sweep-wot/waveform.png), [snarl flat12](../../out/analysis/snarl/flat12/sweep-wot/waveform.png)). The engine-sim envelope is flat.
- Histograms: the snarl inline4 samples have a sharp negative edge and a long positive tail ([snarl inline4](../../out/analysis/snarl/inline4/sweep-wot/histogram.png)). The engine-sim inline4 has a broad flat top with sharp edges on both sides, which the leveller ceiling gives.

## 4. PNG paths

All under `out/analysis/snarl/<profile>/<scenario>/`, for the profiles flat12, flat6, inline4 and v8 and the scenarios `sweep-wot`, `holds-t000`, `holds-t050` and `holds-t100`: `spectrogram.png`, `waveform.png`, `waveform-log.png` and `histogram.png`. The engine-sim images of the same names are under `out/analysis/engine-sim/<profile>/<scenario>/`.

## 5. Limits

- The strongest order uses whole orders only, at the mean RPM of the window. The half orders are in `describe.md`.
- `holds-t050` has gap tables (`gap.md`), but this document does not summarise them.
- The residual of a hold is only valid when the cycle period is inside the search range. Every hold here has a valid residual.

## 6. Result

Measured on branch `overnight/engine-sim-match` after the formant bank change. "Before" is `a73b874` (the leveller, the 1.9 kHz pulse low pass and the noise AM, with the EngineLab path stages and `hf_gain` 0). "After" is the working tree with these changes:

- The formant bank (`[[formant]]`), fitted by `tools/fit --base ir --hi 16000` to the engine-sim impulse response alone, runs once on the sum of the paths before the leveller. It replaces the wall loss, the low shelf, the expansion chamber, the mode cutoff and the outlet.
- The wave steepening and the jet noise are removed. The collector stays.
- `hf_gain`, `am` and the new `input_jitter` (the engine-sim input jitter) are the engine-sim values of each script. The square-root noise fall is removed.
- The pulse envelope has a squared attack, with `attack` 4, so that each pulse starts with no slope.
- The pops pass a pop bank fitted to their earlier path stages.

Renders: `out/phase4c/head/<profile>/` (before) and `out/phase4c/after/<profile>/` (after), with the `snarl-compare --gap` tables `gap-<scenario>.md` beside them, and the `just analyse` images of the after renders in `out/phase4c/after/<profile>/<scenario>/`. Ranges are over the windows of `holds-t100` unless the row names another scenario. The loudness and peak bands are the spreads of the 0.5 s windows of `sweep-wot`.

### Answer

The formant bank, the engine-sim noise settings and the smooth pulse close most gaps. The strongest order of the top full-throttle hold now sits at the engine-sim formant for flat6, inline4 and v8, and at 340 Hz against 227 Hz for flat12 (680 Hz to 1.1 kHz before). The 6 to 16 kHz noise is now within 4 dB of engine-sim at most holds, and up to 6 dB above it at two holds, where it was 2 to 32 dB below. The peak band of the sweep is 1.3 to 2.9 dB (engine-sim 1.3 to 2.3 dB), and the crest factor at the top holds fell by 2 to 7 dB. Three gaps remain:

1. The loudness band of the sweep is 5.8 LU for inline4 and 3.7 LU for v8, against 2.3 and 1.7 LU in engine-sim. Their crest factor rises to 10 to 12 dB at the top of the sweep and below 1500 RPM, against 7 to 9 dB in engine-sim. The engine-sim runner pressure fills the interval between firings at high RPM, so its firing order dominates. The snarl pulses of inline4 leave gaps at 8000 RPM, so the orders 6 to 10 are 5 to 14 dB stronger than in engine-sim.
2. The residual of the full-throttle holds is 5 to 13 dB above engine-sim at high RPM for flat6, inline4 and v8. The slow `[pulse] jitter` (approved) gives about 6 dB of it: in a test render of the v8 with `jitter` 0, the residual at 3000 to 6000 RPM was -21 to -27 dB, against -26 to -30 dB in engine-sim. The input jitter adds less than 1 dB to the residual.
3. The centroid of v8 is 1066 to 1464 Hz against 868 to 1049 Hz in engine-sim, and the flat12 centroid is 1426 to 1856 Hz against 1251 to 1525 Hz, and the v8 hold at 5000 RPM has 5.5 dB more power above 2.84 kHz. Order 12 meets the narrow formant at 1 kHz there.

### flat12

| Measure | engine-sim | Before | After |
| --- | --- | --- | --- |
| Loudness band, `sweep-wot` | 1.3 LU | 2.3 LU | 2.0 LU |
| Peak band, `sweep-wot` | 1.3 dB | 2.8 dB | 1.5 dB |
| Crest factor | 7.0 to 8.3 dB | 7.3 to 14.8 dB | 6.9 to 9.6 dB |
| Centroid | 1251 to 1525 Hz | 532 to 1557 Hz | 1426 to 1856 Hz |
| Power above 2.84 kHz | -25.6 to -30.3 dB | -28.7 to -37.7 dB | -24.4 to -29.6 dB |
| Strongest order, top hold (6800 RPM) | 2 (227 Hz) | 6 (680 Hz) | 3 (340 Hz) |
| Strongest order, all holds | 100 to 400 Hz | 100 to 750 Hz | 100 to 500 Hz |
| Residual | -6.0 to -12.8 dB | -12.6 to -18.9 dB | -15.3 to -23.7 dB |
| AM peak / trough | +3.0 to +4.7 / -5.8 to -12.5 dB | +5.1 to +9.2 / -7.4 to -12.6 dB | +5.3 to +8.5 / -6.6 to -10.4 dB |
| Residual 6-16 kHz | -29.0 to -33.8 dB | -37.8 to -62.7 dB | -28.0 to -33.2 dB |
| Idle, closed throttle (1000 RPM): residual, 6-16 kHz | -3.2, -30.5 dB | -3.3, -62.6 dB | -2.8, -32.9 dB |

### flat6

| Measure | engine-sim | Before | After |
| --- | --- | --- | --- |
| Loudness band, `sweep-wot` | 3.0 LU | 3.1 LU | 3.1 LU |
| Peak band, `sweep-wot` | 2.3 dB | 3.7 dB | 1.3 dB |
| Crest factor | 3.6 to 10.0 dB | 7.0 to 10.7 dB | 5.2 to 10.3 dB |
| Centroid | 846 to 1168 Hz | 662 to 1364 Hz | 733 to 1136 Hz |
| Power above 2.84 kHz | -26.5 to -39.0 dB | -30.1 to -38.5 dB | -29.4 to -38.2 dB |
| Strongest order, top hold (6600 RPM) | 3 (330 Hz) | 10 (1100 Hz) | 3 (330 Hz) |
| Strongest order, all holds | 240 to 400 Hz | 40 to 1100 Hz | 150 to 330 Hz |
| Residual | -16.7 to -32.4 dB | -15.0 to -20.3 dB | -19.5 to -27.0 dB |
| AM peak / trough | +2.5 to +5.4 / -2.8 to -6.5 dB | +4.6 to +7.3 / -8.9 to -23.5 dB | +3.2 to +7.0 / -5.1 to -10.3 dB |
| Residual 6-16 kHz | -33.1 to -45.9 dB | -43.8 to -63.6 dB | -36.5 to -45.4 dB |
| Idle, closed throttle (800 RPM): residual, 6-16 kHz | -16.1, -37.8 dB | -11.1, -64.2 dB | -6.0, -36.5 dB |

### inline4

| Measure | engine-sim | Before | After |
| --- | --- | --- | --- |
| Loudness band, `sweep-wot` | 2.3 LU | 4.1 LU | 5.8 LU |
| Peak band, `sweep-wot` | 2.3 dB | 2.4 dB | 2.9 dB |
| Crest factor | 3.8 to 8.3 dB | 8.4 to 14.6 dB | 2.8 to 12.0 dB |
| Centroid | 337 to 1088 Hz | 221 to 1646 Hz | 321 to 1575 Hz |
| Power above 2.84 kHz | -34.2 to -51.7 dB | -24.6 to -54.4 dB | -30.4 to -52.6 dB |
| Strongest order, top hold (8400 RPM) | 4 (560 Hz) | 6 (840 Hz) | 4 (560 Hz) |
| Strongest order, all holds | 133 to 560 Hz | 27 to 1067 Hz | 100 to 560 Hz |
| Residual | -23.9 to -41.8 dB | -13.3 to -33.1 dB | -15.5 to -34.6 dB |
| AM peak / trough | +4.3 to +11.2 / -5.9 to -18.3 dB | +10.4 to +12.5 / -10.8 to -23.6 dB | +4.6 to +9.9 / -7.8 to -20.6 dB |
| Residual 6-16 kHz | -37.4 to -55.0 dB | -39.5 to -73.4 dB | -33.7 to -55.7 dB |
| Idle, closed throttle (800 RPM): residual, 6-16 kHz | -23.0, -52.9 dB | -33.1, -74.2 dB | -26.6, -53.1 dB |

### v8

| Measure | engine-sim | Before | After |
| --- | --- | --- | --- |
| Loudness band, `sweep-wot` | 1.7 LU | 4.3 LU | 3.7 LU |
| Peak band, `sweep-wot` | 2.2 dB | 3.7 dB | 1.5 dB |
| Crest factor | 5.6 to 8.5 dB | 5.6 to 12.7 dB | 5.8 to 9.5 dB |
| Centroid | 868 to 1049 Hz | 530 to 1365 Hz | 1066 to 1464 Hz |
| Power above 2.84 kHz | -28.2 to -36.3 dB | -30.8 to -42.1 dB | -26.8 to -37.8 dB |
| Strongest order, top hold (6000 RPM) | 4 (400 Hz) | 8 (800 Hz) | 4 (400 Hz) |
| Strongest order, all holds | 180 to 417 Hz | 45 to 1000 Hz | 45 to 400 Hz |
| Residual | -16.7 to -30.3 dB | -13.4 to -21.7 dB | -15.9 to -21.6 dB |
| AM peak / trough | +2.4 to +5.6 / -3.0 to -9.4 dB | +5.0 to +10.4 / -7.2 to -17.6 dB | +4.6 to +7.3 / -5.8 to -12.2 dB |
| Residual 6-16 kHz | -34.7 to -42.4 dB | -39.1 to -66.9 dB | -33.0 to -44.1 dB |
| Idle, closed throttle (675 RPM): residual, 6-16 kHz | -11.3, -35.3 dB | -5.9, -66.7 dB | -6.1, -33.9 dB |

### What the images show

- `sweep-wot` spectrograms: the v8 now has the fixed bright band at 340 to 420 Hz of engine-sim, and its harmonic lines stop near 1.4 kHz ([v8 after](../../out/phase4c/after/v8/sweep-wot/spectrogram.png) against [engine-sim v8](../../out/analysis/engine-sim/v8/sweep-wot/spectrogram.png)). The fixed lines of the expansion chamber from 2 to 6 kHz are gone in every profile. Diffuse noise fills 2 to 15 kHz, as in engine-sim.
- The inline4 harmonic lines still reach about 2 kHz above 6000 RPM, where engine-sim stops near 1.4 kHz ([inline4 after](../../out/phase4c/after/inline4/sweep-wot/spectrogram.png) against [engine-sim inline4](../../out/analysis/engine-sim/inline4/sweep-wot/spectrogram.png)).
- The flat12 noise above 2 kHz is weaker than in engine-sim, and it has none of the vertical stripes at the firing rate ([flat12 after](../../out/phase4c/after/flat12/sweep-wot/spectrogram.png) against [engine-sim flat12](../../out/analysis/engine-sim/flat12/sweep-wot/spectrogram.png)).
- The v8 sweep waveform has a flat peak envelope from the first window to the last, as engine-sim has ([v8 after](../../out/phase4c/after/v8/sweep-wot/waveform.png)).
- The inline4 histogram has a broad flat top with sharp edges on both sides, as engine-sim has ([inline4 after](../../out/phase4c/after/inline4/sweep-wot/histogram.png)).
- At the closed-throttle idle, the high-frequency noise of engine-sim is now there ([v8 holds-t000 after](../../out/phase4c/after/v8/holds-t000/spectrogram.png)).

### Pops

`TestPopColour` renders draw 4 of the 7 calibration pops of every kind on every path of the tuned profiles through the pop bank and through the EngineLab stages of `a73b874` (`testdata/legacy/`), with the same pressure jump and crack. The pops correlate by 0.9981 to 0.9999 at the best lag of up to 1 ms (0 to 2 samples), and their level at unit gain differs by -0.13 to +0.45 dB. The calibration sets the level of each kind over the engine from the same pops, so the levels over the engine stay at their targets (`TestPopLevelOverEngine`).

### Cost

`BenchmarkStates`, 5 runs each, with the test binaries of `a73b874` and of the working tree run one after the other on the AMD Ryzen 9 7940HS (`out/phase4c/bench-before.txt` and `bench-after.txt`). Per cent of real time per 256-frame block:

| Profile | Before | After | Change |
| --- | --- | --- | --- |
| flat12 | 0.25 to 0.37 | 0.27 to 0.41 | +4 to +10 per cent |
| flat6 | 0.24 to 0.25 | 0.25 to 0.27 | +4 to +6 per cent |
| inline4 | 0.18 to 0.19 | 0.24 to 0.25 | +28 to +35 per cent |
| v8 | 0.25 to 0.27 | 0.27 to 0.28 | +0 to +13 per cent |

The sum over the 16 states rises by 11 per cent. Every state stays under 0.35 per cent except the flat12 limiter hold (0.41, against 0.37 before), where the ignition cut bangs run the pop stages. inline4 rises most: its one path had the cheapest EngineLab stages, and its formant bank has 10 sections. The per-sample cost is the formant bank (41 to 51 multiply-adds), the input jitter (about 12 operations per path) and the 10 Hz DC removal of the pressure term, against the EngineLab stages, the steepening and the jet noise that they replace.

### Measured pulse (phase 4d)

"Before" is `2ca2a0d` (the "After" of the tables above). "After" is the working tree with these changes:

- The pulse is the engine-sim runner pressure (measured), in place of the computed envelope and the squared attack. `tools/fit --pulse` fits one pulse per firing, scaled by the strength of the firing, plus a constant, to the sum of the runner pressures of each exhaust system in the per-step CSV of `out/analysis/engine-sim/<profile>/full/holds-t100.csv` (flat6 also `holds-t050.csv`), by least squares with a ridge term that grows with the time from the onset. The onsets are at the spark angles, moved to the crossing of half of the own extreme of each runner pulse. The fit explains the sums within -18 to -41 dB. Reports: `out/phase4d/pulse/<profile>.md`.
- The table is one row of 128 values per hold RPM (`internal/model/pulses/<profile>.txt`, `[pulse] table`), blended between rows in steps of 1/16 and kept by each firing. A single table with a length that follows the RPM to a power holds the measured pulses within -1 to -16 dB only, and the rows hold them within -40 to -62 dB at the holds and 0 to -28 dB between them (left out), so the rows stay.
- The inline4 collector overlap is 0. The cam crossfade sets the weight between the inline4 rows at 5000 and 6000 RPM.
- The formant banks are fitted with `--tolerance 1.5`: 4, 4, 7 and 7 sections for flat12, flat6, inline4 and v8, in place of 9, 8, 10 and 10 (weighted error 1.48, 1.44, 1.31 and 1.42 dB, against 0.90 to 0.98 dB).
- `leveller_db` puts the closed-throttle idle 3 dB below the knee again (-2.2, 23.5, 10.9 and 13.0 dB), and `output_db` holds the sweep at -16 LUFS.

Renders, gap tables and analyses: `out/phase4d/after/<profile>/` (`gap-<scenario>.md` and `<scenario>/`). Ranges are over the windows of `holds-t100` unless the row names another scenario.

#### Answer

The measured pulse closes the inline4 gaps: the loudness band of the sweep falls from 5.8 to 4.1 LU (engine-sim 2.3 LU), the peak band from 2.9 to 1.5 dB, and the crest factor at the top holds from 12.0 to 8.9 dB (engine-sim 8.3 dB). The inline4 sweep spectrogram now has the harmonic pattern of engine-sim, with the step at the cam switch ([inline4 after](../../out/phase4d/after/inline4/sweep-wot/spectrogram.png) against [engine-sim inline4](../../out/analysis/engine-sim/inline4/sweep-wot/spectrogram.png)). The v8 strongest order now sits at 180 to 400 Hz at every hold, as in engine-sim. Four gaps remain:

1. The loudness band of the sweep is 2.7 to 5.2 LU against 1.3 to 3.0 LU. The closed-throttle idle is 3 dB below the knee, and the overrun level is close to the full-throttle level (both approved), so the full-throttle sweep below 1500 RPM is below the knee too. engine-sim has its full-throttle idle 11 to 13 dB over its closed-throttle idle. The flat6 band rises from 3.1 to 5.2 LU: its measured pulse below 2000 RPM is a long, smooth dip.
2. The v8 and flat12 centroids are still up to 400 to 500 Hz high, at the v8 hold at 5000 RPM and the flat12 holds at 4000 and 5000 RPM. With `input_jitter` 0, the flat12 centroid at 4000 RPM falls to 401 Hz against 1251 Hz in engine-sim. The mean pulse leaves out the cycle-to-cycle change of the engine-sim pressure, which the derivative of engine-sim lifts above 1 kHz, so the jitter noise of snarl carries the treble alone.
3. The residual is still 5 to 13 dB above engine-sim at high RPM for flat6, inline4 and v8. The slow `[pulse] jitter` (approved) gives about 6 dB of it.
4. The closed-throttle idle has more noise than engine-sim (residual -1.2 dB for v8 against -11.3 dB): its long measured pulse leaves little harmonic power at the formants.

#### flat12

| Measure | engine-sim | Before | After |
| --- | --- | --- | --- |
| Loudness band, `sweep-wot` | 1.3 LU | 2.0 LU | 2.7 LU |
| Peak band, `sweep-wot` | 1.3 dB | 1.5 dB | 2.7 dB |
| Crest factor | 7.0 to 8.3 dB | 6.9 to 9.6 dB | 6.3 to 9.0 dB |
| Centroid | 1251 to 1525 Hz | 1426 to 1856 Hz | 1363 to 1907 Hz |
| Power above 2.84 kHz | -25.6 to -30.3 dB | -24.4 to -29.6 dB | -23.2 to -30.0 dB |
| Strongest order, top hold (6800 RPM) | 2 (227 Hz) | 3 (340 Hz) | 3 (340 Hz) |
| Strongest order, all holds | 100 to 400 Hz | 100 to 500 Hz | 100 to 400 Hz |
| Residual | -6.0 to -12.8 dB | -15.3 to -23.7 dB | -15.7 to -23.9 dB |
| AM peak / trough | +3.0 to +4.7 / -5.8 to -12.5 dB | +5.3 to +8.5 / -6.6 to -10.4 dB | +3.9 to +6.6 / -5.0 to -10.1 dB |
| Residual 6-16 kHz | -29.0 to -33.8 dB | -28.0 to -33.2 dB | -26.5 to -33.5 dB |
| Idle, closed throttle (1000 RPM): residual, 6-16 kHz | -3.2, -30.5 dB | -2.8, -32.9 dB | -2.6, -33.8 dB |

#### flat6

| Measure | engine-sim | Before | After |
| --- | --- | --- | --- |
| Loudness band, `sweep-wot` | 3.0 LU | 3.1 LU | 5.2 LU |
| Peak band, `sweep-wot` | 2.3 dB | 1.3 dB | 1.8 dB |
| Crest factor | 3.6 to 10.0 dB | 5.2 to 10.3 dB | 4.6 to 10.9 dB |
| Centroid | 846 to 1168 Hz | 733 to 1136 Hz | 739 to 1106 Hz |
| Power above 2.84 kHz | -26.5 to -39.0 dB | -29.4 to -38.2 dB | -33.0 to -39.6 dB |
| Strongest order, top hold (6600 RPM) | 3 (330 Hz) | 3 (330 Hz) | 3 (330 Hz) |
| Strongest order, all holds | 240 to 400 Hz | 150 to 330 Hz | 150 to 330 Hz |
| Residual | -16.7 to -32.4 dB | -19.5 to -27.0 dB | -20.0 to -26.2 dB |
| AM peak / trough | +2.5 to +5.4 / -2.8 to -6.5 dB | +3.2 to +7.0 / -5.1 to -10.3 dB | +4.0 to +7.7 / -7.6 to -11.8 dB |
| Residual 6-16 kHz | -33.1 to -45.9 dB | -36.5 to -45.4 dB | -41.3 to -47.7 dB |
| Idle, closed throttle (800 RPM): residual, 6-16 kHz | -16.1, -37.8 dB | -6.0, -36.5 dB | -3.7, -41.1 dB |

#### inline4

| Measure | engine-sim | Before | After |
| --- | --- | --- | --- |
| Loudness band, `sweep-wot` | 2.3 LU | 5.8 LU | 4.1 LU |
| Peak band, `sweep-wot` | 2.3 dB | 2.9 dB | 1.5 dB |
| Crest factor | 3.8 to 8.3 dB | 2.8 to 12.0 dB | 3.7 to 8.9 dB |
| Centroid | 337 to 1088 Hz | 321 to 1575 Hz | 290 to 1499 Hz |
| Power above 2.84 kHz | -34.2 to -51.7 dB | -30.4 to -52.6 dB | -31.2 to -54.6 dB |
| Strongest order, top hold (8400 RPM) | 4 (560 Hz) | 4 (560 Hz) | 2 (280 Hz) |
| Strongest order, all holds | 133 to 560 Hz | 100 to 560 Hz | 100 to 533 Hz |
| Residual | -23.9 to -41.8 dB | -15.5 to -34.6 dB | -17.1 to -35.8 dB |
| AM peak / trough | +4.3 to +11.2 / -5.9 to -18.3 dB | +4.6 to +9.9 / -7.8 to -20.6 dB | +4.1 to +8.9 / -6.7 to -16.6 dB |
| Residual 6-16 kHz | -37.4 to -55.0 dB | -33.7 to -55.7 dB | -34.3 to -57.9 dB |
| Idle, closed throttle (800 RPM): residual, 6-16 kHz | -23.0, -52.9 dB | -26.6, -53.1 dB | -29.9, -56.7 dB |

#### v8

| Measure | engine-sim | Before | After |
| --- | --- | --- | --- |
| Loudness band, `sweep-wot` | 1.7 LU | 3.7 LU | 3.9 LU |
| Peak band, `sweep-wot` | 2.2 dB | 1.5 dB | 1.6 dB |
| Crest factor | 5.6 to 8.5 dB | 5.8 to 9.5 dB | 5.3 to 12.7 dB (12.7 dB at the 675 RPM hold, 9.2 dB and less from 2000 RPM) |
| Centroid | 868 to 1049 Hz | 1066 to 1464 Hz | 990 to 1460 Hz |
| Power above 2.84 kHz | -28.2 to -36.3 dB | -26.8 to -37.8 dB | -26.2 to -37.1 dB |
| Strongest order, top hold (6000 RPM) | 4 (400 Hz) | 4 (400 Hz) | 4 (400 Hz) |
| Strongest order, all holds | 180 to 417 Hz | 45 to 400 Hz | 180 to 400 Hz |
| Residual | -16.7 to -30.3 dB | -15.9 to -21.6 dB | -16.4 to -22.3 dB |
| AM peak / trough | +2.4 to +5.6 / -3.0 to -9.4 dB | +4.6 to +7.3 / -5.8 to -12.2 dB | +4.7 to +8.6 / -8.3 to -21.1 dB |
| Residual 6-16 kHz | -34.7 to -42.4 dB | -33.0 to -44.1 dB | -32.5 to -43.6 dB |
| Idle, closed throttle (675 RPM): residual, 6-16 kHz | -11.3, -35.3 dB | -6.1, -33.9 dB | -1.2, -36.1 dB |

#### What the images show

- inline4 `sweep-wot`: the harmonic lines and their levels follow engine-sim from 1 to 8 kHz, with the step at the cam switch near 5.2 s in both. Above the cam switch, the snarl lines from 500 Hz to 1.4 kHz are a little stronger ([inline4 after](../../out/phase4d/after/inline4/sweep-wot/spectrogram.png)). The waveform has a flat peak envelope from the first window to the last ([inline4 waveform](../../out/phase4d/after/inline4/sweep-wot/waveform.png)).
- v8 `sweep-wot`: the bright band at 340 to 420 Hz is there, but engine-sim has more power from 50 to 300 Hz all through the sweep, and the snarl lines from 130 to 400 Hz move with the RPM more clearly ([v8 after](../../out/phase4d/after/v8/sweep-wot/spectrogram.png) against [engine-sim v8](../../out/analysis/engine-sim/v8/sweep-wot/spectrogram.png)).
- flat12 `sweep-wot`: the band at 200 to 400 Hz follows engine-sim, and the noise from 2 to 6 kHz is brighter from 3.6 to 5.5 s ([flat12 after](../../out/phase4d/after/flat12/sweep-wot/spectrogram.png)).

#### Pops, limiter and overrun

The pops pass the same tests as before (`TestPopColour`, `TestPopLevelOverEngine`, `TestHissPops`, `TestPopsFire`). The limiter bounce keeps its rate, dip, cut, notch and bark within the engine-sim bounds (`TestLimiterBounce`). The flat12 ignition cut bangs lift the band above 3 kHz by 3.0 dB over full throttle, against 1.2 dB before, so its bound is 3.5 dB. The overrun stays within 3 dB of full throttle (`TestOverrunLevel`), and the final gear hold is unchanged.

#### Cost

`BenchmarkStates` and `BenchmarkRender` with a fixed 20000 blocks (107 s of audio) per run, 8 runs each, with the test binaries of `2ca2a0d` and of the working tree run in turn, two rounds of 4 (`out/phase4d/final-head-*.txt` and `final-new-*.txt`). A fixed number of blocks gives every run the same limiter bounces and bangs, which the default benchmark time does not. Median per cent of real time per 256-frame block on the AMD Ryzen 9 7940HS:

| Profile | State | Before | After | Change |
| --- | --- | --- | --- | --- |
| flat12 | wot4000 | 0.288 | 0.221 | -23 per cent |
| flat12 | idle | 0.281 | 0.205 | -27 per cent |
| flat12 | overrun | 0.289 | 0.219 | -24 per cent |
| flat12 | limiter | 0.423 | 0.287 (highest run 0.316) | -32 per cent |
| inline4 | wot4000 | 0.255 | 0.204 | -20 per cent |
| inline4 | idle | 0.233 | 0.202 | -13 per cent |
| inline4 | overrun | 0.243 | 0.210 | -14 per cent |
| inline4 | limiter | 0.243 | 0.212 | -13 per cent |
| flat6 | wot4000 | 0.266 | 0.220 | -17 per cent |
| flat6 | idle | 0.248 | 0.203 | -18 per cent |
| flat6 | overrun | 0.258 | 0.211 | -18 per cent |
| flat6 | limiter | 0.259 | 0.228 | -12 per cent |
| v8 | wot4000 | 0.283 | 0.248 | -12 per cent |
| v8 | idle | 0.279 | 0.226 | -19 per cent |
| v8 | overrun | 0.281 | 0.229 | -19 per cent |
| v8 | limiter | 0.288 | 0.236 | -18 per cent |
| flat12 | `BenchmarkRender` | 0.296 | 0.224 | -24 per cent |
| flat6 | `BenchmarkRender` | 0.275 | 0.206 | -25 per cent |
| inline4 | `BenchmarkRender` | 0.257 | 0.203 | -21 per cent |
| v8 | `BenchmarkRender` | 0.292 | 0.234 | -20 per cent |

The sum over the 16 states falls by 19 per cent, from 4.42 to 3.56. Every median is at or below 0.30 per cent. Scaled as before (0.30 per cent on this PC was 2.07 per cent of a OnePlus 3T slow core, and the Switch estimate 3.2 to 4.2 per cent), the flat12 limiter at 0.287 per cent is about 2.0 per cent of the slow core and 3.1 to 4.0 per cent of the Switch (estimate). Each saving, measured A against B on the state that it changes:

| Change | Saving |
| --- | --- |
| The table pulse alone, with two voices per cylinder for the longer pulses | +12 to +18 per cent on flat12 and v8 at first: the measured pulses last longer than the envelope, so more voices sound |
| A list of the active voices, so the pulse loop visits no idle voice | flat12 wot4000 0.317 to 0.268, v8 wot4000 0.288 to 0.261 |
| Subnormal values of the decaying pop levels snap to zero at 1e-20 | flat12 limiter 0.465 to 0.429. In the profile, the pop tick fell from 17 to 7 per cent and the one-poles from 13 to 4 per cent |
| Each voice runs over the chunk in one loop, from its firing sample | flat12 limiter 0.43 to 0.38 |
| Formant bank tolerance 1.5 dB (5, 4, 3 and 3 fewer sections, 25, 20, 15 and 15 fewer multiply-adds) | flat12 limiter: the formant bank fell from 14 to 3 per cent of the profile |
| No output low pass at 44.4 kHz and up | flat12 limiter 0.338 to 0.323 |
| One jitter offset for every path, no engine DC block after a bank high pass, one bounds check in the voice loop | flat12 limiter 0.318 to 0.298 (fixed blocks) |
| One pop bank on the sum of the paths, the pop stages stop 100 dB down, a rung-out pop slot skips its stages | within the run-to-run spread (2 per cent or less) |
| Tried and dropped: the stages one at a time over the chunk, and a binomial in place of the jitter low pass | both slower, by about 20 and 10 per cent on the flat12 limiter |
