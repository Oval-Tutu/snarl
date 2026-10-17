# Prior art: low-cost engine sound synthesis for games

Status: research complete for the six questions set by the coordinator. Date: 2026-10-17.

Scope: published procedural and physically informed methods, sample-based methods in shipped racing games, procedural middleware, crackle and pop techniques, objective comparison methods, and a recommended tiered model for `snarl`. The C++ projects engine-sim and EngineLab are out of scope (other workers cover them).

Each material claim carries a source URL. Claims without a source are marked `unverified`. Secondary mirrors of talks are marked as mirrors.

## Summary

The pulse-train-plus-resonator structure is the common core of every documented procedural engine model from Farnell (2010) through Baldan et al. (2015) to Doerfler and Wyse (2026). Each firing injects a shaped pressure pulse at a crank-phase offset set by the firing order. The pulses pass through delay-line resonators (Karplus-Strong or digital waveguides) that stand in for the manifold, pipe and muffler. Throttle gates a combustion noise layer, and overrun gates an air-flow layer. Pops are stochastic impulses injected into the exhaust during RPM decay. No published source reports a measured CPU figure for a direct physically informed model on an ARM core, so `snarl` must measure its own cost. The nearest figures are a wavetable lookup version at 5 to 6 per cent of a desktop CPU core (AK Audio) and a granular ramp player at 0.96 per cent of a 2.7 GHz core (Crankcase REV, slide mirror). A pulse train with three to five delay lines is far cheaper than either, so the 5 per cent budget on a Cortex-A57 is plausible, but it is an estimate until measured.

For the go or no-go gate, the sample-based comparison point is a loop crossfade system: steady-RPM loops every 500 RPM across exhaust, engine and intake perspectives, with on-load and off-load layers. Project CARS used well over one hundred wave files per car. Granular ramp players need only three files per car but cost more CPU than loops and need tonal source ramps.

## 1. Procedural and physically informed methods

