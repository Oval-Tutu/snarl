package cycles

import "slices"

const (
	// closeFadeS is the time in seconds over which the closing transient
	// fades to the steady closed-throttle row after its last slice. No
	// reference gives it: the closing steps of the records end there.
	closeFadeS = 1.0
	// closedThrottle is the highest throttle that counts as closed for the
	// closing transient.
	closedThrottle = 0.01
)

// closeRow is the closing transient at one RPM: its slices by the time
// since the close, and the reciprocal of each gap to the next slice.
type closeRow struct {
	rpm    float64
	points []int
	since  []float64
	inv    []float64
}

// closeSet is the closing transient of the tables: one row per RPM of the
// closing steps, by RPM, with the reciprocal of each RPM gap to the next
// row.
type closeSet struct {
	rows []closeRow
	rpm  []float64
	inv  []float64
}

// addClose adds point i, a slice of a closing transient, to its row.
func (b *Bank) addClose(i int) {
	p := &b.t.Points[i]
	k := slices.IndexFunc(b.close.rows, func(r closeRow) bool { return r.rpm == p.RPM })
	if k < 0 {
		b.close.rows = append(b.close.rows, closeRow{rpm: p.RPM})
		k = len(b.close.rows) - 1
	}
	b.close.rows[k].points = append(b.close.rows[k].points, i)
}

// sort orders the rows by RPM and the slices of each row by the time since
// the close, and fills the gaps.
func (s *closeSet) sort(t *Tables) {
	slices.SortFunc(s.rows, func(a, b closeRow) int { return cmpFloat(a.rpm, b.rpm) })
	s.rpm, s.inv = s.rpm[:0], s.inv[:0]
	for r := range s.rows {
		row := &s.rows[r]
		slices.SortFunc(row.points, func(a, b int) int { return cmpFloat(t.Points[a].Since, t.Points[b].Since) })
		row.since, row.inv = row.since[:0], row.inv[:0]
		for i, p := range row.points {
			row.since = append(row.since, t.Points[p].Since)
			if i > 0 {
				row.inv = append(row.inv, 0)
				if d := row.since[i] - row.since[i-1]; d > 0 {
					row.inv[i-1] = 1 / d
				}
			}
		}
		s.rpm = append(s.rpm, row.rpm)
		if r > 0 {
			s.inv = append(s.inv, 0)
			if d := s.rpm[r] - s.rpm[r-1]; d > 0 {
				s.inv[r-1] = 1 / d
			}
		}
	}
}

// share returns the share of the closed-throttle row that the transient of
// the row takes at the time since the close: 1 up to its last slice, then
// falling linearly to 0 over closeFadeS.
func (r *closeRow) share(since float64) float64 {
	last := r.since[len(r.since)-1]
	if since <= last {
		return 1
	}
	return max(0, 1-(since-last)*(1/closeFadeS))
}

// appendRow appends the slices of row r around the time since the close,
// with the total weight w.
func (r *closeRow) appendRow(out []Weight, since, w float64) []Weight {
	n := len(r.since)
	switch {
	case n == 1 || since <= r.since[0]:
		return append(out, Weight{r.points[0], w})
	case since >= r.since[n-1]:
		return append(out, Weight{r.points[n-1], w})
	}
	i := 1
	for r.since[i] < since {
		i++
	}
	g := (since - r.since[i-1]) * r.inv[i-1]
	if w0 := w * (1 - g); w0 > 0 {
		out = append(out, Weight{r.points[i-1], w0})
	}
	if w1 := w * g; w1 > 0 {
		out = append(out, Weight{r.points[i], w1})
	}
	return out
}

// appendWeights appends the transient slices for the RPM and the time
// since the close, out of the weight w of the closed-throttle row: linear
// in the RPM between the two nearest rows, and in the time between the two
// nearest slices of each row. It returns the weight that the transient
// takes, and the rest stays with the steady row.
func (s *closeSet) appendWeights(out []Weight, rpm, since, w float64) ([]Weight, float64) {
	n := len(s.rows)
	a, b, g := 0, 0, 0.0
	switch {
	case n == 1 || !(rpm > s.rpm[0]):
	case rpm >= s.rpm[n-1]:
		a, b = n-1, n-1
	default:
		b = 1
		for s.rpm[b] < rpm {
			b++
		}
		a = b - 1
		g = (rpm - s.rpm[a]) * s.inv[a]
	}
	wa := w * (1 - g) * s.rows[a].share(since)
	wb := w * g * s.rows[b].share(since)
	if wa > 1e-6 {
		out = s.rows[a].appendRow(out, since, wa)
	}
	if wb > 1e-6 {
		out = s.rows[b].appendRow(out, since, wb)
	}
	return out, wa + wb
}

// closeClock advances the time since the throttle closed by one step. A
// throttle of at least CloseFrom arms the clock, a closed throttle after
// that starts it, and a throttle above closedThrottle stops it.
func (p *Live) closeClock(throttle float64) {
	switch {
	case throttle >= CloseFrom:
		p.armed, p.since = true, -1
	case throttle > closedThrottle:
		p.since = -1
	case p.armed:
		p.armed, p.since = false, 0
	case p.since >= 0:
		p.since += p.dt
	}
}
