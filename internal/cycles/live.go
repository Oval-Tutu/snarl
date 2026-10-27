package cycles

import (
	"errors"
	"math"
	"slices"
)

// Bank holds the playback data of one set of tables for Live players: the
// wave table of each point and channel, the throttle rows of each ignition
// state, and the residual band filters. It does not change after NewBank,
// so the players of several engines can share it.
type Bank struct {
	t *Tables
	// rows are the throttle rows of the ignition off (0) and on (1).
	rows [2][]liveRow
	// close is the closing transient.
	close closeSet
	waves [][]liveWave
	bands []band
	// norm brings the band noise to unit power.
	norm [BandCount]float64
	// modulated is true when a point has a modulation.
	modulated bool
}

// liveWave is the wave table of one point and channel, with its
// modulation beside it, so that one pass over the weights blends both: the
// RMS gain, the correlation of neighbouring parts, and the mean of the
// average cycle.
type liveWave struct {
	table            WaveTable
	depth, rho, mean float64
}

// liveRow is one throttle row: its points by RPM, their RPM, the
// reciprocal of each RPM gap to the next point, and the reciprocal of the
// throttle gap to the next row.
type liveRow struct {
	throttle float64
	invNext  float64
	points   []int
	rpm      []float64
	inv      []float64
}

// NewBank builds the playback data of the tables.
func NewBank(t *Tables) (*Bank, error) {
	if len(t.Points) == 0 || len(t.Channels) == 0 || !(t.Rate > 0) {
		return nil, errors.New("cycles: the tables have no points, no channels or no rate")
	}
	b := &Bank{t: t}
	for i, pt := range t.Points {
		if pt.Since > 0 {
			b.addClose(i)
			continue
		}
		ign := 0
		if pt.Ignition {
			ign = 1
		}
		k := slices.IndexFunc(b.rows[ign], func(r liveRow) bool { return r.throttle == pt.Throttle })
		if k < 0 {
			b.rows[ign] = append(b.rows[ign], liveRow{throttle: pt.Throttle})
			k = len(b.rows[ign]) - 1
		}
		b.rows[ign][k].points = append(b.rows[ign][k].points, i)
	}
	for ign := range b.rows {
		rs := b.rows[ign]
		slices.SortFunc(rs, func(a, c liveRow) int { return cmpFloat(a.throttle, c.throttle) })
		for r := range rs {
			row := &rs[r]
			if r+1 < len(rs) {
				row.invNext = 1 / (rs[r+1].throttle - row.throttle)
			}
			slices.SortFunc(row.points, func(a, c int) int { return cmpFloat(t.Points[a].RPM, t.Points[c].RPM) })
			for i, p := range row.points {
				row.rpm = append(row.rpm, t.Points[p].RPM)
				if i > 0 {
					row.inv = append(row.inv, 0)
					if d := row.rpm[i] - row.rpm[i-1]; d > 0 {
						row.inv[i-1] = 1 / d
					}
				}
			}
		}
	}
	if len(b.close.rows) > 0 && (len(b.rows[1]) == 0 || b.rows[1][0].throttle != 0) {
		return nil, errors.New("cycles: the tables have a closing transient but no steady closed-throttle row")
	}
	b.close.sort(t)
	b.waves = make([][]liveWave, len(t.Points))
	for i, pt := range t.Points {
		if len(pt.Wave) != len(t.Channels) || len(pt.Noise) != len(t.Channels) || len(pt.Speed) != SpeedPoints {
			return nil, errors.New("cycles: a point has the wrong array sizes")
		}
		b.waves[i] = make([]liveWave, len(t.Channels))
		for c := range t.Channels {
			w := &b.waves[i][c]
			w.table, w.mean = NewWaveTable(pt.Wave[c]), pt.Wave[c][0]
			if t.ModulationSlots > 0 && c < len(pt.Modulation) {
				w.depth = pt.Modulation[c]
				b.modulated = b.modulated || w.depth > 0
			}
			if c < len(pt.ModulationRho) {
				w.rho = pt.ModulationRho[c]
			}
		}
	}
	b.bands = bankFor(t.Rate)
	gains, _ := bankGains(b.bands)
	for i, g := range gains {
		b.norm[i] = 1 / math.Sqrt(g)
	}
	return b, nil
}

// Tables returns the tables of the bank.
func (b *Bank) Tables() *Tables { return b.t }