| Method | Model | Inputs | Reported CPU cost | Reported quality | Fit to the snarl budget |
| --- | --- | --- | --- | --- | --- |
| Farnell, Designing Sound, Practical 22 (2010) | Phasor per engine cycle, four cylinders at 0, 0.25, 0.5, 0.75 phase offsets, cosine half-cycle pulses with low-frequency noise jitter, waveshaping and FM overtones, delay-chain exhaust with feedback | Engine speed 0 to 1 | Not reported. Pure Data patch of a few dozen objects (`unverified` cost) | Baldan et al. judge it poor in higher harmonics and hard to control | Good as a floor. The jitter and four-phase pulse layout are reusable |
| Baldan, Lachambre, Delle Monache, Boussard (2015) | Four-stroke functions per cylinder (intake, exhaust, piston, ignition) drive mutually connected digital waveguides for cylinders, intake collectors, extractors, straight pipe, four muffler elements and outlet. Three outputs: intake, block, exhaust. Cycle asymmetry gives growl | RPM, throttle 0 to 1, cylinder count 1 to 12, volume, compression, ignition time, asymmetry, pipe lengths in metres, muffler action | "Low computational cost, and no audible delay" from informal tests. No number. Max/MSP external | Informal listening tests: "convincing degree of realism". Idle spectrogram matches a recorded car but has less mid-high energy | Good. Waveguides are delay lines with gains. The full interacting network is the Tier 2 option |
| Doerfler and Wyse, PTR model (EUSIPCO 2026) | Per-cylinder pulse train: derivative-of-cosine harmonic sum with learned harmonic decay, pressure-release envelope `(1-exp(-a*phi))*exp(-b*phi)`, thermodynamic phase bending, V8 firing order with +/-40 degree per-cylinder timing, ERB noise bank gated by throttle factor and DFCO factor, two bank Karplus-Strong resonators into one shared resonator | RPM, torque, first and second differences, at 125 Hz frame rate | Not reported. Audio at 16 kHz, neural controller (MLP and GRU) | 21 per cent better harmonic reconstruction and 5.7 per cent lower total loss than a harmonic-plus-noise baseline on 7.5 hours of audio across an inline-four and two V8s | The synthesis side (pulses plus three resonators) is cheap and is the best current blueprint. Drop the neural controller and use hand-tuned parameter tables |
| Doerfler and Wyse, dataset paper (EUSIPCO 2026, arXiv 2603.07584) | Analysis-driven harmonic-plus-noise synthesis: per-order fingerprint (frequency deviation and magnitude for orders 0.5 to 64) stored as tables over RPM and torque, 128 sine oscillators read by bilinear interpolation, plus parameterised noise and resonators | RPM, torque | Not reported (offline dataset generation at 48 kHz) | Per-order magnitudes against RPM and torque match the source recordings across all eight subsets | The order table is a compact additive alternative to pulses and the analysis code is the basis for our comparison tool |
| Doerfler, Kuntz, Zimmer, EONE (AES Automotive Audio 2026, arXiv 2606.21521) | Engine order plus noise band synthesis. Gain curves over RPM and torque per order and per noise band, learned by analysis-by-synthesis, then exported as JSON to a conventional DSP: interpolated lookup oscillators plus pre-rendered noise bands at 16 kHz | RPM, torque | Not reported as a number. Runs on automotive embedded hardware inside EVx Suite with no neural network at runtime | Listening test: naive listeners rated it comparable to real recordings, expert listeners could tell the two apart, and it rated significantly above conventional order enhancement for both groups | First formal listening test in this literature. The two-curve gain table per order is a compact control scheme that snarl can copy for an additive tier |
| Amman and Das, Ford (2001) | Deterministic-stochastic decomposition: synchronous DFT extracts engine orders, residual modelled by multipulse-excited time series | Recorded engine audio and RPM | Not stated in the abstract | Numerical and subjective tests reported | Analysis side is useful for the comparison tool. Synthesis is analysis-resynthesis, so it needs recordings |
| Umemura and Kozawa, SAE 910619 (1991) | Inverse FFT from order components per segment with waveform continuity between segments | Order amplitudes versus RPM | About 100 times faster than a direct additive method on a DSP of the time | "Good sound quality" for timbre modulation | Shows that an order-domain table plus inverse FFT is a viable cheap additive path |
| Yamaha patent US5835605A (1995) | Stored exhaust waveforms selected by RPM and engine output, then a waveguide exhaust pipe circuit of delays and scattering junctions | Accelerator position, engine speed | Not stated | Not stated | Confirms the pattern "small excitation table plus waveguide pipe" that Tier 0 can use |
| GeneCars, Genesis (cited by Baldan) | Additive synthesis of Bark-scale partials conditioned on RPM and pedal load | RPM, load | Not stated | Industrial sound design use | Additive is a fallback if pulses fail, but it needs analysis data |
| Jagla, Maillard, Martin, JASA (2012) | Pitch-synchronous overlap-add on one engine cycle per grain, extracted from a continuous RPM sweep | Target RPM trajectory | "Low computational load" | "High quality audio restitution" | Sample-based. This is the academic form of the granular ramp method in section 2 |

Firing-rate facts used by every model. One four-stroke cycle is two crank revolutions, so the cycle phasor runs at `RPM/120` Hz (Baldan et al.; PTR uses the same `f0 = RPM/120`). For N cylinders with even firing, the firing rate is `N/2 * RPM/60` Hz. Engine orders sit at integer and half-integer multiples of crank frequency, and the half-integer spacing exists because each cylinder fires once per two revolutions (Doerfler and Wyse, dataset paper). The PTR authors note that combustion rates of 600 to over 8000 RPM give 10 to 133 Hz per cylinder, that harmonics reach down to 5 Hz, and that pulse intervals fall below 2 ms, which is why they argue for pulse modelling rather than harmonic synthesis.

Open-source implementations of these methods:

