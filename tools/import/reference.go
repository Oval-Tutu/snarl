package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/oval-tutu/snarl/internal/core"
	"github.com/oval-tutu/snarl/internal/cycles"
	"github.com/oval-tutu/snarl/internal/scenario"
)

// The reference set: five engine-sim engines converted from upstream with
// the source of every value recorded. Each value is copied from the
// upstream engine, measured from its own upstream reference renders, or a
// snarl default for a feature that upstream does not have. A value that the
// references cannot give is "not measured" and takes the core default.

// refEngine is one engine of the reference set: its id, the upstream file
// and the default directory of its reference renders.
type refEngine struct {
	id, file, ref string
}

var referenceEngines = []refEngine{
	{"i4-vtec", "atg-video-1/05_honda_vtec.mr", "out/reference/upstream/i4-vtec"},
	{"v8-ls", "atg-video-2/07_gm_ls.mr", "out/reference/upstream/v8-ls"},
	{"f4-ej25", "atg-video-1/06_subaru_ej25.mr", "out/reference/upstream/f4-ej25"},
	{"v12-412", "atg-video-2/12_ferrari_412_t2.mr", "out/reference/upstream/v12-412"},
	{"v12-merlin", "atg-video-2/11_merlin_v12.mr", "out/reference/upstream/v12-merlin"},
}

// popDensity is the pop density of the reference set, so that the own pops
// play. The core default of 0 turns them off.
const popDensity = 0.15

// The source kinds of a profile key.
const (
	kindUpstream    = "upstream"
	kindMeasured    = "measured"
	kindSnarl       = "snarl"
	kindNotMeasured = "not-measured"
)

type source struct{ kind, from string }

// refKey is one key of a reference profile with its value and source.
type refKey struct {
	key, value string
	src        source
}

// refSection is one [table] or one [[array]] entry of a reference profile,
// or the top level when name is empty.
type refSection struct {
	name  string
	array bool
	keys  []refKey
}

// refOptions are the inputs of the reference mode.
type refOptions struct {
	// out is the profile directory, which also holds the cycle tables
	// (<out>/cycles/<id>.bin) and gets the impulse responses (<out>/ir),
	// scenarios the directory of the upstream scenarios, grid the directory
	// of the grid records of just cycles-grid, and share the share/engine-sim
	// directory of the engine-sim package.
	out, scenarios, grid, share string
	// refs maps an engine id to its reference directory.
	refs map[string]string
}

// refProfile is one reference profile while it is built.
type refProfile struct {
	re refEngine
	// ref is the reference directory, scenarios the directory of the
	// upstream scenarios, grid the grid record directory, and commit the
	// upstream commit.
	ref, scenarios, grid, commit string
	e                            engine
	dump                         *esDump
	tables                       *cycles.Tables
	// stepRPM is the RPM of the throttle steps of the intake lag.
	stepRPM float64
	// stalled reports an engine that stalls at speed control 0.
	stalled     bool
	idle        idleRun
	idleFrom    string
	bounce      limiterBounce
	limiterFrom string
}

