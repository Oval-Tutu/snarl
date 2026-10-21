package esynth

// fill writes the next len(dst) rand() values to dst, the same values that
// len(dst) calls of Rand return. The glibc generator is the recurrence
// x[n] = x[n-31] + x[n-3] mod 2^32 with the output x[n] >> 1, so fill runs
// it over a linear history in work instead of the ring of the state. work
// must hold at least len(dst) + 31 values.
func (g *Random) fill(dst []int32, work []uint32) {
	const deg = len(g.state)
	n := len(dst)
	h := work[:n+deg]
	// h[k] is the state slot written 31-k calls ago, oldest first.
	s := g.f
	for k := range deg {
		h[k] = uint32(g.state[s]) //nolint:gosec // glibc adds as uint32_t.
		s++
		if s == deg {
			s = 0
		}
	}
	for j := range n {
		v := h[j] + h[j+deg-3]
		h[j+deg] = v
		dst[j] = int32(v >> 1) //nolint:gosec // v >> 1 is below 2^31.
	}
	f := (g.f + n) % deg
	s = f
	for k := range deg {
		g.state[s] = int32(h[n+k]) //nolint:gosec // glibc stores the sum as int32_t.
		s++
		if s == deg {
			s = 0
		}
	}
	g.f = f
	g.r = f - 3
	if g.r < 0 {
		g.r += deg
	}
	g.calls += int64(n)
}