// rowsFor returns the index of the rows of the ignition state, or of the
// other state when it has none.
func (b *Bank) rowsFor(ignition bool) int {
	ign := 0
	if ignition {
		ign = 1
	}
	if len(b.rows[ign]) == 0 {
		return 1 - ign
	}
	return ign
}

// Live plays a Bank in real time: one input sample per channel and
// simulation step, for an RPM, a throttle and an ignition state that may
// change at every step. It is the playback of Player with the arithmetic
// of the audio goroutine: the throttle rows and the RPM points are found
// without division or allocation, the lag coefficients are computed once,
// and the residual noise is the sum of four uniform values, which has the
// mean and the variance of a unit Gaussian. Next does not allocate, lock,
// divide or call math.Sin, math.Exp or math.Pow. A Live is not safe for
// concurrent use.
type Live struct {
	b     *Bank
	dt    float64
	phase float64
	// lag holds the lagged row weights of each ignition state, and mix the
	// share of the ignition-on state. started is false before the first
	// step.
	lag     [2][]float64
	mix     float64
	started bool
	// since is the time in seconds since the throttle closed, or -1 with
	// no closing transient, and armed is true after an open throttle.
	since         float64
	armed         bool
	kClose, kOpen float64
	target        []float64
	w             []Weight
	bands         [][]band
	rng           uint64
	noResidual    bool
	speedPoints   float64
	sectors       float64
	// bandNoise holds the band noise of the next noiseSteps steps,
	// normalised: the noiseSteps values of band b of channel c start at
	// (c*BandCount+b)*noiseSteps. noisePos is the step of the next Next.
	// The draws and the band filters do not depend on the input, so that
	// fillNoise computes them ahead, in the order of the draws of one step
	// after the other.
	bandNoise []float64
	noisePos  int
	// mod holds the modulation of each channel, which draws from modRng.
	mod    []modulation
	modRng uint64
}

// noiseSteps is the number of steps of band noise that fillNoise computes
// at once.
const noiseSteps = 64

// LiveOptions set a Live player.
type LiveOptions struct {
	// Lag is the intake lag of the throttle rows.
	Lag ThrottleLag
	// Seed seeds the residual noise.
	Seed uint64
	// NoResidual turns the residual noise off.
	NoResidual bool
}

// NewLive returns a player of the bank at crank phase 0.
func (b *Bank) NewLive(o LiveOptions) *Live {
	p := &Live{
		b: b, dt: 1 / b.t.Rate, kClose: 1, kOpen: 1, since: -1,
		rng:         o.Seed*0x9e3779b97f4a7c15 + 0x2545f4914f6cdd1d,
		modRng:      o.Seed*0xbf58476d1ce4e5b9 + 0x94d049bb133111eb,
		noResidual:  o.NoResidual,
		speedPoints: SpeedPoints, sectors: Sectors,
		mod: make([]modulation, len(b.t.Channels)),
	}
	if o.Lag.Close > 0 {
		p.kClose = -math.Expm1(-p.dt / o.Lag.Close)
	}
	if o.Lag.Open > 0 {
		p.kOpen = -math.Expm1(-p.dt / o.Lag.Open)
	}
	n := max(len(b.rows[0]), len(b.rows[1]))
	p.target = make([]float64, n)
	for i := range p.lag {
		p.lag[i] = make([]float64, len(b.rows[i]))
	}
	p.w = make([]Weight, 0, 4*n+4)
	p.bands = make([][]band, len(b.t.Channels))
	for c := range p.bands {
		p.bands[c] = make([]band, len(b.bands))
		for k := range b.bands {
			p.bands[c][k].sections = append([]biquad(nil), b.bands[k].sections...)
		}
	}
	p.bandNoise = make([]float64, len(b.t.Channels)*BandCount*noiseSteps)
	p.noisePos = noiseSteps
	return p
}

// fillNoise computes the band noise of the next noiseSteps steps: the draws
// in the order of the steps, the channels and the bands, then each band
// filter over its draws and the normalisation. Each value is the value
// that the filter of the step would give.
func (p *Live) fillNoise() {
	const nb = BandCount
	for s := range noiseSteps {
		for c := range p.bands {
			for b := range nb {
				p.bandNoise[(c*nb+b)*noiseSteps+s] = p.normal()
			}
		}
	}
	for c := range p.bands {
		for b := range nb {
			x := p.bandNoise[(c*nb+b)*noiseSteps : (c*nb+b+1)*noiseSteps]
			p.bands[c][b].block(x)
			norm := p.b.norm[b]
			for i := range x {
				x[i] *= norm
			}
		}
	}
	p.noisePos = 0
}