- `antonio-r1/engine-sound-generator`: a JavaScript AudioWorklet and a C++ to WebAssembly port of the Baldan et al. model, with waveguide lengths in samples, reflection factors, four muffler element lengths and an `action` parameter. The author ported to WebAssembly "for making the sound generator more efficient and removing the glitches". <https://antonio-r1.github.io/engine-sound-generator/>
- `mrdoob/Starter-Kit-Racing`, commit 40942fb (2026-07-07): a 427-line `EngineWorklet.js` with "4-stroke firing model, fuel-cut overrun crackle, exhaust-pipe comb, formants, mechanical layer" that replaced sampled engine audio. <https://github.com/mrdoob/Starter-Kit-Racing/commit/40942fbcefc833a80db6583040fcba27e0e7e6c1>
- PTR code, weights and audio examples. <https://rdoerfler.github.io/ptr-model-page/>
- Farnell's Pure Data patches. <https://aspress.co.uk/sd/practical22.html>

Sources for this section:

- Baldan et al. 2015, PDF: <https://air.iuav.it/bitstream/11578/264484/1/SIVE15_submission_4.pdf> (DOI <https://doi.org/10.1109/sive.2015.7361287>)
- Doerfler and Wyse 2026, PTR: <https://arxiv.org/abs/2603.09391>
- Doerfler and Wyse 2026, dataset paper: <https://arxiv.org/abs/2603.07584>, project page <https://rdoerfler.github.io/procedural-engine-sounds-page/>, analysis code <https://github.com/rdoerfler/engine-order-analysis>
- Doerfler, Kuntz, Zimmer 2026, EONE: <https://arxiv.org/abs/2606.21521>, audio examples <https://rdoerfler.github.io/eone-model-page/>
- Amman and Das 2001: <https://doi.org/10.1109/41.904583>
- Umemura and Kozawa 1991: <https://doi.org/10.4271/910619>
- Yamaha patent: <https://patents.google.com/patent/US5835605A/en>
- Jagla et al. 2012: <https://doi.org/10.1121/1.4754663>
- Digital waveguide definition (bidirectional delay line): <https://ccrma.stanford.edu/~jos/pasp/Digital_Waveguides.html>

## 2. Sample-based methods in commercial racing games

| Method | Assets per car | Cost and memory | Strengths | Weaknesses |
| --- | --- | --- | --- | --- |
| Steady-RPM loop crossfade (Wwise Blend Container, FMOD parameter sheets) | Idle plus a loop every 500 RPM (250 RPM at low RPM where possible), for each of exhaust, engine and intake, with on-load and off-load variants. Project CARS: "well over one hundred separate wave files" per car in FMOD | Cheapest CPU of the sample methods (BOOM primer). Memory is the sum of all loops. No figure published for a full car (`unverified`) | Widely used, simple, keeps recorded character, best for simulators per Greg Hill | Audible looping at static RPM (AK Audio). Pitch stretch beyond 500 RPM from the loop's native RPM (BOOM). Spectral jump between loops at 1000 RPM spacing forced heavy smoothing that lost engine character (Bleeps and Pops). Needs a dynamometer or neutral revs recording session |
| Granular ramp player (Crankcase REV, Krotos Igniter, Codemasters DiRT 2 and F1 2010, GTA V, Forza Horizon 5) | Minimum three files: acceleration ramp, deceleration ramp, idle loop. REV can simulate deceleration by reversed acceleration through a low-pass filter | REV tech brief: 0.96 per cent of a 2.7 GHz core for a single ramp, 4 KB single model and 8 KB full model, ADPCM 3.7:1 (slide mirror, the KB figures look like runtime state rather than audio, `unverified`). Granular "will hit your CPU harder than a loop based system" (BOOM) | Continuous RPM, no loop seams, far fewer assets, "more natural and dynamic" (Audiokinetic). REV slices ramps at individual piston firing cycles and skips or repeats cycles for speed | Needs tonal ramps. Noisy ramps "may be poorly analyzed and captured" and this "cannot be corrected" (Audiokinetic). Interpolation "pretty synthetic sounding in certain situations" so filters mask it (Greg Hill). Crackles and pops "scramble the tracking" so they must be separate layers (Greg Hill). Deceleration ramps need track recording, not a dynamometer (Forza Horizon 5) |
| Hybrid loop plus granular plus synthesis | Forza Horizon 5: sweep up, sweep down, gear cracks, limiter samples and idles per car, plus a shared library of pops and bangs assembled at runtime by engine type. GTA V: granular engine, synthesised intake and transmission, NPC engines from exported loop data | Not published | Shared libraries make 500 plus cars "a measurable quantity" (Forza Horizon 5). NPC fallback to loops saves CPU (GTA V) | Highest production effort |
| Physics-derived wavetable (AK Audio) | One precomputed 2D table over RPM and throttle per engine, about 22 MB for a Pontiac G8 at high quality, plus an exhaust impulse response | Engine synth alone 5 to 6 per cent CPU, full vehicle system about 8 per cent (platform not stated) | No recordings. New engine in hours from spec sheet | 22 MB per engine is large for a mobile target. Lookup cost is still above the snarl budget on its own |

