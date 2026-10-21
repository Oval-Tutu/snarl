# engine-sim synthesiser port

`internal/esynth` is a Go port of the engine-sim synthesiser at commit `85f7c3b`: `src/synthesizer.cpp` and the filters that it uses. The null test passes for all five engine-sim engines (`scenarios/upstream/README.md`) on a hold scenario and a sweep, with noise and jitter off and on:

- In `Float32` precision the port output is identical to the engine-sim output in every sample of all 20 renders, the random parts included.
- In `Float64` precision the peak difference is 3 or 4 16-bit steps, 74.0 dB or more below the signal peak. The RMS difference is 0.49 to 0.63 steps, 83.7 dB or more below the signal RMS. The difference is the float rounding of engine-sim.
- With noise and jitter on, the `Float64` output and engine-sim agree within 0.05 dB in the third-octave distance of every window, within 0.1 dB in the residual and burst tables below 16 kHz, and within 2.7 dB in the residual band from 16 to 24 kHz, which is 77 to 94 dB below the signal.

## Tools

The renderer records the synthesiser input with `--dump-synth DIR` (`reference/engine-sim/README.md`). `--no-noise` sets the jitter and the air noise to 0.

```sh
R=$(nix build -f reference/engine-sim --print-out-paths --no-link)
$R/bin/engine-sim-render --script $R/share/engine-sim/engines/atg-video-2/07_gm_ls.mr \
  --sweep 690:6500:1:8 --volume 0.5 --rate 44100 --no-noise \
  --out out/esynth/v8-ls/sweep-wot-quiet/ref.wav --dump-synth out/esynth/v8-ls/sweep-wot-quiet
go run ./tools/esynth --dump out/esynth/v8-ls/sweep-wot-quiet --out out/esynth/v8-ls/sweep-wot-quiet/port.wav
go run ./tools/esynth --dump out/esynth/v8-ls/sweep-wot-quiet --precision float32
```

`tools/esynth` runs the port on the record, prints the sample-by-sample difference from the engine-sim output in the record, and writes the port output after the pre-roll (`--out`), which is the span of the renderer `--out`. `--es DIR` reads the impulse responses from the `es/` directory of the package instead of the copies in the record.

## Stages

Line numbers are in the pinned engine-sim source. The Go functions are in `internal/esynth`.

| Stage | engine-sim | Go |
| --- | --- | --- |
| Synthesiser parameters of the simulator: 44.1 kHz output, input and output buffers of 44100 samples, input rate the simulation frequency | `Simulator::initializeSynthesizer`, `src/simulator.cpp:204` | `SimulatorParameters` |
| Filter set-up: air noise low pass 2 kHz, derivative and DC filter time step 1/44100, DC low pass 10 Hz, jitter of 10 samples with a 10 kHz noise low pass, input low pass 1900 Hz in double, output low pass at 0.45 times 44.1 kHz, leveller limits, 44100 zeros in the output buffer | `Synthesizer::initialize`, `src/synthesizer.cpp:37-84` | `New` |
| Impulse response: trim after the last sample above 100 in magnitude, at most 10000 taps, scale by `volume / INT16_MAX` | `Synthesizer::initializeImpulseResponse`, `src/synthesizer.cpp:86-105` | `Synthesizer.InitializeImpulseResponse` |
| Input per simulation step: exhaust pressure of each cylinder, delayed, scaled and summed per exhaust system | `PistonEngineSimulator::writeToSynthesizer`, `src/piston_engine_simulator.cpp:370-413` | recorded by the renderer, read by `LoadDump` |
| Input rate per frame | `Simulator::startFrame`, `src/simulator.cpp:72`; `Synthesizer::setInputSampleRate`, `src/synthesizer.cpp:274-279` | `Synthesizer.SetInputSampleRate` |
| Linear interpolation to 44.1 kHz with the write offset and its wrap at the buffer size, then the 1900 Hz double low pass per channel | `Synthesizer::writeInput`, `src/synthesizer.cpp:168-195`; `Synthesizer::inputDistance`, lines 268-272 | `Synthesizer.WriteInput`, `inputDistance` |
| End of a frame: drop the rendered input, record the latency | `Synthesizer::endInputBlock`, `src/synthesizer.cpp:197-213` | `Synthesizer.EndInputBlock` |
| Render block: at most 2000 samples in the output buffer, per-block refresh of the air noise cutoff and the jitter scale | `Synthesizer::renderAudio()`, `src/synthesizer.cpp:222-256` | `Synthesizer.RenderAudio` |
| Per sample and channel: unused `rand()` draw `r_0`, jitter, DC removal, derivative, air noise from `rand()` through the low pass of channel 0 for every channel, mix with `dF_F_mix`, subnormal flush, convolution | `Synthesizer::renderAudio(int)`, `src/synthesizer.cpp:281-316` | `Synthesizer.renderSample` |
| Per sample: output low pass, leveller, volume, `lround`, clamp to int16 | `src/synthesizer.cpp:318-330` | `Synthesizer.renderSample` |
| Output read | `Synthesizer::readAudioOutput`, `src/synthesizer.cpp:141-159` | `Synthesizer.ReadAudioOutput` |
| Jitter: 10-sample history read at a random, low-passed fractional position | `JitterFilter::fast_f`, `include/jitter_filter.h:22-47`; `JitterFilter::initialize`, `src/jitter_filter.cpp:14-26` | `jitter.fastF`, `newJitter` |
| Fourth-order Butterworth low pass | `ButterworthLowPassFilter::fast_f` and `setCutoffFrequency`, `include/butterworth_low_pass_filter.h:28-73` | `butterworthLowPass.fastF`, `setCutoffFrequency` |
| One-pole low pass | `LowPassFilter::fast_f`, `setCutoffFrequency`, `include/low_pass_filter.h:15-24` | `lowPass.fastF`, `setCutoffFrequency` |
| Derivative | `DerivativeFilter::f`, `src/derivative_filter.cpp:12-17` | `derivative.f` |
| Direct-form convolution over a circular shift register | `ConvolutionFilter::f`, `src/convolution_filter.cpp:37-52` | `convolution.f` |
| Leveller: peak decay 0.999 per sample, gain clamped and smoothed 0.9/0.1 | `LevelingFilter::f`, `src/leveling_filter.cpp:17-34` | `leveling.f` |
| Ring buffer, including the full buffer that reports size 0 | `RingBuffer`, `include/ring_buffer.h` | `ringBuffer` |
| Driver order: per frame the steps, `endFrame`, then blocks until the input is empty | `engine_sim_render.cpp` main loop | `Render` |

