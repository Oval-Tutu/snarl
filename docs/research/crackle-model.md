# Short exterior crackles: the crackle model and its check

Status: model, fit and check done, with the grain voice and the slow ring of phase 2b. Phase 3 connects the model to the engine as the voice of the snarl pops (section 9). Date: 2026-10-22. Branch `pop-model`. This report gives the evidence for the form of the crackle model, the model in `internal/crackle`, the fit in `tools/crackle --fit`, and the check of 240 model trains against the 30 short exterior crackle clips of [crackle-measure.md](crackle-measure.md) with the same measures. No audio and no measure of a single clip is in the repository. The model parameters are statistics and fits of the whole set.

## 1. Answer

The body near 52 Hz is not an engine tone and is not driven by the shocks. It is a narrow band of random sound with no harmonics and a Rayleigh envelope, independent of the shocks. The model keeps it as a separate part with its own gain. The shock times are random: a renewal process with a dead time of about 4 ms, with no period and no lock to the body. The levels of consecutive shocks correlate (0.29), so loud shocks come in groups.

The model is a random train of shocks over a bed and a body. Each shock is a sharp crack and a ring, each through its own bank of octave band filters. The bed is a third bank, and the activity of the shocks plays the ring bank as a slow ring that follows the shocks. The body is noise through a resonant band pass at 51 Hz. A damped sine at 27 Hz is the tail at the end of the train. The voice plays the shocks from a table of 64 rendered grains, and the bed, the ring and the body from rendered loops.

On 240 held-out trains with the seeds 5001 to 5240, which no step of the fit or of the selection used, the check passes 183 of 237 measures (77 per cent) with the model median inside the p25 to p75 of the clips. On the 235 measures of phase 2, it passes 181, against 169 for the phase 2 model on the same 240 trains. Two targets of phase 2b pass: the strong shock rise (3.02 ms, clip p75 3.44 ms) and the decay above 2 kHz to -20 dB (35 ms against 40 ms for the clips). The onset of the strongest shock misses by 0.1 ms (128.9 ms against a clip p25 of 129.0 ms), from 89.9 ms for the phase 2 model. Section 6 lists every miss.

One voice costs 0.47 ms per second of audio at 44.1 kHz: 0.05 per cent of one core, against 1.4 per cent in phase 2 and the target of 0.15 per cent. A bank takes 42 ms and 1.9 MB to make, once for all voices that share it.

The model closes the gaps that phase 1 found in the snarl pops:

| Measure | Clips | Model | Snarl pop kinds |
| --- | --- | --- | --- |
| Share above 2 kHz, dB | -16.6 | -16.2 | -24.3 to -27.4 |
| Crest factor of the strong shocks, pooled, dB | 8.5 | 8.4 | 6.3 to 7.0 |
| Body peak, Hz | 52 | 51 | 86 to 421 |
| Gap between strong shocks, pooled, ms | 32.2 | 20.3 | 8.0 to 10.2 |
| Share below 125 Hz, dB | -1.1 | -0.8 | -5.4 to -13.8 |

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
- A shock is a sharp crack. The fall measure (`FallMS`) is the time from the peak of the 0.25 ms RMS envelope above 1 kHz to 10 dB below it. It is 0.57 ms for the strong shocks. The band energy after the strongest shock stays high for 10 to 20 ms (phase 1, section 5), so the crack has a ring after it.

Decision: the shocks are a renewal process with the measured gap distribution, and levels from the measured level distribution with a correlation of consecutive levels. The shock times do not follow the body.

## 3. The model

`internal/crackle` is the model. `crackle.NewBank(params, rate, seed)` renders the sound of the model once at a sample rate. `crackle.New(bank, seed)` makes a voice that plays from the bank, `Voice.Trigger(Options)` starts a train, and `Voice.Process(block)` adds the train to a block of float32 samples. Many voices can share one bank, because a voice only reads it. The voice does not allocate (`TestNoAllocations`), and the output for a bank, a seed and a sequence of calls is the same on every run (`TestDeterministic`).

The bank holds:

- 64 shock grains. Each grain is one shock at 0 dB: a linear rise, then the crack with a fast decay and the ring with a slow decay, each on its own carriers through its own bank of 11 octave band filters, and the low pass at the top. A grain ends when the ring falls to -60 dB, at most 200 ms.
- Three loops of 2 s: the bed bank, the ring bank with its carriers at full amplitude, and the body. A cross fade of 50 ms joins the end of each loop to its start, so the loops wrap without a step (`TestBank`).

The carrier of a band is a sine at the band centre whose phase takes a random walk, so that it has the spectrum of a band of noise with a constant envelope. Each band has its own carrier, so the band powers add, and the bank fit is linear in the squared gains. White noise in the banks failed in phase 2: its envelope above 1 kHz rises and falls by several dB within a shock, and the shock detector of the measures counts each rise as a shock.

The parts of a train:

| Part | Form | Parameters |
| --- | --- | --- |
| Shock times | The first shock at the first sample, then gaps drawn from the measured quantile table of the gaps, times a scale | `gap_ms`, `gap_scale` |
| Shock levels | Drawn from the measured quantile table of the levels against the strongest shock, with a scaled spread and a correlation of consecutive levels through a normal value. An offset for each tenth of the train adds in full to the weakest level and in part to the top, by the fitted weight | `level_db`, `level_scale`, `level_corr`, `place_db`, `place_weight` |
| Shock sound | A grain at random from the table, with a random sign, at the level of the shock | `attack_ms`, `decay_ms`, `ring_ms`, `shock_db`, `ring_db` |
| Slow ring | The activity of the shocks: each shock adds its amplitude, which decays with a time constant. The activity plays the ring loop at a gain | `act_ms`, `act_db` |
| Bed | The bed loop, constant over the train | `bed_db` |
| Top | An eighth-order Butterworth low pass on the banks | `top_hz` |
| Body | The body loop: white noise through two equal band pass stages, independent of the shocks | `body_hz`, `body_q`, `body_gain_db` |
| End | After the last shock and a delay, an exponential fade, and at the fade the tail: a damped sine | `fade_delay_ms`, `fade_db_per_ms`, `tail_hz`, `tail_decay_ms`, `tail_gain_db` |
| Start | A random place in the loops, so that the bed and the body are under way, and a short linear fade | `fade_in_ms` |
| Level | The output gain at 0 dB of the level control | `out_db` |

The controls:

- Level: `Options.LevelDB` is a gain in dB over the fitted level. At 0 dB, the median sample peak of a train is the median sample peak of the clips, -0.4 dBFS.
- Length: `Options.Shocks` sets the count of shocks, clamped to the measured range of 28 to 61. With `Options.Short`, the count can go down to 1: a short train, which is derived, not measured, because no clip holds one. `Options.LengthMS` sets the active length, clamped to the measured range of 368 to 637 ms. With neither, the length comes from the measured quartiles.
- Single shock: `Options.Single` plays one shock at the level of the strongest shock, with the bed and the body until the fade. This mode is derived, not measured: no clip holds an isolated shock (phase 1, section 10).
- Rate: `NewBank` renders the bank at its sample rate. The fit and the check run at 44.1 kHz, the rate of the clips and of the snarl mix.

### 3.1 Repetition of the grains

A train of about 40 shocks plays from 64 grains with two signs, so grains repeat within a train. The check measures the repetition in numbers, with the same code for the clips and the model: the likeness of two strong shocks is the peak of the normalised cross correlation of their first 4 ms above 1 kHz over lags up to 0.5 ms. It is 1 for the same wave at any level and sign, and near 0 for independent noise (`TestLikeness`).

| Pairs of strong shocks, pooled | Clips | Model |
| --- | --- | --- |
| Of one clip or train | 0.45 (0.36 to 0.57, range 0.14 to 0.98) | 0.41 (0.32 to 0.50, range 0.11 to 0.99) |
| Of a clip or train and the next | 0.43 (0.34 to 0.54, range 0.11 to 1.00) | 0.40 (0.32 to 0.49, range 0.08 to 0.99) |

The model shocks are no more alike than the clip shocks, within a train or between trains. The overlap of each grain with the shocks and the ring before it, and the random sign, make a repeated grain differ in the measure. Both rows pass.

## 4. Parameters and sources

`internal/crackle/crackle.toml` holds the parameters, and `internal/crackle/crackle.sources.toml` beside it holds one source for each key, as `profiles/*.sources.toml` do. `TestSources` fails when a key has no entry or an entry names no key. The kinds:

| Kind | Keys |
| --- | --- |
| measured, a statistic of the 30 clips | `length_ms` (minimum, quartiles, maximum), `shocks_range` (least and most shocks), `gap_ms` (21 quantiles of 1,221 pooled gaps), `level_db` (21 quantiles of 1,251 pooled levels), `tail_hz`, `tail_decay_ms` |
| fitted, by the searches of section 5 | `length_offset_ms`, `gap_scale`, `level_scale`, `level_corr`, `place_db` (the measured offsets of each tenth times a fitted scale), `attack_ms`, `decay_ms`, `ring_ms`, `act_ms`, `act_db`, `top_hz`, `body_hz`, `body_q`, `fade_delay_ms`, `fade_db_per_ms`, `tail_gain_db`, `place_weight` (0.50) |
| fitted, by the bank fit of section 5 | `shock_db`, `ring_db`, `bed_db`, `body_gain_db` |
| fitted, by the scaling | `out_db` |
| snarl default | `fade_in_ms` (1 ms against a click at the start: the clips start cut, so they cannot give a fade-in) |

The quantile tables hold the quantiles at the probabilities (i + 0.5) / 21, so they hold no extreme value of the set. Each table is pooled over all clips, so it cannot rebuild a clip. The minimum and maximum of `length_ms` and `shocks_range` are the range of the controls, and phase 1 gives the same ranges. The range of a distribution is a statistic of the class, and it cannot rebuild a clip. The grain count, the grain length, the loop length and the cross fade are constants of the code, not parameters: they set the realisation of the generator, not its statistics.

## 5. The fit

`just crackle-fit` runs `tools/crackle --fit` on the 30 clips (`tools/crackle/fit.go`). The fit is deterministic. It uses no learned model. Each step renders 96 model trains with the seeds 1 to 96, from 6 banks of 16 trains, so that the fit does not follow the grains of one bank. It scales each train to the median clip peak, as the pack scales its clips, and measures the trains with the same code as the clips. A fit takes about 20 minutes on 16 cores.

