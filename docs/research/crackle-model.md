# Short exterior crackles: the crackle model and its check

Status: model, fit and check done. The model is not connected to the engine. Date: 2026-10-22. Branch `pop-model`. This report gives the evidence for the form of the crackle model, the model in `internal/crackle`, the fit in `tools/crackle --fit`, and the check of 60 model trains against the 30 short exterior crackle clips of [crackle-measure.md](crackle-measure.md) with the same measures. No audio and no measure of a single clip is in the repository. The model parameters are statistics and fits of the whole set.

## 1. Answer

The body near 52 Hz is not an engine tone and is not driven by the shocks. It is a narrow band of random sound with no harmonics and a Rayleigh envelope, independent of the shocks. The model keeps it as a separate part with its own gain. The shock times are random: a renewal process with a dead time of about 4 ms, with no period and no lock to the body. The levels of consecutive shocks correlate (0.29), so loud shocks come in groups.

The model is a random train of shocks over a bed and a body. Each shock is a sharp crack (rise 2.9 ms, fall time constant 1.9 ms) and a ring (time constant 5.9 ms), each through its own bank of octave band filters. The bed is a third bank. The body is noise through a resonant band pass at 51 Hz. A damped sine at 27 Hz is the tail at the end of the train.

The check passes 178 of 235 measures (76 per cent) with the model median inside the p25 to p75 of the clips. 8 more measures have no value on the model side: the 5 channel measures of the mono model and 3 measures of the edit of the pack: the start and the peak of the fragment after the event, and the quiet span between two bursts. Of the 57 misses, 33 are within 1.5 units of the clip quartiles (dB, ms or Hz). Section 6 lists every miss and its reason.

The model closes the gaps that phase 1 found in the snarl pops:

| Measure | Clips | Model | Snarl pop kinds |
| --- | --- | --- | --- |
| Share above 2 kHz, dB | -16.6 | -16.3 | -24.3 to -27.4 |
| Crest factor of the strong shocks, pooled, dB | 8.5 | 8.5 | 6.3 to 7.0 |
| Body peak, Hz | 52 | 54 | 86 to 421 |
| Gap between strong shocks, pooled, ms | 32.2 | 28.5 | 8.0 to 10.2 |
| Share below 125 Hz, dB | -1.1 | -0.8 | -5.4 to -13.8 |

One model voice costs 13.6 ms per second of audio at 44.1 kHz, 1.4 per cent of one core.

## 2. Body and timing

`tools/crackle` measures the body and the timing of each clip (`tools/crackle/nature.go`). The body band is one octave around the body peak. The measures are below, median (p25 to p75) over the 30 clips, or pooled where the row says so.

### 2.1 The body

| Measure | Clips | A stable tone with harmonics | Narrow band noise |
| --- | --- | --- | --- |
| Body cycle frequency, Hz | 51.9 (49.3 to 53.8) | | |
| Spread of the body cycles, interquartile range over median | 0.23 (0.19 to 0.29) | below 0.05 | above 0.05 |
| Drift of the cycle frequency, fit of the line, r squared | 0.06 (0.01 to 0.17) | | |
| Prominence at 2 times the body peak, dB | 1.1 (-0.1 to 3.6) | above 10 | below 8 |
| Prominence at 1.5 and 2.5 times the body peak, dB | -0.9 and 1.3 | | |
| Prominence at 0.5 times the body peak, dB | -15.1 (-20.7 to -10.9) | | |
| Spread over the mean of the body envelope | 0.55 (0.52 to 0.58) | below 0.15 | 0.52 (Rayleigh) |
| Rise of the body envelope at the strong shocks, dB | 1.6 (-0.4 to 4.1) | | |
| Rise of the body envelope at random times, dB | 0.5 (-1.3 to 2.5) | | |
| Correlation of the shock level and the rise of the body envelope | 0.13 (-0.00 to 0.30) | | |

The prominence is the highest level within 4 per cent of a frequency against the mean power a quarter of the body peak to each side. The columns of the test signals come from `go test ./tools/crackle` (`TestNatureTone`, `TestNatureNoise`).

- The body has no harmonics. The prominence at 2, 3, 4 and 5 times the body peak (1.1 to 3.8 dB) is the same as at the half multiples between them (-0.9 to 4.4 dB). A peak search in a noisy spectrum gives about this value. A harmonic tone gives more than 10 dB.
- The body cycles are not stable. Their spread is 0.23, and the frequency has no consistent drift (r squared 0.06). An engine on overrun slows down, which would give a drift.
- The envelope of the body is that of narrow band random sound. Its spread over its mean is 0.55, and the Rayleigh envelope of narrow band Gaussian noise gives 0.52.
- The shocks do not drive the body. At a strong shock, the body envelope rises 1.6 dB, against 0.5 dB at random times, and the correlation of the shock level with the rise is 0.13. Phase 1 found that the 31.25 to 62.5 Hz band stays level from 10 ms before the strongest shock to 320 ms after it.

Decision: the body is neither an engine firing tone nor a resonance that the shocks excite. It is a narrow band of random sound near 52 Hz, independent of the shocks. The snarl engine makes a harmonic tone, so the snarl engine does not supply this body. The model keeps the body as a separate part with its own gain (`body_gain_db`), and phase 3 can set its level against the engine.

### 2.2 The timing of the shocks

| Measure | Clips |
| --- | --- |
| Spread over the mean of the gaps between shocks | 0.67 (0.52 to 0.95) |
| Fano factor of the shock count in 50 ms windows | 0.89 (0.69 to 1.20) |
| Correlation of the logarithms of consecutive gaps | 0.03 (-0.12 to 0.08) |
| Kolmogorov-Smirnov distance of the 1,221 pooled gaps to an exponential above a dead time of 3.13 ms | 0.060 |
| Pair correlation of all shocks, pooled, 0 to 2 / 2 to 4 / 4 to 80 ms | 0.06 / 0.54 / 0.94 to 1.40, no peak |
| Pair correlation in body cycles, pooled, at 1.0, 2.0 and 3.0 cycles | 1.32, 1.18, 1.03 (between them 1.0 to 1.3) |
| Phase of the shock onsets in the body cycle, pooled, mean resultant length | 0.10 for 1,054 shocks (p 3e-5), 0.13 for 157 strong shocks (p 0.07) |
| Correlation of the levels of consecutive shocks, pooled | 0.29 |
| Correlation of the level of a shock and the logarithm of the gap before it, pooled | 0.04 |
| Fall above 1 kHz to -10 dB of the strong shocks, pooled, ms | 0.57 (0.39 to 1.16) |

The pair correlation is the count of pairs of shocks of one clip at each lag over the count for independent uniform times: 1 for a Poisson timing, below 1 for a dead time, and peaks at the multiples of a period.

- The timing is random. A Poisson process gives a gap spread and a Fano factor of 1. The clips give 0.67 and 0.89, which a dead time lowers. Consecutive gaps are independent (0.03). The pooled gaps are near an exponential above a dead time of 3.13 ms.
- The timing has no period. The pair correlation is low below 4 ms (the dead time) and level from 4 to 80 ms.
- The timing is not locked to the body. The pair correlation has no peaks at whole body cycles. The phase of the shocks in the body cycle has a weak preference (mean resultant length 0.10, where 1 is full lock), and the strong shocks have none that is significant.
- The levels are not independent. Consecutive levels correlate (0.29), so loud shocks come in groups. The level does not depend on the gap before.
- A shock is a sharp crack. The new fall measure (`FallMS`) is the time from the peak of the 0.25 ms RMS envelope above 1 kHz to 10 dB below it. It is 0.57 ms for the strong shocks. The band energy after the strongest shock stays high for 10 to 20 ms (phase 1, section 5), so the crack has a ring after it.

Decision: the shocks are a renewal process with the measured gap distribution, and levels from the measured level distribution with a correlation of consecutive levels. The shock times do not follow the body.

## 3. The model

`internal/crackle` is the model. `crackle.New(params, rate, seed)` makes a voice at a sample rate, `Voice.Trigger(Options)` starts a train, and `Voice.Process(block)` adds the train to a block of float32 samples. The voice does not allocate after `New` (`TestNoAllocations`), and the output for a seed and a sequence of calls is the same on every run (`TestDeterministic`).

The parts of a train:

| Part | Form | Parameters |
| --- | --- | --- |
| Shock times | The first shock at the first sample, then gaps drawn from the measured quantile table of the gaps, times a scale | `gap_ms`, `gap_scale` |
| Shock levels | Drawn from the measured quantile table of the levels against the strongest shock, with a scaled spread, a correlation of consecutive levels through a normal value, and an offset for each tenth of the train | `level_db`, `level_scale`, `level_corr`, `place_db` |
| Crack | A linear rise and an exponential fall of the envelope, on a carrier in each octave band, through a bank of 11 octave band filters | `attack_ms`, `decay_ms`, `shock_db` |
| Ring | An exponential fall from the peak of the crack, on its own carriers, through its own bank | `ring_ms`, `ring_db` |
| Bed | Carriers through a third bank, constant over the train | `bed_db` |
| Top | An eighth-order Butterworth low pass after the three banks | `top_hz` |
| Body | White noise through two equal band pass stages, independent of the shocks | `body_hz`, `body_q`, `body_gain_db` |
| End | After the last shock and a delay, an exponential fade. At the fade, the body input stops, the body rings out, and the tail starts: a damped sine | `fade_delay_ms`, `fade_db_per_ms`, `tail_hz`, `tail_decay_ms`, `tail_gain_db` |
| Start | A short linear fade, with the bed and the body run before the first sample | `fade_in_ms`, `preroll_ms` |
| Level | The output gain at 0 dB of the level control | `out_db` |

The carrier of a band is a sine at the band centre whose phase takes a random walk, so that it has the spectrum of a band of noise with a constant envelope. Each band has its own carrier, so the band powers add, and the bank fit is linear in the squared gains. White noise in the banks failed: its envelope above 1 kHz rises and falls by several dB within a shock, and the shock detector of the measures counts each rise as a shock. With noise, the model trains gave about ten detected shocks for each shock that the model made.

The controls:

- Level: `Options.LevelDB` is a gain in dB over the fitted level. At 0 dB, the median sample peak of a train is the median sample peak of the clips, -0.4 dBFS.
- Length: `Options.Shocks` sets the count of shocks, clamped to the measured range of 28 to 61. `Options.LengthMS` sets the active length, clamped to the measured range of 368 to 637 ms. With neither, the length comes from the measured quartiles.
- Single shock: `Options.Single` plays one shock at the level of the strongest shock, with the bed and the body until the fade. This mode is derived, not measured: no clip holds an isolated shock (phase 1, section 10).
- Rate: the filters, carriers and envelopes take the sample rate of `New`. The fit and the check run at 44.1 kHz, the rate of the clips and of the snarl mix.

## 4. Parameters and sources

`internal/crackle/crackle.toml` holds the parameters, and `internal/crackle/crackle.sources.toml` beside it holds one source for each key, as `profiles/*.sources.toml` do. `TestSources` fails when a key has no entry or an entry names no key. The kinds:

| Kind | Keys |
| --- | --- |
| measured, a statistic of the 30 clips | `length_ms` (minimum, quartiles, maximum), `shocks_range` (least and most shocks), `gap_ms` (21 quantiles of 1,221 pooled gaps), `level_db` (21 quantiles of 1,251 pooled levels), `tail_hz`, `tail_decay_ms` |
| fitted, by the search of section 5 | `length_offset_ms`, `gap_scale`, `level_scale`, `level_corr`, `place_db` (the measured offsets of each tenth times a fitted scale, 0.75), `attack_ms`, `decay_ms`, `ring_ms`, `top_hz`, `body_hz`, `body_q`, `fade_delay_ms`, `fade_db_per_ms`, `tail_gain_db` |
| fitted, by the bank fit of section 5 | `shock_db`, `ring_db`, `bed_db`, `body_gain_db` |
| fitted, by the scaling | `out_db` |
| snarl default | `fade_in_ms` (1 ms against a click at the start: the clips start cut, so they cannot give a fade-in), `preroll_ms` (50 ms: the clips start with the event under way, so the bed and the body run for more than two body cycles and the ring of the body filter before the first sample) |

The quantile tables hold the quantiles at the probabilities (i + 0.5) / 21, so they hold no extreme value of the set. Each table is pooled over all clips, so it cannot rebuild a clip. The minimum and maximum of `length_ms` and `shocks_range` are the range of the controls, and phase 1 gives the same ranges.

## 5. The fit

`just crackle-fit` runs `tools/crackle --fit` on the 30 clips (`tools/crackle/fit.go`). The fit is deterministic. It uses no learned model. Each step renders 64 model trains with the seeds 1 to 64, scales each train to the median clip peak, as the pack scales its clips, and measures the trains with the same code as the clips.

1. Start: the measured tables and medians set the start of every parameter.
2. Bank fit: the band energy over time around the strongest shock (phase 1, section 5) gives the band powers of the bed (frames 10 and 5 ms before the shock), of the crack (frames 0 and 5 ms, less the bed) and of the ring (frames 10, 20 and 40 ms, less the bed). 500 Richardson-Lucy steps fit the squared gains of each bank to its band powers, over the matrix of the band powers that each carrier and filter gives in each measure band. The body gain gives the 31.25 to 62.5 Hz background power. A search over the corner from 8 to 19 kHz sets the low pass at the top for the least error of the bank fits.
3. Trim: 15 steps correct the band powers of the three banks by the error of the band energy over time of the model trains. The body stays fixed, and the other parts move against it, because the band energy is against the loudest frame of a train.
4. Search: a compass search moves 18 knobs, each in turn up and down by its step, keeps a move that lowers the loss, and halves the step of a knob that no move improves, for 600 renders. The loss is the sum over the sections of the check of the mean over the measures of the square of (model median - clip median) / clip interquartile range, limited to 16. The loss leaves out the file edges, the channels and the sample peak. The knobs are the fitted parameters of section 4 and the levels of the three banks and the body. After the first pass over the knobs the loss is 1.42, and at the end it is 0.719.

The search on the clip measures replaced fixed pairs of one parameter and one measure. With fixed pairs, the fit ran into degenerate states: for example, a ring as loud as the crack and a decay at its lower bound, or a level spread so wide that one shock towered over the rest.

## 6. The check

`just crackle-model` renders 60 model trains with the committed parameters and the seeds 1001 to 1060, which the fit did not use, writes them to `out/crackle/model/`, and checks them against the clips with the same measures (`out/crackle/model/compare.md`). It also measures the trains as clips (`out/crackle/model/summary.md`).

| Section | Pass | Measures | No value on the model side |
| --- | --- | --- | --- |
| Clip measures | 41 | 57 | 8 |
| All shocks, pooled | 7 | 7 | 0 |
| Strong shocks, pooled | 6 | 7 | 0 |
| Band shares | 35 | 44 | 0 |
| Band energy over time | 89 | 120 | 0 |
| All | 178 | 235 | 8 |

The 64 fit trains pass 186 of the 235 measures. The held-out trains pass 178, so the fit does not depend much on its seeds.

### 6.1 Measures that cannot pass

- The channel measures (5): the model is mono. Phase 1 found that the clips are mono to a correlation of 0.995, and that the mean of the two channels loses nothing that the measures can see.
- The fragment after the active span (start and peak) and the sample peak of the last 10 ms of the file (-110.1 dBFS against -44.4, 59.8 dB below the p25): the pack files end with a fragment 32 to 58 dB below full scale at 600 to 618 ms, after a quiet span. This is an edit of the pack, which the model does not copy.
- The quiet span between bursts: only 2 clips have two bursts, and no model train has two.

### 6.2 Measures that miss

Each miss is the distance of the model median outside the clip p25 to p75.

| Group | Measure | Miss | Reason |
| --- | --- | --- | --- |
| Level | RMS level, crest factor | -0.4 dB, +0.4 dB | The model trains are a little more peaky than the clips. |
| Level | Strong shocks within 12 dB, full-band peak of the strong shocks | +0.5 shocks, -0.3 dB | The search trades the count of strong shocks against the spread of all levels, which passes. |
| Timing | Onset of the strongest shock | -20.5 ms (108.5 against 165.0 ms) | The first tenth of the clips is 6.7 dB louder (median), but the strongest shock comes at 30 per cent of the train. The model offset for each tenth raises the whole first tenth, its top too, so the strongest shock comes earlier. The form has no separate spread for each tenth. |
| Shock shape | Median rise above 1 kHz of the strong shocks | +0.71 ms (4.15 against 2.39 ms) | The rise is from the lowest envelope since the shock before, so the bed and the ring of the shock before set it as much as the attack does. The search trades it against the onset frame and the fall, which pass. |
| Body | Share below 125 Hz, body cycle frequency, 31.25 to 62.5 Hz share in the strong windows and the active span | +0.2 dB, +0.2 Hz, +0.5 dB, +0.0 dB | The body is a little too strong against the rest. |
| Body | Prominence at 0.5 times the body peak | +2.9 dB | The tail sine at 27 Hz, which the clips show after the last shock, adds energy near half the body peak to the spectrum of the active span. |
| Tail | Spread of the half cycles, decay to -20 dB, fit of the decay, last half-cycle peak | -0.03, less than 0.5 ms, less than 0.005, -3.3 dB | 13 clips give these, with very narrow quartiles (22 to 24 ms, 1.00 to 1.00), likely the fade of the edit (phase 1, section 6). The model tail is one damped sine. |
| Spectrum | 250 to 500 Hz between the shocks and in the active span | -1.1 dB, -1.5 dB | See the band energy over time below. |
| Spectrum | 500 Hz to 1 kHz in the strong windows, 2 to 4 kHz in the active span | -0.1 dB, -0.0 dB | Small. |
| Spectrum | 4 to 8 kHz between the shocks, 8 to 16 kHz and 16 to 20 kHz in the strong windows | +1.1, +0.6, +2.7 dB | The top of the strong shocks of the model is brighter. |
| Time | Decay above 2 kHz to -20 dB | -6 ms (25 against 40 ms) | After the loudest frame, the clips keep their energy above 2 kHz longer than the model. |
| Time | Power between the shock windows against inside, all bands and above 2 kHz | +0.1 dB, +0.5 dB | The bed of the model is a little too strong against the shocks. |
| Time | Band energy over time, 31 of 120 | 13 within 1.5 dB. Largest: 8 to 16 kHz before the strongest shock +4.2 to +5.7 dB and at 160 ms +6.8 dB, 16 to 20 kHz at 20 ms -6.2 dB, 500 Hz to 1 kHz before the shock +3.7 dB and at 80 ms +5.0 dB, 250 to 500 Hz at 160 ms +3.2 dB | The bed of the model has one spectrum and level for the whole train. In the clips, the frames before the strongest shock and long after it have less energy from 500 Hz to 1 kHz and above 4 kHz than the model, while the sound between the shocks has the model level. A constant bed cannot give both. A bed that follows the shock activity would be the next form to try. |