The port has no render thread. `RenderAudio` renders where the engine-sim thread wakes, and returns 0 where the thread would wait.

## Numerics

`Float64` computes in float64 throughout, with the engine-sim constants: `constants::pi` is 3.14159265359, and the float filters use it cast to float. Float literals and the parameters that engine-sim stores as float keep their float values, for example `dF_F_mix` 0.002f is 0.0020000000949949026.

`Float32` rounds to float32 after each operation that engine-sim computes in float, and keeps double where engine-sim computes in double: the input interpolation, the 1900 Hz input filter, the coefficient sums of the Butterworth filters, and the leveller smoothing `0.9 * a + 0.1 * b`. Each float operation of engine-sim is one float64 operation followed by a rounding to float32, which gives the same result because float64 holds more than twice the float32 precision. The Go compiler does not fuse multiply-adds at `GOAMD64=v1`. On a platform that fuses them, `Float32` still converts each product, but `Float64` can differ in the last bit.

Two numerical details decided the bit-exact match:

- `math.Tan` is 1 ulp above glibc for the 1900 Hz input filter. That changed the rounding of a few input samples to float and gave differences of up to 3 steps in 2 per cent of the samples of the `v8-ls` sweep without noise. The port computes tan and cos with `math/big` and rounds them correctly. The glibc results of the pinned build agree with the correctly rounded values for these arguments, which the null test confirms.
- The float filters call `tanf`. The port rounds the exact value to float32 directly.

## Random numbers

engine-sim uses two generators, and the port reproduces both exactly.

- `rand()`: `renderAudio(int)` draws twice per channel and sample. glibc defines `rand()` as `(int) random()`, the additive feedback generator of degree 31 (TYPE_3). engine-sim never calls `srand`, so the seed is 1. `Random` ports `srandom_r` and `random_r`. `CombustionChamber::ignite` also draws from `rand()` (`src/combustion_chamber.cpp:211`) during the simulation steps, between the render blocks. The renderer counts the `rand()` calls with its own `rand()` definition, and records the calls outside the synthesiser before each block. `Render` discards that many values before each block.
- `std::default_random_engine` in each `JitterFilter`: `std::minstd_rand0` with seed 1 in libstdc++. `std::uniform_real_distribution<float>(0, 9)` calls `generate_canonical<float, 24>`. libstdc++ 16, which the pinned build links, implements the P0952 algorithm (`__generate_canonical_any` in `bits/random.tcc`): one engine draw, divided as an integer by 127, converted to float, divided by 2^24, and drawn again when the result is 1. `minstdRand0.uniformFloat32` ports it. An older libstdc++ gives other jitter values.

## Null test

Each render uses `--volume 0.5 --rate 44100` and the default 3 s pre-roll. Holds is the scenario `holds-t050.scn` of the engine, sweep is `sweep-wot.scn`. Off and on are the noise and jitter, with and without `--no-noise`. The difference is over the whole output, the pre-roll included, in 16-bit steps.

