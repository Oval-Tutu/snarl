package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/model"
)

// The reference set: six engines converted from upstream with the source
// of every value recorded. Each value is copied from the upstream engine,
// measured from its own upstream reference render, or a snarl default for
// a feature that upstream does not have. A value that the references
// cannot give is "not measured" and takes the model default.

// refEngine is one engine of the reference set: its id, the upstream
// project and file, and the default directory of its reference renders.
type refEngine struct {
	id, project, file, ref string
}

var referenceEngines = []refEngine{
	{"i4-vtec", "engine-sim", "atg-video-1/05_honda_vtec.mr", "out/reference/upstream/i4-vtec"},
	{"v8-ls", "engine-sim", "atg-video-2/07_gm_ls.mr", "out/reference/upstream/v8-ls"},
	{"f4-ej25", "engine-sim", "atg-video-1/06_subaru_ej25.mr", "out/reference/upstream/f4-ej25"},
	{"v12-412", "engine-sim", "atg-video-2/12_ferrari_412_t2.mr", "out/reference/upstream/v12-412"},
	{"v12-merlin", "engine-sim", "atg-video-2/11_merlin_v12.mr", "out/reference/upstream/v12-merlin"},
	{"f6-aircooled", "EngineLab", "09_porsche_flat6_like.engine.yaml", "out/reference/enginelab/f6-aircooled"},
}

// elPulseTable is the pulse table of the EngineLab engine, which writes no
// per-cylinder exhaust pressure: the table of the flat engine of the set.
const elPulseTable = "f4-ej25"

// popDefault is the pop stage set that the pop bank of every reference
// profile follows: the tools/import defaults of an exhaust path with no
// pipe size.
var popDefault = analysis.LegacyPath{
	PipeRadiusM: 0.025, OutletM: 0.025, WallLossDB: legacyWallLossDB, WallLossHz: legacyWallLossHz,
	ChamberM: defaultChamberM, ChamberArea: defaultChamberArea,
}

// popDensity is the pop density of the reference set: the tools/import
// default, so that the own pops play. The model default of 0 turns them
// off.
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
	// out is the output directory, work the directory of the tools/fit
	// --pulse --json files (<work>/<id>/pulse.json), enginelab the
	// directory of the EngineLab JSON files, scenarios the directory of
	// the upstream scenarios, elScenarios the EngineLab scenarios, and
	// share the share/engine-sim directory of the engine-sim package.
	out, work, enginelab, scenarios, elScenarios, share string
	// refs maps an engine id to its reference directory.
	refs map[string]string
}

// refProfile is one reference profile while it is built.
type refProfile struct {
	re refEngine
	// ref is the reference directory, scenarios the directory of the
	// upstream scenarios, and commit the upstream commit.
	ref, scenarios, commit string
	e                      engine
	// dump is the engine-sim dump, nil for EngineLab.
	dump *esDump
	// stalled reports an engine-sim engine that stalls at speed control 0.
	stalled      bool
	idle         idleRun
	idleFrom     string
	bounce       limiterBounce
	limiterFrom  string
	spread       cylinderSpread
	cam          float64
	speedOfSound float64
	cycleCoV     float64
	pop          analysis.FormantBank
	popDelayMS   float64
}

