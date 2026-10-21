package snarl

import (
	"strings"
	"testing"
)

type tomlTestDoc struct {
	Name  string  `toml:"name"`
	Count int     `toml:"count"`
	Ratio float64 `toml:"ratio"`
	On    bool    `toml:"on"`
	Sub   struct {
		X float64 `toml:"x"`
	} `toml:"sub"`
	Items []struct {
		Y float64 `toml:"y"`
	} `toml:"item"`
}

func TestTOMLDecode(t *testing.T) {
	src := `# comment
name = "a # b" # trailing comment
count = 3
ratio = -0.25
on = true

[sub]
x = 1e3

[[item]]
y = 1
[[item]] # second
y = 2
`
	tb, err := parseTOML(src)
	if err != nil {
		t.Fatal(err)
	}
	d := tomlTestDoc{Ratio: 9}
	if err := decodeTOML(tb, &d); err != nil {
		t.Fatal(err)
	}
	if d.Name != "a # b" || d.Count != 3 || d.Ratio != -0.25 || !d.On || d.Sub.X != 1000 ||
		len(d.Items) != 2 || d.Items[1].Y != 2 {
		t.Errorf("decoded %+v", d)
	}
}

func TestTOMLErrors(t *testing.T) {
	cases := map[string]string{
		"unknown key":   "nope = 1",
		"unknown table": "[nope]\nx = 1",
		"duplicate key": "count = 1\ncount = 2",
		"bad value":     "count = one",
		"wrong type":    `count = "1"`,
		"not integer":   "count = 1.5",
		"bad string":    `name = "a\"b"`,
		"table twice":   "[sub]\n[sub]",
		"bad header":    "[sub",
		"no equals":     "count",
		"array table":   "[[sub]]\nx = 1\n[sub]",
	}
	for name, src := range cases {
		tb, err := parseTOML(src)
		if err == nil {
			var d tomlTestDoc
			err = decodeTOML(tb, &d)
		}
		if err == nil {
			t.Errorf("%s: no error for %q", name, src)
		}
	}
}

func TestProfileErrors(t *testing.T) {
	good := `name = "x"
idle_rpm = 800
redline_rpm = 6000
[[cylinder]]
crank_deg = 0
bank = 0
primary_m = 0.4
[[path]]
collector_m = 1
reflection = -0.4
pipe_radius_m = 0.03
outlet_radius_m = 0.03
gain = 1
`
	if _, err := parseProfile(good); err != nil {
		t.Fatalf("good profile: %v", err)
	}
	bad := map[string]string{
		"no path":      strings.Replace(good, "[[path]]", "[[nopath]]", 1),
		"bad bank":     strings.Replace(good, "bank = 0", "bank = 1", 1),
		"bad crank":    strings.Replace(good, "crank_deg = 0", "crank_deg = 720", 1),
		"bad redline":  strings.Replace(good, "redline_rpm = 6000", "redline_rpm = 700", 1),
		"bad limiter":  good + "\nlimiter_kind = \"spark\"",
		"bad fall":     strings.Replace(good, "redline_rpm = 6000", "redline_rpm = 6000\nlimiter_fall_rpm_s = -1", 1),
		"bad rise":     strings.Replace(good, "redline_rpm = 6000", "redline_rpm = 6000\nlimiter_rise_rpm_s = 0", 1),
		"bad scale":    strings.Replace(good, "redline_rpm = 6000", "redline_rpm = 6000\nlimiter_load_scale = 0", 1),
		"bad injector": strings.Replace(good, "redline_rpm = 6000", "redline_rpm = 6000\nlimiter_kind = \"injector\"", 1),
		"bad pump":     strings.Replace(good, "redline_rpm = 6000", "redline_rpm = 6000\nlimiter_pump_db = 1", 1),
		"bad notch":    strings.Replace(good, "redline_rpm = 6000", "redline_rpm = 6000\nlimiter_notch_db = 0", 1),
		"bad bark":     strings.Replace(good, "redline_rpm = 6000", "redline_rpm = 6000\nlimiter_bark_db = -1", 1),
		"bad jitter":   strings.Replace(good, "redline_rpm = 6000", "redline_rpm = 6000\nlimiter_jitter = 0.6", 1),
		"bad waver":    strings.Replace(good, "redline_rpm = 6000", "redline_rpm = 6000\nfinal_gear_waver = 0.1", 1),
		"bad crossing": strings.Replace(good, "[[cylinder]]", "[exhaust]\ncrossover = \"some\"\n[[cylinder]]", 1),
		"bad shelf":    strings.Replace(good, "[[cylinder]]", "[exhaust]\nlow_shelf_db = 30\n[[cylinder]]", 1),
		"bad am cut":   strings.Replace(good, "[[cylinder]]", "[noise]\nam_cutoff_hz = 9000\n[[cylinder]]", 1),
		"bad am":       strings.Replace(good, "[[cylinder]]", "[noise]\nam = 1.5\n[[cylinder]]", 1),
		"bad hf gain":  strings.Replace(good, "[[cylinder]]", "[noise]\nhf_gain = -0.1\n[[cylinder]]", 1),
		"bad overrun":  strings.Replace(good, "[[cylinder]]", "[overrun]\nlevel_db = 10\n[[cylinder]]", 1),
		"bad idle":     strings.Replace(good, "[[cylinder]]", "[idle]\nfade_rpm = 500\n[[cylinder]]", 1),
		"bad hunt":     strings.Replace(good, "[[cylinder]]", "[idle]\nhunt = 0.5\n[[cylinder]]", 1),
		"bad leveller": strings.Replace(good, "[[cylinder]]", "[voice]\nleveller_db = 70\n[[cylinder]]", 1),
		"bad weight":   strings.Replace(good, "[[cylinder]]", "[pops]\nweight_boom = -0.1\n[[cylinder]]", 1),
		"no weight":    strings.Replace(good, "[[cylinder]]", "[pops]\nweight_pop = 0\nweight_boom = 0\nweight_crackle = 0\nweight_burble = 0\n[[cylinder]]", 1),
	}
	for name, src := range bad {
		if _, err := parseProfile(src); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	// The removed keys are unknown keys, and the removed [mechanical] table
	// is an unknown table.
	for _, kv := range []string{
		"[noise]\nair = 0.03", "[noise]\nair_cutoff_hz = 1800", "[voice]\ntier1_db = 1", "[voice]\ntier1_idle_gain_db = 1",
		"[noise]\nintake = 0.08", "[noise]\nintake_hz = 420", "[noise]\nintake_cam_gain = 1.2", "[noise]\nintake_cam_hz = 560",
		"[mechanical]\ntick_gain = 0.01", "[mechanical]\ntick_hz = 3000", "[mechanical]\ntick_lp_hz = 9000",
		"[mechanical]\nfan_ratio = 1.8", "[mechanical]\nfan_blades = 5", "[mechanical]\nfan_gain = 0.02",
		"[voice]\nidle_gain_db = 1", "[voice]\nidle_gain_rpm = 3000", "[noise]\nam_redline = 0.25", "[noise]\nam_idle = 0.5",
	} {
		_, err := parseProfile(strings.Replace(good, "[[cylinder]]", kv+"\n[[cylinder]]", 1))
		if err == nil || !strings.Contains(err.Error(), "unknown key") && !strings.Contains(err.Error(), "unknown table mechanical") {
			t.Errorf("%q: got %v, want an unknown key error", kv, err)
		}
	}
}