The form was changed four times on this evidence: carriers in place of white noise (the detector), a ring bank after the crack (the energy after the strong shocks and between them), a tail sine at the end (the 27 Hz tail), and a run of the bed and the body before the first sample (the cut start of the clips). The remaining misses are small or come from the parts that the last row names.

### 6.3 Check table

Each cell is the median (p25 to p75, range min to max). Miss is the distance of the model median outside the clip p25 to p75, 0 for a pass.

Clip measures, median over the 30 clips and the 60 trains:

| Measure | Clips | Model | Pass | Miss |
| --- | --- | --- | --- | --- |
| Active length, ms | 544 (476 to 574, range 368 to 637) | 539 (485 to 571, range 371 to 630) | yes | 0 |
| Start of the active span in the file, ms | 0.0 (0.0 to 0.0, range 0.0 to 0.0) | 0.0 (0.0 to 0.0, range 0.0 to 0.0) | yes | 0.0 |
| Sample peak of the first 10 ms of the file, dBFS | -5.1 (-8.7 to -3.5, range -15.5 to -0.3) | -7.4 (-10.1 to -4.7, range -16.5 to -0.4) | yes | 0.0 |
| Sample peak of the last 10 ms of the file, dBFS | -44.4 (-50.3 to -32.4, range -58.1 to -5.6) | -110.1 (-112.4 to -107.7, range -116.4 to -104.7) | no | -59.8 |
| Bursts, between quiet spans of at least 20 ms | 1 (1 to 1, range 1 to 2) | 1 (1 to 1, range 1 to 1) | yes | 0 |
| Quiet span between bursts, ms | 34.3 (28.6 to 40.0, range 22.9 to 45.7), n=2 | not measured | - | - |
| Start of the sound after the active span, ms | 608.3 (604.8 to 610.5, range 600.8 to 617.9), n=23 | not measured | - | - |
| Sample peak of the sound after the active span, dBFS | -47.3 (-51.1 to -42.8, range -58.1 to -32.2), n=23 | not measured | - | - |
| Level of the last 5 ms against the loudest 5 ms, dB | -72.6 (-74.9 to -70.7, range -81.3 to -3.6) | -74.2 (-74.8 to -72.8, range -77.9 to -63.4) | yes | 0.0 |
| Level of the last 5 ms against the 5 ms that end 20 ms before, dB | -18.0 (-19.7 to -11.5, range -20.7 to 2.7) | -17.1 (-17.6 to -16.1, range -20.3 to -13.3) | yes | 0.0 |
| Sample peak, dBFS | -0.4 (-0.4 to -0.4, range -0.5 to -0.3) | -0.4 (-0.4 to -0.4, range -0.4 to -0.4) | yes | 0.0 |
| RMS level, dBFS | -12.8 (-13.0 to -12.4, range -15.6 to -11.5) | -13.5 (-15.2 to -12.3, range -19.4 to -10.7) | no | -0.4 |
| Crest factor, dB | 12.4 (12.0 to 12.6, range 11.0 to 15.2) | 13.1 (11.9 to 14.8, range 10.3 to 19.0) | no | 0.4 |
| Shocks | 40 (37 to 48, range 28 to 61) | 44 (40 to 48, range 24 to 63) | yes | 0 |
| Shocks within 6 dB of the strongest | 4 (3 to 5, range 1 to 8) | 4 (3 to 6, range 1 to 10) | yes | 0 |
| Strong shocks, within 12 dB of the strongest | 8 (6 to 8, range 2 to 14) | 8 (6 to 11, range 3 to 17) | no | 0.5 |
| Shocks per second of active length | 77 (70 to 91, range 56 to 105) | 85 (79 to 89, range 65 to 100) | yes | 0 |
| Median gap between shocks, ms | 8.6 (8.0 to 9.7, range 6.9 to 11.8) | 8.4 (7.9 to 9.0, range 6.3 to 11.4) | yes | 0.0 |
| Median gap between strong shocks, ms | 51.2 (17.2 to 74.0, range 10.5 to 356.2) | 32.0 (20.3 to 45.0, range 10.8 to 142.2) | yes | 0.0 |
| Median rise above 1 kHz of the shocks of the clip, ms | 2.25 (2.00 to 2.82, range 1.63 to 3.38) | 2.68 (2.38 to 2.92, range 2.03 to 3.42) | yes | 0.00 |
| Median rise above 1 kHz of the strong shocks of the clip, ms | 2.39 (1.50 to 3.44, range 0.41 to 4.59) | 4.15 (3.36 to 4.62, range 2.14 to 8.66) | no | 0.71 |
| Onset of the strongest shock, ms | 165.0 (129.0 to 235.8, range 2.6 to 477.9) | 108.5 (33.5 to 240.9, range 1.0 to 475.1) | no | -20.5 |
| Share above 2 kHz, dB | -16.6 (-17.5 to -14.6, range -20.4 to -11.3) | -16.3 (-17.8 to -15.2, range -22.5 to -12.6) | yes | 0.0 |
| Share below 125 Hz, dB | -1.1 (-1.3 to -0.9, range -2.2 to -0.8) | -0.8 (-1.0 to -0.6, range -1.5 to -0.3) | no | 0.2 |
| Decay above 2 kHz to -10 dB, ms | 10 (10 to 15, range 5 to 25) | 10 (10 to 15, range 5 to 25) | yes | 0 |
| Decay above 2 kHz to -20 dB, ms | 40 (31 to 54, range 20 to 195) | 25 (20 to 35, range 15 to 75) | no | -6 |
| Mean power between the shock windows against inside them, dB | -1.2 (-1.7 to -0.7, range -2.7 to -0.2) | -0.6 (-1.0 to 0.3, range -2.4 to 1.1) | no | 0.1 |
| Mean power above 2 kHz between the shock windows against inside them, dB | -3.5 (-5.3 to -1.6, range -9.1 to 2.0) | -1.2 (-2.6 to 0.0, range -5.8 to 5.3) | no | 0.5 |
| Body peak of the active span, 10 to 500 Hz, Hz | 52 (46 to 56, range 34 to 79) | 54 (50 to 57, range 39 to 64) | yes | 0 |
| Tail after the last shock, ms | 69 (55 to 74, range 0 to 144) | 71 (66 to 77, range 60 to 88) | yes | 0 |
| Tail peak, 10 to 500 Hz, Hz | 28 (27 to 30, range 25 to 39), n=24 | 27 (25 to 29, range 22 to 51) | yes | 0 |
| Half cycles of the tail | 3 (2 to 3, range 1 to 8), n=24 | 3 (3 to 4, range 2 to 7) | yes | 0 |
| Tail frequency from the median half cycle, Hz | 27 (27 to 28, range 24 to 38), n=13 | 28 (27 to 29, range 26 to 565), n=49 | yes | 0 |
| Spread of the tail half cycles, interquartile range over median | 0.17 (0.16 to 0.24, range 0.11 to 0.68), n=13 | 0.13 (0.09 to 0.97, range 0.01 to 19.60), n=49 | no | -0.03 |
| Tail decay to -20 dB, ms | 22 (22 to 24, range 20 to 83), n=13 | 22 (21 to 23, range 11 to 27), n=42 | no | -0 |
| Tail decay fit, r squared | 1.00 (1.00 to 1.00, range 0.84 to 1.00), n=13 | 1.00 (0.99 to 1.00, range 0.84 to 1.00), n=42 | no | -0.00 |
| Last half-cycle peak of the tail against the highest, dB | -32.0 (-32.2 to -29.1, range -39.9 to -12.4), n=13 | -35.5 (-36.7 to -34.0, range -97.9 to -14.6), n=49 | no | -3.3 |
| Channel correlation at no delay | 0.995 (0.994 to 0.995, range 0.990 to 0.996) | not measured | - | - |
| Channel correlation at the best delay | 0.995 (0.994 to 0.995, range 0.990 to 0.996) | not measured | - | - |
| Delay of the right channel behind the left, ms | -0.001 (-0.001 to -0.000, range -0.003 to 0.000) | not measured | - | - |
| Left level over right level, dB | -0.13 (-0.15 to -0.11, range -0.22 to -0.07) | not measured | - | - |
| Side (L-R) level over mid (L+R) level, dB | -25.8 (-26.2 to -25.3, range -27.0 to -22.9) | not measured | - | - |
| Body cycle frequency, median, Hz | 51.9 (49.3 to 53.8, range 38.8 to 70.9) | 54.1 (52.3 to 55.6, range 46.6 to 60.7) | no | 0.2 |
| Spread of the body cycles, interquartile range over median | 0.23 (0.19 to 0.29, range 0.15 to 0.48) | 0.21 (0.17 to 0.25, range 0.09 to 0.34) | yes | 0.00 |
| Drift of the body cycle frequency, per cent per second | -30.4 (-74.7 to 5.5, range -176.3 to 133.9) | -34.4 (-77.8 to -5.5, range -155.6 to 85.6) | yes | 0.0 |
| Fit of the drift line, r squared | 0.06 (0.01 to 0.17, range 0.00 to 0.67) | 0.06 (0.02 to 0.14, range 0.00 to 0.49) | yes | 0.00 |
| Prominence of the spectrum at 0.5 times the body peak, dB | -15.1 (-20.7 to -10.9, range -30.7 to 0.6) | -8.0 (-12.0 to -3.9, range -23.6 to 5.6) | no | 2.9 |
| Prominence of the spectrum at 1 times the body peak, dB | 7.9 (5.9 to 10.0, range 4.6 to 16.1) | 9.5 (7.6 to 12.5, range 4.6 to 18.7) | yes | 0.0 |
| Prominence of the spectrum at 1.5 times the body peak, dB | -0.9 (-3.6 to 1.8, range -6.3 to 9.7) | -0.1 (-3.5 to 3.4, range -7.6 to 10.3) | yes | 0.0 |
| Prominence of the spectrum at 2 times the body peak, dB | 1.1 (-0.1 to 3.6, range -8.9 to 9.1) | 1.4 (-1.3 to 3.8, range -9.9 to 9.4) | yes | 0.0 |
| Prominence of the spectrum at 2.5 times the body peak, dB | 1.3 (-0.7 to 4.0, range -3.7 to 11.1) | 2.5 (-0.0 to 4.3, range -6.0 to 8.8) | yes | 0.0 |
| Prominence of the spectrum at 3 times the body peak, dB | 3.2 (1.0 to 5.4, range -4.0 to 7.1) | 3.8 (2.2 to 5.1, range -3.3 to 10.3) | yes | 0.0 |
| Prominence of the spectrum at 3.5 times the body peak, dB | 3.9 (1.8 to 5.3, range -2.0 to 9.1) | 3.4 (2.5 to 4.7, range -2.0 to 9.0) | yes | 0.0 |
| Prominence of the spectrum at 4 times the body peak, dB | 3.5 (2.4 to 4.6, range -0.5 to 7.4) | 3.9 (2.6 to 5.2, range -3.2 to 7.2) | yes | 0.0 |
| Prominence of the spectrum at 4.5 times the body peak, dB | 4.4 (3.2 to 5.9, range -0.2 to 8.5) | 4.3 (3.1 to 5.5, range -0.8 to 9.0) | yes | 0.0 |
| Prominence of the spectrum at 5 times the body peak, dB | 3.8 (2.7 to 4.9, range 0.2 to 9.4) | 4.6 (3.4 to 5.6, range 0.1 to 8.9) | yes | 0.0 |
| Spread over the mean of the body envelope | 0.55 (0.52 to 0.58, range 0.46 to 0.82) | 0.58 (0.54 to 0.62, range 0.41 to 0.74) | yes | 0.00 |
| Rise of the body envelope at the strong shocks, median, dB | 1.6 (-0.4 to 4.1, range -2.7 to 9.1) | 3.0 (1.3 to 4.2, range -1.6 to 8.6) | yes | 0.0 |
| Rise of the body envelope at random times, median, dB | 0.5 (-1.3 to 2.5, range -4.9 to 20.1) | 2.1 (0.2 to 3.8, range -4.3 to 9.0) | yes | 0.0 |
| Correlation of the shock level and the rise of the body envelope | 0.13 (-0.00 to 0.30, range -0.27 to 0.45) | 0.09 (-0.05 to 0.27, range -0.34 to 0.53) | yes | 0.00 |
| Mean resultant length of the shock phases in the body cycle | 0.15 (0.08 to 0.22, range 0.04 to 0.28) | 0.15 (0.08 to 0.20, range 0.02 to 0.41) | yes | 0.00 |
| Mean resultant length of the strong shock gaps in body cycles | 0.33 (0.24 to 0.59, range 0.11 to 0.91), n=28 | 0.32 (0.24 to 0.48, range 0.09 to 0.93), n=57 | yes | 0.00 |
| Spread over the mean of the gaps between shocks | 0.67 (0.52 to 0.95, range 0.44 to 1.62) | 0.62 (0.58 to 0.67, range 0.49 to 0.78) | yes | 0.00 |
| Fano factor of the shock count in 50 ms windows | 0.89 (0.69 to 1.20, range 0.24 to 1.77) | 0.77 (0.57 to 0.94, range 0.26 to 1.53) | yes | 0.00 |
| Correlation of the logarithms of consecutive gaps | 0.03 (-0.12 to 0.08, range -0.33 to 0.30) | -0.02 (-0.12 to 0.07, range -0.41 to 0.38) | yes | 0.00 |

