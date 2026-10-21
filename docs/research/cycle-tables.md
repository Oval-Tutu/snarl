# Cycle tables: engine-sim physics as tables for the engine-sim synthesiser

Measured on the branch `upstream-profiles` after `488f64e`. The tables replace the engine-sim physics: they hold the synthesiser input of each exhaust channel over one 720-degree engine cycle, measured from engine-sim itself on a grid of RPM and throttle holds, and a model of the part of the input that does not repeat from cycle to cycle. Playback reads them at the crank angle and feeds the exact synthesiser port (`internal/esynth`).

## Answer

Table playback through the port scores 98.2 to 100 on every holds scenario and 100 on every full-throttle sweep of the five engine-sim engines (`snarl-compare --align` against the engine-sim references). Every window of every holds scenario scores 91.6 or more. The current snarl scores 24 to 45 on the same holds (`docs/research/alignment-scorecard.md`).

| Engine | Holds t000 | Holds t050 | Holds t100 | Sweep | Overrun | Lowest window of the holds scenarios |
| --- | --- | --- | --- | --- | --- | --- |
| `i4-vtec` | 99.1 | 99.6 | 99.8 | 100.0 | 92.2 | 91.6 (770 rpm, t000) |
| `v8-ls` | 100.0 | 100.0 | 100.0 | 100.0 | 99.5 | 100.0 |
| `f4-ej25` | 99.2 | 99.5 | 99.3 | 100.0 | 92.2 | 94.3 (830 rpm, t000) |
| `v12-412` | 100.0 | 100.0 | 100.0 | 100.0 | 97.6 | 100.0 |
| `v12-merlin` | 98.2 | 99.3 | 99.4 | 100.0 | 100.0 | 94.3 (600 rpm, t000) |

Three hold windows score below 90, all in the overrun scenario:

- The closed-throttle hold right after full throttle at the top RPM: `i4-vtec` 58.7 and `f4-ej25` 58.9. engine-sim has a slow transient after the throttle closes, which no steady table holds (section 6.1).
- The first full-throttle hold of `v12-412` at 18000 rpm: 89.4. engine-sim differs from itself on this hold by more than the floor (section 6.2).

The data is 0.32 to 1.02 MB per engine, 3.7 MB in total. The grid has a 250 rpm step from idle to the top at five throttles, and the average cycle keeps every harmonic up to the Nyquist frequency of the simulation rate.

## 1. How to run

```sh
# Render the grid of one engine (or all without an argument) and write profiles/cycles/<engine>.bin.
just cycles-grid v8-ls

# Play reference scenarios from the tables through the synthesiser port.
R=$(nix build -f reference/engine-sim --print-out-paths --no-link)
go run ./tools/cycles play --tables profiles/cycles/v8-ls.bin --es $R/share/engine-sim/es \
  --out-dir out/cycles/play/v8-ls scenarios/upstream/v8-ls/holds-t100.scn

# Score the playback against engine-sim.
go run ./cmd/snarl-compare --align --scenario scenarios/upstream/v8-ls/holds-t100.scn \
  out/reference/upstream/v8-ls/holds-t100.wav out/cycles/play/v8-ls/holds-t100.wav
```

`play` renders at 44.1 kHz with the 3 s pre-roll and `--volume 0.5` of the references. `snarl-compare` resamples it to the 48 kHz of the reference. Other `tools/cycles` commands: `build` (the tables and a report of each point, the residual bands and the intake lag), `scenarios` (the grid files) and `spectrum` (the input and residual spectrum of one hold of a record). The outputs of this study are in `out/cycles/`: `grid/` (records and build reports), `play/`, `score/`, `thin/` (half-grid checks) and `diag/`.

## 2. The record

`engine-sim-render --dump-synth` now writes `crank.f64`: the cycle angle of the output crankshaft and its angular speed after each step (`reference/engine-sim/README.md`). The record had no angle or speed before, so the angle cannot be derived from it. `synth.json` also gives the segments, the step, the pre-roll and the exhaust delay and weight of each cylinder. `--cut RPM:THROTTLE:SECONDS` holds the RPM with the ignition off. The `Float32` null test of the port stays bit-exact on a record with a cut.

## 3. The grid

`tools/cycles scenarios` writes the grid files of each engine into `scenarios/upstream/<engine>/grid/` (`scenarios/upstream/README.md`): five throttle rows (0, 0.25, 0.5, 0.75, 1) with holds at idle, every multiple of 250 rpm and the top, ignition-off holds at throttle 0 and 1 at every fourth grid RPM and the top, and a throttle step. Each hold has 0.5 s of settling and at least 24 whole cycles. The recipe renders the seven files of an engine in parallel at the simulation rate of the unchanged script, 5 kHz for `v12-412`.

