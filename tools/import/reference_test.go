package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/cycles"
)

// TestMeasureBounce checks the limiter measure on an RPM that climbs to
// 6000 rpm, then bounces with a cut of 80 ms at 4000 rpm per second and a
// climb at 8000 rpm per second, with a firing ripple of 20 rpm at 200 Hz.
func TestMeasureBounce(t *testing.T) {
	const rate = 10000
	d := &analysis.RunnerData{Rate: rate}
	rpm, falling, cut := 5000.0, false, 0.0
	for s := range 3 * rate {
		switch {
		case falling:
			rpm -= 4000.0 / rate
			cut += 1.0 / rate
			if cut >= 0.08 {
				falling = false
			}
		case rpm >= 6000:
			falling, cut = true, 0
		default:
			rpm += 8000.0 / rate
		}
		d.RPM = append(d.RPM, rpm+20*math.Sin(2*math.Pi*200*float64(s)/rate))
	}
	b, err := measureBounce(d, 0, 3, 6000)
	if err != nil {
		t.Fatal(err)
	}
	if b.bounces < 15 || math.Abs(b.cutMS-80) > 2 || math.Abs(b.fallRPMS/4000-1) > 0.03 || math.Abs(b.riseRPMS/8000-1) > 0.03 ||
		math.Abs(b.peak-6000) > 15 || math.Abs(b.trough-5680) > 15 {
		t.Errorf("bounce %+v, want a cut of 80 ms, 4000 and 8000 rpm per second, from 6000 to 5680 rpm", b)
	}
	if _, err := measureBounce(d, 0, 0.1, 6000); err == nil {
		t.Error("a run of 0.1 s gave no error")
	}
}

// TestMeasureIdle checks the idle measure on an RPM of 800 with a hunt of
// 20 rpm at 0.6 Hz and a firing ripple of 30 rpm at 26.7 Hz: the cycle
// mean leaves out the ripple, so the hunt gives the engine a spread of
// 20/sqrt(2) rpm at 0.6 Hz.
func TestMeasureIdle(t *testing.T) {
	const rate = 2000
	d := &analysis.RunnerData{Rate: rate}
	for s := range 11 * rate {
		u := float64(s) / rate
		d.RPM = append(d.RPM, 800+20*math.Sin(2*math.Pi*0.6*u)+30*math.Sin(2*math.Pi*800/30*u))
	}
	r, err := measureIdle(d, 0, 11, 4, 800)
	if err != nil {
		t.Fatal(err)
	}
	want := 20 / math.Sqrt2 / 800 / huntLFOSD
	if math.Abs(r.cycleSD-20/math.Sqrt2) > 0.3 || math.Abs(r.hunt/want-1) > 0.03 || math.Abs(r.huntHz-0.6) > 0.03 || r.st.SD < 20 {
		t.Errorf("idle %+v, want a cycle spread of %.2f rpm, hunt %.4f at 0.6 Hz", r, 20/math.Sqrt2, want)
	}
}

// TestRefFiles checks that the profile of a reference engine parses, and
// that its sources file has one entry for each key that it sets and no
// other entry. The profile takes the embedded v8-ls tables, so the
// channels name their impulse responses.
func TestRefFiles(t *testing.T) {
	var d esDump
	src := `{"simulation_frequency_hz": 10000, "redline_rpm": 6000, "rev_limit_rpm": 6200, "idle": {"measured_rpm": 700},
	"banks": [{"angle_deg": 0}],
	"exhaust_systems": [{"length_m": 2, "audio_volume": 1, "impulse_response": "a.wav"}],
	"cylinders": [{"spark_deg": 0, "spark_enabled": true, "sound_attenuation": 1}, {"spark_deg": 360, "spark_enabled": true, "sound_attenuation": 1}]}`
	if err := json.Unmarshal([]byte(src), &d); err != nil {
		t.Fatal(err)
	}
	e, err := fromEngineSim("test.mr", &d)
	if err != nil {
		t.Fatal(err)
	}
	ir := cycles.Channel{ImpulseResponse: "sound-library/smooth/smooth_39.wav"}
	r := &refProfile{
		re: refEngine{id: "v8-ls", file: "test.mr"}, ref: "ref", scenarios: "scn", grid: "grid", e: e, dump: &d,
		tables:  &cycles.Tables{Channels: []cycles.Channel{ir, ir}, ThrottleLag: cycles.ThrottleLag{Close: 0.07, Open: 0.03}, Points: make([]cycles.Point, 3)},
		stepRPM: 4000,
		idle:    idleRun{st: analysis.RPMStats{Mean: 700, SD: 10, WanderHz: 0.5}, cycleSD: 8, hunt: 0.016, huntHz: 0.5},
		bounce:  limiterBounce{bounces: 5, cutMS: 100, fallRPMS: 3000, riseRPMS: 9000, peak: 6230, trough: 5930},
	}
	r.e.LimiterMS = 100
	text, sources, err := r.files()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snarl.ParseProfile([]byte(text)); err != nil {
		t.Fatalf("%v\n%s", err, text)
	}
	set := map[string]bool{}
	section := ""
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "["):
			section = strings.Trim(line, "[]") + "."
		default:
			k, _, _ := strings.Cut(line, " = ")
			set[section+k] = true
		}
	}
	entries := map[string]bool{}
	for line := range strings.SplitSeq(sources, "\n") {
		if k, ok := strings.CutPrefix(line, "key = "); ok {
			k = strings.Trim(k, `"`)
			if entries[k] || !set[k] {
				t.Errorf("source entry %s is repeated or names no key of the profile", k)
			}
			entries[k] = true
		}
	}
	for k := range set {
		if !entries[k] {
			t.Errorf("key %s has no source entry", k)
		}
	}
	if !entries["channel.impulse_response"] || !entries["throttle.close_lag_s"] || entries["voice.output_db"] || entries["pulse.table"] {
		t.Errorf("entries %v", entries)
	}
	r.ref = `a "quoted" dir`
	if _, _, err := r.files(); err == nil {
		t.Error("a quote in a source gave no error")
	}
}

// TestRefReaders checks the scenario and Nix readers on small files.
func TestRefReaders(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a, b, err := freeSegment(write("idle-free.scn", "# idle\nhold 770 0 1\nsweep 770 800 0 0.5 # x\nfree 0 10\n"))
	if err != nil || a != 1.5 || b != 11.5 {
		t.Errorf("free segment %v to %v, %v, want 1.5 to 11.5", a, b, err)
	}
	if _, _, err := freeSegment(write("none.scn", "hold 770 0 1\n")); err == nil {
		t.Error("a scenario with no free segment gave no error")
	}
	if rev, err := nixRev(write("p.nix", `rev = "85f7c3b959a908ed5232ede4f1a4ac7eafe6b630";`)); err != nil || rev != "85f7c3b" {
		t.Errorf("rev %q, %v", rev, err)
	}
}
