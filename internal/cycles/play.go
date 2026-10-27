package cycles

import (
	"math"
	"math/rand/v2"
	"slices"
)

// PlayOptions set the playback.
type PlayOptions struct {
	// Seed seeds the residual noise.
	Seed uint64
	// NoResidual turns the residual noise off.
	NoResidual bool
	// UniformSpeed advances the crank phase at the mean speed, without the
	// speed profile of the cycle.
	UniformSpeed bool
	// NoDelayShift interpolates between grid points in the crank angle,
	// not in the emission angle.
	NoDelayShift bool
	// NoLag changes the throttle rows at once, without the intake lag of
	// the tables.
	NoLag bool
}

// Player produces the synthesiser input from the tables, one sample per
// simulation step, for an RPM, a throttle and an ignition state that may
// change at every step.
type Player struct {
	t      *Tables
	opt    PlayOptions
	dt     float64
	phase  float64
	rows   map[bool][]row
	waves  [][]WaveTable
	gains  []float64
	banks  [][]band
	rng    *rand.Rand
	weight []Weight
	// lagRows are the lagged throttle row weights of the ignition state
	// lagIgnition.
	lagRows     []float64
	lagIgnition bool
	target      []float64
	// mod holds the modulation of each channel, which draws from modRng.
	mod    []modulation
	modRng uint64
}

// modulation is the random gain of the average cycle of one channel: a
// Gaussian value of unit variance per part of the cycle, with the
// correlation rho to the value of the part before, interpolated linearly
// between the part centres. slot is the part before the phase, a0 its
// value and a1 the value of the next part. started is false before the
// first value.
type modulation struct {
	slot    int
	a0, a1  float64
	started bool
}

// at returns the unit gain at the emission phase psi, from 0 to 1, for
// slots parts per cycle and the correlation rho, with a value from the
// xorshift state rng (xorshiftNormal) for each new part. Each change of
// the part takes the next value.
func (m *modulation) at(psi float64, slots int, rho float64, rng *uint64) float64 {
	u := psi*float64(slots) - 0.5
	// u is at least -0.5, so the conversion of u + 1 rounds down. The part
	// counts modulo the parts of a cycle, so that a phase that wraps at the
	// end of the cycle stays in its part.
	k := int(u+1) - 1
	f := u - float64(k)
	if k < 0 {
		k += slots
	}
	switch {
	case !m.started:
		m.a0 = xorshiftNormal(rng)
		m.a1 = rho*m.a0 + math.Sqrt(1-rho*rho)*xorshiftNormal(rng)
		m.slot, m.started = k, true
	case k != m.slot:
		m.a0, m.slot = m.a1, k
		m.a1 = rho*m.a0 + math.Sqrt(1-rho*rho)*xorshiftNormal(rng)
	}
	return m.a0 + f*(m.a1-m.a0)
}

// row is the points of one throttle and ignition state, by RPM.
type row struct {
	throttle float64
	points   []int
}

// Weight is the weight W of the table point Point in the playback.
type Weight struct {
	Point int
	W     float64
}