1. Start: the measured tables and medians set the start of every parameter.
2. Bank fit: the band energy over time around the strongest shock (phase 1, section 5) gives the band powers of the bed (frames 10 and 5 ms before the shock), of the crack (frames 0 and 5 ms, less the bed) and of the ring (frames 10, 20 and 40 ms, less the bed). 500 Richardson-Lucy steps fit the squared gains of each bank to its band powers, over the matrix of the band powers that each carrier and filter gives in each measure band. The body gain gives the 31.25 to 62.5 Hz background power. A search over the corner from 8 to 19 kHz sets the low pass at the top.
3. Trim: 15 steps correct the band powers of the three banks by the error of the band energy over time of the model trains. The body stays fixed, and the other parts move against it, because the band energy is against the loudest frame of a train.
4. First search: a compass search moves 20 knobs, each in turn up and down by its step, keeps a move that lowers the loss, and halves the step of a knob that no move improves, for 600 renders. The loss is the sum over the sections of the check of the mean over the measures of the square of (model median - clip median) / clip interquartile range, limited to 16. The loss leaves out the file edges, the channels and the sample peak. The place offsets add to the whole level (`place_weight` 0), with a scale up to 1. The loss falls to 0.602.
5. Second search: `place_weight` starts at 1, so that the place offsets add to the median and not the top, and becomes a knob. The place scale goes up to 2. The search goes on from the values of the first for 900 renders, and the loss on the fit trains alone selects `place_weight`: 0.50. The loss ends at 0.592. The second search exists for the onset of the strongest shock: with the offsets on the whole level, the louder first tenth of the clips also lifts its top shocks, and the strongest shock comes too early.

The seed sets: the fit uses the seeds 1 to 96 for its trains and banks, and nothing else. The check uses the seeds 5001 to 5240, which no step of the fit, of the selection or of an earlier check used. Earlier checks of phase 2b used the seeds 1001 to 1240, and one of them chose a `place_weight` of 1. That choice is gone, so the check is independent of the fit.

## 6. The check

`just crackle-model 240` renders 240 model trains with the committed parameters and the seeds 5001 to 5240, from 15 banks that the fit did not use, writes them to `out/crackle/model/`, and checks them against the clips with the same measures (`out/crackle/model/compare.md`). The phase 2 check used 60 trains from one bank, and the pass count changed by up to 10 measures from one set of trains to another. The check with 240 trains is steadier.

Both models are checked on the same 240 trains, seeds 5001 to 5240:

| Section | Phase 2 model | Phase 2b model | Measures | No value on the model side |
| --- | --- | --- | --- | --- |
| Clip measures | 42 | 48 | 57 | 8 |
| All shocks, pooled | 7 | 7 | 7 | 0 |
| Strong shocks, pooled | 6 | 6 | 7 | 0 |
| Shock likeness, pooled | not in the phase 2 check | 2 | 2 | 0 |
| Band shares | 31 | 33 | 44 | 0 |
| Band energy over time | 83 | 87 | 120 | 0 |
| All | 169 of 235 | 183 of 237 (181 of the 235) | 237 | 8 |

The phase 2 model passed 178 of 235 on its 60 check trains of phase 2, and 176 of 235 on the seeds 1001 to 1240.

| Step | Kept | Evidence |
| --- | --- | --- |
| Grain voice in place of the live banks | yes | The cost falls from 1.4 to 0.05 per cent of one core. The shock likeness matches the clips. |
| Fit over 6 banks in place of 1 | yes | With one bank, the fit followed the grains of that bank: it passed 185 of 235 measures on its own trains and 171 on the trains of another bank. |
| Slow ring of the shock activity (`act_ms`, `act_db`) | yes | The decay above 2 kHz to -20 dB passes (35 against 40 ms, from 30 ms), and the strong shock rise passes (3.02 ms, from 3.69 ms). |
| Place offsets in part on the top (`place_weight` 0.50) | yes | The onset of the strongest shock comes to 128.9 ms, from 89.9 ms, 0.1 ms below the clip p25. |
| A bed that follows the shock activity | in part | The slow ring is this bed for the spectrum of the ring. The constant bed stays, and the frames from 500 Hz to 1 kHz before the strongest shock and 80 ms after it are still 4.7 to 6.0 dB high. |

### 6.1 Measures that cannot pass

- The channel measures (5): the model is mono. Phase 1 found that the clips are mono to a correlation of 0.995, and that the mean of the two channels loses nothing that the measures can see.
- The fragment after the active span (start and peak) and the sample peak of the last 10 ms of the file (-95.0 dBFS against -44.4, 44.6 dB below the p25): the pack files end with a fragment 32 to 58 dB below full scale at 600 to 618 ms, after a quiet span. This is an edit of the pack, which the model does not copy.
- The quiet span between bursts: only 2 clips have two bursts, and no model train has two.

### 6.2 Measures that miss

Each miss is the distance of the model median outside the clip p25 to p75.

| Group | Measure | Miss | Reason |
| --- | --- | --- | --- |
| Level | RMS level, crest factor, full-band peak of the strong shocks | -0.7 dB, +0.7 dB, -0.2 dB | The model trains are a little more peaky than the clips. |
| Timing | Onset of the strongest shock | -0.1 ms (128.9 against 129.0 ms) | With a weight of 0.50, the louder first tenth still lifts some of its top shocks. |
| Body | Share below 125 Hz | +0.1 dB | The body is a little too strong against the rest. |
| Body | Prominence at 0.5 times the body peak | +5.2 dB | The tail sine at 27 Hz, which the clips show after the last shock, adds energy near half the body peak to the spectrum of the active span. |
| Tail | Decay to -20 dB, fit of the decay, last half-cycle peak | less than 0.5 ms, -0.01, -2.4 dB | 13 clips give these, with very narrow quartiles, likely the fade of the edit (phase 1, section 6). The model tail is one damped sine. |
| Spectrum | 11 band shares | 0.0 to 3.7 dB, 9 within 1 dB | The largest: 16 to 20 kHz in the strong windows +3.7 dB, 8 to 16 kHz in the strong windows +1.3 dB, 1 to 2 kHz between the shocks -0.8 dB. |
| Time | 33 of 120 frames of the band energy over time | 17 within 1.5 dB | The largest: 500 Hz to 1 kHz before the strongest shock +4.7 and +4.9 dB, 40 ms after it +2.6 dB, 80 ms after it +6.0 dB and 320 ms after it +3.2 dB. 8 to 16 kHz before it +2.9 and +3.9 dB and 160 ms after it +3.9 dB. 250 to 500 Hz 160 ms after it +3.5 dB. 1 to 2 kHz 10 ms after it -3.0 dB. |

