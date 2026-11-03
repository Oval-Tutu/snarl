package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/crackle"
	"github.com/oval-tutu/snarl/internal/wav"
)

const (
	// decayEnvS is the centred RMS window of the decay envelope.
	decayEnvS = 0.005
	// decayHoldS is how long the envelope must stay below a level for the
	// time to that level, so that a dip between two body cycles does not
	// count.
	decayHoldS = 0.01
	// decayPeakS is the half width of the peak window, and decaySearchS
	// the span after the shock peak in which each band takes its own peak.
	decayPeakS   = 0.0025
	decaySearchS = 0.005
	// The decay fit runs on points every decayStepS from the peak to the
	// fall of the last of decayDrops, for at most decayFitS. A point below
	// decayFloorDB counts as decayFloorDB.
	decayStepS   = 0.0005
	decayFitS    = 0.5
	decayFloorDB = -60
	// A decay is two-stage when two lines leave at most twoStageSSE of the
	// squared error of one line, and the second line falls at most half as
	// fast as the first.
	twoStageSSE = 0.5
	// The end event is the last shock within strongDB of the strongest
	// shock in the last endLastS of the active span.
	endLastS = 1
	// An end is cut when the active span reaches the last edgeS of the
	// sound, or when the envelope of the last frameS of the active span is
	// within cutDB of the peak of the end event.
	cutDB = 30
	// padS is the silence after a model train and an impulse response.
	padS = 0.3
	// tailSeeds is the count of model trains of each kind, from checkSeed.
	tailSeeds = 60
	// irMaxTaps and irMinAmp are the trim of engine-sim: the response ends
	// after its last sample above 100 of 32767, at most 10000 taps.
	irMaxTaps = 10000
	irMinAmp  = 100.0 / 32767
)

// decayDrops are the falls from the peak, in dB, and decayWindows the
// spans after the peak of the tail levels, in seconds.
var (
	decayDrops   = [...]float64{10, 20, 30, 40}
	decayWindows = [...][2]float64{{0.02, 0.1}, {0.1, 0.3}}
)

// decayBand is one band of the decay measures. A band with hi 0 is the
// full band.
type decayBand struct {
	name   string
	lo, hi float64
}

var decayBands = []decayBand{
	{"full band", 0, 0},
	{"below 200 Hz", bodyLoHz, 200},
	{"200 Hz to 2 kHz", 200, 2000},
	{"above 2 kHz", 2000, shockLPHz},
}

// decay is the decay of one band after one peak.
type decay struct {
	// DropMS is the time from the peak of the envelope to each fall of
	// decayDrops, NaN when the envelope does not stay below it before the
	// end of the window.
	DropMS [len(decayDrops)]float64
	// WinDB is the mean power in each of decayWindows over the mean power
	// of the peak window, NaN when the sound ends first.
	WinDB [len(decayWindows)]float64
	// Slope is the slope of one line through the envelope in dB from the
	// peak, in dB per millisecond. Fast and Slow are the slopes of the
	// best two lines, which meet at BreakMS, at BreakDB below the peak.
	// All are NaN when the fit has fewer than 6 points.
	Slope, Fast, Slow, BreakMS, BreakDB float64
	TwoStage                            bool
}

// decayEnvs returns the envelope of x in each of decayBands.
func decayEnvs(x []float64, rate int) [][]float64 {
	env := make([][]float64, len(decayBands))
	for b, d := range decayBands {
		y := x
		if d.hi > 0 {
			y = analysis.BandPass(x, rate, d.lo, d.hi)
		}
		env[b] = rmsEnv(y, rate, decayEnvS)
	}
	return env
}