The NADS driving simulator paper gives a concrete crossfade rule: at 1650 RPM the 1500 and 2000 RPM loops play at 70 and 30 per cent, each pitch-shifted to the current RPM, with a pure tone at the firing frequency added at low RPM to restore the low end lost in recording. <https://www.nads-sc.uiowa.edu/bio_pub_pdf/Heitbrink_&_Cable_-_Design_of_a_driving_simulation_sound_engine.pdf>

Sources for this section:

- Audiokinetic blog, sampling versus granular in Wwise (2025): <https://www.audiokinetic.com/en/blog/engine-sound-modeling-from-sampling-to-granular-synthesis-in-wwise/>
- BOOM Library car engine primer (Mike Caviezel, 2026): <https://www.boomlibrary.com/blog/the-car-engine-sound-primer-mike-caviezel/>
- Designing Sound interview, Project CARS, Forza 5 and REV (2014): <https://designingsound.org/2014/08/11/vehicle-engine-design-project-cars-forza-motorsport-5-and-rev/>
- Forza Horizon 5 interview (2021): <https://www.asoundeffect.com/forza-5-game-audio/>
- GTA V audio, GDC 2014 slides (mirror): <https://gta.com.ua/img/news/sound-of-gta5-gdc2014.pdf>
- Codemasters granular synthesis history (Simon Goodwin): <http://simon.mooli.org.uk/AF/article/codies.html>
- Crankcase REV tech brief (slide mirror): <https://www.slideserve.com/dee/crankcase-audio-inc-presents-powerpoint-ppt-presentation>
- Crankcase REV product page: <https://www.crankcaseaudio.com/>
- Bleeps and Pops interactive car audio system (2011): <https://bleepsandpops.com/post/2926721368/interactive-car-audio-system>
- AK Audio procedural engine system: <https://akaudio.com/en/portfolio/akenginesynth.html>
- Turn 10 talk report, AES PNW (2013), 800 plus cars recorded, loops plus granular, full synthesis "of great interest for the future": <https://www.aes-media.org/sections/pnw/pnwrecaps/2013/jan_vroom/>

## 3. Procedural engine audio middleware and libraries