The bed of the model has one spectrum and level for the whole train. In the clips, the frames before the strongest shock and long after it have less energy from 500 Hz to 1 kHz and above 4 kHz than the model, while the sound between the shocks has about the model level. The slow ring follows the shock activity with the spectrum of the ring, which has little energy from 500 Hz to 1 kHz, so it does not move these frames. A slow ring with its own spectrum, fitted to the frames after the strongest shock, is the next form to try.

### 6.3 Check table

Each cell is the median (p25 to p75, range min to max) over the 30 clips and the 240 trains, or pooled where the section says so. Miss is the distance of the model median outside the clip p25 to p75, 0 for a pass. The band shares and the band energy over time are in section 6.2 and in `out/crackle/model/compare.md` of `just crackle-model 240`.

| Measure | Clips | Model | Pass | Miss |
| --- | --- | --- | --- | --- |
| Active length, ms | 544 (476 to 574, range 368 to 637) | 502 (442 to 538, range 333 to 606) | yes | 0 |
| Start of the active span in the file, ms | 0.0 (0.0 to 0.0, range 0.0 to 0.0) | 0.0 (0.0 to 0.0, range 0.0 to 0.1) | yes | 0.0 |
| Sample peak of the first 10 ms of the file, dBFS | -5.1 (-8.7 to -3.5, range -15.5 to -0.3) | -7.6 (-10.5 to -4.4, range -17.2 to -0.4) | yes | 0.0 |
| Sample peak of the last 10 ms of the file, dBFS | -44.4 (-50.3 to -32.4, range -58.1 to -5.6) | -95.0 (-97.0 to -93.3, range -102.9 to -88.4) | no | -44.6 |
| Bursts, between quiet spans of at least 20 ms | 1 (1 to 1, range 1 to 2) | 1 (1 to 1, range 1 to 1) | yes | 0 |
| Quiet span between bursts, ms | 34.3 (28.6 to 40.0, range 22.9 to 45.7), n=2 | not measured | - | - |
| Start of the sound after the active span, ms | 608.3 (604.8 to 610.5, range 600.8 to 617.9), n=23 | not measured | - | - |
| Sample peak of the sound after the active span, dBFS | -47.3 (-51.1 to -42.8, range -58.1 to -32.2), n=23 | not measured | - | - |
| Level of the last 5 ms against the loudest 5 ms, dB | -72.6 (-74.9 to -70.7, range -81.3 to -3.6) | -72.3 (-73.8 to -69.1, range -79.7 to -57.9) | yes | 0.0 |
| Level of the last 5 ms against the 5 ms that end 20 ms before, dB | -18.0 (-19.7 to -11.5, range -20.7 to 2.7) | -16.0 (-18.6 to -13.3, range -25.9 to 6.7) | yes | 0.0 |
| Sample peak, dBFS | -0.4 (-0.4 to -0.4, range -0.5 to -0.3) | -0.4 (-0.4 to -0.4, range -0.4 to -0.4) | yes | 0.0 |
| RMS level, dBFS | -12.8 (-13.0 to -12.4, range -15.6 to -11.5) | -13.7 (-14.9 to -12.3, range -19.4 to -9.3) | no | -0.7 |
| Crest factor, dB | 12.4 (12.0 to 12.6, range 11.0 to 15.2) | 13.3 (11.9 to 14.5, range 8.9 to 19.0) | no | 0.7 |
| Shocks | 40 (37 to 48, range 28 to 61) | 42 (37 to 48, range 20 to 60) | yes | 0 |
| Shocks within 6 dB of the strongest | 4 (3 to 5, range 1 to 8) | 4 (3 to 6, range 1 to 13) | yes | 0 |
| Strong shocks, within 12 dB of the strongest | 8 (6 to 8, range 2 to 14) | 8 (6 to 10, range 1 to 32) | yes | 0 |
| Shocks per second of active length | 77 (70 to 91, range 56 to 105) | 85 (79 to 94, range 56 to 107) | yes | 0 |
| Median gap between shocks, ms | 8.6 (8.0 to 9.7, range 6.9 to 11.8) | 8.4 (7.6 to 9.3, range 6.2 to 13.3) | yes | 0.0 |
| Median gap between strong shocks, ms | 51.2 (17.2 to 74.0, range 10.5 to 356.2) | 24.0 (15.3 to 41.3, range 6.4 to 236.3), n=236 | yes | 0.0 |
| Median rise above 1 kHz of the shocks of the clip, ms | 2.25 (2.00 to 2.82, range 1.63 to 3.38) | 2.45 (2.23 to 2.71, range 1.69 to 3.63) | yes | 0.00 |
| Median rise above 1 kHz of the strong shocks of the clip, ms | 2.39 (1.50 to 3.44, range 0.41 to 4.59) | 3.02 (2.49 to 3.77, range 1.34 to 7.05) | yes | 0.00 |
| Onset of the strongest shock, ms | 165.0 (129.0 to 235.8, range 2.6 to 477.9) | 128.9 (31.2 to 249.6, range 0.7 to 483.3) | no | -0.1 |
| Share above 2 kHz, dB | -16.6 (-17.5 to -14.6, range -20.4 to -11.3) | -16.2 (-18.1 to -14.2, range -25.1 to -10.6) | yes | 0.0 |
| Share below 125 Hz, dB | -1.1 (-1.3 to -0.9, range -2.2 to -0.8) | -0.8 (-1.2 to -0.7, range -2.4 to -0.3) | no | 0.1 |
| Decay above 2 kHz to -10 dB, ms | 10 (10 to 15, range 5 to 25) | 15 (10 to 15, range 10 to 40) | yes | 0 |
| Decay above 2 kHz to -20 dB, ms | 40 (31 to 54, range 20 to 195) | 35 (30 to 50, range 20 to 275) | yes | 0 |
| Mean power between the shock windows against inside them, dB | -1.2 (-1.7 to -0.7, range -2.7 to -0.2) | -0.9 (-1.4 to -0.4, range -3.1 to 1.4) | yes | 0.0 |
| Mean power above 2 kHz between the shock windows against inside them, dB | -3.5 (-5.3 to -1.6, range -9.1 to 2.0) | -3.1 (-4.4 to -1.5, range -9.6 to 2.8) | yes | 0.0 |
| Body peak of the active span, 10 to 500 Hz, Hz | 52 (46 to 56, range 34 to 79) | 51 (48 to 54, range 38 to 71) | yes | 0 |
| Tail after the last shock, ms | 69 (55 to 74, range 0 to 144) | 71 (65 to 76, range 50 to 110) | yes | 0 |
| Tail peak, 10 to 500 Hz, Hz | 28 (27 to 30, range 25 to 39), n=24 | 27 (26 to 29, range 16 to 35) | yes | 0 |
| Half cycles of the tail | 3 (2 to 3, range 1 to 8), n=24 | 3 (3 to 3, range 1 to 9) | yes | 0 |
| Tail frequency from the median half cycle, Hz | 27 (27 to 28, range 24 to 38), n=13 | 28 (27 to 31, range 22 to 368), n=184 | yes | 0 |
| Spread of the tail half cycles, interquartile range over median | 0.17 (0.16 to 0.24, range 0.11 to 0.68), n=13 | 0.16 (0.11 to 0.24, range 0.01 to 10.32), n=184 | yes | 0.00 |
| Tail decay to -20 dB, ms | 22 (22 to 24, range 20 to 83), n=13 | 22 (20 to 23, range 4 to 35), n=175 | no | -0 |
| Tail decay fit, r squared | 1.00 (1.00 to 1.00, range 0.84 to 1.00), n=13 | 0.99 (0.98 to 1.00, range 0.74 to 1.00), n=175 | no | -0.01 |
| Last half-cycle peak of the tail against the highest, dB | -32.0 (-32.2 to -29.1, range -39.9 to -12.4), n=13 | -34.6 (-36.9 to -32.2, range -67.8 to -15.7), n=184 | no | -2.4 |
| Channel correlation at no delay | 0.995 (0.994 to 0.995, range 0.990 to 0.996) | not measured | - | - |
| Channel correlation at the best delay | 0.995 (0.994 to 0.995, range 0.990 to 0.996) | not measured | - | - |
| Delay of the right channel behind the left, ms | -0.001 (-0.001 to -0.000, range -0.003 to 0.000) | not measured | - | - |
| Left level over right level, dB | -0.13 (-0.15 to -0.11, range -0.22 to -0.07) | not measured | - | - |
| Side (L-R) level over mid (L+R) level, dB | -25.8 (-26.2 to -25.3, range -27.0 to -22.9) | not measured | - | - |
| Body cycle frequency, median, Hz | 51.9 (49.3 to 53.8, range 38.8 to 70.9) | 53.2 (50.6 to 55.3, range 43.3 to 66.2) | yes | 0.0 |
| Spread of the body cycles, interquartile range over median | 0.23 (0.19 to 0.29, range 0.15 to 0.48) | 0.21 (0.17 to 0.26, range 0.09 to 0.47) | yes | 0.00 |
| Drift of the body cycle frequency, per cent per second | -30.4 (-74.7 to 5.5, range -176.3 to 133.9) | -41.3 (-71.2 to -14.6, range -309.1 to 155.1) | yes | 0.0 |
| Fit of the drift line, r squared | 0.06 (0.01 to 0.17, range 0.00 to 0.67) | 0.07 (0.02 to 0.15, range 0.00 to 0.54) | yes | 0.00 |
| Prominence of the spectrum at 0.5 times the body peak, dB | -15.1 (-20.7 to -10.9, range -30.7 to 0.6) | -5.7 (-9.9 to -2.2, range -23.8 to 10.9) | no | 5.2 |
| Prominence of the spectrum at 1 times the body peak, dB | 7.9 (5.9 to 10.0, range 4.6 to 16.1) | 9.2 (6.4 to 11.5, range 2.3 to 19.4) | yes | 0.0 |
| Prominence of the spectrum at 1.5 times the body peak, dB | -0.9 (-3.6 to 1.8, range -6.3 to 9.7) | 0.7 (-2.1 to 3.1, range -10.0 to 13.3) | yes | 0.0 |
| Prominence of the spectrum at 2 times the body peak, dB | 1.1 (-0.1 to 3.6, range -8.9 to 9.1) | 1.7 (-0.1 to 3.7, range -11.7 to 10.3) | yes | 0.0 |
| Prominence of the spectrum at 2.5 times the body peak, dB | 1.3 (-0.7 to 4.0, range -3.7 to 11.1) | 3.2 (1.2 to 5.3, range -4.0 to 11.5) | yes | 0.0 |
| Prominence of the spectrum at 3 times the body peak, dB | 3.2 (1.0 to 5.4, range -4.0 to 7.1) | 3.0 (1.1 to 5.0, range -7.1 to 9.7) | yes | 0.0 |
| Prominence of the spectrum at 3.5 times the body peak, dB | 3.9 (1.8 to 5.3, range -2.0 to 9.1) | 3.1 (1.5 to 4.6, range -2.2 to 9.6) | yes | 0.0 |
| Prominence of the spectrum at 4 times the body peak, dB | 3.5 (2.4 to 4.6, range -0.5 to 7.4) | 3.8 (2.4 to 5.1, range -1.6 to 8.9) | yes | 0.0 |
| Prominence of the spectrum at 4.5 times the body peak, dB | 4.4 (3.2 to 5.9, range -0.2 to 8.5) | 3.8 (2.8 to 4.9, range -2.9 to 8.4) | yes | 0.0 |
| Prominence of the spectrum at 5 times the body peak, dB | 3.8 (2.7 to 4.9, range 0.2 to 9.4) | 4.3 (3.1 to 5.3, range -2.4 to 8.5) | yes | 0.0 |
| Spread over the mean of the body envelope | 0.55 (0.52 to 0.58, range 0.46 to 0.82) | 0.55 (0.50 to 0.59, range 0.40 to 0.73) | yes | 0.00 |
| Rise of the body envelope at the strong shocks, median, dB | 1.6 (-0.4 to 4.1, range -2.7 to 9.1) | 2.3 (0.6 to 4.1, range -3.1 to 13.5), n=238 | yes | 0.0 |
| Rise of the body envelope at random times, median, dB | 0.5 (-1.3 to 2.5, range -4.9 to 20.1) | 1.8 (0.0 to 3.8, range -5.1 to 13.0), n=238 | yes | 0.0 |
| Correlation of the shock level and the rise of the body envelope | 0.13 (-0.00 to 0.30, range -0.27 to 0.45) | 0.03 (-0.12 to 0.23, range -0.52 to 0.58) | yes | 0.00 |
| Mean resultant length of the shock phases in the body cycle | 0.15 (0.08 to 0.22, range 0.04 to 0.28) | 0.14 (0.09 to 0.21, range 0.01 to 0.40) | yes | 0.00 |
| Mean resultant length of the strong shock gaps in body cycles | 0.33 (0.24 to 0.59, range 0.11 to 0.91), n=28 | 0.38 (0.25 to 0.52, range 0.03 to 0.98), n=229 | yes | 0.00 |
| Spread over the mean of the gaps between shocks | 0.67 (0.52 to 0.95, range 0.44 to 1.62) | 0.62 (0.57 to 0.67, range 0.39 to 0.90) | yes | 0.00 |
| Fano factor of the shock count in 50 ms windows | 0.89 (0.69 to 1.20, range 0.24 to 1.77) | 0.86 (0.67 to 1.04, range 0.09 to 1.60) | yes | 0.00 |
| Correlation of the logarithms of consecutive gaps | 0.03 (-0.12 to 0.08, range -0.33 to 0.30) | -0.01 (-0.10 to 0.13, range -0.43 to 0.54) | yes | 0.00 |

