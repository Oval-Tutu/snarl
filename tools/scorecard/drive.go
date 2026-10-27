package main

import (
	"fmt"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
)

// The drive scenarios, drive.scn, which snarl-harness -scenario writes from
// the scripted drive of the harness and just ref-drive renders, change the
// RPM and the throttle all the time. Their windows are --drive-window
// seconds long, one after the other from the start, and each has the
// measures of analysis.AlignTrack along the RPM of the scenario
// (docs/research/drive-scorecard.md).
const (
	driveName = "drive"
	// driveShiftRevs is the RPM rate, in shares of the range from idle to
	// the limiter per second, above which a segment is a gear change: the
	// harness drive climbs by at most about 0.72 of the range per second,
	// and a gear change moves the RPM by a quarter of the range or more in
	// one tick.
	driveShiftRevs = 3
	// driveLimiter is the share of the limiter RPM from which a window is
	// at the limiter, where the snarl limiter bounces.
	driveLimiter = 0.998
)

// The marks of a drive window.
const (
	markShift    = "shift"
	markLift     = "lift"
	markTipIn    = "tip-in"
	markLimiter  = "limiter"
	markNoCycles = "no cycles"
)

// driveInfo is the scenario state of a drive window.
type driveInfo struct {
	rpm0, rpm1 float64 // The RPM at the start and the end.
	lo, hi     float64 // The lowest and the highest RPM.
	th0, th1   float64 // The throttle at the start and the end.
	marks      []string
}

// rate returns the mean RPM rate of the window in rpm per second.
func (d *driveInfo) rate(w analysis.Window) float64 {
	return (d.rpm1 - d.rpm0) / (w.End - w.Start)
}

// driveWindows returns the windows of length seconds over the first
// duration seconds of sc, with the scenario state of each, for an engine
// with the idle and limiter RPM.
func driveWindows(sc *scenario.Scenario, length, duration, idle, limiter float64) ([]analysis.Window, []driveInfo) {
	var ws []analysis.Window
	var ds []driveInfo
	for i := range int(duration/length + 1e-9) {
		t0, t1 := float64(i)*length, float64(i+1)*length
		d := driveInfo{lo: math.Inf(1), hi: math.Inf(-1)}
		d.rpm0, _ = sc.At(t0)
		// The throttle just before the window, so that a change at the
		// start of the window counts.
		_, d.th0 = sc.At(max(0, t0-1e-6))
		d.th1 = d.th0
		shift := false
		start := 0.0
		for _, seg := range sc.Segments {
			s0, s1 := start, start+seg.Seconds
			start = s1
			// The segment times are sums, so a segment that meets the
			// window at its edge can overlap it by a rounding error.
			if s1 <= t0+1e-9 || s0 >= t1-1e-9 {
				continue
			}
			at := func(t float64) float64 { return seg.RPM0 + (seg.RPM1-seg.RPM0)*(t-s0)/seg.Seconds }
			a, b := at(max(s0, t0)), at(min(s1, t1))
			d.lo, d.hi = min(d.lo, a, b), max(d.hi, a, b)
			d.rpm1, d.th1 = b, seg.Throttle
			shift = shift || math.Abs(seg.RPM1-seg.RPM0)/seg.Seconds > driveShiftRevs*(limiter-idle)
		}
		if shift {
			d.marks = append(d.marks, markShift)
		}
		switch {
		case d.th1 < d.th0:
			d.marks = append(d.marks, markLift)
		case d.th1 > d.th0:
			d.marks = append(d.marks, markTipIn)
		}
		if d.hi >= driveLimiter*limiter {
			d.marks = append(d.marks, markLimiter)
		}
		w := analysis.Window{
			Kind: driveName, Start: t0, End: t1, RPMStart: d.rpm0, RPMEnd: d.rpm1, Throttle: d.th0,
			Label: fmt.Sprintf("%.2f-%.2f s %.0f-%.0f rpm", t0, t1, d.rpm0, d.rpm1),
		}
		ws, ds = append(ws, w), append(ds, d)
	}
	return ws, ds
}

