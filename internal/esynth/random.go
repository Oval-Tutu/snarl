package esynth

// RandMax is RAND_MAX of glibc.
const RandMax = 2147483647

// Random ports the glibc random() generator that rand() returns: the
// additive feedback generator of degree 31 and separation 3 (TYPE_3), as
// srandom_r and random_r in glibc stdlib/random_r.c define it. engine-sim
// does not call srand(), so rand() starts with seed 1.
type Random struct {
	state [31]int32
	f, r  int
	calls int64
}

// NewRandom ports srandom_r for the glibc TYPE_3 state with the given seed.
func NewRandom(seed uint32) *Random {
	if seed == 0 {
		seed = 1
	}
	g := &Random{f: 3}
	g.state[0] = int32(seed)   //nolint:gosec // glibc stores the seed as int32_t.
	word := int64(int32(seed)) //nolint:gosec // glibc computes from the int32_t seed.
	for i := 1; i < len(g.state); i++ {
		hi := word / 127773
		lo := word % 127773
		word = 16807*lo - 2836*hi
		if word < 0 {
			word += 2147483647
		}
		g.state[i] = int32(word) //nolint:gosec // word is in 0 to 2^31-1.
	}
	for range 10 * len(g.state) {
		g.next()
	}
	g.calls = 0
	return g
}

// next ports random_r for TYPE_3.
func (g *Random) next() int32 {
	val := uint32(g.state[g.f]) + uint32(g.state[g.r]) //nolint:gosec // glibc adds as uint32_t.
	g.state[g.f] = int32(val)                          //nolint:gosec // glibc stores the sum as int32_t.
	g.f++
	if g.f >= len(g.state) {
		g.f = 0
		g.r++
	} else {
		g.r++
		if g.r >= len(g.state) {
			g.r = 0
		}
	}
	g.calls++
	return int32(val >> 1) //nolint:gosec // val >> 1 is below 2^31.
}

// Rand ports rand(), which glibc defines as (int) random().
func (g *Random) Rand() int32 {
	return g.next()
}

// Skip discards n values: the rand() calls that engine-sim makes outside the
// synthesiser, for example in CombustionChamber::ignite.
func (g *Random) Skip(n int64) {
	for range n {
		g.next()
	}
}

// Calls returns the number of values drawn since NewRandom.
func (g *Random) Calls() int64 {
	return g.calls
}

// minstdRand0 ports std::minstd_rand0, the std::default_random_engine of
// libstdc++, with its default seed 1.
type minstdRand0 struct {
	x uint64
}

const (
	minstdMultiplier = 16807
	minstdModulus    = 2147483647
)

func newMinstdRand0() minstdRand0 {
	return minstdRand0{x: 1}
}

// next ports linear_congruential_engine::operator().
func (g *minstdRand0) next() uint64 {
	g.x = g.x * minstdMultiplier % minstdModulus
	return g.x
}

// uniformFloat32 ports std::uniform_real_distribution<float>(a, b) with
// std::minstd_rand0 in libstdc++ 16, which the renderer links. The
// distribution returns generate_canonical<float, 24>() * (b - a) + a in
// float. generate_canonical (__generate_canonical_any in bits/random.tcc)
// draws one value, because the engine range R = 2^31 - 2 holds 30 bits,
// divides it by x = R / 2^24 = 127, and draws again when the float result
// is not below 1.
func (g *minstdRand0) uniformFloat32(a, b float32) float32 {
	const (
		engineMin = 1
		r         = minstdModulus - 1
		rd        = 1 << 24
		x         = r / rd
	)
	for {
		sum := uint32(g.next() - engineMin) //nolint:gosec // The engine value is below 2^31.
		ret := float32(sum/x) / float32(rd)
		if ret < 1 {
			// The conversion stops a fused multiply-add, which C++ does not use here.
			return float32(ret*(b-a)) + a
		}
	}
}
