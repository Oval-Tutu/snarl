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

// benchStates are the states of BenchmarkStates: full throttle at 4000
// RPM, idle, overrun with pops, and held at the rev limiter.
var benchStates = []benchState{
	{"wot4000", func(Profile) Snapshot {
		return Snapshot{RPM: 4000, Throttle: 1, Load: 1, Gear: 3, Flags: FlagRunning}
	}},
	{"idle", func(p Profile) Snapshot {
		return Snapshot{RPM: p.IdleRPM, Flags: FlagRunning}
	}},
	{"overrun", func(Profile) Snapshot {
		return Snapshot{RPM: 5000, Gear: 3, Flags: FlagRunning | FlagOverrun}
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
	// fan is true when the variant only applies to a profile with a fan.
	fan bool
}

// layerVariants are the layers that a profile key can mute. The
// intake, fan and tick layers skip their work at zero gain. The jet, AM
// and steepening keys set a gain of zero, but the model still runs their
// filters and arithmetic, so their variants measure no saving. The
// wall-loss shelf, the expansion chamber and the mode cutoff are keys of
// [[path]], which an override cannot set, and the live pulse shaping has
// no key. BenchmarkPrimitives gives their cost per instance.
var layerVariants = []layerVariant{
	{name: "full"},
	{name: "no-intake", overrides: []string{"noise.intake=0"}},
	{name: "no-fan", overrides: []string{"mechanical.fan_gain=0"}, fan: true},
	{name: "no-tick", overrides: []string{"mechanical.tick_gain=0"}},
	{name: "no-jet", overrides: []string{"noise.jet=0"}},
	{name: "no-steep", overrides: []string{"exhaust.steepening=0"}},
	{name: "no-am", overrides: []string{"noise.am=0"}},
	{name: "no-gated", overrides: []string{"noise.intake=0", "mechanical.fan_gain=0", "mechanical.tick_gain=0"}},
}

// BenchmarkLayers measures full throttle at 4000 RPM with each layer
// variant. The cost of a layer is the full time minus the time of its
// variant.
func BenchmarkLayers(b *testing.B) {
	st := benchStates[0]
	for _, name := range Profiles() {
		base, err := LoadProfile(name)
		if err != nil {
			b.Fatal(err)
		}
		for _, v := range layerVariants {
			if v.fan && base.spec.Mechanical.FanGain == 0 {
				continue
			}
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
// override can mute, for one instance over one 256-frame block. A path
// runs one wall-loss shelf, one expansion chamber (one delay write
// and two reads) and a mode cutoff of two biquads per sample. The jet runs
// one band-pass biquad and one noise draw per sample for the engine. The
// fan-sine case is the fan layer, for comparison with its mute variant.
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
	b.Run("shelf", func(b *testing.B) {
		f := dsp.NewHighShelf(2000, 0.7, testRate)
		var acc float32
		for b.Loop() {
			for _, x := range in {
				acc += f.Process(x)
			}
		}
		sink = acc
	})
	b.Run("chamber", func(b *testing.B) {
		const d = 12
		ch := dsp.NewDelay(2 * d)
		var acc float32
		for b.Loop() {
			for _, x := range in {
				w := x + 0.6*ch.Read(2*d-1)
				ch.Write(w)
				acc += 0.4 * ch.Read(d)
			}
		}
		sink = acc
	})
	b.Run("fan-sine", func(b *testing.B) {
		// The fan layer: a phase accumulator into the sine table.
		const inc = 0.0275
		var phase, acc float32
		for b.Loop() {
			for range blockFrames {
				phase += inc
				if phase >= 1 {
					phase--
				}
				acc += dsp.Sine(phase)
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