// measureDecay measures the decay of the envelope env from its peak near
// the sample at. The drops and the fit stop at end, and the tail levels
// at limit.
func measureDecay(env []float64, rate, at, end, limit int) decay {
	nan := math.NaN()
	d := decay{Slope: nan, Fast: nan, Slow: nan, BreakMS: nan, BreakDB: nan}
	for i := range d.DropMS {
		d.DropMS[i] = nan
	}
	for i := range d.WinDB {
		d.WinDB[i] = nan
	}
	fr := float64(rate)
	lo, hi := max(0, at-int(decayPeakS*fr)), min(end, at+int(decaySearchS*fr)+1)
	if lo >= hi {
		return d
	}
	q := lo
	for i := lo; i < hi; i++ {
		if env[i] > env[q] {
			q = i
		}
	}
	ref := env[q]
	if ref == 0 {
		return d
	}
	hold := int(decayHoldS * fr)
	for k, drop := range decayDrops {
		thr := ref * dbAmp(-drop)
		run := 0
		for i := q; i < end; i++ {
			if env[i] < thr {
				run++
			} else {
				run = 0
			}
			// At the end of the sound, a fall that holds to the last
			// sample counts.
			if run >= hold || (run > 0 && i == len(env)-1) {
				d.DropMS[k] = float64(i-run+1-q) / fr * 1000
				break
			}
		}
	}
	pw := int(decayPeakS * fr)
	peak := meanSquare(env[max(0, q-pw):min(len(env), q+pw+1)])
	for k, w := range decayWindows {
		if a, b := q+int(w[0]*fr), q+int(w[1]*fr); b <= limit {
			d.WinDB[k] = powDB(meanSquare(env[a:b]) / peak)
		}
	}
	stop := min(end, q+int(decayFitS*fr))
	if t := d.DropMS[len(d.DropMS)-1]; !math.IsNaN(t) {
		stop = min(stop, q+int(t/1000*fr)+1)
	}
	var ts, ys []float64
	for i := q; i < stop; i += max(1, int(decayStepS*fr)) {
		ts = append(ts, float64(i-q)/fr*1000)
		ys = append(ys, max(ampDB(env[i]/ref), decayFloorDB))
	}
	if len(ts) < 6 {
		return d
	}
	var one float64
	d.Slope, one = lineSSE(ts, ys)
	best := math.Inf(1)
	for k := 3; k <= len(ts)-3; k++ {
		sa, ea := lineSSE(ts[:k], ys[:k])
		sb, eb := lineSSE(ts[k-1:], ys[k-1:])
		if ea+eb < best {
			best = ea + eb
			d.Fast, d.Slow, d.BreakMS, d.BreakDB = sa, sb, ts[k-1], ys[k-1]
		}
	}
	d.TwoStage = one > 0 && best <= twoStageSSE*one && d.Slow > d.Fast/2
	return d
}

// lineSSE fits a straight line to the points (t, y) and returns its slope
// and the sum of the squared residuals.
func lineSSE(ts, y []float64) (slope, sse float64) {
	slope, _ = line(ts, y)
	var mt, my float64
	for i, v := range y {
		mt += ts[i]
		my += v
	}
	mt /= float64(len(y))
	my /= float64(len(y))
	for i, v := range y {
		r := v - my - slope*(ts[i]-mt)
		sse += r * r
	}
	return slope, sse
}

// tailSeg is one sound for the decay measures: a clip, a model train or
// one pop, mono.
type tailSeg struct {
	env    [][]float64
	shocks []shock // Onsets and peaks in samples from the start of the sound.
	// hi is the end of the active span, and end the end of the windows:
	// the first fragment after the active span, or the end of the sound.
	hi, end, n int
}

// newTailSeg finds the active span and the shocks of x, or returns nil
// when x has no shock.
func newTailSeg(x []float64, rate int) *tailSeg {
	active, late := activeSpan(&wav.Audio{SampleRate: rate, Channels: [][]float64{x}}, dbAmp(defaultFloorDB))
	if len(active) == 0 {
		return nil
	}
	lo, hi := active[0].lo, active[len(active)-1].hi
	if hi-lo < int(shockWinS*float64(rate)) {
		return nil
	}
	hp := analysis.BandPass(x, rate, shockHPHz, shockLPHz)
	sh := findShocks(x[lo:hi], hp[lo:hi], rate)
	if len(sh) == 0 {
		return nil
	}
	for k := range sh {
		sh[k].at += lo
		sh[k].pk += lo
	}
	end := len(x)
	if len(late) > 0 {
		end = late[0].lo
	}
	return &tailSeg{env: decayEnvs(x, rate), shocks: sh, hi: hi, end: end, n: len(x)}
}

