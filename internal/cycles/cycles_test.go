package cycles

import (
	"bytes"
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"

	"github.com/oval-tutu/snarl/internal/esynth"
)

func TestHalfRoundTrip(t *testing.T) {
	for _, c := range []struct {
		v    float64
		bits uint16
	}{
		{0, 0},
		{1, 0x3c00},
		{-2, 0xc000},
		{0.5, 0x3800},
		{65504, 0x7bff},
		{1e6, 0x7c00},
		{math.Exp2(-24), 0x0001},
		{math.Exp2(-14), 0x0400},
		// 1 + 2^-11 is halfway between 1 and the next half; ties go to even.
		{1 + math.Exp2(-11), 0x3c00},
		{1 + 3*math.Exp2(-11), 0x3c02},
	} {
		if got := toHalf(c.v); got != c.bits {
			t.Errorf("toHalf(%g) = %#04x, want %#04x", c.v, got, c.bits)
		}
	}
	for b := range uint32(0x7c00) {
		h := uint16(b)
		if got := toHalf(fromHalf(h)); got != h {
			t.Fatalf("toHalf(fromHalf(%#04x)) = %#04x", h, got)
		}
	}
}

func TestUnwrapCrank(t *testing.T) {
	for _, dir := range []float64{-1, 1} {
		const n = 1000
		crank := make([]float64, 2*n)
		want := make([]float64, n)
		for i := range n {
			p := 0.3 + 0.0123*float64(i)
			want[i] = p
			f := p - math.Floor(p)
			a := f * 4 * math.Pi
			if dir > 0 {
				// Counter-clockwise: the cycle angle counts down.
				a = (1 - f) * 4 * math.Pi
			}
			crank[2*i], crank[2*i+1] = a, dir*3000*2*math.Pi/60
		}
		phase, rpm := unwrapCrank(crank)
		for i := range n {
			if math.Abs(phase[i]-want[i]-math.Round(phase[0]-want[0])) > 1e-9 {
				t.Fatalf("direction %g: phase %d is %g, want %g", dir, i, phase[i], want[i])
			}
			if math.Abs(rpm[i]-3000) > 1e-9 {
				t.Fatalf("rpm %g", rpm[i])
			}
		}
	}
}

// synthRecord returns a record of one hold at rpm with a speed that varies
// over the cycle, whose input is wave at the emission phase (delay steps
// earlier) plus noise from noise(phase).
func synthRecord(rate, rpm float64, seconds float64, delay int, wave func(psi float64) float64, noise func(psi float64) float64) *Record {
	n := int(seconds * rate)
	r := &Record{
		Dump:      &esynth.Dump{InputChannelCount: 1, InputSampleRate: rate, SimulationFrequency: rate},
		Timestep:  1 / rate,
		Phase:     make([]float64, n),
		RPM:       make([]float64, n),
		Cylinders: []RecordCylinder{{ExhaustSystem: 0, DelaySteps: delay, Weight: 1}},
		Segments:  []RecordSegment{{Mode: "hold", RPM0: rpm, RPM1: rpm, Throttle: 1, Seconds: seconds, Start: 0, End: seconds}},
	}
	in := make([]float64, n)
	p := 0.0
	for i := range n {
		// Integrate the speed profile 1 + 0.05 cos(2 pi * 4 * phase). The
		// record holds the phase and the speed after each step.
		p += rpm * (1 + 0.05*math.Cos(2*math.Pi*4*p)) / 120 / rate
		r.Phase[i] = p
		r.RPM[i] = rpm * (1 + 0.05*math.Cos(2*math.Pi*4*p))
	}
	for i := range n {
		psi := r.Phase[max(i-delay, 0)]
		in[i] = wave(psi) + noise(psi)
	}
	r.Inputs = [][]float64{in}
	return r
}

func testWave(psi float64) float64 {
	return 0.3 + math.Sin(2*math.Pi*4*psi) + 0.5*math.Cos(2*math.Pi*13*psi+0.4) + 0.1*math.Sin(2*math.Pi*40*psi)
}

