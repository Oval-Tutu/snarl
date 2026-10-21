package snarl

import (
	"testing"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// blockFrames is the length of one render block, as the game renders it.
const blockFrames = 256

// benchState is one engine state for the cost benchmarks.
type benchState struct {
	name string
	snap func(p Profile) Snapshot
}

// wotRPM is the RPM of the full-throttle benchmarks: 4000 RPM, or 90 per
// cent of the redline when that is lower.
func wotRPM(p Profile) float32 {
	return min(4000, 0.9*p.RedlineRPM)
}

// benchStates are the states of BenchmarkStates: full throttle at wotRPM,
// idle, overrun with pops at 5000 RPM or the redline when that is lower,
// and held at the rev limiter.
var benchStates = []benchState{
	{"wot", func(p Profile) Snapshot {
		return Snapshot{RPM: wotRPM(p), Throttle: 1, Load: 1, Gear: 3, Flags: FlagRunning}
	}},
	{"idle", func(p Profile) Snapshot {
		return Snapshot{RPM: p.IdleRPM, Flags: FlagRunning}
	}},
	{"overrun", func(p Profile) Snapshot {
		return Snapshot{RPM: min(5000, p.RedlineRPM), Gear: 3, Flags: FlagRunning | FlagOverrun}
	}},
	{"limiter", func(p Profile) Snapshot {
		return Snapshot{RPM: p.LimiterRPM, Throttle: 1, Load: 1, Gear: 3, Flags: FlagRunning | FlagRedZone}
	}},
}

// benchBlocks renders blocks of e in state s and reports the cost. It
// renders one second first, so that the smoothed controls settle. The
// time of one op is the time of one 256-frame block.
func benchBlocks(b *testing.B, e Engine, s Snapshot) {
	b.Helper()
	buf := make([]float32, blockFrames)
	for range testRate / blockFrames {
		e.Set(s)
		e.Render(buf)
	}
	b.ReportAllocs()
	for b.Loop() {
		e.Set(s)
		e.Render(buf)
	}
	blockNs := blockFrames / float64(testRate) * 1e9
	nsPerBlock := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
	b.ReportMetric(100*nsPerBlock/blockNs, "%realtime")
}

// BenchmarkStates measures each profile in each state.
func BenchmarkStates(b *testing.B) {
	for _, name := range Profiles() {
		p, err := LoadProfile(name)
		if err != nil {
			b.Fatal(err)
		}
		for _, st := range benchStates {
			b.Run(name+"/"+st.name, func(b *testing.B) {
				benchBlocks(b, newEngine(b, p), st.snap(p))
			})
		}
	}
}

// layerVariant mutes one or more layers with profile overrides.
type layerVariant struct {
	name      string
	overrides []string
}

// layerVariants are the layers that a profile key can mute. An input
// jitter of zero skips the jitter stage. The AM key sets a gain of zero,
// but the model still runs its filters and arithmetic, so its variant
// measures no saving. The formant bank is an array, which an override
// cannot set, and the live pulse shaping has no key. BenchmarkPrimitives
// gives their cost per instance.
var layerVariants = []layerVariant{
	{name: "full"},
	{name: "no-jitter", overrides: []string{"noise.input_jitter=0"}},
	{name: "no-am", overrides: []string{"noise.am=0"}},
}

// BenchmarkLayers measures full throttle at 4000 RPM with each layer
// variant. The cost of a layer is the full time minus the time of its
// variant.
func BenchmarkLayers(b *testing.B) {
	st := benchStates[0]
	for _, name := range Profiles() {
		for _, v := range layerVariants {
			p, err := LoadProfileWith(name, v.overrides...)
			if err != nil {
				b.Fatal(err)
			}
			b.Run(name+"/"+v.name, func(b *testing.B) {
				benchBlocks(b, newEngine(b, p), st.snap(p))
			})
		}
	}
}

// sink keeps the primitive results live.
var sink float32

// BenchmarkPrimitives measures the DSP parts of the layers that no
// override can mute, for one instance over one 256-frame block. The
// formant bank runs one biquad per section per sample, and the noise AM
// one noise draw per sample.
func BenchmarkPrimitives(b *testing.B) {
	in := make([]float32, blockFrames)
	nz := dsp.NewNoise(7)
	for i := range in {
		in[i] = nz.Bipolar()
	}
	b.Run("biquad", func(b *testing.B) {
		var f dsp.Biquad
		f.SetLowPass(4000, 0.7071, testRate)
		var acc float32
		for b.Loop() {
			for _, x := range in {
				acc += f.Process(x)
			}
		}
		sink = acc
	})
	b.Run("noise", func(b *testing.B) {
		n := dsp.NewNoise(9)
		var acc float32
		for b.Loop() {
			for range blockFrames {
				acc += n.Bipolar()
			}
		}
		sink = acc
	})
}

// BenchmarkNew measures New for each profile after the first call, which
// takes the pop calibration from the cache.
func BenchmarkNew(b *testing.B) {
	for _, name := range Profiles() {
		p, err := LoadProfile(name)
		if err != nil {
			b.Fatal(err)
		}
		newEngine(b, p)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				newEngine(b, p)
			}
		})
	}
}
