package scenario

import (
	"math"
	"strings"
	"testing"
)

// example is the example in docs/scenario-format.md.
const example = `# Full-throttle sweep, then overrun at the top.
sweep 1500 6800 1 8
hold 6800 0 2
`

func TestParseExample(t *testing.T) {
	s, err := Parse(strings.NewReader(example))
	if err != nil {
		t.Fatal(err)
	}
	want := []Segment{
		{Kind: Sweep, RPM0: 1500, RPM1: 6800, Throttle: 1, Seconds: 8, Line: 2},
		{Kind: Hold, RPM0: 6800, RPM1: 6800, Throttle: 0, Seconds: 2, Line: 3},
	}
	if len(s.Segments) != len(want) {
		t.Fatalf("got %d segments, want %d", len(s.Segments), len(want))
	}
	for i := range want {
		if s.Segments[i] != want[i] {
			t.Errorf("segment %d: got %+v, want %+v", i, s.Segments[i], want[i])
		}
	}
	if d := s.Duration(); d != 10 {
		t.Errorf("Duration() = %v, want 10", d)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"unknown", "hold 1000 0.5 1\n\nidle 800 0 1", "line 3: unknown segment"},
		{"hold fields", "hold 1000 0.5", "line 1: hold needs 3 values"},
		{"sweep fields", "# c\nsweep 1000 2000 1", "line 2: sweep needs 4 values"},
		{"number", "hold 1000 x 1", `line 1: "x" is not a number`},
		{"nan", "hold NaN 0.5 1", `line 1: "NaN" is not a number`},
		{"rpm", "sweep 1000 0 1 1", "line 1: RPM must be positive"},
		{"throttle", "hold 1000 1.5 1", "line 1: throttle must be"},
		{"seconds", "hold 1000 0.5 0", "line 1: seconds must be positive"},
		{"empty", "# nothing\n", "scenario has no segments"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tc.src))
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("got error %v, want prefix %q", err, tc.want)
			}
		})
	}
}

func TestAt(t *testing.T) {
	s, err := Parse(strings.NewReader(example))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		t, rpm, throttle float64
	}{
		{-1, 1500, 1},
		{0, 1500, 1},
		{2, 2825, 1},
		{4, 4150, 1},
		{7.9, 1500 + 5300*7.9/8, 1},
		{8, 6800, 0},
		{9, 6800, 0},
		{10, 6800, 0},
		{11, 6800, 0},
	}
	for _, tc := range tests {
		rpm, throttle := s.At(tc.t)
		if math.Abs(rpm-tc.rpm) > 1e-9 || throttle != tc.throttle {
			t.Errorf("At(%v) = %v, %v, want %v, %v", tc.t, rpm, throttle, tc.rpm, tc.throttle)
		}
	}
}