func TestMeasureRecoversWave(t *testing.T) {
	r := synthRecord(10000, 3000, 2, 50, testWave, func(float64) float64 { return 0 })
	p, err := r.Measure(r.Segments[0], MeasureOptions{Settle: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if p.Harmonics != 200 || p.Cycles < 37 {
		t.Fatalf("harmonics %d, cycles %d", p.Harmonics, p.Cycles)
	}
	if math.Abs(p.MeasuredRPM-3000)/3000 > 0.002 {
		t.Errorf("measured rpm %g", p.MeasuredRPM)
	}
	table := NewWaveTable(p.Wave[0])
	var errSum, sigSum float64
	for i := range 1000 {
		psi := float64(i) / 1000
		e := table.At(psi) - testWave(psi)
		errSum += e * e
		sigSum += testWave(psi) * testWave(psi)
	}
	if rel := 10 * math.Log10(errSum/sigSum); rel > -50 {
		t.Errorf("wave error %.1f dB below the signal, want 50 dB or more", -rel)
	}
	// The speed profile: 1 + 0.05 cos(8 pi phase), relative to its
	// harmonic mean.
	for i, s := range p.Speed {
		want := 1 + 0.05*math.Cos(2*math.Pi*4*float64(i)/SpeedPoints)
		if math.Abs(s/want-1) > 0.003 {
			t.Fatalf("speed %d is %g, want about %g", i, s, want)
		}
	}
}

// TestMeasureAliases checks that a harmonic above the Nyquist frequency of
// the steps, which the steps fold to a tone that is not a harmonic, comes
// back as that harmonic, with its phase.
func TestMeasureAliases(t *testing.T) {
	// 387.1 steps per cycle: harmonic 250 is above the Nyquist frequency,
	// harmonic 193.
	wave := func(psi float64) float64 { return testWave(psi) + 0.1*math.Cos(2*math.Pi*250*psi+0.3) }
	r := synthRecord(10000, 3100, 2, 50, wave, func(float64) float64 { return 0 })
	p, err := r.Measure(r.Segments[0], MeasureOptions{Settle: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if p.Harmonics < 250 {
		t.Fatalf("%d harmonics, want 250 or more", p.Harmonics)
	}
	c := complex(p.Wave[0][499], p.Wave[0][500])
	if a, ph := cmplx.Abs(c), cmplx.Phase(c); math.Abs(a-0.05) > 0.0025 || math.Abs(ph-0.3) > 0.05 {
		t.Errorf("harmonic 250: magnitude %.4f phase %.3f, want 0.05 and 0.3", a, ph)
	}
	off, err := r.Measure(r.Segments[0], MeasureOptions{Settle: 0.5, NoAliases: true})
	if err != nil {
		t.Fatal(err)
	}
	if off.Harmonics != 193 {
		t.Errorf("%d harmonics without the aliases, want 193", off.Harmonics)
	}
}

// TestMeasureModulation checks that a random gain of the cycle in eight
// parts of each cycle comes back as the modulation, with most of its power,
// and leaves little to the band noise. The gain of each part is a mean over
// the part, so that the measure smooths the gains and finds a correlation
// of neighbouring parts.
func TestMeasureModulation(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6)) //nolint:gosec // Test noise, not security.
	gains := map[int]float64{}
	gain := func(k int) float64 {
		if _, ok := gains[k]; !ok {
			gains[k] = rng.NormFloat64()
		}
		return gains[k]
	}
	const depth = 0.05
	noise := func(psi float64) float64 {
		u := psi*ModulationSlots - 0.5
		k := int(math.Floor(u))
		f := u - float64(k)
		return depth * (gain(k) + f*(gain(k+1)-gain(k))) * (testWave(psi) - 0.3)
	}
	r := synthRecord(10000, 1500, 6, 0, testWave, noise)
	p, err := r.Measure(r.Segments[0], MeasureOptions{Settle: 0.5, ModulationSlots: ModulationSlots})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Modulation) != 1 || len(p.ModulationRho) != 1 {
		t.Fatalf("modulation %v, correlation %v", p.Modulation, p.ModulationRho)
	}
	// The power of the modulation: the gain times the cycle without its
	// mean, of power 1.37, with 2/3 of the interpolation of independent
	// values, or 2/3 + rho/3 of correlated ones.
	want := depth * depth * 2.0 / 3 * 1.37
	rho := p.ModulationRho[0]
	got := p.Modulation[0] * p.Modulation[0] * (2.0/3 + rho/3) * 1.37
	// The means over the parts smooth the gains, which takes about 2 dB of
	// the power into the band noise.
	if d := 10 * math.Log10(got/want); d < -2.5 || d > 0.5 {
		t.Errorf("modulation power %.2f dB from the truth, want -2.5 to 0.5 dB (RMS gain %.4f, correlation %.2f)", d, p.Modulation[0], rho)
	}
	var left float64
	for _, v := range p.Noise[0] {
		left += v * v
	}
	left /= Sectors
	if rel := 10 * math.Log10(left/want); rel > -6 {
		t.Errorf("band noise %.1f dB below the modulation, want 6 dB or more", -rel)
	}
}

// TestMeasureDrift checks that the cycle of a slice whose level grows by 7
// % per cycle is, with Drift, the cycle at the middle of the slice, without
// the jump at the end of the cycle that the average has.
func TestMeasureDrift(t *testing.T) {
	slice := RecordSegment{Mode: "hold", RPM0: 3000, RPM1: 3000, Throttle: 1, Start: 0.3, End: 0.42}
	r := synthRecord(10000, 3000, 0.5, 0, func(psi float64) float64 { return (1 + 0.2*psi) * (testWave(psi) - 0.3) }, func(float64) float64 { return 0 })
	s, err := r.HoldSpan(slice, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The cycle at the middle of the slice.
	gain := 1 + 0.2*(r.Phase[s.I0]+float64(s.Cycles)/2)
	errorDB := func(drift bool) float64 {
		p, err := r.Measure(slice, MeasureOptions{Drift: drift})
		if err != nil {
			t.Fatal(err)
		}
		table := NewWaveTable(p.Wave[0])
		var e, sig float64
		for i := range 1000 {
			psi := float64(i) / 1000
			want := gain * (testWave(psi) - 0.3)
			e += (table.At(psi) - want) * (table.At(psi) - want)
			sig += want * want
		}
		return 10 * math.Log10(e/sig)
	}
	with, without := errorDB(true), errorDB(false)
	if with > -40 || without < with+15 {
		t.Errorf("cycle error %.1f dB below the cycle with Drift and %.1f dB without, want 40 dB or more and 15 dB less", -with, -without)
	}
}

func TestMeasureNoiseEnvelope(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4)) //nolint:gosec // Test noise, not security.
	// Flat white noise with an RMS of 0.1 in the first half of the cycle
	// and 0.02 in the second half.
	noise := func(psi float64) float64 {
		f := psi - math.Floor(psi)
		if f < 0.5 {
			return 0.1 * rng.NormFloat64()
		}
		return 0.02 * rng.NormFloat64()
	}
	r := synthRecord(10000, 1500, 6, 0, testWave, noise)
	p, err := r.Measure(r.Segments[0], MeasureOptions{Settle: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		sector int
		rms    float64
	}{{8, 0.1}, {24, 0.1}, {40, 0.02}, {56, 0.02}} {
		var pw float64
		for b := range BandCount {
			pw += p.Noise[0][b*Sectors+c.sector] * p.Noise[0][b*Sectors+c.sector]
		}
		// The low bands smooth the envelope over their time resolution,
		// which spreads a little of the loud half into the quiet half.
		if d := 10 * math.Log10(pw/(c.rms*c.rms)); math.Abs(d) > 1.5 {
			t.Errorf("sector %d: noise power %.2f dB from the truth", c.sector, d)
		}
	}
}

// TestPlayerNoiseRoundTrip plays a table with a fixed band envelope,
// measures the residual of the playback, and checks each band.
func TestPlayerNoiseRoundTrip(t *testing.T) {
	const rate = 10000
	wave := make([]float64, 2*20+1)
	wave[3], wave[4] = 0.4, -0.2
	env := make([]float64, BandCount*Sectors)
	levels := []float64{0.1, 0.04, 0.05, 0.02, 0.01, 0.03, 0.005, 0.002, 0.001}
	for b, l := range levels {
		for s := range Sectors {
			env[b*Sectors+s] = l
		}
	}
	tab := &Tables{Rate: rate, Channels: []Channel{{DelaySteps: 0}}, Points: []Point{{
		RPM: 3000, Throttle: 1, Ignition: true, MeasuredRPM: 3000, Harmonics: 20,
		Speed: ones(SpeedPoints), Wave: [][]float64{wave}, Noise: [][]float64{env},
	}}}
	pl := NewPlayer(tab, PlayOptions{Seed: 7, UniformSpeed: true})
	n := 8 * rate
	r := &Record{
		Dump:      &esynth.Dump{InputChannelCount: 1, InputSampleRate: rate, SimulationFrequency: rate, Inputs: [][]float64{make([]float64, n)}},
		Timestep:  1.0 / rate,
		Phase:     make([]float64, n),
		RPM:       make([]float64, n),
		Cylinders: []RecordCylinder{{Weight: 1}},
		Segments:  []RecordSegment{{Mode: "hold", RPM0: 3000, RPM1: 3000, Throttle: 1, Seconds: 8, End: 8}},
	}
	out := make([]float64, 1)
	for i := range n {
		pl.Next(3000, 1, true, out)
		r.Inputs[0][i] = out[0]
		r.Phase[i] = pl.phase
		r.RPM[i] = 3000
	}
	p, err := r.Measure(r.Segments[0], MeasureOptions{Settle: 0.5, MaxHarmonics: 20})
	if err != nil {
		t.Fatal(err)
	}
	// The unmixing amplifies the estimation error of a weak band next to
	// strong ones, so a band may be up to 2 dB off. The total keeps 0.3 dB.
	var total, want float64
	for b, l := range levels {
		var pw float64
		for s := range Sectors {
			pw += p.Noise[0][b*Sectors+s] * p.Noise[0][b*Sectors+s]
		}
		pw /= Sectors
		total += pw
		want += l * l
		if d := 10 * math.Log10(pw/(l*l)); math.Abs(d) > 2 {
			t.Errorf("band %d: measured %.2f dB from the played level", b, d)
		}
	}
	if d := 10 * math.Log10(total/want); math.Abs(d) > 0.3 {
		t.Errorf("total: measured %.2f dB from the played level", d)
	}
	if math.Abs(p.Wave[0][3]-0.4) > 0.01 || math.Abs(p.Wave[0][4]+0.2) > 0.01 {
		t.Errorf("harmonic 2 measured %g %g, want 0.4 -0.2", p.Wave[0][3], p.Wave[0][4])
	}
}

func ones(n int) []float64 {
	x := make([]float64, n)
	for i := range x {
		x[i] = 1
	}
	return x
}

func gridTables() *Tables {
	t := &Tables{Rate: 10000, Channels: []Channel{{}}}
	for _, th := range []float64{0, 0.5, 1} {
		for _, rpm := range []float64{1000, 2000, 3000} {
			t.Points = append(t.Points, Point{
				RPM: rpm, Throttle: th, Ignition: true, MeasuredRPM: rpm, Harmonics: 1,
				Speed: ones(SpeedPoints), Wave: [][]float64{{rpm * (1 + th), 0, 0}}, Noise: [][]float64{make([]float64, BandCount*Sectors)},
			})
		}
	}
	t.Points = append(t.Points, Point{
		RPM: 2000, Throttle: 1, Ignition: false, MeasuredRPM: 2000, Harmonics: 1,
		Speed: ones(SpeedPoints), Wave: [][]float64{{-1, 0, 0}}, Noise: [][]float64{make([]float64, BandCount*Sectors)},
	})
	return t
}

func TestWeights(t *testing.T) {
	tab := gridTables()
	pl := NewPlayer(tab, PlayOptions{})
	value := func(rpm, th float64, ign bool) float64 {
		var v, sum float64
		for _, w := range pl.Weights(rpm, th, ign) {
			v += w.W * tab.Points[w.Point].Wave[0][0]
			sum += w.W
		}
		if math.Abs(sum-1) > 1e-12 {
			t.Fatalf("weights sum to %g", sum)
		}
		return v
	}
	for _, c := range []struct {
		rpm, th float64
		ign     bool
		want    float64
	}{
		{2000, 0.5, true, 3000},
		{1500, 0.25, true, 1500 * 1.25},
		{500, 0, true, 1000},
		{4000, 1.2, true, 6000},
		{2500, 0.75, true, 2500 * 1.75},
		{3000, 0, false, -1},
	} {
		if got := value(c.rpm, c.th, c.ign); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("rpm %g throttle %g ignition %t: %g, want %g", c.rpm, c.th, c.ign, got, c.want)
		}
	}
	if got := len(pl.Weights(2000, 0.5, true)); got != 1 {
		t.Errorf("a grid point takes %d points, want 1", got)
	}
}

