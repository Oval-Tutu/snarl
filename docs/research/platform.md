# Audio platform constraints for real-time synthesis in Go with Ebitengine

Research date: 2026-10-17. Sources are the Ebitengine fork and oto version that FiveHorizons pins, read from the local Go module cache, plus the web sources linked inline. Claims marked "unverified" have no source.

Conclusion first. Stock Ebitengine audio cannot meet the 30 ms control-to-sound budget on any target. The Ebitengine `audio` package never passes a device buffer size to oto, so every platform runs on the oto driver default (43 to 128 ms), and the Ebitengine player adds its own buffer on top (default 500 ms, in practice 1 to 2 times the value given to `Player.SetBufferSize`). Expect 80 to 150 ms on desktop and 100 to 250 ms on phones with the stock path. The budget needs changes in the `flexiondotorg/ebiten` fork, and on Android also in oto's C++ binding.

## 1. How Ebitengine and oto pull samples

Module paths used below:

- Ebitengine fork: `/home/martin/.cache/go/mod/github.com/flexiondotorg/ebiten/v2@v2.0.0-20261016133855-5d80ed875d23` (module name `github.com/hajimehoshi/ebiten/v2`).
- oto: `/home/martin/.cache/go/mod/github.com/ebitengine/oto/v3@v3.5.1`.

Pipeline, source to speaker:

1. `audio.Context.NewPlayerF32(src io.Reader)` wraps `src` in a `timeStream` (a mutex plus a byte counter) and hands it to `oto.Context.NewPlayer` (`audio/audio.go:447`, `audio/player.go:666`).
2. oto's `mux.loop` goroutine (one per process, started in `mux.New`, `internal/mux/mux.go:63-115`) calls `src.Read(buf)` whenever a player's byte buffer holds fewer than `bufferSize` bytes. `buf` is `bufferSize` bytes long, taken from a `sync.Pool`. The result is appended to the player buffer, so the player buffer can hold up to about 2 times `bufferSize` when `Read` fills the whole slice (`mux.go:556-614`). This goroutine is the "audio goroutine" that snarl synthesises on. It is a normal goroutine with no OS thread lock and no priority.
3. The device thread calls `mux.ReadFloat32s(buf)`, which zeroes `buf`, adds each playing player's bytes into it (decoding float32 little endian per sample, applying volume), then signals the loop goroutine to refill (`mux.go:155-165`, `475-541`). If a player buffer holds fewer samples than the device asks for, the rest stays silent. That is the glitch mode.
4. The device thread differs per platform (section 2).

Facts that matter for snarl:

- Format. `audio.NewContext` opens oto with `Format: oto.FormatFloat32LE` and 2 channels (`audio/context.go:36-40`). `NewPlayerF32` takes interleaved stereo float32 little endian bytes. `NewPlayer` takes int16 and converts to float32 first (`audio/audio.go:412-415`). Use `NewPlayerF32`.
- Sample rate. One per process, set in `audio.NewContext(sampleRate)`. The Ebitengine examples use 48000 (`examples/realtimepcm/main.go:31`).
- Device buffer. Ebitengine passes no `BufferSize` to `oto.NewContextOptions` (`audio/context.go:36-40`), so the oto driver default applies and the game cannot change it.
- Player buffer. `Player.SetBufferSize(d time.Duration)` sets the oto player `bufferSize` in bytes (`audio/player.go:527-555`). Default is 0.5 s (`mux.go:634-639`). Ebitengine's own real-time example uses `time.Second / 20` and warns that quality is not guaranteed (`examples/realtimepcm/main.go:125`).
- Read policy. Because the loop refills whenever the buffer is below `bufferSize`, a source that returns one block per `Read` keeps the buffer between `bufferSize` and `bufferSize + block`. A source that fills the whole slice keeps it between `bufferSize` and `2 * bufferSize`. The maintainer confirms "the latency is always closer to Player.BufferSize*2" in [ebiten issue 2727](https://github.com/hajimehoshi/ebiten/issues/2727), and rejects a callback-style player.
- Device readiness. The oto device is created on the first `Update` after the UI backend starts (`audio/audio.go:140-170`). A player created before that buffers source data, so the first sound plays stale samples ([ebiten issue 2727](https://github.com/hajimehoshi/ebiten/issues/2727)). Create the player after `Context.IsReady()`.
- Position bookkeeping. A second Ebitengine goroutine sweeps players every 10 ms for position and state (`audio/audio.go:172-190`). It does not touch the sample path.
- Reads can arrive from more than one goroutine over time, so `Read` must be safe against concurrent `Seek` ([oto issue 240](https://github.com/ebitengine/oto/issues/240)). snarl's source is not seekable, so a single-writer design is enough.

## 2. Output latency per platform

Driver defaults in oto v3.5.1, with the latency they add before the Ebitengine player buffer. All at 48 kHz stereo.

| Platform | Backend | Default device buffer | Added latency | Source |
| --- | --- | --- | --- | --- |
| Linux, PulseAudio or PipeWire-pulse | `jfreymuth/pulse`, tried first | `tlength` 100 ms, max 200 ms | about 100 ms | `driver_pulseaudio_unix.go:75-82`, `pulse@v0.1.3/playback.go:394-399` |
| Linux, ALSA fallback | purego ALSA | period 1024 frames, 2 periods | 21.3 ms per read, 42.7 ms buffer | `driver_alsa_unix.go:139-150` |
| Windows | WASAPI shared, event driven | 50 ms requested | about 50 ms plus the 10 ms engine period | `driver_wasapi_windows.go:360-368` |
| Windows fallback | WinMM | 4096 byte headers | small, rarely used | `driver_winmm_windows.go:32` |
| macOS and iOS | AudioQueue, 4 buffers of 12288 bytes | 4 x 1536 frames = 6144 frames | up to 128 ms (the code comment says 100 ms, the arithmetic gives 128) | `driver_darwin.go:31,137-152` |
| Android | Oboe, `LowLatency`, `Shared`, Float | stream buffer B = 2 bursts (Oboe default), oto FIFO holds 3B to 6B | B + 3B to 6B + HAL | `internal/oboe/binding_android.cpp:220-235, 258-275, 483-500` |
| Switch | `oto_OpenAudio` C callback from the closed `uwagaki` package | unknown | unknown | `driver_console.go:49-60`, `driver_nintendosdk.cpp` |
| Browser | ScriptProcessor 2048 frames | 42.7 ms | not a target | `driver_js.go:54-57` |

Platform notes:

- Linux. PipeWire's pulse server honours the requested `tlength` and clamps small requests to `pulse.min.req` 256/48000 (5.3 ms) and `pulse.min.quantum` 256/48000 ([PipeWire protocol-pulse docs](https://docs.pipewire.org/page_module_protocol_pulse.html)). The graph default quantum is 1024/48000 (21.3 ms) ([pipewire.conf](https://docs.pipewire.org/page_man_pipewire_conf_5.html)). So a fork that passes 20 to 25 ms as `BufferSize` would get about that on Linux.
- Windows. WASAPI shared mode uses the Windows audio engine period, 10 ms by default. oto initialises with `AUDCLNT_STREAMFLAGS_EVENTCALLBACK | AUTOCONVERTPCM | SRC_DEFAULT_QUALITY` and writes `bufferFrames - padding` each event (`driver_wasapi_windows.go:369-377, 464-494`). The render thread calls `runtime.LockOSThread`.
- macOS and iOS. The AudioQueue thread is locked to an OS thread (`driver_darwin.go:169-170`). The queue depth of 128 ms dominates. iOS hardware I/O buffer default is about 20 ms at 44.1 kHz and apps can request 5 ms through `AVAudioSession.setPreferredIOBufferDuration` ([Apple Audio Session guide](https://developer.apple.com/library/archive/documentation/Audio/Conceptual/AudioSessionProgrammingGuide/OptimizingForDeviceHardware/OptimizingForDeviceHardware.html)). oto v3.5.1 does not set a preferred duration or category (no match for `setPreferredIOBufferDuration` or `setCategory` in the module).
- Android. Oboe picks AAudio on Android 8.1 (API 27) and later and OpenSL ES on 8.0 and earlier ([Android Oboe docs](https://developer.android.com/games/sdk/oboe), [Oboe history](https://github.com/google/oboe/blob/main/docs/AndroidAudioHistory.md)). The OnePlus 3T on Android 9 gets AAudio, on 8.0 it gets OpenSL ES. Oboe sets the stream buffer to 2 bursts in low latency mode ([Android low latency guide](https://developer.android.com/games/sdk/oboe/low-latency-audio)). oto then reads `3 * getBufferSizeInFrames()` frames per Go call into a lock-free FIFO of capacity 6B, and refills only when 3B frames are free (`binding_android.cpp:258-275, 483-500`). The FIFO therefore holds 3B to 6B of audio. With a 192 frame burst (4 ms), B = 8 ms and the FIFO adds 24 to 48 ms. With a 960 frame burst on a device without the fast mixer, B = 40 ms and the FIFO adds 120 to 240 ms. Shared mode costs about 6 ms over exclusive, and a 44.1 kHz stream on AAudio without Oboe's resampler costs 160 ms against 20 ms at 48 kHz ([Android low latency guide](https://developer.android.com/games/sdk/oboe/low-latency-audio)). oto does not call `setSampleRateConversionQuality`, so use 48 kHz on Android.
- OnePlus 3T. One owner measured 80 ms round trip on OxygenOS 4.1.6 (Android 7.1.1) with the Superpowered test app ([OnePlus forum thread](https://community.oneplus.com/thread/565140)). Output is about half of round trip ([Android NDK audio latency](https://developer.android.com/ndk/guides/audio/audio-latency)), so about 40 ms of output path before any app buffering. The burst size and the `android.hardware.audio.low_latency` flag for this device are unverified. Pixel class devices of the same era measured 18 ms round trip ([AOSP latency measurements](https://source.android.com/docs/core/audio/latency/measurements)).
- Switch. Ebitengine builds natively with the `nintendosdk` tag and the closed `uwagaki` package ([Ebitengine 2.3 notes](https://ebitengine.org/en/documents/2.3.html), [native Switch blog](https://ebitengine.org/en/blog/native_compiling_for_nintendo_switch.html)). oto's console driver hands the SDK a C callback that calls `mux.ReadFloat32s` directly on the SDK audio thread (`driver_console.go:38-41`). There is no Go-side FIFO. Buffer size, sample rate and thread model are unverified without SDK access.

## 3. Measuring control-to-sound latency on device

Ebitengine ships no latency tool. Its only real-time example is `examples/realtimepcm`. Methods that work:

- Acoustic tap-to-tone, all platforms including Switch. Record the controller button click and the speaker output with one microphone, then read the gap in an editor. Resolution about 1 ms. OboeTester's "External Tap-to-Tone" automates this with a second Android phone as the recorder, and its on-device "Tap to Tone" measures touch plus output latency and notes touch latency of 15 to 30 ms ([OboeTester usage](https://github.com/google/oboe/blob/main/apps/OboeTester/docs/Usage.md)).
- In-app instrumentation. Stamp `time.Now()` when the game writes a new RPM atomic, stamp again when `Read` first sees that value, and log the difference. That gives the Go-side part. Add the known device figures from section 2. oto exposes `Player.BufferedSize()` but Ebitengine does not, so the player buffer occupancy is not observable without the fork.
- Android device baseline. OboeTester "Round Trip Latency" with a 3.5 mm loopback dongle or in a quiet room ([AOSP measure guide](https://source.android.com/docs/core/audio/latency/measure)). The Superpowered app gives the same number ([Superpowered latency](https://superpowered.com/latency)). Oboe's `getTimestamp` method estimates output latency from presentation timestamps ([oboe issue 69](https://github.com/google/oboe/issues/69)), but oto does not expose it.
- Precision. Google WALT measures output latency against an external clock to under 1 ms, but needs a Teensy board and a 3.5 mm jack ([WALT usage](https://android.googlesource.com/platform/external/walt/+/HEAD/docs/usage/WALT_usage.md)).
- iOS. `AVAudioSession.outputLatency` plus `ioBufferDuration` gives the system estimate ([Apple forum thread](https://developer.apple.com/forums/thread/25197)), not reachable from Ebitengine without the fork.

A camera at 240 fps gives 4 ms resolution and is worse than the microphone method.

## 4. Go runtime concerns

- GC pauses. Go's stop-the-world pauses are sub-millisecond and typically under 100 µs since Go 1.8 ([golang-dev announcement](https://groups.google.com/g/golang-dev/c/Ab1sFeoZg_8), [GC guide](https://go.dev/doc/gc-guide)). Pauses scale with `GOMAXPROCS`, not heap size. Other latency sources listed by the GC guide: 25 percent CPU taken by mark workers, mark assists for fast allocators, write barriers, and suspension of a goroutine for stack scanning. Go 1.26 made the Green Tea collector the default ([Green Tea blog](https://go.dev/blog/greenteagc)).
- Scheduler. Goroutines are asynchronously preemptible since Go 1.14 ([Go 1.14 notes](https://go.dev/doc/go1.14)). The forced time slice is 10 ms (`forcePreemptNS`), so a mux loop goroutine that is runnable while every P runs game code can wait up to about 10 ms ([sysmon description](https://golang.design/under-the-hood/en/part3concurrency/ch09sched/sysmon/)). This is the largest Go-side jitter and sets the floor for the player buffer, not GC.
- Thread priority. Pure Go has no API to raise a thread's priority. `runtime.LockOSThread` pins a goroutine to a thread and nothing more. oto locks the WASAPI and AudioQueue threads, and the Oboe callback runs on Android's real-time thread, but the Go `Read` call always runs on an ordinary goroutine.
- Allocation-free hot path. Needed to keep the game's GC frequency the only GC driver. Techniques: fixed arrays in the synth struct, `math.Float32bits` plus `binary.LittleEndian.PutUint32` or an `unsafe.Slice` view to write bytes, no interface conversions, no closures in `Read`, verify with `testing.AllocsPerRun`. FiveHorizons already has such a test in `internal/frametime/clock_test.go` (`TestNowIsMonotonicAndAllocationFree`).
- GOGC and memory limit. Raise `GOGC` (for example 200 to 400) to cut GC frequency when the live heap is small and steady, and set `GOMEMLIMIT` or `debug.SetMemoryLimit` as a ceiling on phones ([GC guide](https://go.dev/doc/gc-guide)). The limit is soft and caps GC CPU at about 50 percent.
- Known glitch reports. Real-time PCM users hit two problems: latency of about 2 times the player buffer, and silence when the player buffer is empty because the device thread ran before the refill goroutine ([ebiten issue 2727](https://github.com/hajimehoshi/ebiten/issues/2727)). oto fixed a data race in the shared read buffer in 2024 ([ebiten issue 3173](https://github.com/hajimehoshi/ebiten/issues/3173)) and a nil pool crash in `realtimepcm` in 2025 ([ebiten issue 3214](https://github.com/hajimehoshi/ebiten/issues/3214)). Both are fixed in v3.5.1.

## 5. CPU facts for DSP in Go

- Cortex-A57 (Switch) scalar FP. `FADD`, `FSUB`, `FMUL`: 5 cycle latency, 2 per cycle, same for single and double. `FMADD`: 9 cycles. `FDIV` single 7 to 17 cycles, double 7 to 32. ASIMD `FDIV` 4 x float32: 36 cycles, 2 x float64: 64 cycles ([Cortex-A57 Software Optimization Guide](https://documentation-service.arm.com/static/5ed75ee1ca06a95ce53f93c0), [LLVM A57 model](https://lists.llvm.org/pipermail/llvm-commits/Week-of-Mon-20140929/237214.html)). So float32 and float64 cost the same for add and multiply. float32 wins on divide, square root, cache footprint and SIMD lane count. Avoid per-sample division.
- ARMv7 VFP (32-bit Android builds only). `VMUL` single: throughput 1, latency 4. Double: throughput 2, latency 6. `VDIV` single 10/15, double 20/25 ([Arm FPU timing table](https://support.arm.com/documentation/ddi0458/d/Floating-Point-Unit-Programmers-Model/Instruction-throughput-and-latency/Definitions-of-throughput-and-latency?lang=en)). The OnePlus 3T is arm64, and iOS has been arm64 only since iOS 11, so ARMv7 matters only if the Android build ships `armeabi-v7a`.
- Go standard `math`. All functions take float64. On arm64 `math.Sin` and `math.Exp` are pure Go. Compute coefficients per block in float64, run the per-sample loop in float32, and use table or polynomial oscillators instead of `math.Sin` per sample.
- SIMD in Go. Go 1.26 added the experimental `simd/archsimd` package for amd64 behind `GOEXPERIMENT=simd` ([Go 1.26 notes](https://go.dev/doc/go1.26)). Go 1.27 added arm64 NEON and wasm to `archsimd` and a portable `simd` package with size-agnostic types such as `Float32s` ([Go 1.27 notes](https://go.dev/doc/go1.27), [archsimd blog](https://go.dev/blog/archsimd), [portable simd blog](https://go.dev/blog/simd-experiment)). The API is unstable and needs the experiment flag at build time, including under gomobile. Without it, the Go compiler does not auto-vectorise. Plan scalar float32 first and treat SIMD as an optional later gain.
- Relative CPU speed. The Switch CPU is four Cortex-A57 at 1020 MHz in both modes, with three cores available to games ([Digital Foundry 2016](https://www.digitalfoundry.net/articles/digitalfoundry-2016-nintendo-switch-spec-analysis), [Digital Foundry 2025](https://www.digitalfoundry.net/articles/digitalfoundry-2025-nintendo-switch-2-final-tech-specs-and-system-reservations-confirmed)). The Snapdragon 821 has two Kryo cores at 2.34 GHz and two at 1.6 GHz ([nanoreview](https://nanoreview.net/en/soc/qualcomm-snapdragon-821)). Geekbench 6 single core: OnePlus 3T 359, Shield TV (A57 at 2.0 GHz) 306 ([Geekbench Android chart](https://browser.geekbench.com/android_devices/)). Scaling the A57 to 1.02 GHz gives about 155, so one Switch core delivers roughly 40 to 45 percent of a OnePlus 3T big core. Switch units running Android at 2.09 GHz score about 222 ([Geekbench search](https://browser.geekbench.com/search?q=Switch)), which scales to about 108 at 1.02 GHz, or 30 percent. Take 30 to 45 percent. Confidence: medium.
- Budget in cycles. 5 percent of a 1.02 GHz core is 51 M cycles per second, or about 1060 cycles per stereo frame at 48 kHz. That is ample for a few oscillators, a noise source and a handful of biquads in scalar float32 if the loop allocates nothing and avoids `math` calls per sample. The OnePlus 3T big core has about 2.3 times the headroom, its little cores about the same as the Switch.

## 6. FiveHorizons versions and audio code

From `/home/martin/Development/Oval-Tutu/FiveHorizons/go.mod` only:

- `go 1.26.9`, `toolchain go1.27.2`. The snarl flake provides `go_1_27`.
- `github.com/hajimehoshi/ebiten/v2 v2.10.4`, replaced by `github.com/flexiondotorg/ebiten/v2 v2.0.0-20261016133855-5d80ed875d23`. The fork's `go.mod` requires `github.com/ebitengine/oto/v3 v3.5.1` and `go 1.26.0`. Upstream v2.10.4 pins oto v3.5.0. The fork's `audio` package differs from v2.10.4 in `audio.go`, `context.go`, `player.go` and `internal/convert`, and adds a `vmaudio` backend that routes sound to a virtual device inside a VM guest (`audio/context.go:25-34`). Latency measured inside a VM is therefore not representative.
- Other relevant inputs: `github.com/quasilyte/ebitengine-input v0.9.1`, `github.com/ebitengine/purego v0.11.1`, `github.com/ebitengine/gomobile` (indirect).
- Audio code: none. A grep for `ebiten/v2/audio`, `oto` and `audio.` across FiveHorizons Go files finds no imports or calls. Related code: `internal/input/rumble.go` drives gamepad motors per tick, and `internal/frametime` has a monotonic clock with an allocation-free test.

## 7. Recommendations

Common to all tiers:

- Sample rate 48000. It is the native rate on Android and avoids the 160 ms AAudio resampling path. WASAPI and CoreAudio convert when the device differs.
- Output float32 stereo through `NewPlayerF32`. Synthesise mono in float32, duplicate to stereo when writing bytes.
- Internal state: float32 samples and coefficients, float64 or 32-bit fixed point phase accumulators, coefficients recomputed once per block.
- Block size 256 frames (5.3 ms). The game writes atomics at 60 Hz (800 frames), so one read of RPM, throttle and load per block with a per-sample linear ramp is enough. `Read` returns exactly one block, never more, so the player buffer stays near `bufferSize`.
- Create the player after `Context.IsReady()`, call `SetBufferSize` before `Play`.
- Player buffer rule: at least the largest device read plus 10 ms of Go scheduling jitter, rounded up to whole blocks.

Per tier, with stock Ebitengine:

| Tier | Device read per callback | `SetBufferSize` | Expected total output latency |
| --- | --- | --- | --- |
| Linux and Windows PC | ALSA 21.3 ms, Pulse unknown, WASAPI about 10 ms | 32 ms (6 blocks) | 75 to 135 ms (Pulse) or about 75 ms (ALSA) |
| macOS | 32 ms | 43 ms (8 blocks) | about 170 ms |
| iOS, old iPhone and iPad | 32 ms | 43 ms | about 170 ms, plus the hardware I/O buffer |
| Android, good device (192 frame burst) | 24 ms | 37 ms (7 blocks) | 110 to 150 ms |
| Android, OnePlus 3T | unknown, possibly 120 ms if bursts are 960 frames | measure first, start at 64 ms | 150 to 300 ms |
| Switch | unknown | start at 43 ms, tune on hardware | unknown |

These figures do not meet 30 ms. What would:

1. Fork change in `audio/context.go`: pass `BufferSize` to `oto.NewContextOptions` and expose it, for example `audio.NewContextWithOptions`. Target 10 to 20 ms on desktop. This alone brings Linux and Windows to about 40 to 60 ms total.
2. Fork change in oto's Oboe binding: drop the 3x read multiplier and the 6B FIFO, or make them configurable. Target B plus one read.
3. On macOS and iOS, reduce `bufferCount` or `defaultOneBufferSizeInBytes`, and on iOS set a preferred I/O buffer duration.
4. Expose oto's `Player.BufferedSize()` through the fork so the player buffer can be tuned adaptively from underrun counts.
5. Design-side mitigation independent of the engine: predict RPM a fixed horizon ahead from the known throttle and gear state, so the perceived lag is the physics lag rather than the audio lag. A racer's RPM is smooth, so prediction over 50 to 100 ms is plausible.

Lean: do item 1 first, measure on Linux with the microphone method, then decide whether item 2 is worth a C++ fork for the OnePlus 3T, whose own output path already consumes most of the 30 ms.

## Implications for snarl

- snarl's public surface is an `io.Reader` that returns one block of interleaved stereo float32 little endian bytes per call, never blocks, never allocates, and reads three atomics once per block. It must also work when `Read` is called with a slice larger than one block (return a short read) and smaller than one block (return a partial block and keep the remainder).
- Timing arrives from the device thread through oto, not from the game loop, so the block cadence is irregular and bursty. The synth state must be self-clocked by sample count, not by wall time.
- Expect silence insertion, not slowdown, when snarl misses a refill. Detect this in development by counting bytes returned per second against 48000 x 8.
- The 30 ms budget is an engine fork task before it is a DSP task. Document the stock latency per platform from measurement, then negotiate the fork change with the FiveHorizons owner.
- CPU budget is comfortable at scalar float32 on all tiers. Keep `math.Sin`, `math.Exp` and division out of the per-sample loop, and leave SIMD as an experiment behind a build tag.
- Keep GC pressure in the game, not in snarl. Add an `AllocsPerRun` test on `Read` and a benchmark per block on amd64 and arm64.

## Open questions

- OnePlus 3T burst size, `android.hardware.audio.low_latency` flag, and whether the stock ROM is Android 8.0 (OpenSL ES) or 9 (AAudio). Needs a run of OboeTester on the device.
- PulseAudio read size per callback in `driver_pulseaudio_unix.go` (`pulseContext.read` is pull-based but the request size was not traced).
- Switch audio buffer size, sample rate and callback thread, all inside the closed `uwagaki` package. Needs the FiveHorizons Switch build owner.
- Whether the `flexiondotorg/ebiten` fork owner accepts a `BufferSize` option, and whether an oto fork is acceptable for Android.
- iOS category and session setup in the fork, if any, since oto v3.5.1 sets none.
- Whether gomobile builds for Android still include `armeabi-v7a`, which decides if ARMv7 FP timing matters.
- Real measured numbers. Every latency figure above is derived from buffer sizes, not measured.
