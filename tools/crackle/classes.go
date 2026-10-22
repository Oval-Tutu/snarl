package main

import (
	"math"
	"math/rand/v2"
	"slices"
)

const (
	// kmeansStarts is the number of k-means runs from random seeds, and
	// kmeansSteps the most steps of each run.
	kmeansStarts = 50
	kmeansSteps  = 100
	// maxK is the most classes that classify tries.
	maxK = 5
)

// featureNames name the features of the classes, in the order of
// features.
var featureNames = []string{
	"log10 active length", "log10 shocks", "log10 strong shocks",
	"share above 2 kHz dB", "share below 125 Hz dB", "crest factor dB",
}

// features returns the features of a clip for the classes.
func features(c *clip) []float64 {
	strong := 0
	for _, s := range c.Shocks {
		if s.Strong {
			strong++
		}
	}
	return []float64{
		math.Log10(c.Length), math.Log10(float64(max(1, len(c.Shocks)))), math.Log10(float64(max(1, strong))),
		c.HiDB, c.LowDB, c.CrestDB,
	}
}

// classes is the grouping of the clips.
type classes struct {
	K      int
	Assign []int
	// Sil holds the mean silhouette for each k from 2 when classify
	// chose k, and Chosen the silhouette of K.
	Sil    []float64
	Chosen float64
}

// classify groups the clips into k classes with k-means on the
// standardised features, or into the number of classes from 2 to maxK
// with the highest mean silhouette when k is 0. The classes are in the
// order of their mean number of shocks.
func classify(clips []*clip, k int) classes {
	n := len(clips)
	cl := classes{K: 1, Assign: make([]int, n), Chosen: math.NaN()}
	if k == 1 || n < 3 {
		return cl
	}
	x := make([][]float64, n)
	for i, c := range clips {
		x[i] = features(c)
	}
	standardise(x)
	rng := newRand()
	if k > 0 {
		cl.K = min(k, n-1)
		cl.Assign = kmeans(x, cl.K, rng)
		cl.Chosen = silhouette(x, cl.Assign, cl.K)
	} else {
		cl.Chosen = math.Inf(-1)
		for kk := 2; kk <= min(maxK, n-1); kk++ {
			a := kmeans(x, kk, rng)
			s := silhouette(x, a, kk)
			cl.Sil = append(cl.Sil, s)
			if s > cl.Chosen {
				cl.K, cl.Assign, cl.Chosen = kk, a, s
			}
		}
	}
	mean := make([]float64, cl.K)
	count := make([]float64, cl.K)
	for i, a := range cl.Assign {
		mean[a] += x[i][1]
		count[a]++
	}
	order := make([]int, cl.K)
	for j := range order {
		order[j] = j
		mean[j] /= count[j]
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmpFloat(mean[a], mean[b]) })
	rank := make([]int, cl.K)
	for r, j := range order {
		rank[j] = r
	}
	for i, a := range cl.Assign {
		cl.Assign[i] = rank[a]
	}
	return cl
}

// newRand returns the random source of the k-means seeds, with a fixed
// seed so that two runs give the same classes.
func newRand() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// standardise scales each column of x to zero mean and unit spread. A
// column with no spread becomes zero.
func standardise(x [][]float64) {
	for j := range x[0] {
		var m, v float64
		for _, r := range x {
			m += r[j]
		}
		m /= float64(len(x))
		for _, r := range x {
			v += (r[j] - m) * (r[j] - m)
		}
		sd := math.Sqrt(v / float64(len(x)))
		for _, r := range x {
			r[j] -= m
			if sd > 0 {
				r[j] /= sd
			}
		}
	}
}

func dist2(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += (a[i] - b[i]) * (a[i] - b[i])
	}
	return s
}

// kmeans returns the assignment of the run with the least sum of squared
// distances of kmeansStarts runs of k-means from k-means++ seeds.
func kmeans(x [][]float64, k int, rng *rand.Rand) []int {
	var best []int
	bestCost := math.Inf(1)
	for range kmeansStarts {
		a, cost := kmeansRun(x, k, rng)
		if cost < bestCost {
			best, bestCost = a, cost
		}
	}
	return best
}

func kmeansRun(x [][]float64, k int, rng *rand.Rand) ([]int, float64) {
	n, d := len(x), len(x[0])
	centres := [][]float64{slices.Clone(x[rng.IntN(n)])}
	near := make([]float64, n)
	for len(centres) < k {
		var sum float64
		for i, p := range x {
			near[i] = math.Inf(1)
			for _, c := range centres {
				near[i] = min(near[i], dist2(p, c))
			}
			sum += near[i]
		}
		u := rng.Float64() * sum
		pick := n - 1
		for i, v := range near {
			if u -= v; u < 0 {
				pick = i
				break
			}
		}
		centres = append(centres, slices.Clone(x[pick]))
	}
	assign := make([]int, n)
	for i := range assign {
		assign[i] = -1
	}
	for range kmeansSteps {
		changed := false
		for i, p := range x {
			b := 0
			for j, c := range centres {
				if dist2(p, c) < dist2(p, centres[b]) {
					b = j
				}
			}
			if b != assign[i] {
				assign[i], changed = b, true
			}
		}
		if !changed {
			break
		}
		count := make([]int, k)
		for j := range centres {
			clear(centres[j])
		}
		for i, p := range x {
			count[assign[i]]++
			for m := range d {
				centres[assign[i]][m] += p[m]
			}
		}
		for j := range centres {
			for m := range d {
				centres[j][m] /= float64(max(1, count[j]))
			}
		}
		for j := range centres {
			if count[j] > 0 {
				continue
			}
			// An empty class takes the point farthest from its centre.
			far, fd := 0, -1.0
			for i, p := range x {
				if v := dist2(p, centres[assign[i]]); v > fd {
					far, fd = i, v
				}
			}
			copy(centres[j], x[far])
			assign[far] = j
		}
	}
	var cost float64
	for i, p := range x {
		cost += dist2(p, centres[assign[i]])
	}
	return assign, cost
}

// silhouette returns the mean silhouette of the assignment: for each
// point, the mean distance to the nearest other class less the mean
// distance to its own class, over the larger of the two. A point alone in
// its class counts 0.
func silhouette(x [][]float64, assign []int, k int) float64 {
	var total float64
	for i, p := range x {
		sum := make([]float64, k)
		count := make([]int, k)
		for j, q := range x {
			if j != i {
				sum[assign[j]] += math.Sqrt(dist2(p, q))
				count[assign[j]]++
			}
		}
		own := assign[i]
		if count[own] == 0 {
			continue
		}
		a := sum[own] / float64(count[own])
		b := math.Inf(1)
		for c := range k {
			if c != own && count[c] > 0 {
				b = min(b, sum[c]/float64(count[c]))
			}
		}
		if m := max(a, b); m > 0 && !math.IsInf(b, 1) {
			total += (b - a) / m
		}
	}
	return total / float64(len(x))
}