Shocks, pooled over the 30 clips (1,251 shocks, 231 strong) and the 60 trains:

| Shocks | Measure | Clips | Model | Pass | Miss |
| --- | --- | --- | --- | --- | --- |
| All | Gap to the shock before, ms | 8.9 (5.8 to 13.2, range 0.7 to 169.0), n=1221 | 8.4 (5.6 to 13.0, range 0.0 to 42.4), n=2580 | yes | 0.0 |
| All | Level against the strongest shock of the clip, dB | -21.0 (-24.0 to -15.1, range -30.0 to 0.0) | -18.7 (-21.7 to -14.4, range -29.8 to 0.0) | yes | 0.0 |
| All | Full-band sample peak, dBFS | -7.3 (-10.3 to -5.0, range -22.7 to -0.3) | -9.0 (-12.2 to -5.9, range -24.7 to -0.4) | yes | 0.0 |
| All | Rise above 1 kHz, 10 to 90 per cent, ms | 2.31 (1.22 to 4.08, range 0.00 to 17.60) | 2.68 (1.38 to 4.33, range 0.05 to 22.31) | yes | 0.00 |
| All | Full-band rise, 10 to 90 per cent, ms | 2.04 (0.57 to 3.98, range 0.00 to 17.78) | 2.31 (0.63 to 4.31, range 0.00 to 19.86) | yes | 0.00 |
| All | Fall above 1 kHz to -10 dB, ms | 0.59 (0.36 to 1.04, range 0.11 to 4.97), n=1248 | 0.68 (0.50 to 1.07, range 0.14 to 4.58), n=2634 | yes | 0.00 |
| All | Crest factor over the shock span, dB | 5.7 (4.6 to 7.3, range 0.8 to 14.7) | 5.9 (4.8 to 7.3, range 0.0 to 15.5) | yes | 0.0 |
| Strong | Gap to the strong shock before, ms | 32.2 (12.5 to 77.9, range 2.0 to 356.2) | 28.5 (13.1 to 59.2, range 3.3 to 338.3) | yes | 0.0 |
| Strong | Level against the strongest shock of the clip, dB | -5.6 (-8.7 to -2.0, range -12.0 to 0.0) | -5.8 (-9.1 to -2.6, range -12.0 to 0.0) | yes | 0.0 |
| Strong | Full-band sample peak, dBFS | -2.5 (-4.0 to -0.5, range -11.4 to -0.3) | -4.3 (-6.8 to -2.2, range -14.0 to -0.4) | no | -0.3 |
| Strong | Rise above 1 kHz, 10 to 90 per cent, ms | 2.49 (1.17 to 4.23, range 0.20 to 10.34) | 3.99 (2.73 to 6.00, range 0.27 to 14.90) | yes | 0.00 |
| Strong | Full-band rise, 10 to 90 per cent, ms | 1.79 (0.67 to 3.68, range 0.02 to 9.41) | 3.56 (1.93 to 6.20, range 0.00 to 19.86) | yes | 0.00 |
| Strong | Fall above 1 kHz to -10 dB, ms | 0.57 (0.39 to 1.16, range 0.14 to 3.65) | 0.91 (0.50 to 1.38, range 0.14 to 4.31) | yes | 0.00 |
| Strong | Crest factor over the shock span, dB | 8.5 (7.0 to 9.6, range 3.6 to 13.2) | 8.5 (7.1 to 9.8, range 3.9 to 15.5) | yes | 0.0 |