// strong returns the decay in each band after each strong shock, up to
// the next strong shock. The tail levels run on to the end of the sound.
func (s *tailSeg) strong(rate int) [][]decay {
	var idx []int
	for k, sh := range s.shocks {
		if sh.Strong {
			idx = append(idx, k)
		}
	}
	var out [][]decay
	for j, k := range idx {
		end := s.end
		if j+1 < len(idx) {
			end = s.shocks[idx[j+1]].at
		}
		out = append(out, s.bands(rate, s.shocks[k].pk, end))
	}
	return out
}

func (s *tailSeg) bands(rate, at, end int) []decay {
	row := make([]decay, len(decayBands))
	for b := range decayBands {
		row[b] = measureDecay(s.env[b], rate, at, end, s.end)
	}
	return row
}

// Cut kinds of the end of a sound.
const (
	uncut = iota
	cutAtEnd
	cutAbrupt
)

// endEvent returns the decay in each band after the end event, to the
// end of the sound, and the cut kind of the end.
func (s *tailSeg) endEvent(rate int) ([]decay, int) {
	fr := float64(rate)
	from := s.hi - int(endLastS*fr)
	top := math.Inf(-1)
	for _, sh := range s.shocks {
		if sh.at >= from {
			top = max(top, sh.RelDB)
		}
	}
	last := len(s.shocks) - 1
	for k, sh := range s.shocks {
		if sh.at >= from && sh.RelDB >= top-strongDB {
			last = k
		}
	}
	if s.hi >= s.n-int(edgeS*fr) {
		return nil, cutAtEnd
	}
	row := s.bands(rate, s.shocks[last].pk, s.end)
	full := s.env[0]
	pk := s.shocks[last].pk
	ref := 0.0
	for i := max(0, pk-int(decayPeakS*fr)); i < min(s.n, pk+int(decaySearchS*fr)+1); i++ {
		ref = max(ref, full[i])
	}
	if f := int(frameS * fr); s.hi >= f && math.Sqrt(meanSquare(full[s.hi-f:s.hi])) > ref*dbAmp(-cutDB) {
		return nil, cutAbrupt
	}
	return row, uncut
}

// tailRow is the decay in each band of one event.
type tailRow struct {
	group, source, event string
	bands                []decay
}

// tailGroup is one set of events and its count of excluded cut ends.
type tailGroup struct {
	name, event   string
	rows          []tailRow
	atEnd, abrupt int
}

// tailCollector gathers the rows of all events into groups.
type tailCollector struct {
	mu     sync.Mutex
	groups map[string]*tailGroup
	order  []string
}

func (t *tailCollector) group(name, event string) *tailGroup {
	key := name + "|" + event
	g := t.groups[key]
	if g == nil {
		g = &tailGroup{name: name, event: event}
		t.groups[key] = g
		t.order = append(t.order, key)
	}
	return g
}

// add adds the strong events of s, when strong is set, and its end event
// to the groups of name, or counts the end as cut.
func (t *tailCollector) add(name, source string, s *tailSeg, rate int, strong bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if strong {
		g := t.group(name, "strong shocks")
		for _, r := range s.strong(rate) {
			g.rows = append(g.rows, tailRow{name, source, g.event, r})
		}
	}
	g := t.group(name, "end")
	switch r, cut := s.endEvent(rate); cut {
	case cutAtEnd:
		g.atEnd++
	case cutAbrupt:
		g.abrupt++
	default:
		g.rows = append(g.rows, tailRow{name, source, g.event, r})
	}
}

// tailInputs are the arguments of --pop-tail.
type tailInputs struct {
	short, long []string
	cases       []levelCase
}

// parseTailArgs sorts the arguments short=CLIP.wav, long=CLIP.wav and
// PROFILE=SCENARIO.
func parseTailArgs(args []string) (tailInputs, error) {
	var in tailInputs
	for _, a := range args {
		k, v, ok := strings.Cut(a, "=")
		switch {
		case !ok || k == "" || v == "":
			return in, fmt.Errorf("argument %q: want short=CLIP.wav, long=CLIP.wav or PROFILE=SCENARIO", a)
		case k == "short":
			in.short = append(in.short, v)
		case k == "long":
			in.long = append(in.long, v)
		default:
			in.cases = append(in.cases, levelCase{k, v})
		}
	}
	if len(in.short) == 0 {
		return in, errors.New("--pop-tail needs one or more short=CLIP.wav")
	}
	return in, nil
}

