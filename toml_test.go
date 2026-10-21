package snarl

import (
	"errors"
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
cylinders = 8
idle_rpm = 690
redline_rpm = 6500
limiter_rpm = 6800
tables = "v8-ls"
[[channel]]
impulse_response = "smooth_39.wav"
[[channel]]
impulse_response = "smooth_39.wav"
`
	if _, err := parseProfile(good); err != nil {
		t.Fatalf("good profile: %v", err)
	}
	// set adds the line to the table, or to the top level for "".
	set := func(table, line string) string {
		if table == "" {
			return strings.Replace(good, "tables = ", line+"\ntables = ", 1)
		}
		return strings.Replace(good, "[[channel]]", "["+table+"]\n"+line+"\n[[channel]]", 1)
	}
	bad := map[string]string{
		"bad redline":     strings.Replace(good, "redline_rpm = 6500", "redline_rpm = 600", 1),
		"no cylinders":    strings.Replace(good, "cylinders = 8", "cylinders = 0", 1),
		"unknown tables":  strings.Replace(good, `tables = "v8-ls"`, `tables = "nonesuch"`, 1),
		"path tables":     strings.Replace(good, `tables = "v8-ls"`, `tables = "../v8-ls"`, 1),
		"empty tables":    strings.Replace(good, `tables = "v8-ls"`, `tables = ""`, 1),
		"one channel":     strings.Replace(good, "[[channel]]\nimpulse_response = \"smooth_39.wav\"\n", "", 1),
		"no channel":      strings.ReplaceAll(good, "[[channel]]\nimpulse_response = \"smooth_39.wav\"\n", ""),
		"wrong ir":        strings.Replace(good, `"smooth_39.wav"`, `"mild_exhaust.wav"`, 1),
		"missing ir":      strings.Replace(good, `"smooth_39.wav"`, `"nonesuch.wav"`, 1),
		"bad volume":      set("", "volume = 0"),
		"loud volume":     set("", "volume = 1.5"),
		"bad seed":        set("", "seed = -1"),
		"bad close lag":   set("throttle", "close_lag_s = -0.1"),
		"bad open lag":    set("throttle", "open_lag_s = 3"),
		"bad cut":         set("limiter", "cut_ms = 600"),
		"bad fall":        set("limiter", "fall_rpm_s = -1"),
		"bad rise":        set("limiter", "rise_rpm_s = 0"),
		"bad scale":       set("limiter", "load_scale = 0"),
		"deep dip":        set("limiter", "cut_ms = 500\nfall_rpm_s = 20000"),
		"bad waver":       set("limiter", "final_gear_waver = 0.1"),
		"bad idle":        set("idle", "fade_rpm = 500"),
		"bad hunt":        set("idle", "hunt = 0.5"),
		"bad hunt hz":     set("idle", "hunt_hz = 0"),
		"bad character":   set("pops", `character = "loud"`),
		"bad density":     set("pops", "density = -1"),
		"bad body":        set("pops", "body_hz = 0"),
		"bad jump":        set("pops", "jump_ms = 0"),
		"bad weight":      set("pops", "weight_boom = -0.1"),
		"no weight":       set("pops", "weight_pop = 0\nweight_boom = 0\nweight_crackle = 0\nweight_burble = 0"),
		"bad bangs":       set("pops", "limiter_bangs = 1"),
		"unknown channel": strings.Replace(good, `impulse_response = "smooth_39.wav"`, "gain = 1", 1),
	}
	for name, src := range bad {
		if _, err := parseProfile(src); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := parseProfile(strings.Replace(good, `tables = "v8-ls"`, "", 1)); !errors.Is(err, ErrNoTables) {
		t.Errorf("no tables: got %v, want ErrNoTables", err)
	}
	// The keys and tables of the model before the cycle tables are unknown.
	for _, kv := range [][2]string{
		{"", "limiter_kind = \"fuel\""},
		{"", "limiter_ms = 50"},
		{"", "limiter_fall_rpm_s = 6000"},
		{"", "final_gear_waver = 0.003"},
		{"", "cam_switch_rpm = 5800"},
		{"pulse", "table = \"v8-ls\""},
		{"exhaust", "speed_of_sound = 343"},
		{"noise", "am = 1"},
		{"voice", "output_db = 1"},
		{"pops", "crack = 2.5"},
		{"pops", "crack_ms = 16"},
		{"cylinder", "crank_deg = 0"},
		{"path", "collector_m = 1"},
		{"formant", "hz = 400"},
		{"pop_formant", "hz = 400"},
	} {
		src := set(kv[0], kv[1])
		if kv[0] == "cylinder" || kv[0] == "path" || kv[0] == "formant" || kv[0] == "pop_formant" {
			src = good + "[[" + kv[0] + "]]\n" + kv[1] + "\n"
		}
		_, err := parseProfile(src)
		if err == nil || !strings.Contains(err.Error(), "unknown key") && !strings.Contains(err.Error(), "unknown table") &&
			!strings.Contains(err.Error(), "unknown array") {
			t.Errorf("%q: got %v, want an unknown key error", kv, err)
		}
	}
}