Shocks, pooled over the 30 clips (1,251 shocks, 231 strong) and the 240 trains:

| Shocks | Measure | Clips | Model | Pass | Miss |
| --- | --- | --- | --- | --- | --- |
| All | Gap to the shock before, ms | 8.9 (5.8 to 13.2, range 0.7 to 169.0), n=1221 | 8.3 (5.6 to 12.4, range 0.0 to 65.6), n=9861 | yes | 0.0 |
| All | Level against the strongest shock of the clip, dB | -21.0 (-24.0 to -15.1, range -30.0 to 0.0) | -19.6 (-21.8 to -15.1, range -30.0 to 0.0) | yes | 0.0 |
| All | Full-band sample peak, dBFS | -7.3 (-10.3 to -5.0, range -22.7 to -0.3) | -8.4 (-11.2 to -5.6, range -25.3 to -0.4) | yes | 0.0 |
| All | Rise above 1 kHz, 10 to 90 per cent, ms | 2.31 (1.22 to 4.08, range 0.00 to 17.60) | 2.45 (1.41 to 4.10, range 0.05 to 24.65) | yes | 0.00 |
| All | Full-band rise, 10 to 90 per cent, ms | 2.04 (0.57 to 3.98, range 0.00 to 17.78) | 2.20 (0.59 to 4.40, range 0.00 to 19.71) | yes | 0.00 |
| All | Fall above 1 kHz to -10 dB, ms | 0.59 (0.36 to 1.04, range 0.11 to 4.97), n=1248 | 0.68 (0.45 to 0.98, range 0.11 to 5.85), n=10082 | yes | 0.00 |
| All | Crest factor over the shock span, dB | 5.7 (4.6 to 7.3, range 0.8 to 14.7) | 5.9 (4.8 to 7.3, range 0.0 to 15.0) | yes | 0.0 |
| Strong | Gap to the strong shock before, ms | 32.2 (12.5 to 77.9, range 2.0 to 356.2) | 20.3 (9.4 to 53.1, range 2.4 to 283.4) | yes | 0.0 |
| Strong | Level against the strongest shock of the clip, dB | -5.6 (-8.7 to -2.0, range -12.0 to 0.0) | -4.9 (-8.5 to -1.8, range -12.0 to 0.0) | yes | 0.0 |
| Strong | Full-band sample peak, dBFS | -2.5 (-4.0 to -0.5, range -11.4 to -0.3) | -4.2 (-6.5 to -2.0, range -15.6 to -0.4) | no | -0.2 |
| Strong | Rise above 1 kHz, 10 to 90 per cent, ms | 2.49 (1.17 to 4.23, range 0.20 to 10.34) | 3.02 (2.06 to 4.95, range 0.16 to 22.79) | yes | 0.00 |
| Strong | Full-band rise, 10 to 90 per cent, ms | 1.79 (0.67 to 3.68, range 0.02 to 9.41) | 2.79 (1.52 to 5.10, range 0.00 to 19.71) | yes | 0.00 |
| Strong | Fall above 1 kHz to -10 dB, ms | 0.57 (0.39 to 1.16, range 0.14 to 3.65) | 0.73 (0.43 to 1.11, range 0.11 to 3.33), n=1982 | yes | 0.00 |
| Strong | Crest factor over the shock span, dB | 8.5 (7.0 to 9.6, range 3.6 to 13.2) | 8.4 (7.2 to 9.8, range 2.6 to 15.0) | yes | 0.0 |