func TestThrottleLag(t *testing.T) {
	tab := gridTables()
	tab.ThrottleLag = ThrottleLag{Close: 0.1, Open: 0.02}
	pl := NewPlayer(tab, PlayOptions{NoResidual: true})
	out := make([]float64, 1)
	// The mean of the wave is rpm * (1 + throttle) at 2000 rpm.
	for range 100 {
		pl.Next(2000, 1, true, out)
	}
	if math.Abs(out[0]-4000) > 1e-9 {
		t.Fatalf("settled at %g, want 4000", out[0])
	}
	// 0.1 s after closing, the old state keeps 1/e of the weight.
	for range 1000 {
		pl.Next(2000, 0, true, out)
	}
	if want := 2000 + 2000*math.Exp(-1); math.Abs(out[0]-want) > 1 {
		t.Errorf("0.1 s after closing: %g, want %g", out[0], want)
	}
	for range 10000 {
		pl.Next(2000, 0, true, out)
	}
	// Opening is faster: 0.02 s leaves 1/e of the closed state.
	for range 200 {
		pl.Next(2000, 1, true, out)
	}
	if want := 4000 - 2000*math.Exp(-1); math.Abs(out[0]-want) > 1 {
		t.Errorf("0.02 s after opening: %g, want %g", out[0], want)
	}
}

