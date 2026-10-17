package analysis

import "math"

// Resample converts x from one sample rate to another with a Blackman
// windowed-sinc interpolator. The cut-off is 0.95 of the lower Nyquist
// frequency, which removes aliasing when the rate goes down.
func Resample(x []float64, from, to int) []float64 {
	if from == to {
		return append([]float64(nil), x...)
	}
	const halfTaps = 32
	ratio := float64(from) / float64(to)
	cutoff := 0.95 * math.Min(1, 1/ratio)
	n := int(float64(len(x)) / ratio)
	y := make([]float64, n)
	for i := range y {
		pos := float64(i) * ratio
		centre := int(math.Floor(pos))
		var sum float64
		for j := centre - halfTaps + 1; j <= centre+halfTaps; j++ {
			if j < 0 || j >= len(x) {
				continue
			}
			d := pos - float64(j)
			w := 0.42 + 0.5*math.Cos(math.Pi*d/halfTaps) + 0.08*math.Cos(2*math.Pi*d/halfTaps)
			sum += x[j] * cutoff * sinc(cutoff*d) * w
		}
		y[i] = sum
	}
	return y
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}
