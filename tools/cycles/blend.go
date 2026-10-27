package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/cmplx"

	"github.com/oval-tutu/snarl/internal/cycles"
)

// runBlend prints, for each pair of neighbouring RPM points of each steady
// throttle row, the level of the average cycle halfway between them in each
// residual band, as the playback blends the two points, relative to the
// mean power of the two points: the evidence for the blend between
// neighbouring RPM tables.
func runBlend(args []string, stdout io.Writer) error {
	fs := newFlags("blend")
	tablesPath := fs.String("tables", "", "tables file (required)")
	ignitionOff := fs.Bool("ignition-off", false, "take the ignition-off rows")
	channel := fs.Int("channel", 0, "exhaust channel")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tablesPath == "" || fs.NArg() != 0 {
		fs.Usage()
		return errors.New("need --tables")
	}
	t, err := cycles.ReadFile(*tablesPath)
	if err != nil {
		return err
	}
	if *channel < 0 || *channel >= len(t.Channels) {
		return fmt.Errorf("channel %d of %d", *channel, len(t.Channels))
	}
	edges := cycles.BandEdges(t.Rate)
	fmt.Fprintf(stdout, "%s, channel %d: the average cycle halfway between neighbouring RPM points, the linear blend of the playback, over the mean power of the two points per residual band (dB)\n\n", t.Engine, *channel)
	fmt.Fprint(stdout, "| Throttle | From (rpm) | To (rpm) |")
	for b := range cycles.BandCount {
		fmt.Fprintf(stdout, " %.0f to %.0f Hz |", edges[b], edges[b+1])
	}
	fmt.Fprint(stdout, "\n| --- | --- | --- |")
	for range cycles.BandCount {
		fmt.Fprint(stdout, " --- |")
	}
	fmt.Fprintln(stdout)
	for i := 1; i < len(t.Points); i++ {
		a, b := &t.Points[i-1], &t.Points[i]
		if !sameRow(a, b) || a.Ignition == *ignitionOff {
			continue
		}
		fmt.Fprintf(stdout, "| %g | %g | %g |", a.Throttle, a.RPM, b.RPM)
		for _, loss := range blendLoss(a.Wave[*channel], b.Wave[*channel], (a.MeasuredRPM+b.MeasuredRPM)/2, edges) {
			fmt.Fprintf(stdout, " %.1f |", loss)
		}
		fmt.Fprintln(stdout)
	}
	return nil
}

// blendLoss returns per band the power of the half-and-half blend of two
// series in the layout of cycles.Point.Wave over the mean power of the two,
// in dB, with the harmonics in each band at the RPM. A band with no
// harmonic in both series gives NaN, and the harmonics above the last edge
// count in no band.
func blendLoss(a, b []float64, rpm float64, edges []float64) []float64 {
	n := min(len(a), len(b))
	mid := make([]float64, len(edges)-1)
	mean := make([]float64, len(edges)-1)
	for h := 1; 2*h < n; h++ {
		f := float64(h) * rpm / 120
		if f >= edges[len(edges)-1] {
			// Above the Nyquist frequency of the steps.
			continue
		}
		k := 0
		for k+1 < len(edges)-1 && f >= edges[k+1] {
			k++
		}
		ca, cb := complex(a[2*h-1], a[2*h]), complex(b[2*h-1], b[2*h])
		m := cmplx.Abs((ca + cb) / 2)
		mid[k] += m * m
		mean[k] += (real(ca)*real(ca) + imag(ca)*imag(ca) + real(cb)*real(cb) + imag(cb)*imag(cb)) / 2
	}
	out := make([]float64, len(mid))
	for k := range out {
		out[k] = math.NaN()
		if mean[k] > 0 {
			out[k] = db(mid[k]) - db(mean[k])
		}
	}
	return out
}