// markBeyondSelf follows the mark of a miss that is also beyond the
// largest value of the second engine-sim render over the drive.
const markBeyondSelf = "!"

// driveMeasures returns the size of the worst band above 2 kHz, the
// tonality distance and the gap distance of p.
func driveMeasures(p *point) [3]float64 {
	hiss, tonality, _, gap, _ := measures(p)
	return [3]float64{math.Abs(hiss), tonality, gap}
}

// driveMisses returns the marks of the hiss measures of a drive window
// that are beyond their floors, each with markBeyondSelf when it is also
// beyond the value of self.
func driveMisses(p *point, self [3]float64) string {
	floors := [3]float64{analysis.AlignFloorHissDB, analysis.AlignFloorTonalityDB, analysis.AlignFloorGapDB}
	var m string
	for i, v := range driveMeasures(p) {
		if v > floors[i] {
			m += []string{markHiss, markTonality, markGap}[i]
			if v > self[i] {
				m += markBeyondSelf
			}
		}
	}
	return m
}

// driveSelf returns the largest value of each of driveMeasures over the
// windows ps of the second engine-sim render, or infinity with none.
func driveSelf(ps []*point) [3]float64 {
	self := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	if len(ps) == 0 {
		return self
	}
	self = [3]float64{}
	for _, p := range ps {
		for i, v := range driveMeasures(p) {
			if !math.IsNaN(v) {
				self[i] = max(self[i], v)
			}
		}
	}
	return self
}