| Product | Approach | Platforms | Published claims | What it shows |
| --- | --- | --- | --- | --- |
| Tsugi GameSynth Engines plug-in | Physical modelling, developed with a former Yamaha Motors acoustical engineer | GameSynth tool, run-time integration on request, also renders loops for FMOD | "Negligible memory footprint", "much higher realism than with granular synthesis", presets for formula 1, sports car, bike, truck | A commercial vendor sells physical modelling as the successor to granular |
| Sirocco (2026) | Per combustion event synthesis from RPM, throttle, load, boost and gear, with intake and exhaust resonance, voicing layers | Unity native plug-in, FMOD export, C FFI, WebAssembly demo | 48 kHz, allocation-free and lock-free on the audio thread, 120 plus named engine presets including a 1975 3.0 L flat-12, overrun crackle, backfire, half-order growl. No CPU figure | A one-person product reaches shippable quality with a per-event model. The preset list covers all four snarl profiles |
| AK Audio procedural engine | Physical model precomputed into a 2D wavetable, IR convolution for the exhaust | Wwise, FMOD, Unity, Unreal, REAPER | 5 to 6 per cent CPU for the engine synth, 22 MB per engine | Precomputation trades memory for CPU and still lands near the snarl budget |
| Nemisindo Engine | Procedural, browser-based models and a Unity plug-in | Unity, web | No technical details published | Market presence only |
| Crankcase REV | Granular ramp player with gearbox, clutch and pop simulation | PC, Mac, iOS, Android, Xbox, PlayStation, Wwise, FMOD, Unreal | Full runtime source available to licensees | The granular reference implementation |
| Krotos Igniter Live | Granular plus loops plus a four-voice wavetable synth layer | Wwise 2019.1 to 2019.2 | "Extremely efficient CPU and memory footprint", six-layer loop export as one voice | Hybrid designs are normal in middleware |
| Impact SFX Synthesizer (Unreal MetaSounds) | Modal and impact synthesis nodes with a vehicle engine synthesizer graph | Unreal Engine | "Low CPU and memory costs", 100 nodes tested in Lyra | Node-graph engines in a mainstream game engine |

Feasibility conclusion. Three vendors (Tsugi, Sirocco, AK Audio) ship procedural engines with RPM and throttle as the control interface, which is the same interface `snarl` receives from the game. None publishes an ARM CPU figure, and the only desktop figure (5 to 6 per cent for a table lookup) is at the top of the snarl budget, so the direct pulse-plus-resonator path is the one most likely to fit on a Cortex-A57.

Sources for this section:

- Tsugi Engines plug-in: <http://www.procedural-audio.com/web/en/products-gamesynth-enginesplugin.html>
- Tsugi GameSynth run-time: <https://tsugi-studio.com/web/en/products-gamesynth-runtime.html>
- Tsugi blog, Engines plug-in to FMOD (2022): <https://tsugi-studio.com/blog/2022/05/19/from-engine-plug-in-to-fmod-studio/>
- Sirocco: <https://audio.siroccoracing.com/>
- Nemisindo: <https://nemisindo.com/>
- Krotos Igniter Live: <https://www.krotosaudio.com/igniter-live/>
- Impact SFX Synthesizer: <https://www.unrealengine.com/marketplace/en-US/product/c7a1e47c4a2f452d8bedf2d74c02303c>

## 4. Exhaust crackles, pops and backfire

Physical cause. A backfire is combustion in the exhaust system rather than the cylinder, caused by ignition with an exhaust valve open or by unburnt fuel reaching the hot exhaust. In high-performance cars, lifting off after an upshift gives a moment of rich running and an incomplete burn that explodes in the exhaust with a pop or bang. Sports exhausts and anti-lag systems make it more frequent. The TVR Cerbera is a documented case of frequent backfire on engine braking. <https://en.wikipedia.org/wiki/Back-fire>

Techniques in the literature and in games:

| Source | Technique | Trigger |
| --- | --- | --- |
| Baldan et al. 2015 | Poisson process: once per engine cycle while RPM falls, test a probability against a random value, on success inject an impulse shaped like the ignition function at the exhaust outlet input, then square the probability so events become rarer over time | RPM decreasing |
| PTR (Doerfler and Wyse 2026) | Deceleration fuel cut-off (DFCO) regime: combustion noise gated off by a throttle factor `max(torque, 0.02)^0.7`, steady air-flow noise gated on by `max(-torque, 0.02)` | Torque sign |
| Forza Horizon 5 | Library of recorded pops and bangs categorised by aggression and assembled per car at runtime, so two cars with the same engine still differ. 32-bit float recorders used because backfires clipped earlier recordings | Game events |
| Crankcase REV | "Pop simulation" in the full model (accel, decel, idle and pops) | Runtime model |
| Greg Hill (Project CARS, iRacing) | Keeps crackles and pops inside hybrid loops because they scramble granular tracking | Baked into loops |
| Audiokinetic blog | Adds a separate Backfire layer on top of the engine model | RTPC events |
| Sirocco | Overrun crackle, backfire, part-throttle crackle and anti-lag bang as toggled per-preset layers that "emerge from physics" | Telemetry |
| mrdoob Starter-Kit-Racing | "Fuel-cut overrun crackle" inside the worklet synth | Throttle cut |