| Engine | Scenario | Noise | Samples | `Float32` differing samples | `Float64` peak (dB below peak) | `Float64` RMS (dB below RMS) |
| --- | --- | --- | --- | --- | --- | --- |
| `v8-ls` | holds | off | 1190701 | 0 | 3 (77.6) | 0.515 (84.4) |
| `v8-ls` | holds | on | 1190701 | 0 | 3 (77.5) | 0.510 (84.4) |
| `v8-ls` | sweep | off | 485101 | 0 | 3 (77.5) | 0.501 (84.3) |
| `v8-ls` | sweep | on | 485101 | 0 | 3 (77.5) | 0.492 (84.4) |
| `i4-vtec` | holds | off | 1499401 | 0 | 3 (76.4) | 0.551 (84.6) |
| `i4-vtec` | holds | on | 1499401 | 0 | 4 (74.0) | 0.554 (84.6) |
| `i4-vtec` | sweep | off | 485101 | 0 | 3 (76.9) | 0.539 (84.7) |
| `i4-vtec` | sweep | on | 485101 | 0 | 3 (77.2) | 0.542 (84.6) |
| `f4-ej25` | holds | off | 1190701 | 0 | 3 (76.5) | 0.554 (84.8) |
| `f4-ej25` | holds | on | 1190701 | 0 | 3 (76.8) | 0.555 (84.7) |
| `f4-ej25` | sweep | off | 485101 | 0 | 3 (76.5) | 0.562 (84.8) |
| `f4-ej25` | sweep | on | 485101 | 0 | 3 (76.7) | 0.561 (84.7) |
| `v12-412` | holds | off | 2425500 | 0 | 3 (77.4) | 0.505 (84.3) |
| `v12-412` | holds | on | 2425500 | 0 | 3 (77.5) | 0.493 (84.3) |
| `v12-412` | sweep | off | 485100 | 0 | 3 (77.4) | 0.510 (84.3) |
| `v12-412` | sweep | on | 485100 | 0 | 3 (77.3) | 0.499 (84.3) |
| `v12-merlin` | holds | off | 573301 | 0 | 3 (77.7) | 0.602 (84.2) |
| `v12-merlin` | holds | on | 573301 | 0 | 3 (79.0) | 0.521 (84.0) |
| `v12-merlin` | sweep | off | 485101 | 0 | 4 (74.8) | 0.628 (83.8) |
| `v12-merlin` | sweep | on | 485101 | 0 | 3 (78.6) | 0.520 (83.7) |

Tolerances: `Float32` must be identical. `Float64` must stay within 6 steps peak and 1 step RMS. `internal/esynth` tests both on two committed `i4-vtec` excerpts in `testdata/esynth/` (0.6 s pre-roll and a 0.2 s hold at 3000 rpm, with and without noise).

### Noise and jitter on

`snarl-compare` compares the engine-sim output (`ref.wav`) with the `Float64` port output (`port.wav`) of the noise-on renders, after the pre-roll.

The worst window of each metric over the scenario:

| Engine | Scenario | Worst third-octave distance (dB) | Worst order spectrum distance (dB) |
| --- | --- | --- | --- |
| `v8-ls` | holds | 0.027 (low band 0.043) | 0.001 |
| `v8-ls` | sweep | 0.005 | 0.001 |
| `i4-vtec` | holds | 0.021 | 0.003 |
| `i4-vtec` | sweep | 0.008 | 0.003 |
| `f4-ej25` | sweep | 0.005 | 0.002 |
| `v12-412` | sweep | 0.002 | 0.001 |
| `v12-merlin` | sweep | 0.000 | 0.001 |

The residual (`--residual`) and burst (`--burst`) tables of the `v8-ls` and `i4-vtec` holds agree within 0.1 dB in every column and band below 16 kHz, and the flatness within 0.001. The residual band from 16 to 24 kHz is 77 to 94 dB below the signal, at the 16-bit rounding level, where the float rounding of engine-sim adds up to 2.7 dB. Tolerances: 0.1 dB for the third-octave distance, 0.2 dB for the residual and burst values below 16 kHz, and 3 dB for the residual from 16 to 24 kHz.

## Not matched, and limits

- `Float64` is not bit exact. The difference is the float rounding of engine-sim, and `Float32` removes it.
- The exact jitter depends on the C++ library of the build. The port follows libstdc++ 16, which the pinned build links. The app on Windows uses the Microsoft library, whose `uniform_real_distribution` and `default_random_engine` differ.
- In the app, the render thread wakes at times that depend on the scheduler, so the block sizes and the interleave of the `rand()` calls are not reproducible. The renderer runs the blocks in one thread, and the record holds their order.
- The impulse responses are not all 10000 taps, as `docs/research/engine-sim.md` states. After the trim, `mild_exhaust.wav` (`i4-vtec`, `f4-ej25`) has 1893 taps, `smooth_39.wav` (`v8-ls`) 6907 and `minimal_muffling_01.wav` (both V12 engines) 5979.
- The port takes the input that engine-sim writes into the synthesiser. It does not port the physics that computes that input.