// runReference writes the reference set.
func runReference(o *refOptions) error {
	share, err := engineSimShare(o.share)
	if err != nil {
		return err
	}
	commit, err := nixRev("reference/engine-sim/package.nix")
	if err != nil {
		return err
	}
	var all []*refProfile
	for _, re := range referenceEngines {
		ref := o.refs[re.id]
		if ref == "" {
			ref = re.ref
		}
		r, err := loadRef(re, ref, o)
		if err != nil {
			return fmt.Errorf("%s: %w", re.id, err)
		}
		r.commit = commit
		all = append(all, r)
	}
	if err := os.MkdirAll(filepath.Join(o.out, "ir"), 0o755); err != nil {
		return err
	}
	for _, r := range all {
		for _, c := range r.tables.Channels {
			data, err := os.ReadFile(filepath.Join(share, "es", c.ImpulseResponse))
			if err != nil {
				return fmt.Errorf("%s: %w", r.re.id, err)
			}
			// The file takes the base name only, so it stays in the ir
			// directory.
			if err := os.WriteFile(filepath.Join(o.out, "ir", path.Base(c.ImpulseResponse)), data, 0o644); err != nil { //nolint:gosec // The base name has no directory part.
				return err
			}
		}
	}
	// The directory also holds the catalogue, so only the files of the set
	// are written.
	for _, r := range all {
		text, sources, err := r.files()
		if err != nil {
			return fmt.Errorf("%s: %w", r.re.id, err)
		}
		if err := os.WriteFile(filepath.Join(o.out, r.re.id+".toml"), []byte(text), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(o.out, r.re.id+".sources.toml"), []byte(sources), 0o644); err != nil {
			return err
		}
		fmt.Printf("%-11s %2d cylinders, %d channels, %3d table points, lag %.3f / %.3f s, cut %.1f ms\n",
			r.re.id, len(r.e.Cylinders), len(r.tables.Channels), len(r.tables.Points),
			r.tables.ThrottleLag.Close, r.tables.ThrottleLag.Open, r.e.LimiterMS)
	}
	return writeRefReadme(filepath.Join(o.out, "README.md"), all, commit)
}

// engineSimShare returns the share/engine-sim directory: dir, or that of
// the reference engine-sim package.
func engineSimShare(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	lib, err := soundLibrary("")
	if err != nil {
		return "", err
	}
	return filepath.Dir(filepath.Dir(lib)), nil
}

// loadRef reads the dump, the cycle tables and the step scenario of an
// engine, and measures its free runs.
func loadRef(re refEngine, ref string, o *refOptions) (*refProfile, error) {
	data, err := os.ReadFile(filepath.Join(ref, "engine.json"))
	if err != nil {
		return nil, err
	}
	d := &esDump{}
	if err := json.Unmarshal(data, d); err != nil {
		return nil, fmt.Errorf("%s/engine.json: %w", ref, err)
	}
	e, err := fromEngineSim(re.file, d)
	if err != nil {
		return nil, err
	}
	e.ID, e.Notes = re.id, nil
	t, err := cycles.ReadFile(filepath.Join(o.out, "cycles", re.id+".bin"))
	if err != nil {
		return nil, err
	}
	if t.ThrottleLag.Close <= 0 || t.ThrottleLag.Open <= 0 {
		return nil, fmt.Errorf("the tables of %s have no intake lag: build them with tools/cycles build --step", re.id)
	}
	scn := filepath.Join(o.scenarios, re.id)
	step, err := scenario.ParseFile(filepath.Join(scn, "grid", "step.scn"))
	if err != nil {
		return nil, err
	}
	r := &refProfile{
		re: re, ref: ref, scenarios: o.scenarios, grid: o.grid, e: e, dump: d, tables: t,
		stepRPM: step.Segments[0].RPM0, stalled: d.Idle.MeasuredRPM < 300,
	}
	if err := r.measureIdleCSV(filepath.Join(ref, "idle-free.csv"), filepath.Join(scn, "idle-free.scn")); err != nil {
		return nil, err
	}
	if r.stalled {
		r.e.IdleRPM = math.Round(r.idle.st.Last/10) * 10
	}
	if err := r.measureLimiter(filepath.Join(ref, "limiter-free.csv"), filepath.Join(scn, "limiter-free.scn")); err != nil {
		return nil, err
	}
	return r, nil
}

// measureIdleCSV measures the free run at idle of an engine-sim CSV.
func (r *refProfile) measureIdleCSV(csv, scn string) error {
	d, err := readRunnerCSV(csv)
	if err != nil {
		return err
	}
	t0, t1, err := freeSegment(scn)
	if err != nil {
		return err
	}
	r.idle, err = measureIdle(d, t0, t1, len(r.e.Cylinders), r.e.IdleRPM)
	if err != nil {
		return fmt.Errorf("%s: %w", csv, err)
	}
	r.idleFrom = fmt.Sprintf("the free run at throttle 0 of %s from %g to %g s", csv, t0+freeSettle, t1)
	return nil
}

