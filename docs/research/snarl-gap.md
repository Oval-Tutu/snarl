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