Band shares, in dB. The shock windows are pooled over the shocks, the other two columns are over the clips or trains:

| Band, Hz | Part | Clips | Model | Pass | Miss |
| --- | --- | --- | --- | --- | --- |
| 15.625-31.25 | Strong shock windows | -21.3 (-30.1 to -15.5, range -68.7 to -4.9) | -17.1 (-21.3 to -13.9, range -73.6 to -5.0) | yes | 0.0 |
| 15.625-31.25 | All shock windows | -16.4 (-22.0 to -13.0, range -68.7 to -2.5) | -15.7 (-19.5 to -12.5, range -73.6 to -4.5) | yes | 0.0 |
| 15.625-31.25 | Between the shocks | -14.5 (-15.4 to -13.1, range -18.4 to -9.4) | -13.4 (-14.5 to -12.7, range -16.1 to -10.3) | yes | 0.0 |
| 15.625-31.25 | Active span | -14.9 (-15.6 to -13.6, range -17.5 to -10.0) | -14.0 (-14.7 to -13.5, range -16.3 to -12.0) | yes | 0.0 |
| 31.25-62.5 | Strong shock windows | -7.0 (-14.2 to -4.2, range -52.0 to -0.9) | -3.6 (-6.9 to -2.2, range -52.4 to -0.5) | no | 0.5 |
| 31.25-62.5 | All shock windows | -2.5 (-5.7 to -1.4, range -52.0 to -0.4) | -2.7 (-4.8 to -1.6, range -52.4 to -0.4) | yes | 0.0 |
| 31.25-62.5 | Between the shocks | -2.8 (-3.2 to -2.4, range -4.3 to -2.0) | -2.6 (-2.8 to -2.3, range -3.8 to -1.7) | yes | 0.0 |
| 31.25-62.5 | Active span | -2.7 (-3.2 to -2.5, range -4.0 to -2.1) | -2.5 (-2.7 to -2.2, range -3.4 to -1.5) | no | 0.0 |
| 62.5-125 | Strong shock windows | -10.1 (-14.8 to -5.8, range -31.9 to -1.3) | -7.5 (-10.9 to -5.7, range -39.4 to -1.3) | yes | 0.0 |
| 62.5-125 | All shock windows | -7.4 (-11.1 to -4.9, range -33.1 to -0.7) | -6.5 (-9.2 to -4.6, range -39.4 to -1.0) | yes | 0.0 |
| 62.5-125 | Between the shocks | -6.9 (-7.5 to -6.4, range -8.3 to -4.9) | -6.6 (-7.0 to -6.3, range -8.3 to -5.2) | yes | 0.0 |
| 62.5-125 | Active span | -6.8 (-7.2 to -6.5, range -8.2 to -5.2) | -6.5 (-6.8 to -6.1, range -7.7 to -5.2) | yes | 0.0 |
| 125-250 | Strong shock windows | -14.6 (-18.3 to -11.8, range -29.0 to -2.6) | -17.9 (-20.8 to -14.4, range -30.8 to -4.2) | yes | 0.0 |
| 125-250 | All shock windows | -18.2 (-22.0 to -13.4, range -33.6 to -2.6) | -18.0 (-21.3 to -14.7, range -36.7 to -3.4) | yes | 0.0 |
| 125-250 | Between the shocks | -15.0 (-16.3 to -13.9, range -17.6 to -11.3) | -16.1 (-17.0 to -14.8, range -20.0 to -12.7) | yes | 0.0 |
| 125-250 | Active span | -15.6 (-16.7 to -14.5, range -17.5 to -11.4) | -16.7 (-17.6 to -15.9, range -20.9 to -13.8) | yes | 0.0 |
| 250-500 | Strong shock windows | -9.5 (-12.6 to -6.1, range -27.8 to -1.7) | -12.6 (-16.2 to -9.6, range -28.2 to -2.5) | yes | 0.0 |
| 250-500 | All shock windows | -14.5 (-19.6 to -8.9, range -34.6 to -1.0) | -14.0 (-17.4 to -10.7, range -28.2 to -2.5) | yes | 0.0 |
| 250-500 | Between the shocks | -9.3 (-10.6 to -8.7, range -12.0 to -6.3) | -11.7 (-12.7 to -10.3, range -15.5 to -9.0) | no | -1.1 |
| 250-500 | Active span | -10.0 (-11.0 to -9.5, range -12.9 to -6.8) | -12.5 (-13.8 to -11.4, range -16.7 to -9.9) | no | -1.5 |
| 500-1k | Strong shock windows | -9.0 (-11.7 to -7.1, range -26.7 to -3.5) | -11.8 (-15.0 to -9.3, range -22.1 to -4.4) | no | -0.1 |
| 500-1k | All shock windows | -17.6 (-22.8 to -11.2, range -35.5 to -3.4) | -13.5 (-16.8 to -10.5, range -29.5 to -2.8) | yes | 0.0 |
| 500-1k | Between the shocks | -14.6 (-15.3 to -13.9, range -17.3 to -11.0) | -14.2 (-14.7 to -13.3, range -16.8 to -11.8) | yes | 0.0 |
| 500-1k | Active span | -14.0 (-14.4 to -13.1, range -14.8 to -10.7) | -14.1 (-14.8 to -13.4, range -17.4 to -11.6) | yes | 0.0 |
| 1k-2k | Strong shock windows | -11.3 (-14.0 to -8.7, range -28.4 to -3.0) | -13.1 (-17.7 to -10.0, range -29.4 to -4.6) | yes | 0.0 |
| 1k-2k | All shock windows | -21.3 (-25.3 to -15.8, range -36.6 to -3.0) | -19.7 (-23.1 to -16.0, range -36.2 to -4.6) | yes | 0.0 |
| 1k-2k | Between the shocks | -17.7 (-18.5 to -16.7, range -22.4 to -13.2) | -18.4 (-19.3 to -17.2, range -21.5 to -15.6) | yes | 0.0 |
| 1k-2k | Active span | -16.7 (-17.4 to -14.9, range -19.2 to -11.5) | -17.4 (-18.4 to -16.6, range -22.4 to -14.0) | yes | 0.0 |
| 2k-4k | Strong shock windows | -13.8 (-17.1 to -10.3, range -28.0 to -4.1) | -14.1 (-19.7 to -10.4, range -36.9 to -4.5) | yes | 0.0 |
| 2k-4k | All shock windows | -24.3 (-28.2 to -19.4, range -38.9 to -4.1) | -27.0 (-31.2 to -20.9, range -46.2 to -4.5) | yes | 0.0 |
| 2k-4k | Between the shocks | -19.7 (-20.5 to -18.2, range -24.0 to -15.1) | -19.6 (-21.2 to -18.7, range -24.5 to -16.9) | yes | 0.0 |
| 2k-4k | Active span | -18.3 (-18.8 to -16.8, range -20.9 to -13.5) | -18.9 (-20.4 to -17.7, range -24.8 to -15.4) | no | -0.0 |
| 4k-8k | Strong shock windows | -20.3 (-25.3 to -15.6, range -34.5 to -7.0) | -17.1 (-22.5 to -13.2, range -42.5 to -8.2) | yes | 0.0 |
| 4k-8k | All shock windows | -30.5 (-35.1 to -24.8, range -49.7 to -7.0) | -29.7 (-34.8 to -23.3, range -54.2 to -8.2) | yes | 0.0 |
| 4k-8k | Between the shocks | -22.3 (-26.0 to -21.0, range -29.2 to -17.0) | -19.9 (-21.5 to -19.1, range -24.8 to -16.2) | no | 1.1 |
| 4k-8k | Active span | -22.1 (-23.5 to -19.8, range -29.8 to -15.9) | -20.5 (-22.0 to -19.3, range -27.0 to -16.4) | yes | 0.0 |
| 8k-16k | Strong shock windows | -32.8 (-39.2 to -24.7, range -54.6 to -11.1) | -24.1 (-29.9 to -20.7, range -50.4 to -15.0) | no | 0.6 |
| 8k-16k | All shock windows | -43.0 (-47.8 to -36.8, range -62.9 to -11.1) | -38.5 (-44.2 to -31.2, range -66.6 to -15.0) | yes | 0.0 |
| 8k-16k | Between the shocks | -30.7 (-34.6 to -27.9, range -41.8 to -22.7) | -28.8 (-30.3 to -28.0, range -33.6 to -25.5) | yes | 0.0 |
| 8k-16k | Active span | -29.5 (-31.2 to -25.9, range -42.6 to -21.5) | -28.8 (-30.1 to -27.5, range -35.4 to -24.5) | yes | 0.0 |
| 16k-20k | Strong shock windows | -58.7 (-66.9 to -51.3, range -83.3 to -34.6) | -48.5 (-54.2 to -44.5, range -82.7 to -37.9) | no | 2.7 |
| 16k-20k | All shock windows | -69.6 (-74.9 to -62.7, range -85.8 to -27.0) | -65.2 (-73.3 to -56.2, range -100.3 to -37.9) | yes | 0.0 |
| 16k-20k | Between the shocks | -54.4 (-58.4 to -49.3, range -67.6 to -44.0) | -55.7 (-57.0 to -54.4, range -61.5 to -52.6) | yes | 0.0 |
| 16k-20k | Active span | -53.5 (-55.6 to -49.1, range -68.5 to -43.5) | -53.8 (-55.3 to -52.7, range -60.3 to -49.7) | yes | 0.0 |