// Phase returns the crank phase in cycles, from 0 to 1.
func (p *Live) Phase() float64 { return p.phase }

// Next advances the crank by one simulation step at the RPM and writes the
// input of each channel to out.
func (p *Live) Next(rpm, throttle float64, ignition bool, out []float64) {
	ws := p.weights(rpm, throttle, ignition)
	pts := p.b.t.Points
	// The speed profile of the cycle.
	f := p.phase * p.speedPoints
	i := int(f)
	if i >= SpeedPoints {
		i = SpeedPoints - 1
	}
	a := f - float64(i)
	j := i + 1
	if j == SpeedPoints {
		j = 0
	}
	speed := 0.0
	for _, w := range ws {
		s := pts[w.Point].Speed
		speed += w.W * (s[i] + a*(s[j]-s[i]))
	}
	p.phase += p.dt * rpm * (1.0 / 120) * speed
	if p.phase >= 1 || p.phase < 0 {
		p.phase -= math.Floor(p.phase)
	}

	out = out[:len(p.b.t.Channels)]
	if !p.noResidual && p.noisePos == noiseSteps {
		p.fillNoise()
	}
	for c := range out {
		psi := p.phase - p.b.t.Channels[c].DelaySteps*p.dt*rpm*(1.0/120)
		psi -= math.Floor(psi)
		v := 0.0
		if !p.b.modulated || p.noResidual {
			for _, w := range ws {
				v += w.W * p.b.waves[w.Point][c].table.At(psi)
			}
		} else {
			// The modulation of Player.modulated, blended in the same pass.
			var depth, rho, mean float64
			for _, w := range ws {
				lw := &p.b.waves[w.Point][c]
				v += w.W * lw.table.At(psi)
				depth += w.W * lw.depth
				rho += w.W * lw.rho
				mean += w.W * lw.mean
			}
			if depth > 0 {
				v += depth * p.mod[c].at(psi, p.b.t.ModulationSlots, rho, &p.modRng) * (v - mean)
			}
		}
		if !p.noResidual {
			v += p.noise(c, ws, psi)
		}
		out[c] = v //nolint:gosec // c ranges over out.
	}
	p.noisePos++
}

// weights returns the point weights. The share of the ignition-on points
// moves to the ignition state over one engine cycle, in which each
// cylinder fires or misses once. In each ignition state, the row weights
// follow the intake lag: they move to their targets with a first-order
// lag, of the closing time constant when the throttle is below the
// effective throttle of the current weights and of the opening one
// otherwise.
func (p *Live) weights(rpm, throttle float64, ignition bool) []Weight {
	on, off := p.b.rowsFor(true), p.b.rowsFor(false)
	p.closeClock(throttle)
	first := !p.started
	p.started = true
	switch cycle := p.dt * max(rpm, 0) * (1.0 / 120); {
	case first:
		p.mix = 0
		if ignition {
			p.mix = 1
		}
	case ignition:
		p.mix = min(1, p.mix+cycle)
	default:
		p.mix = max(0, p.mix-cycle)
	}
	p.lagRows(on, throttle, first)
	if off != on {
		p.lagRows(off, throttle, first)
	}
	out := p.w[:0]
	// lost is the weight of the rows below 1e-6, which the first point
	// takes, so that the weights sum to 1 and the crank speed keeps its
	// scale.
	lost := 0.0
	for _, s := range [2]struct {
		ig    int
		share float64
	}{{on, p.mix}, {off, 1 - p.mix}} {
		if s.share <= 0 {
			continue
		}
		rs := p.b.rows[s.ig]
		for r, rw := range p.lag[s.ig] {
			rw *= s.share
			if rw < 1e-6 {
				lost += rw
				continue
			}
			// After a close, the transient takes the closed-throttle row.
			if s.ig == 1 && r == 0 && p.since >= 0 && len(p.b.close.rows) > 0 {
				var taken float64
				out, taken = p.b.close.appendWeights(out, rpm, p.since, rw)
				if rw -= taken; rw < 1e-6 {
					lost += rw
					continue
				}
			}
			row := &rs[r]
			a, b, g := pointBracket(row, rpm)
			if w := rw * (1 - g); w > 0 {
				out = append(out, Weight{row.points[a], w})
			}
			if w := rw * g; w > 0 {
				out = append(out, Weight{row.points[b], w})
			}
		}
	}
	if len(out) > 0 {
		out[0].W += lost
	}
	p.w = out
	return out
}

