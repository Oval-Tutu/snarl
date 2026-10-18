# Muffler crackle pack as the pop reference

Status: analysis complete. Date: 2026-10-17. Worker report for scoping decision 3 (missing pop reference, [scoping.md](../scoping.md) section 11).

Source pack: `Muffler Crackle Sounds` by Skril Studio (Unity Asset Store publisher 29907, GameDev Market handle `slaczky`), supplied as `/home/martin/Development/Oval-Tutu/FH-audio/muffler-crackle-sounds-slaczky/db7f672823a1d08639bf8da8a96b793fdc0839e7.zip` (85,794,264 bytes). Extracted read-only into `/tmp/snarl-refs/crackles/`. Every number below comes from ffmpeg 8.1.2 (`ffprobe`, `astats`, `ebur128`, `aspectralstats`, band-pass `astats`) or from a WAV header, with the detector scripts in `/tmp/snarl-refs/scripts/` (`measure.sh`, `truepeak.sh`, `decode.sh`, `decode_hp.sh`, `analyze.py`, `cracks.py`, `report.py`). Per-file results are in `/tmp/snarl-refs/out/analysis/metrics.csv`, `cracks.csv` and `bursts.json`.

## 1. Answer

The pack is usable as the pop reference and as a shipped one-shot layer, but not as a redistributed asset. The licence (section 2) allows unlimited personal and commercial games and forbids resale or redistribution of the package or any single file. That means the WAVs can ship inside the FiveHorizons build as game content, but they cannot be committed to the public snarl repository or published in any form that lets a third party extract them as a sound pack. For snarl, treat the pack as a private reference set and as an optional game-side sample layer that the game supplies to snarl at run time.

The measurements change decision 3 in two ways:

1. A recorded single pop in this pack is a 30 to 70 ms event, not a 2 to 4 ms impulse. The compact impulse that EngineLab measured matches only the high-frequency crack at the onset (rise 0.8 to 1.9 ms, decay to -20 dB in 12 to 22 ms). The body is a 120 to 500 Hz resonance that carries about 60 per cent of the energy and rings for 15 to 60 ms. The EngineLab band split (15 / 65 / 20 / 0.4 per cent) is close to the pack's isolated pops (31 / 60 / 8 / 1 per cent), so EngineLab's spectral target was right and its duration target was short by one order of magnitude. The procedural pop needs a resonant body behind the crack, which the exhaust path delay line can provide if the injected pressure jump is wide enough.
2. The pack's pops are much denser than EngineLab's trigger logic. Onset intervals have a median of 174 ms (isolated pops) to 272 ms (dense crackle), against EngineLab's 300 to 700 ms. Only 20 per cent of the pack's intervals fall in the 300 to 700 ms window. The pack's duty (time inside a pop event over active time) is 0.24 to 0.47 for isolated pops and 0.66 to 0.95 for dense crackle, against EngineLab's 8 per cent fuel-slug duty. The two duty figures measure different things (acoustic event length against fuel-slug length), so the trigger interval is the figure to change: start at 120 to 300 ms between pops inside a crackle run, and use 300 to 700 ms only between runs.

## 2. Licence and attribution

The only licence text is page 3 of `MufflerCrackleSounds/Unity Engine/Muffler Sound Pack/Documentation/Muffler Crackle Sounds.pdf`. Quoted in full:

> You can use this asset for unlimited games.
> You can use this asset for personal and commercial use.
> Cannot be used in real products! (like sound generator for cars, toys, etc!)
> Contact for pricing if you want to use these sounds for real physical products like vehicles, toys, etc.
> You can't resell or redistribute the package or any single file from the package in any way!