func TestTablesRoundTrip(t *testing.T) {
	tab := gridTables()
	tab.Engine, tab.Audio = "test", esynth.DefaultAudioParameters()
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // Test noise, not security.
	for i := range tab.Points {
		p := &tab.Points[i]
		p.Harmonics = 5
		p.Wave[0] = make([]float64, 11)
		for k := range p.Wave[0] {
			p.Wave[0][k] = 1000 * rng.NormFloat64()
		}
		for k := range p.Noise[0] {
			p.Noise[0][k] = rng.Float64()
		}
		p.Speed[3] = 1.0123
	}
	var b bytes.Buffer
	if err := tab.Write(&b); err != nil {
		t.Fatal(err)
	}
	got, err := Read(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got.Engine != "test" || got.Audio != tab.Audio || len(got.Points) != len(tab.Points) {
		t.Fatalf("header differs: %+v", got)
	}
	for i := range tab.Points {
		a, g := &tab.Points[i], &got.Points[i]
		if a.RPM != g.RPM || a.Throttle != g.Throttle || a.Ignition != g.Ignition || a.Harmonics != g.Harmonics {
			t.Fatalf("point %d metadata differs", i)
		}
		check := func(name string, x, y []float64) {
			var peak float64
			for _, v := range x {
				peak = max(peak, math.Abs(v))
			}
			for k := range x {
				if math.Abs(x[k]-y[k]) > peak*math.Exp2(-11) {
					t.Fatalf("point %d %s %d: %g, want %g", i, name, k, y[k], x[k])
				}
			}
		}
		check("speed", a.Speed, g.Speed)
		check("wave", a.Wave[0], g.Wave[0])
		check("noise", a.Noise[0], g.Noise[0])
	}
	if _, err := Read(b.Bytes()[:b.Len()-1]); err == nil {
		t.Error("a truncated file reads without an error")
	}
}

func TestRenderLength(t *testing.T) {
	tab := gridTables()
	tab.Audio = esynth.DefaultAudioParameters()
	tab.Channels[0].Volume = 0.01
	ir := make([]int16, 64)
	ir[0] = 20000
	ctl := func(float64) (float64, float64, bool) { return 2000, 0.5, true }
	x, err := Render(tab, ctl, RenderOptions{Settle: 0.2, Seconds: 0.5, Volume: 0.5, ImpulseResponses: [][]int16{ir}})
	if err != nil {
		t.Fatal(err)
	}
	if len(x) != SynthRate/2 {
		t.Errorf("%d samples, want %d", len(x), SynthRate/2)
	}
}

func TestBandEdges(t *testing.T) {
	e := BandEdges(10000)
	want := []float64{0, 19.53125, 39.0625, 78.125, 156.25, 312.5, 625, 1250, 2500, 5000}
	for i := range want {
		if math.Abs(e[i]-want[i]) > 1e-9 {
			t.Fatalf("edges %v, want %v", e, want)
		}
	}
	gain, leak := bankGains(bankFor(10000))
	for b := range gain {
		for c := range gain {
			if c != b && leak[b][c] >= leak[b][b] {
				t.Errorf("band %d measures %.2f of band %d noise and %.2f of its own", b, leak[b][c], c, leak[b][b])
			}
		}
	}
	m := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9}
	x := solve(leak, m)
	for b := range m {
		var y float64
		for c := range x {
			y += leak[b][c] * x[c]
		}
		if math.Abs(y-m[b]) > 1e-9 {
			t.Fatalf("solve: row %d gives %g, want %g", b, y, m[b])
		}
	}
}