// NewPlayer returns a player of the tables at phase 0.
func NewPlayer(t *Tables, opt PlayOptions) *Player {
	p := &Player{t: t, opt: opt, dt: 1 / t.Rate, rows: map[bool][]row{}, rng: rand.New(rand.NewPCG(opt.Seed, 0x5eed)), mod: make([]modulation, len(t.Channels)), modRng: opt.Seed*0xbf58476d1ce4e5b9 + 0x94d049bb133111eb} //nolint:gosec // Noise, not security.
	for i, pt := range t.Points {
		// The player has no closing transient.
		if pt.Since > 0 {
			continue
		}
		rs := p.rows[pt.Ignition]
		k := slices.IndexFunc(rs, func(r row) bool { return r.throttle == pt.Throttle })
		if k < 0 {
			rs = append(rs, row{throttle: pt.Throttle})
			k = len(rs) - 1
		}
		rs[k].points = append(rs[k].points, i)
		p.rows[pt.Ignition] = rs
	}
	for ign, rs := range p.rows {
		slices.SortFunc(rs, func(a, b row) int { return cmpFloat(a.throttle, b.throttle) })
		for _, r := range rs {
			slices.SortFunc(r.points, func(a, b int) int { return cmpFloat(t.Points[a].RPM, t.Points[b].RPM) })
		}
		p.rows[ign] = rs
	}
	p.waves = make([][]WaveTable, len(t.Points))
	for i, pt := range t.Points {
		p.waves[i] = make([]WaveTable, len(t.Channels))
		for c := range t.Channels {
			p.waves[i][c] = NewWaveTable(pt.Wave[c])
		}
	}
	bank := bankFor(t.Rate)
	p.gains, _ = bankGains(bank)
	p.banks = make([][]band, len(t.Channels))
	for c := range p.banks {
		p.banks[c] = make([]band, len(bank))
		for b := range bank {
			p.banks[c][b].sections = append([]biquad(nil), bank[b].sections...)
		}
	}
	return p
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Weights returns the grid points and weights for an RPM, a throttle and
// an ignition state: linear in the throttle between the two nearest
// throttle rows, and in each row linear in the RPM between the two
// nearest points. Outside the grid it takes the nearest edge. With the
// ignition off and no point without ignition, it uses the points with.
func (p *Player) Weights(rpm, throttle float64, ignition bool) []Weight {
	rs := p.rowsFor(ignition)
	return p.pointWeights(rs, p.rowTargets(rs, throttle), rpm)
}

// rowsFor returns the throttle rows of the ignition state, or of the other
// state when it has none.
func (p *Player) rowsFor(ignition bool) []row {
	if rs := p.rows[ignition]; len(rs) > 0 {
		return rs
	}
	return p.rows[!ignition]
}

// rowTargets returns the weight of each throttle row for the throttle:
// linear between the two nearest rows.
func (p *Player) rowTargets(rs []row, throttle float64) []float64 {
	if cap(p.target) < len(rs) {
		p.target = make([]float64, len(rs))
	}
	w := p.target[:len(rs)]
	clear(w)
	r0, r1, f := bracket(len(rs), func(i int) float64 { return rs[i].throttle }, throttle)
	w[r0] += 1 - f
	w[r1] += f
	return w
}

// pointWeights spreads the row weights over the points of each row,
// linear in the RPM between the two nearest points.
func (p *Player) pointWeights(rs []row, rowWeights []float64, rpm float64) []Weight {
	out := p.weight[:0]
	for r, rw := range rowWeights {
		if rw < 1e-6 {
			continue
		}
		pts := rs[r].points
		a, b, g := bracket(len(pts), func(i int) float64 { return p.t.Points[pts[i]].RPM }, rpm)
		if w := rw * (1 - g); w > 0 {
			out = append(out, Weight{pts[a], w})
		}
		if w := rw * g; w > 0 {
			out = append(out, Weight{pts[b], w})
		}
	}
	var sum float64
	for _, w := range out {
		sum += w.W
	}
	for i := range out {
		out[i].W /= sum
	}
	p.weight = out
	return out
}

// laggedWeights returns the point weights with the intake lag of the
// tables: the row weights move to their targets with a first-order lag,
// of the closing time constant when the throttle is below the effective
// throttle of the current weights and of the opening one otherwise. A
// change of the ignition state resets the lag.
func (p *Player) laggedWeights(rpm, throttle float64, ignition bool) []Weight {
	lag := p.t.ThrottleLag
	rs := p.rowsFor(ignition)
	target := p.rowTargets(rs, throttle)
	if p.opt.NoLag || (lag.Close <= 0 && lag.Open <= 0) || p.lagRows == nil || p.lagIgnition != ignition {
		p.lagRows, p.lagIgnition = append(p.lagRows[:0], target...), ignition
		return p.pointWeights(rs, target, rpm)
	}
	var effective float64
	for r, w := range p.lagRows {
		effective += w * rs[r].throttle
	}
	tau := lag.Open
	if throttle < effective {
		tau = lag.Close
	}
	k := 1.0
	if tau > 0 {
		k = -math.Expm1(-p.dt / tau)
	}
	for r := range p.lagRows {
		p.lagRows[r] += k * (target[r] - p.lagRows[r])
	}
	return p.pointWeights(rs, p.lagRows, rpm)
}

// bracket returns the indices of the two values of a sorted list around v
// and the weight of the second.
func bracket(n int, at func(int) float64, v float64) (i0, i1 int, f float64) {
	if n == 1 || v <= at(0) {
		return 0, 0, 0
	}
	if v >= at(n-1) {
		return n - 1, n - 1, 0
	}
	i := 1
	for at(i) < v {
		i++
	}
	return i - 1, i, (v - at(i-1)) / (at(i) - at(i-1))
}

// Next advances the crank by one step at the RPM and writes the input of
// each channel to out.
func (p *Player) Next(rpm, throttle float64, ignition bool, out []float64) {
	ws := p.laggedWeights(rpm, throttle, ignition)
	speed := 1.0
	if !p.opt.UniformSpeed {
		speed = 0
		f := (p.phase - math.Floor(p.phase)) * SpeedPoints
		i := int(f) % SpeedPoints
		a := f - math.Floor(f)
		for _, w := range ws {
			s := p.t.Points[w.Point].Speed
			speed += w.W * (s[i] + a*(s[(i+1)%SpeedPoints]-s[i]))
		}
	}
	p.phase += p.dt * rpm / 120 * speed

	out = out[:len(p.t.Channels)]
	for c := range out {
		ch := p.t.Channels[c]
		shift := ch.DelaySteps * p.dt * rpm / 120
		v := 0.0
		for _, w := range ws {
			psi := p.phase - shift
			if p.opt.NoDelayShift {
				// The table is in the emission angle at its own RPM.
				psi = p.phase - ch.DelaySteps*p.dt*p.t.Points[w.Point].MeasuredRPM/120
			}
			v += w.W * p.waves[w.Point][c].At(psi)
		}
		if !p.opt.NoResidual {
			v += p.modulated(c, ws, p.phase-shift, v)
			v += p.noise(c, ws, p.phase-shift)
		}
		out[c] = v //nolint:gosec // c ranges over out.
	}
}

// modulated returns the modulation of channel c at emission phase psi: the
// unit gain of the channel times the blended RMS gain of the points times
// the blended average cycle v without its blended mean.
func (p *Player) modulated(c int, ws []Weight, psi, v float64) float64 {
	if p.t.ModulationSlots <= 0 {
		return 0
	}
	var depth, rho, mean float64
	for _, w := range ws {
		pt := &p.t.Points[w.Point]
		if c < len(pt.Modulation) {
			depth += w.W * pt.Modulation[c]
		}
		if c < len(pt.ModulationRho) {
			rho += w.W * pt.ModulationRho[c]
		}
		mean += w.W * pt.Wave[c][0]
	}
	if depth == 0 {
		return 0
	}
	return depth * p.mod[c].at(psi-math.Floor(psi), p.t.ModulationSlots, rho, &p.modRng) * (v - mean)
}

// noise returns the residual of channel c at emission phase psi: unit
// Gaussian noise through each band filter, normalised to unit power and
// scaled by the band envelope.
func (p *Player) noise(c int, ws []Weight, psi float64) float64 {
	// Each sector value is the mean over the sector, so it sits at the
	// sector centre.
	f := (psi-math.Floor(psi))*Sectors - 0.5
	fl := math.Floor(f)
	s0 := (int(fl) + Sectors) % Sectors
	s1 := (s0 + 1) % Sectors
	a := f - fl
	v := 0.0
	for b := range p.banks[c] {
		y := p.banks[c][b].process(p.rng.NormFloat64()) / math.Sqrt(p.gains[b])
		env := 0.0
		for _, w := range ws {
			e := p.t.Points[w.Point].Noise[c][b*Sectors:]
			env += w.W * (e[s0] + a*(e[s1]-e[s0]))
		}
		v += env * y
	}
	return v
}