Attribution from the same PDF: "Made by: Skril Studio", contact `info@skrilstudio.com`, website `http://skrilstudio.com`. The credits page thanks unnamed friends and customers for the vehicle recordings, so the original engines are unknown. The Unity short-crackle WAVs carry an Adobe XMP chunk (`_PMX`, 5,042 bytes) that names `Adobe Audition CC 2019.0 (Windows)` as the editor. The GameDev Market listing (<https://www.gamedevmarket.net/asset/muffler-crackle-sounds>) adds "Created in Ableton, Adobe Audition, Fmod", version V2.1.1 of 2024-12-01, and "Allows usage with AI? No". The Unity Asset Store listing (<https://assetstore.unity.com/packages/audio/sound-fx/transportation/muffler-crackle-sounds-114010>) prices the pack at $9.99 and describes it as "game ready one shoot sound clips", 44,100 Hz stereo.

Position for snarl:

- Commercial game use: permitted. The WAVs may ship inside FiveHorizons.
- Redistribution: forbidden. The WAVs must not enter the snarl repository, a public test fixture, a release archive, or a Nix store path that is pushed to a public cache. Keep them in `FH-audio` or the game's private asset tree.
- "Real products": not relevant to a game, but the clause rules out any hardware demo that plays them.
- AI training: the market listing forbids it. Measurement, listening and tuning by ear are not training. Do not use the files as training data for any learned model in snarl.
- Reference-only use: permitted without restriction, because measurement is use inside the project. Metrics and derived numbers in this document are not the files.

Confidence: high on the licence text, which is in hand. Medium on the Unity Asset Store default EULA interaction, which I did not read; the PDF terms are stricter than the store default on redistribution, so following the PDF is safe either way.

## 3. Pack contents

| Item | Measured fact |
| --- | --- |
| Archive | 621 files, 240 WAVs, 85.8 MB zipped |
| Unique audio | 120 WAVs in `Unity Engine/Muffler Sound Pack/Assets/Audio/Muffler/` (35 MB). `Unreal Engine/Content/MufflerCrackleSounds/Wavs/` repeats them: the 60 long files are byte-identical (md5), the 60 short files are the same audio re-exported without the XMP chunk at +0.3 dB (peak 0.00 dBFS against -0.30 dBFS, null residual -29.4 dBFS peak on `crackle_1.wav`) |
| Format | All 120: `pcm_s16le`, 44,100 Hz, 2 channels, 16 bit (ffprobe). No 48 kHz or 24-bit variant exists |
| Short exterior | `Short_Cracles/crackle_1.wav` to `crackle_30.wav`, every file exactly 0.637 s (28,107 frames). 19.11 s total |
| Short interior | `Short_Cracles/Interior/int_crackle_1.wav` to `int_crackle_30.wav`, same durations as the exterior set |
| Long exterior | `Long_Cracles/crackle_long_1.wav` to `crackle_long_30.wav`, 1.780 to 4.942 s, mean 2.696 s, 80.88 s total |
| Long interior | `Long_Cracles/Interior/int_crackle_long_1.wav` to `int_crackle_long_30.wav`, same durations as the exterior set |
| Interior files | Low-passed copies of the exterior files: weighted spectral centroid 190 to 488 Hz against 1,031 to 2,539 Hz (short), first burst onsets within 0.0 to 2.9 ms of the exterior file (30 of 30 long pairs), 0.0 per cent of energy above 2 kHz in all 60 |
| Stereo | Near mono. Side (L-R) RMS is 19 to 22 dB below mid on the exterior files tested (`crackle_1.wav` -33.8 against -14.9 dBFS, `crackle_long_1.wav` -30.8 against -8.6 dBFS). Interior long files have more side content (`int_crackle_long_1.wav` -18.7 against -8.8 dBFS) |
| Level | Every file is peak-normalised to -0.30 dBFS sample peak (`astats` Peak level dB -0.299613, all 120). Oversampled true peak exceeds 0 dBTP on 23 exterior files, up to +1.0 dBTP on `crackle_long_9.wav`; all 60 interior files stay at -0.3 dBTP |
| Silence tails | 10 of the 30 short exterior files end in digital silence (noise floor `-inf`, 1 ms RMS at -94 dBFS) after a hard cut at 0.33 to 0.55 s, for example `crackle_3.wav` (active 0.334 s) and `crackle_1.wav` (0.347 s). The remaining 20 run to the end of the clip |
| Near-duplicates | `crackle_9.wav` and `crackle_18.wav` share every metric (LUFS -13.3, centroid 1,262 Hz, crest 13.29 dB, 26 cracks). `crackle_10.wav` and `crackle_19.wav` differ by one crack. No pair is byte-identical after decoding |
| Loudness range | `ebur128` LRA is 0.0 LU on 108 files; the clips are too short for the 3 s short-term window. Integrated loudness is still valid per BS.1770 gating |

Two production styles exist inside the long set, and the file numbers separate them:

| Group | Files | Integrated loudness | Crest | Weighted centroid | Energy 20 to 120 Hz | Energy 120 to 500 Hz | Character |
| --- | --- | --- | --- | --- | --- | --- | --- |
| A, dense crackle | `crackle_long_1` to `_20` | -12.6 to -7.7 LUFS, median -9.3 | 8.1 to 12.8 dB | 1,798 to 2,828 Hz | 64 to 87 per cent | 9 to 25 per cent | Long overlapping bursts, 51 to 2,057 ms, with a heavy sub-120 Hz body. True peak +0.3 to +1.0 dBTP |
| B, isolated pops | `crackle_long_21` to `_30` | -19.1 to -17.2 LUFS, median -17.9 | 19.0 to 20.6 dB | 2,645 to 3,270 Hz | 27 to 36 per cent | 55 to 62 per cent | Separate 28 to 105 ms pops with gaps, little sub-120 Hz. True peak -0.3 to 0.0 dBTP |

The short exterior set is one 154 to 528 ms crackle burst per file (23 files) or two bursts (7 files), with 68 to 87 per cent of energy below 120 Hz. The short set is the same material as group A, cut to one burst.

## 4. Method

Commands per file, from `measure.sh`, all after a mono downmix `pan=mono|c0=0.5*c0+0.5*c1` unless noted:

- Header: `ffprobe -show_entries stream=codec_name,sample_rate,channels,bits_per_sample,duration`.
- Level: `astats=measure_perchannel=none:measure_overall=Peak_level+RMS_level+DC_offset+Noise_floor` on the stereo file. Crest factor in dB is Peak level minus RMS level, because `astats` prints the crest ratio per channel only.
- Loudness: `ebur128=peak=true` on the stereo file. `I` is integrated loudness, `Peak` under "True peak" is the oversampled true peak.
- Band energy: four cascaded second-order filters, `highpass=f=LO:poles=2,highpass=f=LO:poles=2,lowpass=f=HI:poles=2,lowpass=f=HI:poles=2`, then `astats` RMS, for 20 to 120, 120 to 500, 500 to 2,000, 2,000 to 8,000 and 8,000 to 20,000 Hz. Percentages are linear power shares of the five bands. The 24 dB per octave skirts leak a little between neighbours, so treat the split as plus or minus a few points.
- Spectral statistics: `asetnsamples=n=2048,astats=metadata=1:reset=1,aspectralstats=win_size=2048:measure=centroid+rolloff+flatness+spread,ametadata=mode=print`. The weighted centroid is the per-frame centroid weighted by frame power, frames above -60 dBFS only. The median centroid is also reported, because loud low-frequency frames pull the weighted figure down.
- Bursts (pop events): decoded to mono float32 (`decode.sh`), 1 ms RMS envelope at 0.5 ms hop; a burst is a span where the envelope exceeds the file's envelope peak minus 30 dB, gaps under 30 ms merged, spans under 10 ms dropped. Attack is the 10 to 90 per cent rise time of the running peak of `|x|` from burst start to envelope peak. Decay is the time from the envelope peak until the envelope stays below the peak minus 20 dB. Onset interval is start to start. Duty is burst time over active time, where active time runs from the first to the last 1 ms frame above -50 dBFS.
- Cracks (high-frequency transients): decoded through `highpass=f=1000:poles=2` twice (`decode_hp.sh`), 0.25 ms RMS envelope, local maxima at least 3 ms apart and above the file's high-frequency peak minus 24 dB. Rise is the 10 to 90 per cent time of the running peak of `|x|` over the 4 ms before each maximum. Decay is the first crossing of the maximum minus 20 dB. The 3 ms separation floor saturates on dense crackle (10th percentile of the crack interval is 3.0 ms in every category), so crack counts and rates are lower bounds.
- Images: `showspectrumpic=s=1280x480:scale=log:fscale=log` and `showwavespic` in `/tmp/snarl-refs/out/img/`.

## 5. Inventory

Exterior files with the interior partner's loudness and centroid in the same row. Format is the same for all 120 (`pcm_s16le`, 44,100 Hz, stereo, 16 bit) and sample peak is -0.30 dBFS on all 120, so those columns are collapsed. Short files 1 to 30 are category short; long files 1 to 20 are category long, dense crackle (group A); long files 21 to 30 are category long, isolated pops (group B). Onset interval is the median start-to-start interval between detected bursts in the exterior file.

| File | Duration s | Active s | LUFS ext | LUFS int | True peak ext dBTP | True peak int dBTP | Crest ext dB | Centroid ext Hz | Centroid int Hz | Bursts | Onset interval median ms |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `crackle_1.wav` | 0.637 | 0.35 | -14.4 | -12.9 | -0.1 | -0.3 | 14.5 | 1317 | 221 | 1 | - |
| `crackle_2.wav` | 0.637 | 0.43 | -13.6 | -12.6 | -0.3 | -0.3 | 13.8 | 1035 | 205 | 1 | - |
| `crackle_3.wav` | 0.637 | 0.33 | -17.4 | -15.0 | -0.2 | -0.3 | 17.5 | 1850 | 278 | 1 | - |
| `crackle_4.wav` | 0.637 | 0.64 | -15.5 | -14.1 | -0.0 | -0.3 | 14.4 | 2065 | 262 | 1 | - |
| `crackle_5.wav` | 0.637 | 0.64 | -15.2 | -12.7 | -0.3 | -0.3 | 14.4 | 1493 | 211 | 1 | - |
| `crackle_6.wav` | 0.637 | 0.51 | -11.8 | -12.3 | 0.3 | -0.3 | 13.1 | 2302 | 258 | 1 | - |
| `crackle_7.wav` | 0.637 | 0.50 | -11.7 | -10.8 | -0.0 | -0.3 | 13.3 | 2539 | 238 | 1 | - |
| `crackle_8.wav` | 0.637 | 0.64 | -12.0 | -12.4 | 0.2 | -0.3 | 13.1 | 2025 | 241 | 1 | - |
| `crackle_9.wav` | 0.637 | 0.64 | -13.3 | -12.3 | -0.2 | -0.3 | 13.3 | 1262 | 200 | 1 | - |
| `crackle_10.wav` | 0.637 | 0.64 | -12.2 | -10.1 | -0.2 | -0.3 | 11.7 | 1475 | 243 | 2 | 320.3 |
| `crackle_11.wav` | 0.637 | 0.64 | -12.5 | -10.2 | -0.2 | -0.3 | 11.7 | 1527 | 217 | 2 | 479.4 |
| `crackle_12.wav` | 0.637 | 0.64 | -13.4 | -11.0 | -0.0 | -0.3 | 13.3 | 1512 | 206 | 1 | - |
| `crackle_13.wav` | 0.637 | 0.64 | -13.5 | -14.1 | -0.0 | -0.3 | 13.2 | 1373 | 293 | 2 | 480.9 |
| `crackle_14.wav` | 0.637 | 0.64 | -14.8 | -16.6 | -0.2 | -0.3 | 14.2 | 1215 | 286 | 2 | 480.4 |
| `crackle_15.wav` | 0.637 | 0.64 | -13.1 | -13.8 | -0.1 | -0.3 | 12.2 | 1331 | 249 | 2 | 479.9 |
| `crackle_16.wav` | 0.637 | 0.55 | -12.7 | -9.9 | -0.2 | -0.3 | 12.7 | 1031 | 200 | 1 | - |
| `crackle_17.wav` | 0.637 | 0.64 | -13.3 | -12.9 | -0.0 | -0.3 | 12.5 | 1626 | 488 | 2 | 479.4 |
| `crackle_18.wav` | 0.637 | 0.64 | -13.3 | -12.3 | -0.2 | -0.3 | 13.3 | 1262 | 200 | 1 | - |
| `crackle_19.wav` | 0.637 | 0.64 | -12.2 | -10.1 | -0.2 | -0.3 | 11.7 | 1474 | 243 | 2 | 320.3 |
| `crackle_20.wav` | 0.637 | 0.64 | -13.1 | -10.9 | -0.2 | -0.3 | 12.8 | 1396 | 190 | 1 | - |
| `crackle_21.wav` | 0.637 | 0.35 | -16.4 | -14.3 | -0.3 | -0.3 | 16.7 | 1795 | 212 | 1 | - |
| `crackle_22.wav` | 0.637 | 0.50 | -12.5 | -10.9 | -0.2 | -0.3 | 13.1 | 1757 | 220 | 1 | - |
| `crackle_23.wav` | 0.637 | 0.63 | -13.9 | -15.2 | -0.3 | -0.3 | 13.8 | 1457 | 213 | 1 | - |
| `crackle_24.wav` | 0.637 | 0.46 | -12.2 | -12.3 | -0.3 | -0.3 | 12.9 | 1322 | 208 | 1 | - |
| `crackle_25.wav` | 0.637 | 0.50 | -12.6 | -13.8 | -0.3 | -0.3 | 12.9 | 1226 | 213 | 1 | - |
| `crackle_26.wav` | 0.637 | 0.64 | -13.6 | -13.6 | -0.0 | -0.3 | 13.8 | 1718 | 191 | 1 | - |
| `crackle_27.wav` | 0.637 | 0.64 | -15.2 | -12.1 | -0.2 | -0.3 | 15.3 | 1821 | 215 | 1 | - |
| `crackle_28.wav` | 0.637 | 0.64 | -13.0 | -12.7 | -0.0 | -0.3 | 13.4 | 1666 | 207 | 1 | - |
| `crackle_29.wav` | 0.637 | 0.64 | -13.3 | -13.5 | -0.2 | -0.3 | 14.1 | 1940 | 231 | 1 | - |
| `crackle_30.wav` | 0.637 | 0.64 | -12.2 | -10.7 | 0.2 | -0.3 | 12.6 | 1434 | 212 | 1 | - |
| `crackle_long_1.wav` | 2.258 | 2.26 | -7.7 | -8.4 | 0.9 | -0.3 | 8.3 | 2417 | 331 | 9 | 200.5 |
| `crackle_long_2.wav` | 1.780 | 1.78 | -9.2 | -10.5 | 0.3 | -0.3 | 10.1 | 2828 | 403 | 3 | 864.0 |
| `crackle_long_3.wav` | 2.059 | 2.06 | -8.5 | -10.2 | 0.7 | -0.3 | 9.9 | 2620 | 356 | 1 | - |
| `crackle_long_4.wav` | 2.656 | 2.66 | -9.2 | -10.7 | 0.3 | -0.3 | 9.8 | 2034 | 302 | 6 | 393.6 |
| `crackle_long_5.wav` | 3.668 | 3.67 | -12.6 | -14.0 | 0.6 | -0.3 | 12.8 | 2145 | 313 | 10 | 388.6 |
| `crackle_long_6.wav` | 2.656 | 2.66 | -10.3 | -11.7 | 0.8 | -0.3 | 10.4 | 2374 | 319 | 8 | 276.3 |
| `crackle_long_7.wav` | 2.723 | 2.72 | -8.2 | -9.1 | 0.9 | -0.3 | 8.1 | 1816 | 269 | 5 | 437.7 |
| `crackle_long_8.wav` | 2.191 | 2.19 | -8.7 | -9.6 | 0.4 | -0.3 | 8.6 | 1803 | 276 | 6 | 391.6 |
| `crackle_long_9.wav` | 2.521 | 2.52 | -8.8 | -9.9 | 1.0 | -0.3 | 8.8 | 1918 | 320 | 4 | 570.2 |
| `crackle_long_10.wav` | 2.922 | 2.92 | -9.5 | -10.8 | 0.3 | -0.3 | 9.4 | 1803 | 298 | 6 | 517.9 |
| `crackle_long_11.wav` | 3.852 | 3.85 | -10.5 | -11.8 | 0.6 | -0.3 | 10.4 | 2243 | 283 | 11 | 274.6 |
| `crackle_long_12.wav` | 2.855 | 2.85 | -9.1 | -10.3 | 0.4 | -0.3 | 9.2 | 1938 | 275 | 7 | 273.6 |
| `crackle_long_13.wav` | 3.121 | 3.12 | -9.9 | -11.1 | 0.8 | -0.3 | 10.0 | 2166 | 307 | 8 | 263.9 |
| `crackle_long_14.wav` | 3.586 | 3.59 | -9.7 | -11.3 | 0.7 | -0.3 | 10.1 | 1994 | 313 | 11 | 287.4 |
| `crackle_long_15.wav` | 2.709 | 2.71 | -8.6 | -9.7 | 0.4 | -0.3 | 8.9 | 2036 | 342 | 6 | 271.9 |
| `crackle_long_16.wav` | 2.988 | 2.99 | -9.7 | -11.1 | 0.6 | -0.3 | 10.2 | 2214 | 318 | 10 | 264.9 |
| `crackle_long_17.wav` | 2.391 | 2.39 | -8.4 | -9.6 | 0.6 | -0.3 | 8.7 | 1798 | 290 | 6 | 405.1 |
| `crackle_long_18.wav` | 2.523 | 2.52 | -9.3 | -10.5 | 0.7 | -0.3 | 9.4 | 2045 | 316 | 7 | 365.7 |
| `crackle_long_19.wav` | 1.859 | 1.86 | -9.4 | -11.1 | 0.6 | -0.3 | 10.1 | 2556 | 349 | 7 | 230.5 |
| `crackle_long_20.wav` | 2.590 | 2.59 | -10.1 | -11.6 | 0.7 | -0.3 | 10.9 | 2533 | 337 | 10 | 261.4 |
| `crackle_long_21.wav` | 2.351 | 2.35 | -18.0 | -17.3 | -0.3 | -0.3 | 19.3 | 2926 | 898 | 13 | 219.0 |
| `crackle_long_22.wav` | 2.014 | 2.01 | -17.8 | -18.1 | -0.1 | -0.3 | 19.1 | 3270 | 671 | 12 | 126.2 |
| `crackle_long_23.wav` | 3.009 | 3.01 | -19.1 | -19.1 | -0.3 | -0.3 | 20.6 | 3192 | 1058 | 16 | 199.5 |
| `crackle_long_24.wav` | 4.942 | 4.94 | -17.9 | -17.4 | -0.1 | -0.3 | 20.0 | 2645 | 435 | 27 | 161.7 |
| `crackle_long_25.wav` | 2.248 | 2.20 | -17.4 | -17.8 | -0.1 | -0.3 | 19.0 | 2852 | 478 | 11 | 208.3 |
| `crackle_long_26.wav` | 2.638 | 2.60 | -17.2 | -17.1 | -0.0 | -0.3 | 19.1 | 3139 | 830 | 16 | 153.1 |
| `crackle_long_27.wav` | 2.302 | 2.30 | -17.5 | -18.0 | -0.3 | -0.3 | 19.3 | 3010 | 901 | 14 | 159.1 |
| `crackle_long_28.wav` | 2.014 | 2.01 | -18.5 | -16.9 | -0.3 | -0.3 | 19.9 | 2719 | 524 | 7 | 264.9 |
| `crackle_long_29.wav` | 1.949 | 1.95 | -17.9 | -15.9 | -0.3 | -0.3 | 19.9 | 2718 | 318 | 12 | 136.2 |
| `crackle_long_30.wav` | 3.502 | 3.42 | -17.8 | -18.4 | -0.3 | -0.3 | 19.7 | 2657 | 368 | 19 | 186.6 |

Category summaries (medians across the 30 files, with ranges):

| Category | RMS dBFS | Integrated LUFS | Crest dB | Weighted centroid Hz | 85 per cent roll-off Hz | Spectral flatness |
| --- | --- | --- | --- | --- | --- | --- |
| Short exterior | -13.6 (-17.8 to -12.0) | -13.3 (-17.4 to -11.7) | 13.3 (11.7 to 17.5) | 1,484 (1,031 to 2,539) | 3,171 (2,092 to 5,538) | 0.05 (0.03 to 0.09) |
| Short interior | -12.4 (-15.3 to -9.2) | -12.5 (-16.6 to -9.9) | 12.1 (8.9 to 15.0) | 216 (190 to 488) | 343 (285 to 924) | 0.00 (0.00 to 0.04) |
| Long exterior | -10.5 (-20.9 to -8.4) | -9.8 (-19.1 to -7.7) | 10.2 (8.1 to 20.6) | 2,396 (1,798 to 3,270) | 5,508 (3,847 to 8,524) | 0.10 (0.07 to 0.24) |
| Long interior | -11.2 (-21.1 to -8.7) | -11.2 (-19.1 to -8.4) | 10.9 (8.4 to 20.8) | 326 (269 to 1,058) | 588 (448 to 2,204) | 0.01 (0.00 to 0.10) |

## 6. Acoustic shape of a single pop

Group B (`crackle_long_21` to `_30`) holds 147 separated pops and is the single-pop reference. Group A and the short set hold overlapping bursts where one pop cannot be isolated.

Per-event statistics over the 147 group B bursts (full-band envelope):

| Measure | p10 | p50 | p90 | min | max |
| --- | --- | --- | --- | --- | --- |
| Event duration at -30 dB, ms | 27.9 | 53.4 | 105.0 | 10.5 | 418.5 |
| Attack 10 to 90 per cent, ms | 3.0 | 12.5 | 28.4 | 0.7 | 278.5 |
| Decay from peak to -20 dB, ms | 15.5 | 28.1 | 59.6 | 7.5 | 335.7 |

High-frequency crack inside the event (above 1 kHz), medians of the per-file medians, group B: rise 1.40 ms (0.82 to 1.93 across files), decay to -20 dB 15.6 ms (12.2 to 22.2), width at -10 dB 5.7 ms (4.1 to 15.0). Group A gives rise 1.15 ms (0.84 to 1.78), decay 19.0 ms (14.8 to 23.6), width 4.9 ms (3.7 to 7.1).

Band energy of individual pops in `crackle_long_24.wav` (RMS per band, dBFS, from the burst windows found by the detector):

| Pop window s | 20 to 120 Hz | 120 to 500 Hz | 500 to 2,000 Hz | 2,000 to 8,000 Hz | 8,000 to 20,000 Hz | Share 20-120 / 120-500 / 500-2k / 2k-8k, per cent |
| --- | --- | --- | --- | --- | --- | --- |
| 0.042 to 0.092 | -22.3 | -15.8 | -26.9 | -36.8 | -36.7 | 17 / 76 / 6 / 1 |
| 0.165 to 0.214 | -20.9 | -16.1 | -23.3 | -32.5 | -39.4 | 21 / 65 / 12 / 1.5 |
| 0.280 to 0.338 | -23.8 | -18.3 | -27.2 | -36.1 | -40.4 | 20 / 71 / 9 / 1 |
| 0.533 to 0.599 | -20.6 | -18.0 | -20.9 | -28.4 | -33.3 | 31 / 56 / 29 / 5 |
| 0.729 to 0.802 | -23.2 | -18.4 | -28.9 | -44.2 | -44.7 | 22 / 68 / 6 / 0.2 |
| 0.952 to 1.007 | -22.9 | -20.0 | -31.5 | -42.8 | -48.5 | 31 / 61 / 4 / 0.3 |

Whole-file band shares for group B: 27 to 36 per cent at 20 to 120 Hz, 55 to 62 per cent at 120 to 500 Hz, 4 to 11 per cent at 500 to 2,000 Hz, 0.6 to 1.8 per cent at 2,000 to 8,000 Hz. The group B weighted centroid of 2,645 to 3,270 Hz is pulled up by the broadband crack frames; the per-frame median centroid is 1,260 to 3,043 Hz for long exterior files and the energy centre by band power is inside 120 to 500 Hz.

The spectrogram of the pop at 0.165 s in `crackle_long_24.wav` (`/tmp/snarl-refs/out/img/long24_pop2_spec.png`) shows the structure: a broadband onset that reaches 12 kHz within the first 2 ms, a ridge at 200 to 600 Hz that peaks about 20 ms after onset and rings to 70 ms, and a sub-100 Hz tail that outlasts the ridge. The waveform (`long24_pop2_wave.png`) shows 3 to 4 low-frequency cycles at about 13 ms spacing (75 Hz) under the ridge, so the body is a damped resonance, not a single pressure step.

Comparison with the references:

| Property | EngineLab measured pop (enginelab.md section 5) | Prior art (prior-art.md section 4) | This pack, isolated pop (group B) | Consequence for snarl |
| --- | --- | --- | --- | --- |
| Front | 0.2 to 0.8 ms | Impulse "shaped like the ignition function" (Baldan) | Crack rise 0.8 to 1.9 ms above 1 kHz; full-band attack median 12.5 ms | The crack front matches EngineLab within a factor of two. The full-band attack is slow because the resonant body builds over 10 to 30 ms. Keep the 0.1 ms crack attack and let the body rise through the delay line |
| Duration | 2 to 4 ms | Not stated | 28 to 105 ms at -30 dB, decay to -20 dB 15 to 60 ms | EngineLab's duration is the crack alone. The procedural pop needs a 20 to 60 ms body, which the exhaust path can supply if the pressure jump is 2 to 5 ms wide instead of one sample |
| Energy centroid | 450 to 790 Hz | Not stated | Band power centre inside 120 to 500 Hz; 60 per cent of energy there | The pack sits one octave below EngineLab's centroid. Use 150 to 500 Hz as the body target for the exterior voice |
| 4 to 8 kHz share | 0.2 to 0.5 per cent; crack ratio bracket 0.8 to 2.5 | Not stated | 0.6 to 1.8 per cent at 2 to 8 kHz on group B files, 1.4 to 5.8 per cent on short files | The pack carries two to ten times more crack energy than EngineLab's default. Start `crackRatio` at the top of EngineLab's bracket, about 2.5, and tune by ear |
| Band split 20-120 / 120-500 / 500-2k / 2k-8k | 15 / 65 / 20 / 0.4 per cent | Not stated | 31 / 60 / 8 / 1 per cent (group B), 82 / 11 / 5 / 2 per cent (group A, short set) | EngineLab's lab-twin split is close to an isolated pop. Dense crackle on a real car is bass-heavy, and that difference comes from the overlapping bodies, not from a different pop |
| Level over the engine | ON/OFF peak -3 to +10 dB | Not stated | Not measurable: the clips contain no engine bed | Tune in the harness against the running engine |

## 7. Timing of pops inside long crackles

Pooled over the 30 long exterior files: 288 bursts, 258 onset intervals, 80.70 s active.

| Measure | p10 | p25 | p50 | p75 | p90 | min | max |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Onset interval, ms | 118.5 | 163.1 | 221.0 | 274.2 | 463.2 | 51.9 | 1,526.0 |
| Gap between bursts, ms | 38.7 | 52.5 | 78.6 | 128.6 | 200.5 | 30.4 | 285.3 |
| Burst duration, ms | 36.6 | 52.9 | 118.2 | 216.9 | 420.4 | 10.5 | 2,057.3 |
| Burst peak, dBFS | -7.6 | -3.1 | -0.4 | -0.3 | -0.3 | -27.1 | -0.3 |

Intervals inside 300 to 700 ms: 51 of 258 (20 per cent). Intervals under 300 ms: 78 per cent. Pooled duty 54.27 s of burst in 80.70 s active = 0.67. Bursts per active second: 3.57.

By group:

| Group | Bursts | Onset interval p10 / p50 / p90 ms | Gap p50 ms | Duration p50 ms | Intervals in 300 to 700 ms | Duty range per file | Density per second |
| --- | --- | --- | --- | --- | --- | --- | --- |
| A, dense crackle (`_1` to `_20`) | 141 | 196 / 272 / 652 | 55 | 212 | 45 of 121 (37 per cent) | 0.55 to 1.00 | 0.49 to 4.1 |
| B, isolated pops (`_21` to `_30`) | 147 | 110 / 174 / 264 | 99 | 53 | 6 of 137 (4 per cent) | 0.24 to 0.47 | 3.5 to 6.2 |

The gap floor of about 30 ms is the detector's merge window, so true gaps under 30 ms are absorbed into one burst. Group A durations of 200 ms and longer are therefore chains of pops that the detector could not separate; the high-frequency crack counter finds 77 to 126 cracks per second inside group A bursts and 47 to 151 per second inside group B bursts, with the 3 ms separation floor saturated in both.

Against the EngineLab trigger logic (enginelab.md section 5: deterministic low-discrepancy intervals of 300 to 700 ms, 8 per cent fuel-slug duty, 2 Hz nominal):

- Interval: the pack's isolated pops arrive every 110 to 264 ms (p10 to p90), two to three times faster than EngineLab's window. EngineLab's 300 to 700 ms matches the gap between group A runs (p90 of the onset interval is 652 ms), not the spacing inside a run. A two-level scheduler fits the data: runs of pops at 120 to 300 ms spacing, separated by 300 to 700 ms, with the run length set by the profile's pop density.
- Duty: not comparable directly. EngineLab's 8 per cent is the fraction of the overrun period during which fuel is retained; the acoustic event that follows one slug is what this pack shows as a 30 to 100 ms pop. With pops every 174 ms and events of 53 ms, the acoustic duty is 0.3, and EngineLab's chemistry would need about a 30 per cent slug duty or a longer acoustic tail per slug to reach that. snarl does not model chemistry, so set the acoustic duty directly: 0.25 to 0.45 for a sparse profile, 0.6 to 0.9 for a decat profile like `flat12` at density 0.8.
- Determinism: the pack's intervals are not uniform. Group B's interquartile spread is about 100 ms around a 174 ms median, which a low-discrepancy sequence with plus or minus 40 per cent jitter reproduces. The 40 per cent timing variation that EngineLab documents is the right order.
- Amplitude: group B pop peaks spread from -0.3 to -9 dBFS within one file (`crackle_long_24.wav`), and the run fades over its last second (-18 to -27 dBFS). Per-pop gain should vary by about 6 to 9 dB, with a decaying envelope across the run as the Baldan probability decay suggests (prior-art.md section 4).

## 8. Recommendation

Reference set for tuning the procedural pop, in priority order:

1. `Long_Cracles/crackle_long_24.wav` (27 pops, 4.94 s), `crackle_long_23.wav` (16 pops), `crackle_long_30.wav` (19 pops), `crackle_long_26.wav` (16 pops). These give the isolated single-pop shape, the 110 to 264 ms spacing and the amplitude spread. Use them for the body resonance target (120 to 500 Hz), the crack level (`crackRatio` near 2.5) and the decay (15 to 60 ms to -20 dB).
2. `Long_Cracles/crackle_long_1.wav`, `crackle_long_7.wav`, `crackle_long_14.wav` for dense crackle: what a run of overlapping pops must sound like when `flat12` runs at density 0.8. The sub-120 Hz weight (64 to 87 per cent) is the target for the stacked bodies.
3. `Short_Cracles/crackle_7.wav` and `crackle_6.wav` for the brightest single bursts (centroid 2,539 and 2,302 Hz, 5.5 and 5.8 per cent at 2 to 8 kHz), and `crackle_2.wav` or `crackle_16.wav` for the dullest (1,035 and 1,031 Hz, 0.7 per cent). These bracket the crack-to-body ratio across the pack.
4. Interior files for the in-car voice only. They are the exterior files through a steep low-pass with a centroid of 190 to 488 Hz, so the procedural interior voice is a filter on the exterior voice, not a different pop.

Ignore `crackle_long_3.wav` (one 2.06 s continuous burst, no separable pops) and the near-duplicate pairs `crackle_9` / `crackle_18` and `crackle_10` / `crackle_19`.

One-shot sample layer behind the event API:

- Candidates: the short exterior set (30 files) as the pop one-shots, and group B long files cut into single pops if finer one-shots are wanted. The short files are already single bursts, peak-normalised, with a hard cut on 10 of 30 that needs a 5 ms fade-out to avoid a click at the cut.
- The sample layer must live in the game's private asset tree, not in snarl. snarl's API takes the decoded buffers from the game at construction (`[]float32` per one-shot), because the licence forbids redistribution and the game owns the asset pipeline.
- Memory, computed from the measured durations: short exterior set 19.11 s = 3.67 MB at 48 kHz mono float32, 1.83 MB mono int16, 7.34 MB stereo float32; trimmed to the active 17.22 s, 3.31 MB float32 mono or 1.65 MB int16. Long exterior set 80.88 s = 15.53 MB mono float32, 7.76 MB mono int16, 31.06 MB stereo float32. Group B alone (26.97 s) = 5.18 MB mono float32 or 2.59 MB int16. At the native 44.1 kHz mono int16 the short set is 1.69 MB and the long set 7.13 MB.
- Conversion needed: 44.1 to 48 kHz resample (the game mixer runs at 48 kHz, scoping.md decision 9), stereo to mono downmix (side content is 19 to 22 dB down, so nothing audible is lost), a 5 ms fade on the hard-cut files, and a -1 dB trim on the 23 exterior files whose true peak exceeds 0 dBTP before any further gain. `ffmpeg -af "pan=mono|c0=0.5*c0+0.5*c1,aresample=48000:resampler=soxr,afade=t=out:st=<end-0.005>:d=0.005,volume=-1dB" -c:a pcm_f32le` covers all four. Keep the result out of version control.
- Recommended set if the layer ships: the 30 short exterior files mono int16 at 48 kHz, 1.83 MB, plus `crackle_long_24.wav` cut into its 27 pops for a sparse profile (about 1.5 s of audio, 0.14 MB int16). Load the interior set only if the game adds an in-car camera.

Procedural pop targets derived from this pack, to replace the EngineLab-only numbers in scoping.md decision 3:

| Parameter | Target | Source |
| --- | --- | --- |
| Crack attack | 0.1 ms synthesiser attack; measured rise 0.8 to 1.9 ms after the mic path | EngineLab `ReactionCrackSynthesiser`, group B cracks |
| Crack decay | 12 to 22 ms to -20 dB above 1 kHz | Group A and B crack decay medians |
| Body band | 120 to 500 Hz, 55 to 76 per cent of event energy; about 75 Hz ring visible in the waveform | `crackle_long_24.wav` pop windows |
| Body decay | 15 to 60 ms to -20 dB (p10 to p90 of 147 pops: 15.5 to 59.6 ms) | Group B bursts |
| Event length | 28 to 105 ms at -30 dB | Group B bursts |
| 2 to 8 kHz share | 0.6 to 1.8 per cent of an isolated pop, up to 5.8 per cent in a bright burst | Group B files, `crackle_7.wav` |
| Spacing inside a run | 110 to 264 ms, median 174 ms, plus or minus 40 per cent jitter | Group B onset intervals |
| Spacing between runs | 300 to 700 ms | EngineLab, matched by group A p90 of 652 ms |
| Per-pop gain spread | 6 to 9 dB, with a fade over the last second of a run | `crackle_long_24.wav` peaks -0.3 to -9.3 dBFS, tail -18 to -27 dBFS |
| Stacked-run spectrum | 64 to 87 per cent below 120 Hz when pops overlap | Group A and short set band shares |

## 9. Caveats and open questions

- Provenance: the vehicles, engines and microphones are unknown, and the market listing says the clips were made in Ableton, Adobe Audition and Fmod, so some pops may be layered or processed rather than raw recordings. Treat the spectral balance as a production target, not a physical measurement. Confidence: medium.
- Detector limits: the 30 ms merge window hides gaps under 30 ms, and the 3 ms crack separation saturates on dense crackle. Group A burst durations and all crack rates are therefore bounds, not point values. Confidence: medium on group A timing, high on group B timing.
- Band filters: cascaded second-order filters leak between neighbouring bands by a few per cent. Confidence: high on the ordering of bands, medium on the exact shares.
- No engine bed: the clips have no running engine under them, so the pop-to-engine level ratio that EngineLab measured (-3 to +10 dB) has no counterpart here and must be set in the harness.
- Licence boundary: the Nix dev shell must not copy the WAVs into a store path that reaches a public cache. If `snarl-compare` or the harness loads them, pass a path outside the repository through an environment variable or a flag.
- Not done: no listening test, and no comparison against a snarl or EngineLab render, because neither exists yet. The next step is to feed `crackle_long_24.wav` and `crackle_long_1.wav` into `snarl-compare` once phase 3 delivers it, and to hold the first procedural pop against the table in section 8.

## 10. Pop kinds

Status: analysis complete. Date: 2026-10-18. This section gives the event kinds that the snarl pop kinds (`weight_pop`, `weight_boom`, `weight_crackle` and `weight_burble` in `[pops]`) come from.

### Method

The 60 exterior files (30 short, 30 long) were decoded to mono at 44.1 kHz. The full-band envelope is a 1 ms RMS at a 0.5 ms hop, smoothed over 3 ms. An event is an envelope peak with at least 8 dB of prominence and within 40 dB of the file peak. The spacing between events has two modes: under 30 ms (2,849 of 3,609 intervals) and 60 to 300 ms, with a minimum near 50 to 60 ms. Events less than 50 ms apart therefore form one chain, and the chain is the unit of a pop: 358 chains, 48 of them a single event.

Each chain has these measures: event count, median spacing, duration above the chain peak minus 30 dB, attack (10 to 90 per cent of the running peak), decay from the chain peak to -20 dB, spectral centroid from 30 Hz to 8 kHz, energy shares of 30 to 120 Hz, 120 to 500 Hz, 500 Hz to 2 kHz and 2 to 8 kHz (Hann-windowed FFT over the chain), and sample peak. K-means on nine standardised features (log duration, log count, log centroid, the two low shares, log 2 to 8 kHz share, peak, log attack, log decay) gave silhouettes of 0.357, 0.369, 0.300, 0.258 and 0.265 for 2 to 6 clusters. Three clusters merge the deep burst and the crackle train. Four clusters is the smallest set that separates the kinds a listener names, so snarl uses four.

### Clusters

Values are p10 / p50 / p90 over the chains of each cluster. Relative peak is the chain peak against the loudest chain of the same file, because every file is peak-normalised.

| Kind | Chains | Source | Events | Spacing ms | Duration ms | Decay ms | Centroid Hz | 30-120 Hz % | 120-500 Hz % | 0.5-2 kHz % | 2-8 kHz % | Relative peak dB | Spectral peak Hz |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Pop | 141 | Long 21 to 30 (133), long 1 to 20 (8) | 1 / 2 / 4 | 13 / 18 / 32 | 33 / 66 / 118 | 9 / 20 / 35 | 154 / 218 / 353 | 19 / 37 / 60 | 35 / 56 / 73 | 1.4 / 4.6 / 11 | 0.11 / 0.60 / 2.3 | -7.5 / -2.2 / 0 | 64 / 112 / 168 |
| Boom | 68 | Long 1 to 20 (53), long 21 to 30 (3), short (12) | 4 / 9 / 17 | 10 / 14.5 / 21 | 104 / 183 / 291 | 16 / 50 / 91 | 62 / 120 / 175 | 78 / 91 / 97 | 1.8 / 6.5 / 19 | 0.4 / 1.2 / 3.5 | 0.10 / 0.48 / 1.5 | -3.6 / 0 / 0 | 42 / 52 / 70 |
| Crackle | 128 | Long 1 to 20 (94), short (34) | 6 / 18 / 34 | 11 / 15 / 23 | 165 / 382 / 684 | 23 / 69 / 139 | 167 / 253 / 479 | 62 / 79 / 87 | 7 / 13 / 26 | 2.4 / 4.6 / 9 | 1.4 / 2.9 / 7.2 | 0 / 0 / 0 | 45 / 55 / 73 |
| Burble | 21 | Long 21 to 30 (13), long 1 to 20 (4), short (4) | 1 / 4 / 7 | 9 / 11 / 21 | 38 / 71 / 361 | 11 / 34 / 74 | 41 / 150 / 209 | 15 / 37 / 99.8 | 0.2 / 63 / 83 | 0.001 / 0.6 / 1.8 | 0.001 / 0.009 / 0.05 | -35 / -23 / -18 | 27 / 152 / 172 |

Descriptions:

- Pop: the isolated pop of section 6. A resonant body near 110 Hz, little crack, and pops 97 to 269 ms apart (median 173 ms). This is the pop that snarl had before.
- Boom: a short deep burst. Nine tenths of the energy is below 120 Hz, with a ring at 42 to 70 Hz. The first event is bright (median centroid 402 Hz against 116 Hz for the rest), so a boom starts with a crack.
- Crackle: a long train of small cracks over the same deep ring. The high-frequency transients come every 8 to 23 ms (median 12.5 ms, 23 to 75 per second). The first event is bright (348 Hz against 119 Hz). The 2 to 8 kHz share is the highest of the four kinds.
- Burble: a soft low pop 18 to 35 dB below the loudest event of its file, with no crack. Its 30 to 120 Hz share is bimodal: some burbles are a sub-bass thump, others a quiet body pop.

No separate "sharp crack" cluster exists at the chain level. With per-event features, a bright cluster (42 chains, centroid 566 Hz, 8 per cent at 2 to 8 kHz) appears, but with features measured over the whole chain it merges into the crackle and boom clusters. The sharp crack is the onset of booms and crackles.

### Sequence

Transitions between consecutive chains in a file (count of the next kind after each kind):

| After | Pop | Boom | Crackle | Burble |
| --- | --- | --- | --- | --- |
| Pop | 121 | 7 | 2 | 7 |
| Boom | 5 | 15 | 29 | 1 |
| Crackle | 2 | 33 | 56 | 7 |
| Burble | 3 | 5 | 0 | 5 |

Pops follow pops, because group B holds only isolated pops. Booms and crackles alternate inside the dense files: a boom is followed by a crackle 29 times in 50 and a crackle by a boom 33 times in 98. Burbles sit between runs. Runs (chains less than 300 ms apart) start with a crackle 79 times, a boom 32 times, a pop 18 times and a burble 6 times. The two production styles of the pack (section 3) set most of this order, so snarl picks each kind at random with the profile weights and does not model the order.

### Snarl pop kinds against the clusters

Each kind renders as a train of sub-events in one of four event slots: a rounded pressure jump into an exhaust path through a resonant body, plus a high-frequency crack at the outlet. Medians of 12 variations of one isolated pop through the one-path test engine in `internal/model/pop_test.go`, tier 0, measured as the pack was measured:

| Kind | Duration ms | Decay ms | Centroid Hz | 30-120 Hz % | 120-500 Hz % | 0.5-2 kHz % | 2-8 kHz % | Peak dBFS |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Pop | 44 | 24 | 215 | 4.0 | 95.2 | 0.47 | 0.44 | -11.2 |
| Boom | 112 | 31 | 114 | 93.3 | 5.4 | 0.62 | 0.62 | -8.1 |
| Crackle | 221 | 29 | 217 | 76.7 | 18.3 | 3.99 | 1.79 | -15.8 |
| Burble | 65 | 30 | 130 | 31.8 | 68.0 | 0.11 | 0.008 | -33.9 |

Every value is inside the p10 to p90 range of its cluster, except the pop band shares. The pop keeps its tuned body: its 30 to 120 Hz share is 4 per cent against 19 to 60 per cent in the pack. The boom and the crackle now carry the deep energy. In tier 1, the expansion chamber moves the boom and crackle shares further below 120 Hz (98 and 94 per cent). `TestPopKinds` holds these values to the cluster ranges with stated tolerances.

The pack levels are per file, so the level between kinds is a choice. The burble is 23 dB below the pop, as in the pack. The crackle peaks about 5 dB below the pop, because its length already makes it much louder than a pop (group A is 8.6 LU louder than group B at the same peak, section 3).