// popTail measures the decay after the peaks of the short clips, at the
// end of the short and the long clips, of the model voices of each kind,
// of the pops of each kind on each case, and of the impulse response of
// each case profile in irDir. It writes the summary to w as Markdown and
// the rows of each event to dir/events.csv (docs/research/pop-tail.md).
func popTail(w io.Writer, dir, profiles string, args []string) error {
	in, err := parseTailArgs(args)
	if err != nil {
		return err
	}
	t := &tailCollector{groups: map[string]*tailGroup{}}
	var errs []error
	var emu sync.Mutex
	fail := func(err error) {
		emu.Lock()
		errs = append(errs, err)
		emu.Unlock()
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	run := func(f func() error) {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := f(); err != nil {
				fail(err)
			}
		})
	}
	for _, set := range []struct {
		name   string
		paths  []string
		strong bool
	}{{"clips, short", in.short, true}, {"clips, long", in.long, false}} {
		for _, path := range set.paths {
			run(func() error {
				a, err := wav.ReadFile(path)
				if err != nil {
					return err
				}
				if s := newTailSeg(a.Mono(), a.SampleRate); s != nil {
					t.add(set.name, filepath.Base(path), s, a.SampleRate, set.strong)
				}
				return nil
			})
		}
	}
	for _, k := range []struct {
		name string
		opt  func(i int) crackle.Options
	}{
		{"crackle", func(int) crackle.Options { return crackle.Options{} }},
		// A boom has 1 + subs shocks, with 3 to 15 later sub-events, as
		// the boom shape of internal/core/pops.go.
		{"boom", func(i int) crackle.Options { return crackle.Options{Shocks: 4 + i%13, Short: true} }},
		{"pop and burble", func(int) crackle.Options { return crackle.Options{Single: true} }},
	} {
		bank := crackle.NewBank(crackle.Default(), modelRate, checkSeed)
		for i, seed := range seedRange(checkSeed, tailSeeds) {
			run(func() error {
				y := crackle.Render(bank, seed, k.opt(i))
				x := make([]float64, len(y)+int(padS*modelRate))
				for j, v := range y {
					x[j] = float64(v)
				}
				if s := newTailSeg(x, modelRate); s != nil {
					t.add("model, "+k.name, fmt.Sprintf("seed %d", seed), s, modelRate, true)
				}
				return nil
			})
		}
	}
	for _, c := range in.cases {
		for kind, name := range popKindNames {
			run(func() error {
				d, u, _, starts, err := kindRenders(c, kind, 1)
				if err != nil {
					return fmt.Errorf("%s, %s: %w", c.profile, name, err)
				}
				for j, s := range starts {
					s = popStart(d, s)
					end := min(len(d), s+popSpanS*levelRate)
					if j+1 < len(starts) {
						end = min(end, starts[j+1])
					}
					for _, p := range []struct {
						stage string
						x     []float64
					}{{"after", d}, {"before", u}} {
						if seg := newTailSeg(p.x[s:end], levelRate); seg != nil {
							t.add(fmt.Sprintf("engine %s, %s the limiter", name, p.stage), fmt.Sprintf("%s pop %d", c.profile, j+1), seg, levelRate, true)
						}
					}
				}
				return nil
			})
		}
	}
	wg.Wait()
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	irs, err := measureIRs(in.cases, profiles)
	if err != nil {
		return err
	}
	for _, g := range t.groups {
		slices.SortFunc(g.rows, func(a, b tailRow) int { return strings.Compare(a.source, b.source) })
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if err := writeTailCSV(filepath.Join(dir, "events.csv"), t, irs); err != nil {
		return err
	}
	return writeTailSummary(w, t, irs)
}

// irDecay is the decay of the impulse response of one profile.
type irDecay struct {
	profile, file string
	taps          int
	t20, t40      float64
	bands         []decay
}