// measureLimiter measures the free run into the limiter of an engine-sim
// CSV.
func (r *refProfile) measureLimiter(csv, scn string) error {
	d, err := readRunnerCSV(csv)
	if err != nil {
		return err
	}
	t0, t1, err := freeSegment(scn)
	if err != nil {
		return err
	}
	r.bounce, err = measureBounce(d, t0+freeSettle, t1, r.dump.RevLimitRPM)
	if err != nil {
		return fmt.Errorf("%s: %w", csv, err)
	}
	r.limiterFrom = fmt.Sprintf("the %d bounces of the free run at full throttle of %s from %g to %g s, with the RPM averaged over one engine cycle at the rev limit (tools/import measureBounce)",
		r.bounce.bounces, csv, t0+freeSettle, t1)
	r.e.LimiterMS = round(min(500, r.bounce.cutMS), 1)
	return nil
}

// files returns the profile file and the sources file.
func (r *refProfile) files() (string, string, error) {
	secs := r.sections()
	var p, s strings.Builder
	fmt.Fprintf(&p, "# Reference profile %s, written by just import-reference from engine-sim %s at %s.\n", r.re.id, r.re.file, r.commit)
	fmt.Fprintf(&p, "# Do not edit: change tools/import and run the recipe again. %s.sources.toml\n# gives the source of every key.\n", r.re.id)
	fmt.Fprintf(&s, "# Sources of the keys of %s.toml, written by just import-reference.\n", r.re.id)
	s.WriteString("# kind: upstream (copied from the upstream engine), measured (from its own\n# upstream reference renders), snarl (a snarl default for a feature that\n# upstream does not have) or not-measured (the core default, because the\n# references cannot give it).\n")
	writeSections(&p, secs)
	seen := map[string]source{}
	for _, sec := range secs {
		for _, k := range sec.keys {
			path := k.key
			if sec.name != "" {
				path = sec.name + "." + k.key
			}
			if old, ok := seen[path]; ok {
				if old != k.src {
					return "", "", fmt.Errorf("%s has two sources", path)
				}
				continue
			}
			if strings.ContainsAny(k.src.from+k.src.kind, "\"\\\n") || k.src.from == "" {
				return "", "", fmt.Errorf("%s: the source text is empty or has a quote, a backslash or a new line", path)
			}
			seen[path] = k.src
			fmt.Fprintf(&s, "\n[[source]]\nkey = %q\nkind = %q\nfrom = %q\n", path, k.src.kind, k.src.from)
		}
	}
	return p.String(), s.String(), nil
}

func writeSections(b *strings.Builder, secs []refSection) {
	for _, sec := range secs {
		switch {
		case sec.array:
			fmt.Fprintf(b, "\n[[%s]]\n", sec.name)
		case sec.name != "":
			fmt.Fprintf(b, "\n[%s]\n", sec.name)
		}
		for _, k := range sec.keys {
			fmt.Fprintf(b, "%s = %s\n", k.key, k.value)
		}
	}
}

func quote(s string) string { return strconv.Quote(s) }

func boolText(b bool) string { return strconv.FormatBool(b) }