| Engine | Simulation rate | Grid RPM | Points | Steps per cycle | Bytes |
| --- | --- | --- | --- | --- | --- |
| `i4-vtec` | 20 kHz | 32, 770 to 8400 | 178 | 3117 to 286 | 1019440 |
| `v8-ls` | 10 kHz | 24, 690 to 6500 | 134 | 1739 to 185 | 602495 |
| `f4-ej25` | 20 kHz | 24, 830 to 6400 | 134 | 2892 to 375 | 845590 |
| `v12-412` | 5 kHz | 58, 3810 to 18000 | 322 | 157 to 33 | 908748 |
| `v12-merlin` | 7 kHz | 11, 600 to 3000 | 63 | 1400 to 280 | 318132 |

The change of the average cycle between neighbouring RPM points at full throttle, in the emission angle (section 4), is 0 to 8 dB below the signal near 1000 rpm, 6 to 16 dB at 2000 rpm, and 16 to 26 dB above 5000 rpm. The shape changes most at low RPM. Two checks show that the 250 rpm step is enough:

- On the grid, every hold of the holds scenarios falls on a grid point.
- Off the grid, tables with every second RPM point (a 500 rpm step, `build --thin 2`) score 98.5 on `v8-ls` `holds-t100` and 98.4 on `i4-vtec` `holds-t100`, where every hold from 2000 rpm to 6000 rpm (`v8-ls`) or 8000 rpm (`i4-vtec`) falls halfway between two points. The lowest window is 88.9 (`v8-ls`, 2000 rpm, wave error 10.0 dB). At a 250 rpm step the distance to the nearest point is half of that.

## 4. The tables

Per grid point and channel, `tools/cycles build` (`internal/cycles`) measures:

1. **Crank phase.** The crank phase in cycles comes from the recorded angle, unwrapped in the direction of rotation. The crank of engine-sim jitters by about 0.005 degrees from step to step. A zero-phase low pass at 12 times the cycle rate removes the jitter and keeps the speed changes within the cycle.
2. **Emission angle.** Each input sample is indexed by the crank phase one channel delay earlier: the weighted mean of the exhaust delay filters of the cylinders of the channel. Within a hold this is a fixed shift. Between grid points it removes the shift of the pulses that the fixed delay time gives at each RPM. On `i4-vtec`, with 12.6 m of exhaust, the half-grid check scores 98.4 with the shift and 65.4 without it (`play --no-delay-shift`).
3. **Average cycle.** The input is resampled with a windowed sinc (Kaiser, 16 samples each side) at a power of two of uniform phase points per cycle, at least four per harmonic. The cycles are averaged and transformed with an FFT. The table keeps every harmonic up to the Nyquist frequency of the input rate: the steps per cycle in the table above, as 2H+1 values. A direct sum over the unevenly spaced samples leaked the strong low harmonics into the weak high ones about 62 dB below the signal. On `v12-merlin` that leak was 10 to 40 dB above the real input from 700 Hz to 3.5 kHz, and the holds scored 75 to 80. The uniform resampling put the residual below the input at all frequencies and the holds at 98 to 99.4.
4. **Speed profile.** The crank speed over the cycle relative to its mean, 12 harmonics, at 64 points. Playback advances the phase with it.
5. **Residual.** The input less the average cycle goes through nine bands: octaves down from the Nyquist frequency of the input rate, fourth-order Butterworth edges, the lowest band below 10 to 39 Hz. Per band and per 64 sectors of the emission angle, the table holds the RMS. The band level comes from the mean band powers, unmixed from the leakage of the neighbouring bands with a weighted least-squares fit that keeps each band at least 15 dB below its own measure. The shape over the cycle is the band power per sector, smoothed over the time resolution of the band.
6. **Intake lag.** From the throttle steps of `step.scn`, each cycle after the step is projected onto the two steady tables at that RPM. The share of the old state fits a first-order lag, with an RMS error of 0.03 (closing) and 0.05 (opening) on `v8-ls`. The lag applies at every RPM.

| Engine | Step RPM | Closing lag (s) | Opening lag (s) |
| --- | --- | --- | --- |
| `i4-vtec` | 5000 | 0.052 | 0.037 |
| `v8-ls` | 4000 | 0.068 | 0.028 |
| `f4-ej25` | 3750 | 0.050 | 0.052 |
| `v12-412` | 10750 | 0.019 | 0.015 |
| `v12-merlin` | 1750 | 0.095 | 0.056 |

The format stores the values as float16 with a float32 scale per array (`profiles/cycles/README.md`). The relative precision is 2^-11 of the largest value of each array.