// runReference writes the reference set.
func runReference(o *refOptions) error {
	share, err := engineSimShare(o.share)
	if err != nil {
		return err
	}
	esCommit, err := nixRev("reference/engine-sim/package.nix")
	if err != nil {
		return err
	}
	elCommit, err := nixRev("tools/import/enginelab.nix")
	if err != nil {
		return err
	}
	pop, delay, err := fitPop(popDefault)
	if err != nil {
		return fmt.Errorf("pop bank: %w", err)
	}
	var all []*refProfile
	for _, re := range referenceEngines {
		ref := o.refs[re.id]
		if ref == "" {
			ref = re.ref
		}
		var r *refProfile
		if re.project == "engine-sim" {
			r, err = loadEngineSimRef(re, ref, o, share)
			if r != nil {
				r.commit = esCommit
			}
		} else {
			r, err = loadEngineLabRef(re, ref, o)
			if r != nil {
				r.commit = elCommit
			}
		}
		if err != nil {
			return fmt.Errorf("%s: %w", re.id, err)
		}
		r.pop, r.popDelayMS = pop, delay
		all = append(all, r)
	}
	if err := fitRefBanks(all, filepath.Join(share, "es", "sound-library")); err != nil {
		return err
	}
	var wg sync.WaitGroup
	errs := make([]error, len(all))
	for i, r := range all {
		wg.Go(func() {
			if err := calibrate(&r.e, func(*engine) string { return r.profileText() }); err != nil {
				errs[i] = fmt.Errorf("%s: %w", r.re.id, err)
			}
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}
	// The directory also holds the catalogue, so only the files of the set
	// are written.
	if err := os.MkdirAll(o.out, 0o755); err != nil {
		return err
	}
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
		fmt.Printf("%-13s %2d cylinders, %d banks, leveller_db %5.1f, output_db %6.2f, sweep-wot %.2f LUFS\n",
			r.re.id, len(r.e.Cylinders), len(r.e.Paths), r.e.LevellerDB, r.e.OutputDB, r.e.SweepLUFS)
	}
	return writeRefReadme(filepath.Join(o.out, "README.md"), all, esCommit, elCommit)
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

// loadEngineSimRef reads the dump and measures the references of an
// engine-sim engine.
func loadEngineSimRef(re refEngine, ref string, o *refOptions, share string) (*refProfile, error) {
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
	r := &refProfile{re: re, ref: ref, scenarios: o.scenarios, e: e, dump: d, stalled: d.Idle.MeasuredRPM < 300}
	if r.speedOfSound, err = engineSimExhaust(d, &r.e); err != nil {
		return nil, err
	}
	scn := filepath.Join(o.scenarios, re.id)
	if err := r.measureIdleCSV(filepath.Join(ref, "idle-free.csv"), filepath.Join(scn, "idle-free.scn")); err != nil {
		return nil, err
	}
	if r.stalled {
		r.e.IdleRPM = math.Round(r.idle.st.Last/10) * 10
	}
	if err := r.measureLimiter(filepath.Join(ref, "limiter-free.csv"), filepath.Join(scn, "limiter-free.scn")); err != nil {
		return nil, err
	}
	r.spread, err = readCylinderSpread(filepath.Join(o.work, re.id, "pulse.json"))
	if err != nil {
		return nil, err
	}
	r.cam, err = camSwitchRPM(filepath.Join(share, "engines", re.file), filepath.Join(share, "es", "objects", "objects.mr"))
	return r, err
}

// engineSimExhaust sets the primary of each cylinder and the gain of each
// path of e from the dump, and returns the speed of sound of the profile.
// engine-sim delays each cylinder by its header primary plus the length of
// its exhaust system at 343 m/s, in a delay filter set for 10000 steps per
// second that runs once per simulation step, and weights it by its sound
// attenuation and the audio volume of its system over the square of that
// length (PistonEngineSimulator in src/piston_engine_simulator.cpp). The
// gain of a path is the mean weight of its cylinders, relative to the
// loudest path.
func engineSimExhaust(d *esDump, e *engine) (float64, error) {
	if d.SimHz <= 0 {
		return 0, errors.New("the dump has no simulation frequency")
	}
	keys := make([]int, len(d.Cylinders))
	for i, c := range d.Cylinders {
		keys[i] = c.ExhaustSystem
		e.Cylinders[i].PrimaryM = c.HeaderPrimaryM + d.Exhaust[c.ExhaustSystem].LengthM
	}
	banks, systems := banksInOrder(keys)
	gains := make([]float64, len(systems))
	for b, s := range systems {
		var sum, n float64
		for i, c := range d.Cylinders {
			if banks[i] == b {
				l := c.HeaderPrimaryM + d.Exhaust[s].LengthM
				sum += c.Attenuation / (l * l)
				n++
			}
		}
		gains[b] = d.Exhaust[s].AudioVolume * sum / n
	}
	loudest := slices.Max(gains)
	if loudest <= 0 || len(gains) != len(e.Paths) {
		return 0, errors.New("no exhaust system has a positive weight")
	}
	for b := range e.Paths {
		e.Paths[b].Gain = gains[b] / loudest
	}
	return 343 * d.SimHz / 10000, nil
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

// loadEngineLabRef reads the EngineLab engine and measures its references.
func loadEngineLabRef(re refEngine, ref string, o *refOptions) (*refProfile, error) {
	all, err := loadEngineLab(o.enginelab)
	if err != nil {
		return nil, err
	}
	k := slices.IndexFunc(all, func(e engine) bool { return e.File == re.file })
	if k < 0 {
		return nil, fmt.Errorf("no %s in %s", re.file, o.enginelab)
	}
	r := &refProfile{re: re, ref: ref, scenarios: o.scenarios, e: all[k], speedOfSound: model.DefaultSpec().Exhaust.SpeedOfSound}
	r.e.ID, r.e.Notes, r.e.ImpulseResponse = re.id, nil, ""
	d, err := readOrderMapRPM(filepath.Join(ref, "idle-free", "engine-order-map.csv"))
	if err != nil {
		return nil, err
	}
	pre, err := prerollSeconds(filepath.Join(o.elScenarios, "idle-free.json"))
	if err != nil {
		return nil, err
	}
	t0, t1, err := freeSegment(filepath.Join(o.scenarios, re.id, "idle-free.scn"))
	if err != nil {
		return nil, err
	}
	r.idle, err = measureIdle(d, pre+t0, pre+t1, len(r.e.Cylinders), r.e.IdleRPM)
	if err != nil {
		return nil, fmt.Errorf("idle-free: %w", err)
	}
	r.idleFrom = fmt.Sprintf("the free run at throttle 0 of %s/idle-free/engine-order-map.csv (%.0f RPM values per second) from %g to %g s, after the %g s pre-roll of reference/enginelab/scenarios/idle-free.json",
		ref, d.Rate, pre+t0+freeSettle, pre+t1, pre)
	r.cycleCoV, err = manifestCycleCoV(filepath.Join(ref, "holds-t100", "render-manifest.json"))
	return r, err
}

// fitRefBanks fits the formant bank of each engine-sim engine to the
// impulse response that its dump names. Each response is fitted once, and
// the fits run in parallel.
func fitRefBanks(all []*refProfile, lib string) error {
	var names []string
	for _, r := range all {
		if r.e.ImpulseResponse != "" && !slices.Contains(names, r.e.ImpulseResponse) {
			names = append(names, r.e.ImpulseResponse)
		}
	}
	banks := make([]analysis.FormantBank, len(names))
	errs := make([]error, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Go(func() {
			path, err := findIR(lib, name)
			if err == nil {
				banks[i], err = fitIR(path)
			}
			errs[i] = err
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}
	for _, r := range all {
		if k := slices.Index(names, r.e.ImpulseResponse); k >= 0 {
			r.e.Bank = banks[k]
		}
	}
	return nil
}

// files returns the profile file and the sources file.
func (r *refProfile) files() (string, string, error) {
	secs := r.sections()
	var p, s strings.Builder
	fmt.Fprintf(&p, "# Reference profile %s, written by just import-reference from %s %s at %s.\n", r.re.id, r.re.project, r.re.file, r.commit)
	fmt.Fprintf(&p, "# Do not edit: change tools/import and run the recipe again. %s.sources.toml\n# gives the source of every key.\n", r.re.id)
	fmt.Fprintf(&s, "# Sources of the keys of %s.toml, written by just import-reference.\n", r.re.id)
	s.WriteString("# kind: upstream (copied from the upstream engine), measured (from its own\n# upstream reference render), snarl (a snarl default for a feature that\n# upstream does not have) or not-measured (the model default, because the\n# references cannot give it).\n")
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

// profileText returns the profile file, for the calibration.
func (r *refProfile) profileText() string {
	var b strings.Builder
	writeSections(&b, r.sections())
	return b.String()
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

// sections returns the keys of the profile, with their values and sources.
func (r *refProfile) sections() []refSection {
	def := model.DefaultSpec()
	es := r.dump != nil
	e := &r.e
	up := func(f string, a ...any) source { return source{kindUpstream, fmt.Sprintf(f, a...)} }
	meas := func(f string, a ...any) source { return source{kindMeasured, fmt.Sprintf(f, a...)} }
	sn := func(f string, a ...any) source { return source{kindSnarl, fmt.Sprintf(f, a...)} }
	nm := func(f string, a ...any) source { return source{kindNotMeasured, fmt.Sprintf(f, a...)} }
	dump := r.ref + "/engine.json"
	yaml := "engines/" + r.re.file
	// pick returns the engine-sim source or the EngineLab source.
	pick := func(a, b source) source {
		if es {
			return a
		}
		return b
	}
	noSynth := nm("an engine-sim synthesiser setting, which EngineLab does not have: model default")
	noLimiter := nm("EngineLab reaches about 7240 rpm at full throttle with no load, below its rev limit, so no reference bounces off the limiter (reference/enginelab/README.md): model default")
	if es {
		noLimiter = nm("no reviewed tool measures it from the limiter reference: model default")
	}

	top := refSection{keys: []refKey{
		{"name", quote(r.re.id), sn("the id of the reference engine")},
		{"description", quote(e.Description), sn("the description of %s in tools/import, which names no maker or model", r.re.file)},
		{"seed", strconv.Itoa(seed(r.re.id)), sn("the FNV-1a hash of the name, as tools/import seeds every profile")},
	}}
	idle := pick(up("%s idle.measured_rpm, %.1f rpm (engine-sim-render: an 8 s free run at speed control 0), rounded to 10 rpm", dump, r.dumpIdle()),
		up("%s engine.idle_rpm", yaml))
	if r.stalled {
		idle = meas("the engine stalls at speed control 0 (%s idle.measured_rpm %.1f rpm), so the mean RPM of the last 1 s of %s, %.1f rpm, rounded to 10 rpm",
			dump, r.dumpIdle(), r.idleFrom, r.idle.st.Last)
	}
	lim := []refKey{
		{"idle_rpm", num(e.IdleRPM), idle},
		{"redline_rpm", num(e.RedlineRPM), pick(up("%s redline_rpm", dump), up("%s engine.redline_rpm", yaml))},
		{"limiter_rpm", num(e.LimiterRPM), pick(up("%s rev_limit_rpm", dump), up("%s engine.ignition.rev_limit_rpm", yaml))},
		{"limiter_kind", quote(e.LimiterKind), pick(
			up("the engine-sim ignition module cuts the spark of every cylinder with no afterfire (src/ignition_module.cpp), which is the snarl fuel cut"),
			up("%s engine.ignition.limiter_keeps_fuel is not set, so the limiter cuts the fuel", yaml))},
	}
	if es {
		b := r.bounce
		lim = append(lim,
			refKey{"limiter_ms", num(e.LimiterMS), meas("the median cut, from a peak of the RPM to the next trough where the lines through the climbs and the falls meet, of %s: %.1f ms (the dump limiter_duration_s is %g s)", r.limiterFrom, b.cutMS, r.dump.LimiterS)},
			refKey{"limiter_fall_rpm_s", num(math.Round(b.fallRPMS)), meas("the median fall rate in the cuts, the slope of a straight line through the RPM from half a cycle after a peak to half a cycle before the trough, of %s, with median peaks and troughs of %.0f and %.0f rpm", r.limiterFrom, b.peak, b.trough)},
			refKey{"limiter_rise_rpm_s", num(math.Round(b.riseRPMS)), meas("the median climb rate, the slope of a straight line through the RPM from half a cycle after a trough to half a cycle before the next peak, of %s", r.limiterFrom)},
		)
	} else {
		lim = append(lim,
			refKey{"limiter_ms", num(e.LimiterMS), up("%s engine.ignition.limiter_duration_s", yaml)},
			refKey{"limiter_fall_rpm_s", num(def.LimiterFallRPMS), noLimiter},
			refKey{"limiter_rise_rpm_s", num(def.LimiterRiseRPMS), noLimiter},
		)
	}
	lim = append(lim,
		refKey{"limiter_load_scale", num(def.LimiterLoadScale), sn("the rates of the free run with no load from a gear: model default")},
		refKey{"limiter_pump_db", num(def.LimiterPumpDB), noLimiter},
		refKey{"limiter_notch_db", num(def.LimiterNotchDB), noLimiter},
		refKey{"limiter_bark_db", num(def.LimiterBarkDB), noLimiter},
		refKey{"final_gear_waver", num(def.FinalGearWaver), sn("the final gear hold, a snarl-only feature: model default")},
	)
	if r.cam > 0 {
		lim = append(lim, refKey{"cam_switch_rpm", num(r.cam), up("%s uses the vtec_valvetrain node with its min_rpm default in es/objects/objects.mr", r.re.file)})
	}
	top.keys = append(top.keys, lim...)

	pulse := refSection{name: "pulse"}
	if es {
		pulse.keys = []refKey{
			{"table", quote(r.re.id), meas("tools/fit --pulse --variation --throttle 1 --engine %s --scenarios %s/%s on %s/holds-t100.csv: one row per hold from %.0f to %.0f rpm, in internal/model/pulses/%s.txt",
				dump, r.scenarios, r.re.id, r.ref, r.spread.lo, r.spread.hi, r.re.id)},
		}
		if r.cam > 0 {
			pulse.keys = append(pulse.keys, refKey{"cam_gain", num(def.Pulse.CamGain), nm("each row of the pulse table has a peak of 1, and no reviewed tool measures the level step at the cam switch: model default")})
		}
		spread := fmt.Sprintf("tools/fit --pulse --variation of the table, over its %d holds", r.spread.holds)
		pulse.keys = append(pulse.keys,
			refKey{"cycle_variation", "0", meas("measured_variation 1 takes the measured firing gain spread of the table, so there is no extra uniform spread")},
			refKey{"measured_variation", "1", meas("the firing gain spread of the spread lines of the table (%s)", spread)},
			refKey{"jitter", num(def.Pulse.Jitter), nm("the dynamometer holds the RPM in the references, so they cannot show a slow RPM and gain wander under load: model default")},
			refKey{"gain_variation", num(round(math.Sqrt(3)*r.spread.gainSD, 4)), meas("sqrt(3) times the median spread of the cylinder mean gains (%s), %.4f", spread, r.spread.gainSD)},
			refKey{"timing_variation_deg", num(round(math.Sqrt(3)*r.spread.timeSD, 3)), meas("sqrt(3) times the median spread of the cylinder mean onsets (%s), %.3f degrees", spread, r.spread.timeSD)},
		)
	} else {
		same := up("every cylinder of %s has the cylinder_template and an even offset from engine.firing_order, so the cylinders do not differ", yaml)
		pulse.keys = []refKey{
			{"table", quote(elPulseTable), nm("EngineLab writes no per-cylinder exhaust pressure (reference/enginelab/README.md), so no table is measured: the table of %s, the flat engine of the set, measured from unchanged upstream data", elPulseTable)},
			{"cycle_variation", num(round(math.Sqrt(3)*r.cycleCoV, 4)), up("sqrt(3) times the cycle-to-cycle coefficient of variation that EngineLab sets for this engine, %g (%s/holds-t100/render-manifest.json audio_physics.authored_cycle_variation_cov)", r.cycleCoV, r.ref)},
			{"measured_variation", num(def.Pulse.MeasuredVariation), nm("the engine has no measured table of its own: model default")},
			{"jitter", num(def.Pulse.Jitter), nm("the dynamometer holds the RPM in the references, so they cannot show a slow RPM and gain wander under load: model default")},
			{"gain_variation", "0", same},
			{"timing_variation_deg", "0", same},
		}
	}

	exhaust := refSection{name: "exhaust", keys: []refKey{
		{"speed_of_sound", num(r.speedOfSound), pick(
			up("engine-sim delays the flow of each cylinder by its header primary plus the exhaust length at 343 m/s in a delay filter set for 10000 steps per second, which runs once per simulation step (src/piston_engine_simulator.cpp): 343 times %s simulation_frequency_hz %g over 10000", dump, r.dumpSimHz()),
			nm("EngineLab delays the exhaust in its finite-volume exhaust network, which gives no single speed of sound: model default"))},
		{"crossover", quote(e.Crossover), pick(up("engine-sim exhaust systems do not mix"), up("the exhaust paths of %s have no crossover component", yaml))},
	}}
	popSrc := sn("pops are a snarl-only feature: the pop bank fitted with the tools/fit --pop settings to the tools/import default pop stages (pipe and outlet radius 0.025 m, chamber 0.3 m at 7 times the pipe area, wall loss 2.5 dB at 2 kHz)")
	if es {
		exhaust.keys = append(exhaust.keys, refKey{"formant_gain_db", num(round(e.Bank.GainDB, 2)), r.bankSource()})
	} else {
		exhaust.keys = append(exhaust.keys, refKey{"formant_gain_db", "0", nm("EngineLab loads no impulse response for this engine (%s/holds-t100/render-manifest.json impulse_responses loaded 0): its exhaust network, which snarl does not model, gives the colour, so there is no formant bank", r.ref)})
	}
	exhaust.keys = append(exhaust.keys,
		refKey{"pop_formant_gain_db", num(round(r.pop.GainDB, 2)), popSrc},
		refKey{"pop_delay_ms", num(round(r.popDelayMS, 3)), popSrc},
	)

	var noise refSection
	if es {
		noise = refSection{name: "noise", keys: []refKey{
			{"hf_gain", num(e.HFGain), up("%s hf_gain", dump)},
			{"am", num(e.AM), up("%s noise", dump)},
			{"am_cutoff_hz", num(def.Noise.AMCutoffHz), up("engine-sim Synthesizer::AudioParameters airNoiseFrequencyCutoff (include/synthesizer.h)")},
			{"input_jitter", num(e.InputJitter), up("%s jitter", dump)},
		}}
	} else {
		noise = refSection{name: "noise", keys: []refKey{
			{"hf_gain", num(def.Noise.HFGain), noSynth},
			{"am", num(def.Noise.AM), noSynth},
			{"am_cutoff_hz", num(def.Noise.AMCutoffHz), noSynth},
			{"input_jitter", num(def.Noise.InputJitter), noSynth},
		}}
	}

	pd := def.Pops
	pdef := sn("pops are a snarl-only feature: model default")
	pops := refSection{name: "pops", keys: []refKey{
		{"density", num(popDensity), sn("pops are a snarl-only feature: the tools/import default for imported profiles, so that the own pops play (the model default 0 turns them off)")},
		{"character", quote(pd.Character), pdef},
		{"gain", num(pd.Gain), pdef},
		{"body_hz", num(pd.BodyHz), pdef},
		{"body_ms", num(pd.BodyMS), pdef},
		{"jump_ms", num(pd.JumpMS), pdef},
		{"crack", num(pd.Crack), pdef},
		{"crack_ms", num(pd.CrackMS), pdef},
		{"weight_pop", num(pd.WeightPop), pdef},
		{"weight_boom", num(pd.WeightBoom), pdef},
		{"weight_crackle", num(pd.WeightCrackle), pdef},
		{"weight_burble", num(pd.WeightBurble), pdef},
	}}
	st := r.idle.st
	idleSec := refSection{name: "idle", keys: []refKey{
		{"fade_rpm", num(e.IdleRPM + 1400), nm("the free runs give the hunt at idle only: idle_rpm plus 1400 rpm, the tools/import rule")},
		{"hunt", num(round(r.idle.hunt, 4)), meas("the spread of the RPM averaged over one engine cycle over the mean RPM of %s, %.1f over %.1f rpm (%.1f rpm with no averaging), over %.4f, the spread of the hunt modulation of the model per unit of hunt", r.idleFrom, r.idle.cycleSD, st.Mean, st.SD, huntLFOSD)},
		{"hunt_hz", num(round(r.idle.huntHz, 2)), meas("the main wander frequency of %s, %.3f Hz at %.1f rpm", r.idleFrom, st.WanderHz, st.WanderRPM)},
	}}
	cal := "the tools/import calibration rule, by rendering the profile"
	voice := refSection{name: "voice", keys: []refKey{
		{"gain", num(def.Voice.Gain), sn("model default")},
		{"leveller_db", num(e.LevellerDB), sn("%s: the closed-throttle hold at idle %g dB below the knee of the leveller", cal, kneeMargin)},
		{"output_db", num(e.OutputDB), sn("%s: the sweep-wot scenario at -16 LUFS", cal)},
	}}
	secs := []refSection{top, pulse, exhaust, noise, pops, idleSec, voice}
	secs = append(secs, r.cylinderSections()...)
	secs = append(secs, r.pathSections()...)
	if es {
		secs = append(secs, bankSections("formant", &e.Bank, r.bankSource())...)
	}
	return append(secs, bankSections("pop_formant", &r.pop, popSrc)...)
}

func (r *refProfile) dumpIdle() float64 {
	if r.dump == nil {
		return 0
	}
	return r.dump.Idle.MeasuredRPM
}

func (r *refProfile) dumpSimHz() float64 {
	if r.dump == nil {
		return 0
	}
	return r.dump.SimHz
}

// bankSource is the source of the formant bank of an engine-sim engine.
func (r *refProfile) bankSource() source {
	return source{kindMeasured, fmt.Sprintf("the formant bank fitted to the impulse response %s that %s/engine.json names, with the tools/import fit (fitIR: tools/fit --base ir --hi 16000, with the -60 dB floor when the -40 dB fit is more than 3 dB off from 6 to 10 kHz)", r.e.ImpulseResponse, r.ref)}
}

// firingOrder returns the cylinders, from 1, in rising crank angle.
func firingOrder(cyls []cylinder) string {
	idx := make([]int, len(cyls))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int {
		switch {
		case cyls[a].CrankDeg < cyls[b].CrankDeg:
			return -1
		case cyls[a].CrankDeg > cyls[b].CrankDeg:
			return 1
		}
		return 0
	})
	s := make([]string, len(idx))
	for k, i := range idx {
		s[k] = strconv.Itoa(i + 1)
	}
	return strings.Join(s, "-")
}

func (r *refProfile) cylinderSections() []refSection {
	var crank, bank, primary source
	order := firingOrder(r.e.Cylinders)
	if r.dump != nil {
		dump := r.ref + "/engine.json"
		crank = source{kindUpstream, fmt.Sprintf("%s spark_deg of each cylinder minus that of cylinder 1, in the 720 degree cycle: firing order %s", dump, order)}
		if r.dump.reversed() {
			crank.from = fmt.Sprintf("%s spark_deg of each cylinder, negated because the starter speed is %g rpm and engine-sim turns the crank the other way, minus that of cylinder 1, in the 720 degree cycle: firing order %s", dump, r.dump.StarterRPM, order)
		}
		bank = source{kindUpstream, fmt.Sprintf("%s exhaust_system of each cylinder, one bank per exhaust system in the order of first use", dump)}
		primary = source{kindUpstream, fmt.Sprintf("%s header_primary_length_m of the cylinder plus length_m of its exhaust system, the length that engine-sim delays (src/piston_engine_simulator.cpp)", dump)}
	} else {
		yaml := "engines/" + r.re.file
		crank = source{kindUpstream, fmt.Sprintf("the position of the cylinder in %s engine.firing_order times 720 degrees over the cylinder count, as EngineLab sets the crank offsets: firing order %s", yaml, order)}
		bank = source{kindUpstream, fmt.Sprintf("the exhaust path of the cylinder in %s engine.exhaust_paths", yaml)}
		primary = source{kindUpstream, fmt.Sprintf("primary_length_mm of the exhaust preset that %s names in uses.exhaust (parts/exhausts.yaml)", yaml)}
	}
	var out []refSection
	for _, c := range r.e.Cylinders {
		out = append(out, refSection{name: "cylinder", array: true, keys: []refKey{
			{"crank_deg", num(c.CrankDeg), crank}, {"bank", strconv.Itoa(c.Bank), bank}, {"primary_m", num(c.PrimaryM), primary},
		}})
	}
	return out
}

func (r *refProfile) pathSections() []refSection {
	var collector, refl, gain source
	if r.dump != nil {
		dump := r.ref + "/engine.json"
		collector = source{kindUpstream, fmt.Sprintf("%s length_m of the exhaust system. With reflection 0 the collector passes the sound unchanged", dump)}
		refl = source{kindUpstream, "engine-sim feeds the delayed exhaust flow to the synthesiser with no reflection (src/piston_engine_simulator.cpp)"}
		gain = source{kindUpstream, fmt.Sprintf("%s audio_volume of the exhaust system times the mean over its cylinders of sound_attenuation over the square of header_primary_length_m plus length_m, relative to the loudest system, as engine-sim weights the exhaust flow (src/piston_engine_simulator.cpp)", dump)}
	} else {
		yaml := "engines/" + r.re.file
		collector = source{kindUpstream, fmt.Sprintf("midpipe_length_mm of the exhaust preset that %s names, at least 0.2 m, as tools/import maps a scalar exhaust. With reflection 0 the collector passes the sound unchanged", yaml)}
		refl = source{kindNotMeasured, "EngineLab reflects at the junctions of its finite-volume exhaust network, which gives no single reflection gain: model default"}
		gain = source{kindUpstream, fmt.Sprintf("audio_volume of the exhaust path in %s engine.exhaust_paths, relative to the loudest", yaml)}
	}
	var out []refSection
	for _, p := range r.e.Paths {
		out = append(out, refSection{name: "path", array: true, keys: []refKey{
			{"collector_m", num(p.CollectorM), collector}, {"reflection", "0", refl}, {"gain", num(p.Gain), gain},
		}})
	}
	return out
}

// bankSections returns the [[table]] entries of a bank, as
// analysis.WriteBankTOML writes them.
func bankSections(table string, bank *analysis.FormantBank, src source) []refSection {
	var out []refSection
	for _, keys := range analysis.BankKeys(bank) {
		sec := refSection{name: table, array: true}
		for _, kv := range keys {
			sec.keys = append(sec.keys, refKey{kv[0], kv[1], src})
		}
		out = append(out, sec)
	}
	return out
}

//go:embed reference.md.tmpl
var refReadmeTemplate string

// refReadmeEngine is one engine of the README.
type refReadmeEngine struct {
	ID, Project, File, Commit, Ref, Table string
	Measured, NotMeasured                 []refReadmeKey
}

type refReadmeKey struct{ Key, Value, From string }

// writeRefReadme writes the README of the reference set from the sources
// of each profile.
func writeRefReadme(path string, all []*refProfile, esCommit, elCommit string) error {
	var engines []refReadmeEngine
	for _, r := range all {
		re := refReadmeEngine{ID: r.re.id, Project: r.re.project, File: r.re.file, Commit: r.commit, Ref: r.ref}
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
	err = t.Execute(&b, map[string]any{
		"Engines": engines, "EngineSimCommit": esCommit, "EngineLabCommit": elCommit, "PulseTable": elPulseTable,
		"PopDensity": num(popDensity),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