## 7. Cost

`BenchmarkVoice` renders trains for one second of audio at 44.1 kHz in blocks of 256 samples, with a new train when one ends. It takes 0.47 ms in three runs of 3 s on an AMD Ryzen 9 7940HS with Go 1.27.2: 0.05 per cent of one core for one voice, with no allocations. The phase 2 voice, which ran the three banks of 11 bands live, took 13.6 ms. The voice now adds the grains that sound and reads three loops for each sample.

`BenchmarkNewBank` makes a bank in 42.4 to 42.9 ms, with 1.9 MB in 205 allocations: about 0.8 MB of grains and 1.06 MB of loops. A bank is made once, when the voice set is made, and its voices share it.

## 8. Reproduce

1. Run `just crackle-measure`. It writes the clip measures to `out/crackle/`, with the body and timing measures of section 2 in `summary.md`.
2. Run `just crackle-fit`. It writes `internal/crackle/crackle.toml` and `internal/crackle/crackle.sources.toml`, and the check of the 96 fit trains to `out/crackle/fit.md`. Two runs give the same files.
3. Run `just crackle-model 240`. It writes 240 trains, `compare.md` (the check of section 6) and `summary.md` to `out/crackle/model/`.

`go test ./internal/crackle` checks the parameter file, the sources file, the bank, the determinism, the absence of allocations, the length control, the level control and the rate. `go test ./tools/crackle` checks the body and timing measures on test signals (a harmonic tone, narrow band noise, periodic and random times, even and locked phases), the likeness, the bank fit, and the check command.