The residual matters little for the score on steady holds. The output residual of engine-sim is mostly the air noise and jitter of the synthesiser itself: on `v8-ls` at 3000 rpm and full throttle, playback without the input residual gives -26.8 dB against -26.1 dB for engine-sim.

## 5. Playback

`cycles.Render` runs the port in the frame order of the renderer: per frame of 1/200 s the steps of the frame, then render blocks until the input is empty. Per step, `cycles.Player`:

1. weights the grid points: linear in the throttle between the two nearest rows, the row weights lagged with the intake lag, and linear in the RPM between the two nearest points of each row. Outside the grid it takes the edge. The ignition state selects the ignition-off points and resets the lag.
2. advances the crank phase by the step time times the RPM over 120 times the speed profile.
3. reads each channel at the emission phase (the crank phase less the channel delay at the current RPM) from 8 times oversampled tables with linear interpolation, at most 0.11 dB low at the highest harmonic.
4. adds the residual: Gaussian noise through each band filter, normalised to unit power, times the band envelope at the emission phase.

The synthesiser parameters, the channels and the impulse response volumes come from the tables. The impulse responses come from the engine-sim package. The synthesiser draws its own `rand()` values: the calls that engine-sim makes in its combustion code between blocks are not reproduced, so the noise is the same in distribution only.

An ignition-off hold plays from the ignition-off points. On `v8-ls` at 3750 rpm and full throttle, the playback scores 100 against an engine-sim `--cut 3750:1:4` render (`play --ignition-off`).

## 6. Limits

### 6.1 The transient after the throttle closes

After a step from full to closed throttle, engine-sim needs about 1 s to settle, much longer than the intake lag. On `v8-ls` at 6500 rpm (`out/cycles/diag/v8-overrun-long`), the residual of each cycle against the closed-throttle table is 14000 times the settled residual at 0.1 s, 1220 times at 0.25 s, 43 times at 0.5 s, 1.4 times at 1 s and 1.1 times at 2 s. The RPM stays at 6500. The share of the old state is 0 from 0.25 s, and no steady state between the throttle rows matches these cycles (the best leaves 1000 times the settled residual at 0.25 s). The transient therefore leaves the steady states, and a table cannot hold it. The overrun window starts 0.25 s after the step. Its engine-sim residual is -17 to -22 dB, against -28 dB for the same hold reached by a slow sweep and for the playback. With the intake lag, `v8-ls` rose from 79.0 to 97.4 in this window. `i4-vtec` and `f4-ej25` stay at 58.7 and 58.9. The fix needs a model of the transient itself, for example the decay of the residual level and of the cycle shape measured after steps at several RPM.

### 6.2 engine-sim against itself on `v12-412`

The first overrun hold of `v12-412` (18000 rpm, full throttle, 89.4) is the same state as the last hold of `holds-t100`, which scores 100. engine-sim gives a residual of -13.8 dB in the overrun render and -16.6 dB in `holds-t100`, a difference of 2.8 dB between two engine-sim renders of one state. That is beyond the 2 dB floor of the residual measure. `v12-412` misfires at random below 11000 rpm at its own 5 kHz rate (`docs/research/reference-health.md`), and the tables reproduce that, because they come from the same renders.

### 6.3 What the scores do not cover

- **The limiter.** The tables hold steady ignition-off holds. A limiter cut of engine-sim lasts about 50 ms, and the exhaust does not settle in that time. Playback switches between the ignition-on and ignition-off points at once. The `limiter-free` scenario has a free segment, which `snarl-compare` does not read, so the limiter is not scored.
- **Free running.** In a free run, and in a car, the RPM changes with the load. Playback takes the RPM it is given, and the speed profile and the tables come from dynamometer holds.
- **Throttle between rows.** The scenarios hold throttle 0, 0.5 and 1 only, which are rows. Throttle 0.25 and 0.75 are rows too, but playback between rows is not scored.
- **Sweeps.** The sweep scores have no cycle measures and loose floors (`docs/research/alignment-scorecard.md`), so they are an upper bound.
- **Idle.** The lowest hold windows are the idle holds, 91.6 to 94.3: few cycles per window and the largest shape change per RPM step.

### 6.4 Shipping and runtime

- The tables hold no impulse response. The impulse responses of the engine-sim package have no licence statement, so a product needs its own impulse responses.
- Playback runs the reference port with direct convolution and nine band filters of four sections per channel and input sample. The fast runtime of `internal/esynth` (another change) can replace the port. The residual filters can become one shaping filter per point if the runtime needs it.

## 7. Next steps

1. Model the throttle-closing transient (section 6.1) from step renders at several RPM, and score the overrun again.
2. Test a finer grid below 2000 rpm, where the shape changes most between points, with sweeps and with holds between grid points.
3. Measure and play the limiter cut from free-run records with the crank angle.