Recommendation for `snarl`. Model pops as an event layer that injects shaped impulses into the exhaust resonator, not as a separate sample. Use the Baldan trigger (per cycle while RPM falls, probability that decays after each event) and the PTR regime gate (throttle closed and negative load) so crackle density follows how hard the driver lifted. Give each engine profile its own pop amplitude, duration and spectral tilt. Keep a one-shot sample layer behind the same event API as the gate fallback, which matches how Audiokinetic and Forza Horizon 5 treat pops.

## 5. Objective comparison of two engine renders

| Metric | What it measures | How to compute | Use in snarl | Source |
| --- | --- | --- | --- | --- |
| Order spectrum (order tracking, angular resampling) | Energy at multiples of crank speed, independent of RPM changes. Accelerating shafts smear frequency bands, resampling to constant crank angle turns them into sharp peaks | Integrate RPM to crank phase, resample the audio at constant angle increments, take the FFT. Vold-Kalman filters are the alternative | Both renders have an exact RPM track, so resampling is exact. Compare per-order energy for orders 0.5 to about 20. Half orders carry the cross-plane V8 burble | <https://en.wikipedia.org/wiki/Order_tracking_(signal_processing)> |
| Harmonic (order-masked) loss | Frame-wise energy along predicted engine-order tracks, inspired by Campbell diagrams | Mask a high-resolution magnitude spectrogram along harmonic tracks derived from instantaneous RPM. PTR used FFT 65536, window 16384, hop 256 at 16 kHz | A direct, published metric for engine renders. Use it as the primary tonal distance | <https://arxiv.org/abs/2603.09391> |
| Per-order magnitude against RPM and torque | Energy of each order (0.5 to 64) as a surface over the operating space | Pitch-adaptive resampling, frequency-aligned FFT, centroid-based order tracking. Open analysis code exists | Plot both renders as order surfaces and difference them. This is the published validation method for synthetic engine audio | <https://arxiv.org/abs/2603.07584>, <https://github.com/rdoerfler/engine-order-analysis> |
| Synchronous DFT | Extracts the deterministic engine-order component so the residual noise can be compared separately | DFT synchronised to the engine cycle | Split each render into order part and residual, compare each | <https://doi.org/10.1109/41.904583> |
| Log-spectral distance (LSD) | Root mean square difference of two log power spectra. Symmetric, equal to cepstral distance | `sqrt(mean((10*log10(P/Q))^2))` over bins or bands | Per-band balance on a long-term average spectrum in third-octave bands, per RPM and throttle cell | <https://en.wikipedia.org/wiki/Log-spectral_distance> |
| Multi-resolution STFT loss | Spectral convergence plus linear and log magnitude differences at several FFT sizes | PTR used FFT sizes from 32768 down to 32, 75 per cent overlap, Hann window, equal weights. DDSP introduced the multi-scale form | Captures both long tones and sub-2 ms pulse structure | <https://arxiv.org/abs/2603.09391>, <https://arxiv.org/abs/2001.04643> |
| ffmpeg `aspectralstats` | Per-frame centroid, spread, flatness, crest, rolloff, flux of the magnitude spectrum | `ffmpeg -i in.wav -af aspectralstats,ametadata=mode=print:file=spectral.log -f null -` | Cheap first-pass per-band balance and brightness. Spectral crest and flux track pulse articulation at low RPM | <https://ffmpeg.org/ffmpeg-filters.html> |
| ffmpeg `astats` crest factor | Time-domain peak to RMS ratio (linear) | `ffmpeg -i in.wav -af astats -f null -` | Impulsiveness of the render, which the PTR authors name as the perceptual gain of pulse models | <https://ffmpeg.org/ffmpeg-filters.html> |
| ffmpeg `ebur128` | Integrated loudness (LUFS), loudness range (LU), true peak (dBTP) per ITU-R BS.1770 | `ffmpeg -i in.wav -af ebur128 -f null -` | Normalise both renders to equal integrated loudness before any spectral distance | <https://ffmpeg.org/ffmpeg-filters.html>, <https://www.itu.int/rec/R-REC-BS.1770/en> |
| Spectrogram image | Time against frequency with magnitude as colour | `ffmpeg -i in.wav -lavfi showspectrumpic=s=1280x480:scale=log:fscale=log out.png` | Visual check, the method Baldan et al. used to compare synthetic and recorded idle | <https://ffmpeg.org/ffmpeg-filters.html> |
| Fréchet Audio Distance | Distance between embedding distributions of two sets of clips. Correlation 0.52 with human ratings, against 0.39 for SDR | Needs a pretrained embedding model and a reference set | Low priority. Heavy, and the embedding was trained on music | <https://arxiv.org/abs/1812.08466> |