## 9. The model in the engine

Phase 3 makes the crackle model the voice of the snarl pops. The decision layer of `internal/core/pops.go` stays: the lift-offs, coasts, wind-down bands, throttle history, wall heat, requests with their queue and 50 ms guard, limiter bangs, characters, `FlagNoPops`, the four kinds, the four slots, the pop reports and the levels from `overDB`. The game API does not change.

### 9.1 The mix

Each of the four slots plays its pop on a voice of the model. `Engine.mix` adds the voices to each block of the runtime output at 44.1 kHz, after the 1900 Hz input filter, the impulse responses, the leveller, the volume and the clamp, and before the resampler to the output rate (`esynth.RuntimeOptions.Mix`). A pop limiter keeps the sum below 0.95 of full scale: its gain falls at once to what a peak needs and recovers with a time constant of 50 ms, as the limiter of the side input did. The side input of the runtime had no other user, so it is gone, and a pop no longer costs a second runtime pass. A requested pop that takes a busy slot stops the old train with a ramp from its last output down to 0 over 1.5 ms (`releaseMS` of `internal/crackle`), so that the steal does not click (`TestRetrigger`).

One bank and the kind gains are made once for the process (`sharedModel`, a `sync.OnceValues`) at the first `New`, and every engine only reads them. A bad `crackle.toml` makes every `New` return the error of the file. The bank seed is fixed. The seeds of the four voices of an engine come from the pop random source, so that the engine sound stays independent of the pops. `Render` makes no allocations (`TestEngine`).

### 9.2 The kinds

| Kind | Voice | Source (`internal/core/pops.sources.toml`) |
| --- | --- | --- |
| `KindCrackle` | A train. The rank of the sub-event count in its range (5 to 32 later sub-events, at most 10 for a requested crackle) sets the active length at the same rank of the measured lengths, 368 to 637 ms | measured class |
| `KindBoom` | A short train with one shock for each sub-event, 4 to 16 shocks, over the body (`Options.Short`) | partial; the length is derived |
| `KindPop` | The single-shock mode | derived |
| `KindBurble` | The single-shock mode at the burble level | not measured |
| Limiter bang | The single-shock mode at `bangGain` times an own pop | derived |

`Options.Short` lets only the boom go below the measured minimum of 28 shocks. The crackle uses the length control alone, inside its measured range (`TestVoiceOptions`).

### 9.3 The level