Band energy over time, in dB against the loudest 5 ms frame of a clip or train, at the time of the frame from the frame of the strongest shock:

| ms | Band, Hz | Clips | Model | Pass | Miss |
| --- | --- | --- | --- | --- | --- |
| -10 | 15.625-31.25 | -22.9 (-29.1 to -21.1, range -63.9 to -15.0) | -24.6 (-30.2 to -21.8, range -60.6 to -15.8) | yes | 0.0 |
| -10 | 31.25-62.5 | -9.9 (-15.3 to -4.7, range -46.7 to -3.2) | -12.4 (-17.6 to -9.2, range -39.0 to -2.1) | yes | 0.0 |
| -10 | 62.5-125 | -16.4 (-25.4 to -11.8, range -48.0 to -6.5) | -15.0 (-20.4 to -11.8, range -32.4 to -6.6) | yes | 0.0 |
| -10 | 125-250 | -30.0 (-33.0 to -20.7, range -62.0 to -17.5) | -26.0 (-30.1 to -23.0, range -34.3 to -16.4) | yes | 0.0 |
| -10 | 250-500 | -24.3 (-27.7 to -21.1, range -72.4 to -12.5) | -22.2 (-24.4 to -19.1, range -34.0 to -12.6) | yes | 0.0 |
| -10 | 500-1k | -27.0 (-30.1 to -25.7, range -73.9 to -14.0) | -22.0 (-24.5 to -20.6, range -27.9 to -14.8) | no | 3.7 |
| -10 | 1k-2k | -29.6 (-31.5 to -27.0, range -80.4 to -15.9) | -28.3 (-30.1 to -25.5, range -33.6 to -14.0) | yes | 0.0 |
| -10 | 2k-4k | -33.1 (-35.5 to -30.8, range -91.3 to -18.9) | -31.9 (-36.1 to -25.9, range -41.4 to -15.0) | yes | 0.0 |
| -10 | 4k-8k | -41.2 (-43.9 to -35.5, range -88.7 to -21.7) | -33.5 (-37.7 to -27.6, range -44.1 to -17.3) | no | 2.0 |
| -10 | 8k-16k | -54.1 (-58.0 to -46.4, range -81.9 to -31.4) | -40.6 (-47.4 to -35.1, range -54.9 to -25.0) | no | 5.7 |
| -10 | 16k-20k | -81.1 (-85.2 to -72.0, range -91.3 to -56.1) | -68.5 (-74.8 to -60.3, range -91.3 to -48.9) | no | 3.5 |
| -10 | above 2k | -31.9 (-34.9 to -29.4, range -80.3 to -16.9) | -29.1 (-33.9 to -23.4, range -39.2 to -12.8) | no | 0.2 |
| -5 | 15.625-31.25 | -26.1 (-28.2 to -19.3, range -48.0 to -15.5) | -27.4 (-30.5 to -22.8, range -56.2 to -14.1) | yes | 0.0 |
| -5 | 31.25-62.5 | -10.3 (-16.3 to -7.5, range -33.5 to -4.4) | -12.9 (-17.4 to -8.6, range -41.9 to -3.7) | yes | 0.0 |
| -5 | 62.5-125 | -17.5 (-20.6 to -14.4, range -40.3 to -10.5) | -16.1 (-21.5 to -12.2, range -34.3 to -7.4) | yes | 0.0 |
| -5 | 125-250 | -26.2 (-29.3 to -21.3, range -63.7 to -11.5) | -23.9 (-28.8 to -21.6, range -34.5 to -14.1) | yes | 0.0 |
| -5 | 250-500 | -26.5 (-30.0 to -22.1, range -65.0 to -11.3) | -22.2 (-24.0 to -18.8, range -30.7 to -9.9) | yes | 0.0 |
| -5 | 500-1k | -28.7 (-31.2 to -25.4, range -60.7 to -9.8) | -21.8 (-23.5 to -19.2, range -26.3 to -11.9) | no | 3.7 |
| -5 | 1k-2k | -30.8 (-32.6 to -26.9, range -60.6 to -9.3) | -27.4 (-30.0 to -22.6, range -32.5 to -8.9) | yes | 0.0 |
| -5 | 2k-4k | -34.6 (-36.6 to -29.0, range -65.6 to -15.3) | -31.4 (-37.4 to -23.6, range -40.7 to -10.9) | yes | 0.0 |
| -5 | 4k-8k | -40.4 (-43.2 to -35.8, range -66.1 to -23.4) | -32.3 (-39.1 to -26.7, range -44.7 to -11.9) | no | 3.5 |
| -5 | 8k-16k | -50.6 (-54.9 to -44.7, range -74.0 to -31.3) | -40.5 (-47.0 to -33.9, range -54.1 to -19.4) | no | 4.2 |
| -5 | 16k-20k | -74.8 (-79.6 to -66.1, range -90.7 to -52.6) | -66.7 (-75.1 to -60.2, range -85.5 to -44.1) | yes | 0.0 |
| -5 | above 2k | -34.0 (-35.5 to -28.2, range -57.2 to -14.6) | -28.6 (-34.4 to -21.0, range -38.7 to -8.1) | yes | 0.0 |
| 0 | 15.625-31.25 | -23.0 (-27.7 to -20.9, range -84.3 to -12.3) | -26.6 (-33.3 to -20.8, range -69.7 to -15.5) | yes | 0.0 |
| 0 | 31.25-62.5 | -12.0 (-16.9 to -4.2, range -64.7 to -2.4) | -13.0 (-17.2 to -8.5, range -52.5 to -1.8) | yes | 0.0 |
| 0 | 62.5-125 | -15.7 (-20.2 to -13.1, range -50.8 to -7.3) | -15.3 (-19.6 to -12.3, range -45.1 to -5.2) | yes | 0.0 |
| 0 | 125-250 | -25.6 (-27.8 to -19.7, range -40.2 to -9.7) | -24.3 (-26.3 to -21.4, range -34.6 to -9.6) | yes | 0.0 |
| 0 | 250-500 | -19.4 (-22.4 to -17.7, range -34.2 to -10.6) | -20.6 (-22.7 to -17.0, range -29.7 to -4.7) | yes | 0.0 |
| 0 | 500-1k | -17.8 (-26.0 to -11.6, range -29.1 to -8.7) | -19.2 (-21.7 to -16.3, range -26.5 to -8.7) | yes | 0.0 |
| 0 | 1k-2k | -19.0 (-27.4 to -9.4, range -29.9 to -4.2) | -18.7 (-24.8 to -13.6, range -32.3 to -8.3) | yes | 0.0 |
| 0 | 2k-4k | -18.7 (-27.8 to -9.9, range -31.7 to -5.5) | -18.1 (-25.3 to -13.8, range -39.5 to -7.7) | yes | 0.0 |
| 0 | 4k-8k | -19.8 (-32.4 to -13.8, range -41.7 to -6.7) | -22.4 (-29.6 to -18.4, range -44.8 to -10.1) | yes | 0.0 |
| 0 | 8k-16k | -22.6 (-40.4 to -19.9, range -53.2 to -10.7) | -28.3 (-36.1 to -25.0, range -54.2 to -19.2) | yes | 0.0 |
| 0 | 16k-20k | -48.4 (-60.2 to -44.9, range -82.9 to -34.4) | -52.0 (-58.2 to -48.8, range -86.0 to -43.2) | yes | 0.0 |
| 0 | above 2k | -15.0 (-26.0 to -8.1, range -30.5 to -2.9) | -16.2 (-23.7 to -12.5, range -38.2 to -6.0) | yes | 0.0 |
| 5 | 15.625-31.25 | -24.5 (-28.4 to -20.3, range -64.9 to -14.2) | -24.8 (-32.2 to -21.2, range -52.4 to -15.0) | yes | 0.0 |
| 5 | 31.25-62.5 | -10.7 (-18.4 to -9.0, range -42.7 to -4.6) | -10.8 (-18.7 to -8.1, range -44.9 to -3.6) | yes | 0.0 |
| 5 | 62.5-125 | -19.5 (-21.4 to -14.7, range -36.1 to -7.3) | -15.0 (-18.7 to -10.7, range -33.7 to -4.4) | yes | 0.0 |
| 5 | 125-250 | -17.3 (-18.9 to -14.5, range -27.3 to -8.9) | -20.4 (-24.4 to -17.6, range -31.2 to -9.8) | no | -1.4 |
| 5 | 250-500 | -10.5 (-12.6 to -7.9, range -21.2 to -4.5) | -13.9 (-17.9 to -10.0, range -27.8 to -4.9) | no | -1.3 |
| 5 | 500-1k | -11.1 (-13.1 to -9.2, range -23.4 to -7.4) | -12.5 (-15.6 to -10.5, range -22.9 to -4.2) | yes | 0.0 |
| 5 | 1k-2k | -10.8 (-13.9 to -8.4, range -27.1 to -5.8) | -13.3 (-16.9 to -10.6, range -25.1 to -5.2) | yes | 0.0 |
| 5 | 2k-4k | -11.7 (-15.2 to -9.8, range -28.1 to -8.8) | -14.6 (-17.2 to -11.8, range -26.0 to -6.9) | yes | 0.0 |
| 5 | 4k-8k | -15.6 (-18.7 to -14.1, range -36.6 to -10.4) | -16.1 (-18.6 to -14.0, range -32.6 to -8.7) | yes | 0.0 |
| 5 | 8k-16k | -23.3 (-26.8 to -20.9, range -51.0 to -13.7) | -24.4 (-26.6 to -21.9, range -38.5 to -16.4) | yes | 0.0 |
| 5 | 16k-20k | -46.4 (-51.6 to -43.4, range -75.8 to -37.9) | -50.3 (-52.8 to -46.0, range -62.7 to -40.2) | yes | 0.0 |
| 5 | above 2k | -10.1 (-13.1 to -8.0, range -27.3 to -6.4) | -11.9 (-14.3 to -9.4, range -24.7 to -5.1) | yes | 0.0 |
| 10 | 15.625-31.25 | -22.7 (-33.5 to -17.1, range -50.7 to -13.6) | -23.9 (-30.4 to -20.4, range -45.9 to -14.1) | yes | 0.0 |
| 10 | 31.25-62.5 | -13.8 (-17.0 to -4.2, range -43.7 to -2.4) | -11.5 (-17.6 to -8.3, range -34.3 to -2.8) | yes | 0.0 |
| 10 | 62.5-125 | -14.4 (-23.5 to -11.5, range -31.2 to -9.0) | -15.2 (-18.9 to -11.5, range -36.7 to -6.4) | yes | 0.0 |
| 10 | 125-250 | -16.5 (-21.9 to -14.1, range -30.0 to -11.2) | -17.3 (-19.8 to -12.9, range -26.8 to -6.9) | yes | 0.0 |
| 10 | 250-500 | -10.2 (-11.8 to -5.9, range -17.1 to -2.9) | -10.9 (-13.5 to -8.0, range -20.5 to -2.8) | yes | 0.0 |
| 10 | 500-1k | -13.9 (-17.3 to -12.6, range -23.7 to -7.7) | -15.0 (-17.2 to -12.5, range -24.3 to -6.6) | yes | 0.0 |
| 10 | 1k-2k | -17.3 (-19.2 to -15.4, range -25.5 to -12.1) | -19.3 (-23.5 to -15.8, range -31.5 to -10.1) | no | -0.1 |
| 10 | 2k-4k | -19.4 (-21.7 to -17.4, range -24.6 to -10.8) | -19.9 (-23.3 to -16.3, range -29.1 to -11.3) | yes | 0.0 |
| 10 | 4k-8k | -20.7 (-23.8 to -18.5, range -28.8 to -13.1) | -18.8 (-21.1 to -15.8, range -25.8 to -9.8) | yes | 0.0 |
| 10 | 8k-16k | -29.5 (-31.7 to -23.8, range -37.0 to -18.2) | -27.9 (-30.6 to -24.5, range -36.3 to -19.5) | yes | 0.0 |
| 10 | 16k-20k | -52.2 (-54.7 to -47.2, range -65.7 to -39.4) | -57.2 (-61.4 to -51.0, range -70.3 to -46.8) | no | -2.4 |
| 10 | above 2k | -16.3 (-18.6 to -15.0, range -23.0 to -8.3) | -16.0 (-18.8 to -12.7, range -23.6 to -7.2) | yes | 0.0 |
| 20 | 15.625-31.25 | -22.6 (-28.3 to -17.7, range -48.5 to -13.4) | -24.5 (-27.6 to -21.5, range -39.2 to -14.1) | yes | 0.0 |
| 20 | 31.25-62.5 | -12.9 (-21.3 to -5.7, range -34.8 to -2.6) | -11.6 (-17.2 to -7.6, range -25.8 to -2.7) | yes | 0.0 |
| 20 | 62.5-125 | -14.0 (-16.0 to -12.8, range -27.9 to -3.0) | -14.8 (-19.4 to -11.3, range -25.8 to -5.4) | yes | 0.0 |
| 20 | 125-250 | -19.7 (-21.8 to -17.3, range -32.6 to -12.7) | -18.4 (-21.4 to -15.8, range -30.4 to -10.4) | yes | 0.0 |
| 20 | 250-500 | -16.9 (-21.0 to -15.2, range -24.8 to -8.3) | -17.3 (-19.9 to -13.1, range -27.5 to -7.1) | yes | 0.0 |
| 20 | 500-1k | -22.3 (-25.6 to -19.9, range -31.0 to -14.2) | -20.3 (-22.2 to -18.4, range -27.5 to -11.2) | yes | 0.0 |
| 20 | 1k-2k | -27.7 (-30.9 to -23.1, range -35.4 to -11.6) | -26.2 (-28.2 to -23.7, range -32.7 to -11.8) | yes | 0.0 |
| 20 | 2k-4k | -26.4 (-28.0 to -23.1, range -31.9 to -12.5) | -28.2 (-32.2 to -24.6, range -38.3 to -11.6) | no | -0.2 |
| 20 | 4k-8k | -24.5 (-27.9 to -23.7, range -31.7 to -12.1) | -26.3 (-30.1 to -23.1, range -36.8 to -12.8) | yes | 0.0 |
| 20 | 8k-16k | -34.0 (-35.8 to -29.3, range -43.2 to -24.6) | -35.7 (-40.1 to -32.3, range -46.9 to -22.2) | yes | 0.0 |
| 20 | 16k-20k | -57.6 (-61.8 to -54.4, range -70.7 to -43.1) | -67.9 (-71.3 to -62.1, range -80.6 to -48.1) | no | -6.2 |
| 20 | above 2k | -22.1 (-24.1 to -19.9, range -27.1 to -9.2) | -23.7 (-27.6 to -20.9, range -33.8 to -9.6) | yes | 0.0 |
| 40 | 15.625-31.25 | -24.9 (-27.4 to -20.0, range -33.5 to -13.9) | -23.3 (-30.2 to -19.8, range -43.3 to -15.3) | yes | 0.0 |
| 40 | 31.25-62.5 | -11.3 (-12.7 to -9.8, range -26.9 to -6.4) | -10.9 (-16.8 to -8.1, range -31.0 to -2.1) | yes | 0.0 |
| 40 | 62.5-125 | -11.1 (-14.2 to -9.5, range -17.8 to -6.2) | -16.2 (-18.1 to -10.5, range -36.1 to -4.9) | no | -1.9 |
| 40 | 125-250 | -26.4 (-29.8 to -23.7, range -40.1 to -19.0) | -25.7 (-28.6 to -23.5, range -34.3 to -12.2) | yes | 0.0 |
| 40 | 250-500 | -27.2 (-31.9 to -21.1, range -42.0 to -12.4) | -22.3 (-24.1 to -20.0, range -35.8 to -6.0) | yes | 0.0 |
| 40 | 500-1k | -30.1 (-34.1 to -23.4, range -41.5 to -11.4) | -22.4 (-23.7 to -20.6, range -26.7 to -15.5) | no | 1.0 |
| 40 | 1k-2k | -29.9 (-33.0 to -24.9, range -40.0 to -13.8) | -29.0 (-31.1 to -25.8, range -36.6 to -15.5) | yes | 0.0 |
| 40 | 2k-4k | -30.4 (-33.3 to -25.5, range -39.8 to -19.1) | -35.4 (-37.2 to -29.7, range -41.9 to -19.3) | no | -2.0 |
| 40 | 4k-8k | -29.1 (-32.9 to -27.8, range -38.9 to -22.5) | -35.6 (-39.0 to -29.8, range -47.3 to -16.7) | no | -2.7 |
| 40 | 8k-16k | -40.9 (-45.3 to -37.1, range -47.5 to -32.7) | -44.8 (-48.2 to -38.4, range -57.9 to -25.4) | yes | 0.0 |
| 40 | 16k-20k | -65.6 (-71.9 to -63.0, range -77.1 to -54.2) | -72.5 (-75.8 to -66.9, range -93.2 to -53.1) | no | -0.7 |
| 40 | above 2k | -26.4 (-29.7 to -23.4, range -35.5 to -18.5) | -31.7 (-35.0 to -26.8, range -39.8 to -14.5) | no | -2.0 |
| 80 | 15.625-31.25 | -22.4 (-24.4 to -20.3, range -39.1 to -14.5) | -24.2 (-29.5 to -20.5, range -47.7 to -14.2) | yes | 0.0 |
| 80 | 31.25-62.5 | -9.4 (-12.0 to -7.8, range -21.1 to -5.0) | -11.0 (-16.9 to -7.1, range -32.0 to -2.6) | yes | 0.0 |
| 80 | 62.5-125 | -16.0 (-18.3 to -13.4, range -28.3 to -7.7) | -15.2 (-18.9 to -12.1, range -38.6 to -6.5) | yes | 0.0 |
| 80 | 125-250 | -27.2 (-29.4 to -21.9, range -34.5 to -18.9) | -25.3 (-29.0 to -23.1, range -49.5 to -11.8) | yes | 0.0 |
| 80 | 250-500 | -23.6 (-28.9 to -19.3, range -38.4 to -14.5) | -22.8 (-25.1 to -20.4, range -41.3 to -9.0) | yes | 0.0 |
| 80 | 500-1k | -28.6 (-30.8 to -27.4, range -40.8 to -19.0) | -22.4 (-24.3 to -20.4, range -42.0 to -15.2) | no | 5.0 |
| 80 | 1k-2k | -29.4 (-31.7 to -27.3, range -53.0 to -22.7) | -28.8 (-30.5 to -25.7, range -52.2 to -15.0) | yes | 0.0 |
| 80 | 2k-4k | -33.9 (-35.8 to -29.4, range -65.6 to -23.7) | -36.0 (-39.6 to -29.7, range -61.2 to -17.4) | no | -0.2 |
| 80 | 4k-8k | -37.9 (-39.7 to -34.0, range -82.0 to -30.1) | -36.3 (-43.7 to -29.0, range -67.7 to -18.7) | yes | 0.0 |
| 80 | 8k-16k | -52.1 (-53.7 to -46.9, range -83.6 to -41.6) | -45.9 (-53.2 to -37.9, range -79.7 to -26.5) | no | 1.1 |
| 80 | 16k-20k | -80.6 (-84.0 to -74.1, range -90.4 to -58.4) | -73.4 (-82.3 to -67.1, range -114.7 to -49.5) | no | 0.7 |
| 80 | above 2k | -32.4 (-34.0 to -27.6, range -65.4 to -22.8) | -32.6 (-38.0 to -26.3, range -60.3 to -15.2) | yes | 0.0 |
| 160 | 15.625-31.25 | -25.1 (-27.5 to -21.0, range -44.4 to -16.7) | -22.8 (-26.7 to -20.6, range -43.9 to -14.5) | yes | 0.0 |
| 160 | 31.25-62.5 | -13.4 (-18.2 to -6.0, range -37.3 to -4.1) | -15.5 (-19.1 to -8.4, range -39.3 to -1.3) | yes | 0.0 |
| 160 | 62.5-125 | -18.0 (-25.6 to -13.5, range -77.2 to -7.5) | -15.9 (-20.3 to -13.6, range -75.3 to -7.8) | yes | 0.0 |
| 160 | 125-250 | -31.6 (-37.0 to -27.1, range -98.3 to -13.5) | -26.0 (-30.1 to -23.1, range -96.3 to -18.1) | no | 1.1 |
| 160 | 250-500 | -28.1 (-39.9 to -25.4, range -101.6 to -8.9) | -22.1 (-24.7 to -19.9, range -81.9 to -11.6) | no | 3.2 |
| 160 | 500-1k | -30.1 (-44.3 to -22.9, range -106.7 to -12.9) | -22.7 (-24.0 to -20.8, range -80.1 to -12.9) | no | 0.2 |
| 160 | 1k-2k | -33.5 (-53.1 to -26.7, range -105.7 to -18.2) | -29.4 (-31.1 to -27.2, range -86.7 to -15.1) | yes | 0.0 |
| 160 | 2k-4k | -34.7 (-65.8 to -31.6, range -103.0 to -24.2) | -36.0 (-39.4 to -32.4, range -102.7 to -13.9) | yes | 0.0 |
| 160 | 4k-8k | -43.7 (-79.7 to -37.3, range -100.3 to -31.9) | -37.3 (-43.1 to -32.6, range -107.3 to -17.1) | yes | 0.0 |
| 160 | 8k-16k | -55.8 (-78.9 to -52.5, range -96.0 to -40.8) | -45.7 (-52.6 to -40.7, range -117.5 to -25.0) | no | 6.8 |
| 160 | 16k-20k | -82.9 (-90.1 to -77.2, range -99.4 to -62.4) | -74.3 (-84.9 to -68.3, range -153.7 to -49.4) | no | 2.8 |
| 160 | above 2k | -32.6 (-65.5 to -29.8, range -92.9 to -23.7) | -32.8 (-37.9 to -28.7, range -101.3 to -12.4) | yes | 0.0 |
| 320 | 15.625-31.25 | -25.2 (-32.6 to -20.4, range -71.1 to -17.3) | -23.8 (-27.8 to -19.4, range -40.4 to -14.1) | yes | 0.0 |
| 320 | 31.25-62.5 | -15.5 (-18.1 to -6.3, range -70.6 to -3.7) | -13.2 (-17.2 to -9.6, range -38.7 to -1.7) | yes | 0.0 |
| 320 | 62.5-125 | -16.6 (-25.7 to -11.2, range -91.8 to -8.0) | -18.1 (-24.5 to -13.4, range -88.1 to -7.3) | yes | 0.0 |
| 320 | 125-250 | -28.1 (-38.3 to -26.0, range -99.2 to -17.7) | -28.3 (-38.5 to -25.1, range -100.4 to -18.6) | yes | 0.0 |
| 320 | 250-500 | -38.3 (-48.0 to -22.6, range -104.1 to -15.4) | -24.7 (-28.1 to -23.5, range -83.9 to -15.8) | yes | 0.0 |
| 320 | 500-1k | -34.9 (-67.4 to -25.8, range -107.1 to -15.1) | -24.2 (-33.3 to -22.5, range -83.4 to -14.3) | no | 1.6 |
| 320 | 1k-2k | -37.3 (-77.7 to -32.3, range -103.6 to -22.9) | -30.8 (-40.0 to -28.8, range -92.8 to -16.7) | no | 1.4 |
| 320 | 2k-4k | -38.5 (-97.8 to -35.8, range -101.0 to -23.9) | -40.4 (-45.8 to -35.0, range -102.8 to -17.7) | yes | 0.0 |
| 320 | 4k-8k | -46.0 (-96.0 to -41.5, range -98.5 to -25.2) | -43.9 (-53.9 to -35.3, range -110.2 to -20.9) | yes | 0.0 |
| 320 | 8k-16k | -59.3 (-93.9 to -53.0, range -95.6 to -33.6) | -53.8 (-65.2 to -44.4, range -120.3 to -29.2) | yes | 0.0 |
| 320 | 16k-20k | -85.4 (-97.3 to -82.0, range -98.3 to -55.4) | -83.3 (-100.9 to -73.7, range -157.4 to -55.6) | yes | 0.0 |
| 320 | above 2k | -37.8 (-89.8 to -35.0, range -91.4 to -21.3) | -38.7 (-45.1 to -30.7, range -102.0 to -15.8) | yes | 0.0 |

## 7. Cost

`BenchmarkVoice` renders trains for one second of audio at 44.1 kHz in blocks of 256 samples, with a new train when one ends. It takes 13.5 to 13.7 ms in three runs of 5 s on an AMD Ryzen 9 7940HS with Go 1.27.2: 1.4 per cent of one core for one voice. The three banks of 11 bands with two filter stages each and their carriers are most of the cost.

## 8. Reproduce

1. Run `just crackle-measure`. It writes the clip measures to `out/crackle/`, with the body and timing measures of section 2 in `summary.md`.
2. Run `just crackle-fit`. It writes `internal/crackle/crackle.toml` and `internal/crackle/crackle.sources.toml`, and the check of the 64 fit trains to `out/crackle/fit.md`. It takes about 5 minutes on 16 cores, and two runs give the same files.
3. Run `just crackle-model`. It writes 60 trains, `compare.md` (the check of section 6) and `summary.md` to `out/crackle/model/`.

`go test ./internal/crackle` checks the parameter file, the sources file, the determinism, the absence of allocations, the length control, the level control and the rate. `go test ./tools/crackle` checks the body and timing measures on test signals (a harmonic tone, narrow band noise, periodic and random times, even and locked phases), the bank fit, and the check command.