// measureIRs measures the impulse response of the first channel of each
// case profile, trimmed as engine-sim trims it.
func measureIRs(cases []levelCase, profiles string) ([]irDecay, error) {
	var out []irDecay
	seen := map[string]bool{}
	for _, c := range cases {
		if seen[c.profile] {
			continue
		}
		seen[c.profile] = true
		name, err := irName(filepath.Join(profiles, c.profile+".toml"))
		if err != nil {
			return nil, err
		}
		a, err := wav.ReadFile(filepath.Join(profiles, "ir", name))
		if err != nil {
			return nil, err
		}
		ir := a.Channels[0]
		n := 0
		for i, v := range ir {
			if math.Abs(v) > irMinAmp {
				n = i + 1
			}
		}
		ir = ir[:min(n, irMaxTaps)]
		x := make([]float64, len(ir)+int(padS*float64(a.SampleRate)))
		copy(x, ir)
		pk := 0
		for i, v := range ir {
			if math.Abs(v) > math.Abs(ir[pk]) {
				pk = i
			}
		}
		env := decayEnvs(x, a.SampleRate)
		r := irDecay{profile: c.profile, file: name, taps: len(ir)}
		r.t20, r.t40 = analysis.EnergyDecay(ir, a.SampleRate)
		for b := range decayBands {
			r.bands = append(r.bands, measureDecay(env[b], a.SampleRate, pk, len(x), len(x)))
		}
		out = append(out, r)
	}
	return out, nil
}

// irName returns the impulse response of the first channel of the
// profile file.
func irName(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // The path is a profile in a user-supplied directory.
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(string(data)) {
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "impulse_response" {
			return strconv.Unquote(strings.TrimSpace(v))
		}
	}
	return "", fmt.Errorf("%s: no impulse_response", path)
}

// decayCols are the columns of a decay: the name and the value.
var decayCols = func() []struct {
	name   string
	digits int
	get    func(d decay) float64
} {
	type col = struct {
		name   string
		digits int
		get    func(d decay) float64
	}
	var cs []col
	for k, v := range decayDrops {
		cs = append(cs, col{fmt.Sprintf("-%g dB, ms", v), 1, func(d decay) float64 { return d.DropMS[k] }})
	}
	for k, w := range decayWindows {
		cs = append(cs, col{fmt.Sprintf("%g to %g ms, dB", w[0]*1000, w[1]*1000), 1, func(d decay) float64 { return d.WinDB[k] }})
	}
	return append(cs,
		col{"one line, dB/ms", 2, func(d decay) float64 { return d.Slope }},
		col{"fast, dB/ms", 2, func(d decay) float64 { return d.Fast }},
		col{"slow, dB/ms", 2, func(d decay) float64 { return d.Slow }},
		col{"break, ms", 1, func(d decay) float64 { return d.BreakMS }},
		col{"break, dB", 1, func(d decay) float64 { return d.BreakDB }},
	)
}()

// decayColumn returns the values of column c in band b over the rows.
func decayColumn(rows []tailRow, b int, get func(d decay) float64) []float64 {
	x := make([]float64, len(rows))
	for i, r := range rows {
		x[i] = get(r.bands[b])
	}
	return x
}