The level rule stays: a voice of unit gain is at its `overDB` target over an engine of unit RMS level before the leveller, and each block of the voices takes the engine level (the mean of 8 blocks, as before), the leveller gain and the volume. The side input took the leveller gain at each sample. The mix takes the gain at the end of each block of 256 samples (5.8 ms). `calibrate` now renders 32 voices of each kind and source with no engine, with the sub-event counts in even steps, and sets the kind gains from the mean level in decibels. The calibration no longer depends on the profile. The trim is now 0.1 dB (was 0.5 dB), which puts the mean error over the kinds near 0 dB.

`TestPopLevelOverEngine`, median level over the engine after the leveller, over the five profiles and both sources:

| Kind | Target, dB | Medians, dB |
| --- | --- | --- |
| `KindPop` | 4.8 | 3.6 to 4.8 |
| `KindBoom` | 4.5 | 3.2 to 4.7 |
| `KindCrackle` | 1.5 | 1.5 to 2.6 |
| `KindBurble` | -4.3 | -4.4 to -2.6 |

`TestPopLeveller` gives a leveller gain drop of 0.00 dB. `TestPopPeak` gives peaks of 0.967 to 0.976 against the limit of 0.99.

### 9.4 Checks

- Pops off: `snarl-render --format f32 --set pops.density=0` at 48 kHz of `sweep-wot`, `overrun`, `holds-t100` and `tools/crackle/lift.scn` on each of the five profiles gives the same files, byte for byte, as `main` (`51f94ef`), 20 of 20.
- `just scorecard --dyno --no-pops` gives the same table as `main`: 97.7 to 100 on the holds, sweeps, overruns and limiter.

### 9.5 The new pops against the clips

`just crackle-baseline` on `v8-ls`, as in section 9 of [crackle-measure.md](crackle-measure.md). Each event is one lift-off run of up to 3 pops, so the train kinds join up to 3 voices in one event, and the single-shock kinds give up to 3 shocks about 190 ms apart. Medians:

| Measure | Clips (30) | `KindPop` (6) | `KindBoom` (6) | `KindCrackle` (6) | `KindBurble` (6) |
| --- | --- | --- | --- | --- | --- |
| Active length, ms | 544 | 496 | 532 | 816 | 482 |
| Crest factor, dB | 12.4 | 16.7 | 13.8 | 12.4 | 21.6 |
| Shocks per event | 40 | 14 | 32 | 72 | 11 |
| Strong shocks per event | 8 | 3 | 5 | 20 | 3 |
| Median gap between shocks, ms | 8.6 | 10.9 | 9.3 | 8.5 | 13.9 |
| Gap between strong shocks, pooled, ms | 32.2 | 191.6 | 11.4 | 17.3 | 191.6 |
| Median rise above 1 kHz, ms | 2.25 | 2.00 | 2.36 | 2.70 | 2.00 |
| Crest factor of the strong shocks, pooled, dB | 8.5 | 10.0 | 6.9 | 7.9 | 11.6 |
| Share above 2 kHz, dB | -16.6 | -12.8 | -18.5 | -15.6 | -12.2 |
| Share below 125 Hz, dB | -1.1 | -1.8 | -0.7 | -1.0 | -1.9 |
| Decay above 2 kHz to -20 dB, ms | 40 | 20 | 35 | 72 | 22 |
| Power between the shock windows against inside, dB | -1.2 | -5.2 | -3.7 | -0.6 | -6.1 |
| The same above 2 kHz, dB | -3.5 | -13.2 | -5.0 | -1.3 | -14.0 |
| Body peak, 10 to 500 Hz, Hz | 52 | 50 | 50 | 49 | 51 |
| Tail frequency from the half cycles, Hz | 27 (n=13) | 26 (n=3) | 28 (n=3) | 24 (n=3) | 42 (n=1) |
| Tail decay to -20 dB, ms | 22 (n=13) | 21 (n=3) | 20 (n=2) | 20 (n=3) | 32 (n=1) |
| Fall over the last 20 ms, dB | -18.0 | -17.6 | -17.8 | -15.1 | -16.3 |

Against the old pops (crackle-measure.md section 9), every kind now has its body at 49 to 51 Hz (was 86 to 421 Hz) and 66 to 85 per cent of its energy below 125 Hz (was 4 to 29 per cent). The share above 2 kHz is -12.2 to -18.5 dB (was -24.3 to -27.4 dB), and the crackle is within 1 dB of the clips. The crackle train is near the clips on most measures. Its event is longer and holds more strong shocks because a run joins up to 3 trains. The single-shock kinds have more crack for their level than the clips, because one shock over 140 ms of body is the whole event.

### 9.6 Cost

`BenchmarkRender`, `BenchmarkStates` and `BenchmarkPops` on an AMD Ryzen 7940HS, three runs each, mean of the five profiles, in per cent of real time at 48 kHz, with 0 allocations before and after:

| Benchmark | Before | After |
| --- | --- | --- |
| `BenchmarkRender` | 0.69 | 0.69 to 0.77 (two sets of runs, the code path is the same) |
| `BenchmarkStates` wot / idle / overrun / limiter | 0.68 / 0.67 / 0.72 / 0.65 | 0.62 / 0.63 / 0.67 / 0.62 |
| `BenchmarkPops` | 0.97 | 0.78 |

A pop now costs 0.09 per cent of one core over the engine (`BenchmarkPops` against `BenchmarkRender`), from 0.28 per cent with the side input. The shared model takes less than 0.15 s to make and calibrate, once for the process.