Recommended comparison tool. Drive both renders with the same RPM and throttle script. Normalise with `ebur128` integrated loudness. Then compute, per RPM and throttle cell: order spectrum distance for orders 0.5 to 20, third-octave LSD, multi-resolution STFT distance, and the crest factor and spectral flux time series. Report each cell and the worst cell. Keep a spectrogram pair per cell for the human review at the gate.

## 6. Recommended model structure for snarl, with tiers

Common signal chain, all tiers. Each arrow is a stage.

1. Crank phasor at `RPM/120` Hz (Baldan et al., PTR).
2. Per-cylinder firing phases from the firing order and bank layout, with optional per-cylinder timing offsets of up to 40 crank degrees and per-cylinder gains (PTR) or cycle asymmetry for growl (Baldan et al.).
3. One shaped pulse per firing: attack and decay envelope, harmonic decay for high-frequency roll-off, optional phase bending for the downward pitch glide inside each pulse (PTR). Add slow noise jitter to phase and amplitude (Farnell).
4. Sum per bank, then resonators: one Karplus-Strong or waveguide comb per bank feeding a shared tail resonator (PTR), with an optional set of four partially reflecting muffler elements tuned so each frequency peaks in at most one element (Baldan et al.).
5. Noise layers: combustion turbulence as amplitude modulation on the pulses, intake noise through an intake resonator, both gated by throttle. Steady air-flow noise gated by overrun (PTR, Baldan et al. intake noise).
6. Event layer: pops as impulses into the exhaust outlet (section 4), rev limiter cut as pulse suppression, gear change as a load reversal.
7. Output mix per listening point: exhaust, intake, block (Baldan et al. three outputs).

| Tier | Target | Content | Supporting sources |
| --- | --- | --- | --- |
| 0 | Nintendo Switch, OnePlus 3T | Precomputed pulse shape per engine profile in a small table (the Yamaha and AK Audio pattern, kilobytes not megabytes), two bank combs plus one shared tail with one-pole damping, one filtered noise source gated by throttle and overrun, pops as impulses. Cost estimate: a few multiply-adds per sample plus three delay-line reads (`unverified`, my estimate) | Yamaha US5835605A, AK Audio, PTR resonator layout, Farnell |
| 1 | Mid-range PC and phones | Live per-cylinder pulse shaping with per-cylinder gain and timing offsets, intake resonator output, four muffler elements, thermodynamic phase bending | PTR, Baldan et al. |
| 2 | Modern PC | Full Baldan-style network of interacting waveguides for cylinders, collectors, pipe, muffler and outlet, optional exhaust impulse response convolution, multiple listening points for camera views | Baldan et al., AK Audio |

Engine profile facts that the firing-order stage must encode:

| Profile | Firing geometry | Expected audible character | Source |
| --- | --- | --- | --- |
| Inline-4 | Even 180 degree intervals | "Distinct, even pulses" (Forza 2 GDC slide, mirror) | <https://www.sambuz.com/doc/forza-motorsport-2-pdf-document-839764> |
| Flat-6 | Even 120 degree intervals, overlapping power strokes, perfect primary and secondary balance in boxer form | "Overlapping pulses (chord-like)" (Forza 2 slide) | <https://en.wikipedia.org/wiki/Flat-six_engine> |
| Cross-plane V8 | Even 90 degree intervals overall, but each bank fires at 180-90-180-270 degree intervals. Merging each bank into one pipe produces the "potato-potato" burble | Half-order content and bank-level unevenness. Sirocco lists "half-order growl" as a layer | <https://en.wikipedia.org/wiki/Crossplane> |
| Flat-12 (Ferrari Testarossa 1984 to 1991) | 180 degree V12 crankshaft, pairs of pistons share a crankpin, so it is flat but not a true boxer. Six cylinders per bank. Testarossa: 4.9 L Tipo F113, 48 valves, 390 PS at 6300 rpm, 490 Nm at 4500 rpm | Even 60 degree firing (`unverified` for the Testarossa specifically). Bank exhaust layout `unverified` | <https://en.wikipedia.org/wiki/Flat-twelve_engine>, <https://en.wikipedia.org/wiki/Ferrari_Testarossa> |

## Implications for snarl

- Build the pulse train and the three-resonator exhaust first. Every documented procedural model shares this core, and the PTR results show it beats harmonic-plus-noise on harmonic reconstruction.
- Put the per-engine identity in three places: firing geometry (section 6 table), pulse shape (envelope and harmonic decay), and resonator lengths and damping. Sirocco's 120 presets show that this parameter space covers all four profiles.
- Treat throttle and overrun as regime gates for noise layers, not only as a gain. The PTR authors found that STFT losses alone cannot supervise this, so the gate must be an explicit design choice.
- Treat pops as events into the resonator with a decaying probability, and keep a sample one-shot behind the same event API for the gate fallback.
- Measure CPU on the Cortex-A57 early. No source gives an ARM figure for a direct model. The only desktop figure near the budget is a table lookup at 5 to 6 per cent.
- Build the comparison tool before the prototype judgement. The published metric closest to our need is the order-masked harmonic loss, and loudness normalisation with `ebur128` must come first.
- Keep an additive order-table tier in reserve. EONE and the dataset paper show that per-order gain tables over RPM and load reach listener-rated realism and run on embedded DSP. If pulses fail the gate on timbre but pass on cost, this is a second procedural option before the sample fallback.
- For the sample-based fallback, plan for a loop crossfade with a 500 RPM step and on-load and off-load layers, not a granular player. Granular needs tonal ramps and more CPU, and the recordings do not exist for a 1986 flat-12 in our hands.

## Open questions

- No published CPU measurement exists for a direct physically informed engine model on an ARM core. The Tier 0 cost estimate is mine and is unverified.
- The Crankcase REV figures (0.96 per cent of a 2.7 GHz core, 4 KB and 8 KB) come from a slide mirror and the memory numbers are likely runtime state, not audio. Treat them as unverified.
- The Testarossa exhaust layout (whether each bank of six has its own system) is unverified and matters for the flat-12 bank resonator design.
- Whether a precomputed pulse table or live pulse synthesis is cheaper in Go on a Cortex-A57 is untested.
- Only EONE reports a formal listening test, and that is for an additive order-plus-noise model, not a pulse model. Baldan et al. report informal tests and PTR reports loss values. The quality bar "believable per engine type" has no published benchmark for a pulse-plus-resonator model.
- The EONE paper runs on automotive embedded hardware but its public text gives no CPU percentage. The paid AES version may. <https://aes.org/publications/elibrary-page/?id=23420>
- The PTR paper cites two DDSP engine papers I did not read: an engine-phase conditioned model with DCT-domain transients (its reference 9) and Motor2Synth (AES 2025, static-RPM loop generation for active sound design, its reference 10).
- The Gran Turismo 7 GDC 2023 talk describes an "engine sound synthesis system" but the content is behind the GDC Vault. <https://www.gdcvault.com/play/1034189/-Gran-Turismo-7-Sound>