func writeTailCSV(path string, t *tailCollector, irs []irDecay) (err error) {
	f, err := os.Create(path) //nolint:gosec // The path is in a user-supplied output directory.
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	cw := csv.NewWriter(f)
	head := []string{"group", "source", "event", "band", "two_stage"}
	for _, c := range decayCols {
		head = append(head, c.name)
	}
	if err := cw.Write(head); err != nil {
		return err
	}
	write := func(group, source, event string, bands []decay) error {
		for b, d := range bands {
			rec := []string{group, source, event, decayBands[b].name, strconv.FormatBool(d.TwoStage)}
			for _, c := range decayCols {
				rec = append(rec, num(c.get(d), 3))
			}
			if err := cw.Write(rec); err != nil {
				return err
			}
		}
		return nil
	}
	for _, key := range t.order {
		for _, r := range t.groups[key].rows {
			if err := write(r.group, r.source, r.event, r.bands); err != nil {
				return err
			}
		}
	}
	for _, r := range irs {
		if err := write("impulse response", r.profile, r.file, r.bands); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// twoStageShare returns the share of the fitted decays of band b that are
// two-stage, in per cent.
func twoStageShare(rows []tailRow, b int) float64 {
	var n, two int
	for _, r := range rows {
		if d := r.bands[b]; !math.IsNaN(d.Slope) {
			n++
			two += b2i(d.TwoStage)
		}
	}
	if n == 0 {
		return math.NaN()
	}
	return 100 * float64(two) / float64(n)
}

// reached formats the median (p25 to p75) of x with the count of finite
// values when some have none.
func reached(x []float64, digits int) string {
	s := short(x, digits)
	if n := len(finite(x)); n < len(x) {
		s += fmt.Sprintf(" [%d]", n)
	}
	return s
}

func writeTailSummary(w io.Writer, t *tailCollector, irs []irDecay) error {
	var b strings.Builder
	b.WriteString("# The decay after the pops\n\n")
	fmt.Fprintf(&b, "The envelope is the RMS over a centred %g ms window. A time to a fall counts when the envelope stays below it for %g ms. The tail levels are the mean power over the window against the mean power over the %g ms around the peak. Each cell is the median (p25 to p75) over the events, with [n] the count of events with a value when some have none. Two-stage is the share of the fits with two lines that leave at most %g of the squared error of one line, and a second line at most half as steep.\n\n",
		decayEnvS*1000, decayHoldS*1000, 2*decayPeakS*1000, twoStageSSE)
	b.WriteString("## Events\n\n| Group | Event | Events | Cut to the last sample | Cut abruptly |\n| --- | --- | ---: | ---: | ---: |\n")
	for _, key := range t.order {
		g := t.groups[key]
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d |\n", g.name, g.event, len(g.rows), g.atEnd, g.abrupt)
	}
	for bi, band := range decayBands {
		fmt.Fprintf(&b, "\n## %s\n\n| Group | Event | Events | Two-stage, %% |", band.name)
		for _, c := range decayCols {
			fmt.Fprintf(&b, " %s |", c.name)
		}
		b.WriteString("\n| --- | --- | ---: | ---: |" + strings.Repeat(" --- |", len(decayCols)) + "\n")
		for _, key := range t.order {
			g := t.groups[key]
			fmt.Fprintf(&b, "| %s | %s | %d | %s |", g.name, g.event, len(g.rows), num(twoStageShare(g.rows, bi), 0))
			for _, c := range decayCols {
				fmt.Fprintf(&b, " %s |", reached(decayColumn(g.rows, bi, c.get), c.digits))
			}
			b.WriteString("\n")
		}
		for _, r := range irs {
			d := r.bands[bi]
			fmt.Fprintf(&b, "| impulse response %s (%s, %d taps, Schroeder -20 dB %s ms, -40 dB %s ms) | peak | 1 | %d |", r.profile, r.file, r.taps,
				num(r.t20*1000, 1), num(r.t40*1000, 1), 100*b2i(d.TwoStage))
			for _, c := range decayCols {
				fmt.Fprintf(&b, " %s |", num(c.get(d), c.digits))
			}
			b.WriteString("\n")
		}
	}
	writeTailDiff(&b, t)
	_, err := io.WriteString(w, b.String())
	return err
}

// writeTailDiff writes the model and engine medians less the medians of
// the short clips for the same event, with "in" when the median is inside
// the p25 to p75 of the clips.
func writeTailDiff(b *strings.Builder, t *tailCollector) {
	b.WriteString("\n## Difference to the short clips\n\nThe median of the group less the median of the short clips for the same event, with \"in\" when the median of the group is inside the p25 to p75 of the clips.\n")
	for bi, band := range decayBands {
		fmt.Fprintf(b, "\n### %s\n\n| Group | Event |", band.name)
		for _, c := range decayCols {
			fmt.Fprintf(b, " %s |", c.name)
		}
		b.WriteString("\n| --- | --- |" + strings.Repeat(" --- |", len(decayCols)) + "\n")
		for _, key := range t.order {
			g := t.groups[key]
			ref := t.groups["clips, short|"+g.event]
			if ref == nil || g == ref || strings.HasPrefix(g.name, "clips") {
				continue
			}
			fmt.Fprintf(b, "| %s | %s |", g.name, g.event)
			for _, c := range decayCols {
				cl, m := finite(decayColumn(ref.rows, bi, c.get)), finite(decayColumn(g.rows, bi, c.get))
				if len(cl) == 0 || len(m) == 0 {
					b.WriteString(" - |")
					continue
				}
				med := quantile(m, 0.5)
				mark := ""
				if med >= quantile(cl, 0.25) && med <= quantile(cl, 0.75) {
					mark = " in"
				}
				fmt.Fprintf(b, " %+.*f%s |", c.digits, med-quantile(cl, 0.5), mark)
			}
			b.WriteString("\n")
		}
	}
}