// lagRows moves the lagged row weights of the rows ig one step toward the
// targets of the throttle, or sets them to the targets on the first step.
func (p *Live) lagRows(ig int, throttle float64, first bool) {
	rs := p.b.rows[ig]
	target := p.target[:len(rs)]
	clear(target)
	r0, r1, f := rowBracket(rs, throttle)
	target[r0] += 1 - f
	target[r1] += f
	lag := p.lag[ig]
	if first {
		copy(lag, target)
		return
	}
	var effective float64
	for r, w := range lag {
		effective += w * rs[r].throttle
	}
	k := p.kOpen
	if throttle < effective {
		k = p.kClose
	}
	for r := range lag {
		lag[r] += k * (target[r] - lag[r])
	}
}

// rowBracket returns the two rows around the throttle and the weight of
// the second. Outside the rows it takes the edge.
func rowBracket(rs []liveRow, throttle float64) (r0, r1 int, f float64) {
	n := len(rs)
	if n == 1 || !(throttle > rs[0].throttle) {
		return 0, 0, 0
	}
	if throttle >= rs[n-1].throttle {
		return n - 1, n - 1, 0
	}
	i := 1
	for rs[i].throttle < throttle {
		i++
	}
	return i - 1, i, (throttle - rs[i-1].throttle) * rs[i-1].invNext
}

// pointBracket returns the two points of the row around the RPM and the
// weight of the second. Outside the row it takes the edge.
func pointBracket(row *liveRow, rpm float64) (a, b int, g float64) {
	n := len(row.rpm)
	if n == 1 || !(rpm > row.rpm[0]) {
		return 0, 0, 0
	}
	if rpm >= row.rpm[n-1] {
		return n - 1, n - 1, 0
	}
	// Binary search for the first point above the RPM.
	lo, hi := 1, n-1
	for lo < hi {
		m := (lo + hi) / 2
		if row.rpm[m] < rpm {
			lo = m + 1
		} else {
			hi = m
		}
	}
	return lo - 1, lo, (rpm - row.rpm[lo-1]) * row.inv[lo-1]
}

// normal returns a value with the mean and the variance of a unit
// Gaussian: the sum of four uniform values from one xorshift draw.
func (p *Live) normal() float64 { return xorshiftNormal(&p.rng) }

// xorshiftNormal returns a value with the mean and the variance of a unit
// Gaussian from the xorshift state, as normal describes.
func xorshiftNormal(state *uint64) float64 {
	x := *state
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	*state = x
	const unit = 1.0 / 65536
	s := float64(x&0xffff) + float64(x>>16&0xffff) + float64(x>>32&0xffff) + float64(x>>48)
	// Four uniform values from 0 to 1 have a mean of 2 and a variance of
	// 1/3.
	const sqrt3 = 1.7320508075688772
	return (s*unit - 2) * sqrt3
}

// noise returns the residual of channel c at emission phase psi: noise
// through each band filter, normalised to unit power and scaled by the
// band envelope. The envelope of each band sums the weights in their
// order, and the residual sums the bands in their order.
func (p *Live) noise(c int, ws []Weight, psi float64) float64 {
	// Each sector value is the mean over the sector, so it sits at the
	// sector centre.
	f := psi*p.sectors - 0.5
	s0 := int(f + Sectors)
	a := f + Sectors - float64(s0)
	s0 %= Sectors
	s1 := s0 + 1
	if s1 == Sectors {
		s1 = 0
	}
	pts := p.b.t.Points
	var env [BandCount]float64
	for _, w := range ws {
		nz := pts[w.Point].Noise[c]
		nz = nz[:BandCount*Sectors]
		for b := range env {
			e0, e1 := nz[b*Sectors+s0], nz[b*Sectors+s1]
			env[b] += w.W * (e0 + a*(e1-e0))
		}
	}
	y := p.bandNoise[c*BandCount*noiseSteps+p.noisePos:]
	v := 0.0
	for b := range env {
		v += env[b] * y[b*noiseSteps]
	}
	return v
}