// sections returns the keys of the profile, with their values and sources.
func (r *refProfile) sections() []refSection {
	def := core.DefaultSpec()
	e := &r.e
	up := func(f string, a ...any) source { return source{kindUpstream, fmt.Sprintf(f, a...)} }
	meas := func(f string, a ...any) source { return source{kindMeasured, fmt.Sprintf(f, a...)} }
	sn := func(f string, a ...any) source { return source{kindSnarl, fmt.Sprintf(f, a...)} }
	nm := func(f string, a ...any) source { return source{kindNotMeasured, fmt.Sprintf(f, a...)} }
	dump := r.ref + "/engine.json"
	bin := "profiles/cycles/" + r.re.id + ".bin"
	grid := r.grid + "/" + r.re.id

	idle := up("%s idle.measured_rpm, %.1f rpm (engine-sim-render: an 8 s free run at speed control 0), rounded to 10 rpm", dump, r.dump.Idle.MeasuredRPM)
	if r.stalled {
		idle = meas("the engine stalls at speed control 0 (%s idle.measured_rpm %.1f rpm), so the mean RPM of the last 1 s of %s, %.1f rpm, rounded to 10 rpm",
			dump, r.dump.Idle.MeasuredRPM, r.idleFrom, r.idle.st.Last)
	}
	top := refSection{keys: []refKey{
		{"name", quote(r.re.id), sn("the id of the reference engine")},
		{"description", quote(e.Description), sn("the description of %s in tools/import, which names no maker or model", r.re.file)},
		{"seed", strconv.Itoa(seed(r.re.id)), sn("the FNV-1a hash of the name, as tools/import seeds every profile")},
		{"cylinders", strconv.Itoa(len(e.Cylinders)), up("the cylinders of %s", dump)},
		{"idle_rpm", num(e.IdleRPM), idle},
		{"redline_rpm", num(e.RedlineRPM), up("%s redline_rpm", dump)},
		{"limiter_rpm", num(e.LimiterRPM), up("%s rev_limit_rpm", dump)},
		{"tables", quote(r.re.id), meas("%s of just cycles-grid %s: tools/cycles build of the engine-sim renders of the grid scenarios in %s/%s/grid at the simulation rate of the unchanged script, in %s: %d points, of which %d are slices of the closing steps of close.scn (docs/research/cycle-tables.md)",
			bin, r.re.id, r.scenarios, r.re.id, grid, len(r.tables.Points), closeSlices(r.tables))},
		{"volume", num(def.Volume), sn("half the engine-sim-render --volume 0.5 of the reference renders of just ref-upstream, so that the pops have 6 dB of room above the engine level of the references: core default")},
	}}

	lag := r.tables.ThrottleLag
	lagFrom := func(dir string, tau float64) source {
		return meas("the intake lag of %s, %.3f s: tools/cycles build --step %s/step fits exp(-t/tau) to the share of the state before the step in each cycle after the %s throttle step of %s/%s/grid/step.scn at %g rpm (%s/build.md)",
			bin, tau, grid, dir, r.scenarios, r.re.id, r.stepRPM, grid)
	}
	throttle := refSection{name: "throttle", keys: []refKey{
		{"close_lag_s", num(round(lag.Close, 4)), lagFrom("full to closed", lag.Close)},
		{"open_lag_s", num(round(lag.Open, 4)), lagFrom("closed to full", lag.Open)},
	}}

	b := r.bounce
	limiter := refSection{name: "limiter", keys: []refKey{
		{"cut_ms", num(e.LimiterMS), meas("the median cut, from a peak of the RPM to the next trough where the lines through the climbs and the falls meet, of %s: %.1f ms (the dump limiter_duration_s is %g s)", r.limiterFrom, b.cutMS, r.dump.LimiterS)},
		{"fall_rpm_s", num(math.Round(b.fallRPMS)), meas("the median fall rate in the cuts, the slope of a straight line through the RPM from half a cycle after a peak to half a cycle before the trough, of %s, with median peaks and troughs of %.0f and %.0f rpm", r.limiterFrom, b.peak, b.trough)},
		{"rise_rpm_s", num(math.Round(b.riseRPMS)), meas("the median climb rate, the slope of a straight line through the RPM from half a cycle after a trough to half a cycle before the next peak, of %s", r.limiterFrom)},
		{"load_scale", num(def.Limiter.LoadScale), sn("the rates of the free run with no load from a gear: core default")},
		{"final_gear_waver", num(def.Limiter.FinalGearWaver), sn("the final gear hold, a snarl-only feature: core default")},
	}}

	st := r.idle.st
	idleSec := refSection{name: "idle", keys: []refKey{
		{"fade_rpm", num(e.IdleRPM + 1400), nm("the free runs give the wander at idle only: idle_rpm plus 1400 rpm, the tools/import rule")},
		{"hunt", num(round(r.idle.hunt, 4)), meas("the spread of the RPM averaged over one engine cycle over the mean RPM of %s, %.1f over %.1f rpm (%.1f rpm with no averaging), over %.4f, the spread of the idle wander of the engine per unit of hunt", r.idleFrom, r.idle.cycleSD, st.Mean, st.SD, huntLFOSD)},
		{"hunt_hz", num(round(r.idle.huntHz, 2)), meas("the main wander frequency of %s, %.3f Hz at %.1f rpm", r.idleFrom, st.WanderHz, st.WanderRPM)},
	}}

	pd := def.Pops
	pdef := sn("pops are a snarl-only feature: core default")
	pops := refSection{name: "pops", keys: []refKey{
		{"density", num(popDensity), sn("pops are a snarl-only feature: the tools/import default, so that the own pops play (the core default 0 turns them off)")},
		{"character", quote(pd.Character), pdef},
		{"gain", num(pd.Gain), pdef},
		{"body_hz", num(pd.BodyHz), pdef},
		{"body_ms", num(pd.BodyMS), pdef},
		{"jump_ms", num(pd.JumpMS), pdef},
		{"limiter_bangs", boolText(false), sn("pops are a snarl-only feature: the engine-sim ignition module cuts the spark with no afterfire (src/ignition_module.cpp), so a cut does not bang")},
		{"weight_pop", num(pd.WeightPop), pdef},
		{"weight_boom", num(pd.WeightBoom), pdef},
		{"weight_crackle", num(pd.WeightCrackle), pdef},
		{"weight_burble", num(pd.WeightBurble), pdef},
	}}

	secs := []refSection{top, throttle, limiter, idleSec, pops}
	ir := up("the impulse response of the exhaust system of the channel that %s names, the impulse_response of the exhaust systems of %s, copied unchanged from the es directory of the engine-sim package to profiles/ir", bin, dump)
	for _, c := range r.tables.Channels {
		secs = append(secs, refSection{name: "channel", array: true, keys: []refKey{
			{"impulse_response", quote(path.Base(c.ImpulseResponse)), ir},
		}})
	}
	return secs
}