// writeDrive writes, for each engine with a drive, the number of windows
// that miss each hiss floor in the render and in the second engine-sim
// render, and the table of the windows that miss in the render, with the
// RPM, the RPM rate and the throttle of each, and the same measures of the
// second engine-sim render.
func writeDrive(w io.Writer, engines []string, renders, calibs []point) {
	for _, e := range engines {
		pick := func(ps []point) []*point {
			var out []*point
			for i := range ps {
				if ps[i].engine == e && ps[i].drive != nil {
					out = append(out, &ps[i])
				}
			}
			return out
		}
		rs, cs := pick(renders), pick(calibs)
		if len(rs) == 0 {
			continue
		}
		calib := map[float64]*point{}
		for _, c := range cs {
			calib[c.win.Start] = c
		}
		self := driveSelf(cs)
		count := func(ps []*point) (cycles int, n map[string]int) {
			n = map[string]int{}
			for _, p := range ps {
				if p.r.HissCycle != nil {
					cycles++
				}
				miss := driveMisses(p, self)
				for _, m := range []string{markHiss, markTonality, markGap} {
					if strings.Contains(miss, m) {
						n[m]++
					}
					if strings.Contains(miss, m+markBeyondSelf) {
						n[m+markBeyondSelf]++
					}
				}
			}
			return cycles, n
		}
		fmt.Fprintf(w, "\n### %s %s\n\n", e, driveName)
		rc, rn := count(rs)
		fmt.Fprintf(w, "snarl: %d windows of %.2f s, %d with cycle measures; band level beyond %.1f dB in %d, tonality beyond %.1f dB in %d, gap beyond %.1f dB in %d.\n",
			len(rs), rs[0].win.End-rs[0].win.Start, rc, analysis.AlignFloorHissDB, rn[markHiss], analysis.AlignFloorTonalityDB, rn[markTonality], analysis.AlignFloorGapDB, rn[markGap])
		if len(cs) > 0 {
			cc, cn := count(cs)
			fmt.Fprintf(w, "engine-sim against itself: %d windows, %d with cycle measures; band level in %d, tonality in %d, gap in %d; "+
				"largest band %.1f dB, tonality %.1f dB, gap %.1f dB.\n", len(cs), cc, cn[markHiss], cn[markTonality], cn[markGap], self[0], self[1], self[2])
			fmt.Fprintf(w, "snarl beyond those largest values (%s): band level in %d, tonality in %d, gap in %d.\n",
				markBeyondSelf, rn[markHiss+markBeyondSelf], rn[markTonality+markBeyondSelf], rn[markGap+markBeyondSelf])
		}
		fmt.Fprintln(w)
		writeDriveClasses(w, rs, cs)
		fmt.Fprintln(w, "| Time (s) | RPM | Rate (rpm/s) | Throttle | Marks | Score | Band (dB at Hz) | Tonality (dB) | Gap (dB) | Misses | engine-sim self: band, tonality, gap |")
		fmt.Fprintln(w, "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
		missed := false
		for _, p := range rs {
			miss := driveMisses(p, self)
			if miss == "" {
				continue
			}
			missed = true
			fmt.Fprintf(w, "| %s | %s | %+.0f | %s | %s | %.1f | %s | %s | %s | %s | %s |\n",
				spanf(p.win.Start, p.win.End, "%.2f"), spanf(p.drive.rpm0, p.drive.rpm1, "%.0f"), p.drive.rate(p.win),
				spanf(p.drive.th0, p.drive.th1, "%.2g"), strings.Join(driveMarks(p), ", "), p.r.Score,
				bandf(p), signedf(p, false), signedf(p, true), miss, selff(calib[p.win.Start]))
		}
		if !missed {
			fmt.Fprintln(w, "| no misses | | | | | | | | | | |")
		}
		fmt.Fprintf(w, "\nTonality and gap: mean difference, snarl less engine-sim (negative tonality is noisier, positive gap is more noise between the firings), with the distance in brackets. "+
			"Misses: %s band level, %s tonality, %s gap beyond the floor, each with %s when it is also beyond the largest value of engine-sim against itself.\n",
			markHiss, markTonality, markGap, markBeyondSelf)
	}
}

// The classes of the drive windows, in the order of the class table.
var driveClasses = []string{"idle", "full-throttle pull", "part throttle", "coast", markLift, markTipIn, markShift, markLimiter}

// driveClass returns the class of a drive window: a gear change, a lift,
// a tip-in or the limiter first, else by the throttle and the RPM.
func driveClass(d *driveInfo) string {
	for _, m := range []string{markShift, markLift, markTipIn, markLimiter} {
		if slices.Contains(d.marks, m) {
			return m
		}
	}
	switch {
	case d.th0 == 0 && d.hi == d.lo:
		return "idle"
	case d.th0 == 0:
		return "coast"
	case d.th0 >= 1 && d.rpm1 > d.rpm0:
		return "full-throttle pull"
	}
	return "part throttle"
}

// writeDriveClasses writes the hiss measures of the drive windows rs of the
// render and cs of the second engine-sim render by class: the median and
// the 90th percentile of the size of the worst band above 2 kHz, the band
// with the largest mean difference over the windows of the class, and the
// mean tonality and gap differences. A short window gives a noisy worst
// band, so the mean difference of each band shows a lasting excess.
func writeDriveClasses(w io.Writer, rs, cs []*point) {
	type stats struct {
		n, tonalityN       int
		med, p90, bias, at float64
		tonality, gap      float64
	}
	of := func(ps []*point, class string) stats {
		var s stats
		var worst []float64
		sum, num := map[float64]float64{}, map[float64]int{}
		for _, p := range ps {
			if driveClass(p.drive) != class {
				continue
			}
			s.n++
			if !math.IsNaN(p.r.Hiss.WorstDB) {
				worst = append(worst, math.Abs(p.r.Hiss.WorstDB))
			}
			for _, b := range p.r.Hiss.Bands {
				sum[b.CentreHz] += b.LevelB - b.LevelA
				num[b.CentreHz]++
			}
			if _, _, ts, _, gs := measures(p); !math.IsNaN(ts) {
				s.tonality += ts
				s.gap += gs
				s.tonalityN++
			}
		}
		s.med, s.p90, s.bias, s.at = math.NaN(), math.NaN(), math.NaN(), math.NaN()
		if len(worst) > 0 {
			slices.Sort(worst)
			s.med, s.p90 = worst[len(worst)/2], percentile90(worst)
		}
		for hz, v := range sum {
			if m := v / float64(num[hz]); math.IsNaN(s.bias) || math.Abs(m) > math.Abs(s.bias) {
				s.bias, s.at = m, hz
			}
		}
		if s.tonalityN > 0 {
			s.tonality /= float64(s.tonalityN)
			s.gap /= float64(s.tonalityN)
		} else {
			s.tonality, s.gap = math.NaN(), math.NaN()
		}
		return s
	}
	fmt.Fprintln(w, "| Class | Windows | Worst band median / p90 (dB) | engine-sim self | Largest mean band difference (dB at Hz) | engine-sim self | Mean tonality / gap difference (dB) | engine-sim self |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- | --- | --- | --- |")
	for _, class := range driveClasses {
		r, c := of(rs, class), of(cs, class)
		if r.n == 0 {
			continue
		}
		both := func(s stats, f func(stats) string) string {
			if s.n == 0 {
				return "-"
			}
			return f(s)
		}
		spread := func(s stats) string { return cellf("%.1f", s.med) + " / " + cellf("%.1f", s.p90) }
		bias := func(s stats) string {
			if math.IsNaN(s.bias) {
				return "-"
			}
			return fmt.Sprintf("%+.1f at %.0f", s.bias, s.at)
		}
		hiss := func(s stats) string { return cellf("%+.1f", s.tonality) + " / " + cellf("%+.1f", s.gap) }
		fmt.Fprintf(w, "| %s | %d | %s | %s | %s | %s | %s | %s |\n", class, r.n, spread(r), both(c, spread), bias(r), both(c, bias), hiss(r), both(c, hiss))
	}
	fmt.Fprintln(w)
}

// percentile90 returns the 90th percentile of the sorted values x.
func percentile90(x []float64) float64 {
	return x[min(len(x)-1, max(0, int(math.Ceil(0.9*float64(len(x))))-1))]
}

// driveMarks returns the marks of the drive window p, with markNoCycles
// when it has no cycle measures.
func driveMarks(p *point) []string {
	m := slices.Clone(p.drive.marks)
	if p.r.HissCycle == nil {
		m = append(m, markNoCycles)
	}
	return m
}

// spanf formats a and b with f, or a alone when the two are equal.
func spanf(a, b float64, f string) string {
	if fmt.Sprintf(f, a) == fmt.Sprintf(f, b) {
		return fmt.Sprintf(f, a)
	}
	return fmt.Sprintf(f+"-"+f, a, b)
}

// bandf formats the worst band above 2 kHz of p.
func bandf(p *point) string {
	if math.IsNaN(p.r.Hiss.WorstDB) {
		return "-"
	}
	return fmt.Sprintf("%+.1f at %.0f", p.r.Hiss.WorstDB, p.r.Hiss.WorstHz)
}

// signedf formats the mean difference and the distance of the tonality,
// or with gap true of the gap, of p.
func signedf(p *point, gap bool) string {
	_, t, ts, g, gs := measures(p)
	if gap {
		t, ts = g, gs
	}
	if math.IsNaN(t) {
		return "-"
	}
	return fmt.Sprintf("%+.1f (%.1f)", ts, t)
}

// selff formats the band level, the tonality and the gap of the second
// engine-sim render p, or a dash with none.
func selff(p *point) string {
	if p == nil {
		return "-"
	}
	_, t, _, g, _ := measures(p)
	return fmt.Sprintf("%s, %s, %s", bandf(p), cellf("%.1f", t), cellf("%.1f", g))
}
