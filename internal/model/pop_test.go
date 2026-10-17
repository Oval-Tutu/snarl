package model

import (
	"math"
	"testing"
)

// TestPopShape checks one isolated pop against the measured shape: most
// energy from 120 to 500 Hz, a 2 to 8 kHz crack of a few per cent, and a
// decay to -20 dB in 15 to 60 ms.
func TestPopShape(t *testing.T) {
	s := DefaultSpec()
	s.Name = "p"
	s.IdleRPM, s.RedlineRPM = 800, 6800
	s.Cylinders = []CylinderSpec{{CrankDeg: 0, Bank: 0, PrimaryM: 0.4}}
	s.Paths = []PathSpec{{CollectorM: 1.3, Reflection: -0.4, DampingHz: 3000, PipeRadiusM: 0.025, OutletM: 0.025, WallLossDB: 2, WallLossHz: 2500, ChamberM: 0.4, ChamberArea: 8, Gain: 1}}
	s.Exhaust.OutletDirect = 0.3
	s.Pops.Density = 0.4
	for _, tier1 := range []bool{false, true} {
		y, err := New(&s, rate, tier1)
		if err != nil {
			t.Fatal(err)
		}
		y.voiceGain = 0
		y.pops.heat = 1
		buf := make([]float32, 64)
		c := Control{Running: true}
		for range 200 {
			y.Render(buf, c)
		}
		y.pops.fire(&y.noise, 1, 1)
		x := make([]float32, rate*3/10)
		y.Render(x, c)
		var e [4]float64
		edges := []float64{20, 120, 500, 2000, 8000}
		for f := 20.0; f < 8000; f += 5 {
			var re, im float64
			for i, v := range x {
				w := 2 * math.Pi * f * float64(i) / rate
				re += float64(v) * math.Cos(w)
				im += float64(v) * math.Sin(w)
			}
			for b := range 4 {
				if f >= edges[b] && f < edges[b+1] {
					e[b] += re*re + im*im
				}
			}
		}
		tot := e[0] + e[1] + e[2] + e[3]
		// Envelope: 1 ms RMS, peak and time to -20 dB.
		var env []float64
		for i := 0; i+48 <= len(x); i += 48 {
			var s float64
			for _, v := range x[i : i+48] {
				s += float64(v * v)
			}
			env = append(env, math.Sqrt(s/48))
		}
		pk, pi := 0.0, 0
		for i, v := range env {
			if v > pk {
				pk, pi = v, i
			}
		}
		d := 0
		for i := len(env) - 1; i > pi; i-- {
			if env[i] > pk/10 {
				d = i - pi
				break
			}
		}
		body, crack := 100*e[1]/tot, 100*e[3]/tot
		if body < 45 || crack < 0.3 || crack > 3 || d < 15 || d > 60 {
			t.Errorf("tier1 %v: 120 to 500 Hz %.0f%%, 2 to 8 kHz %.1f%%, -20 dB after %d ms",
				tier1, body, crack, d)
		}
	}
}