// closeSlices returns the points of the closing transient of the tables.
func closeSlices(t *cycles.Tables) int {
	n := 0
	for _, p := range t.Points {
		if p.Since > 0 {
			n++
		}
	}
	return n
}

//go:embed reference.md.tmpl
var refReadmeTemplate string

// refReadmeEngine is one engine of the README.
type refReadmeEngine struct {
	ID, File, Commit, Ref string
	Points, Channels      int
	ImpulseResponses      string
	Measured, NotMeasured []refReadmeKey
}

type refReadmeKey struct{ Key, Value, From string }

// writeRefReadme writes the README of the reference set from the sources
// of each profile.
func writeRefReadme(path string, all []*refProfile, commit string) error {
	var engines []refReadmeEngine
	for _, r := range all {
		re := refReadmeEngine{ID: r.re.id, File: r.re.file, Commit: r.commit, Ref: r.ref, Points: len(r.tables.Points), Channels: len(r.tables.Channels)}
		var irs []string
		for _, c := range r.tables.Channels {
			irs = append(irs, "`"+c.ImpulseResponse+"`")
		}
		re.ImpulseResponses = strings.Join(irs, ", ")
		seen := map[string]bool{}
		for _, sec := range r.sections() {
			for _, k := range sec.keys {
				path := k.key
				if sec.name != "" {
					path = sec.name + "." + k.key
				}
				if seen[path] {
					continue
				}
				seen[path] = true
				value := k.value
				if sec.array {
					value = "per entry"
				}
				switch k.src.kind {
				case kindMeasured:
					re.Measured = append(re.Measured, refReadmeKey{path, value, k.src.from})
				case kindNotMeasured:
					re.NotMeasured = append(re.NotMeasured, refReadmeKey{path, value, k.src.from})
				}
			}
		}
		engines = append(engines, re)
	}
	t, err := template.New("readme").Parse(refReadmeTemplate)
	if err != nil {
		return err
	}
	var b strings.Builder
	if err := t.Execute(&b, map[string]any{"Engines": engines, "EngineSimCommit": commit, "PopDensity": num(popDensity)}); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
