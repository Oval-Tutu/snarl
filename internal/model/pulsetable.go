package model

import (
	"bufio"
	"embed"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
)

// pulseFiles holds the pulse tables. tools/fit --pulse writes each from
// the engine-sim runner pressure of a profile script.
//
//go:embed pulses/*.txt
var pulseFiles embed.FS

// pulseTable is the pulse shape from engine-sim (measured): one row per
// RPM of the measured holds, in rising RPM. Each row has the same number
// of values at equal steps from the start to the end of the pulse, with a
// peak magnitude of 1 and a last value of 0, and lasts lenSec seconds.
type pulseTable struct {
	rpm, lenSec []float64
	v           [][]float32
	// gainSD is the cycle-to-cycle variation of the firings at the RPM of
	// each row, from engine-sim (measured): the standard deviation of the
	// gain of a firing about the mean of its cylinder, relative to the mean
	// gain. It is 0 when the file has no spread lines.
	gainSD []float64
	// blend holds the blends of each two neighbouring rows: blend[a][j]
	// is row a and row a+1 weighted j/blendSteps. A voice takes one blend
	// when it fires and keeps it, so that no table changes under a pulse
	// that sounds.
	blend [][][]float32
}

// blendSteps is the number of steps of the weight between two rows.
const blendSteps = 16

// pulseCache holds the parsed tables by name.
var pulseCache sync.Map

// loadPulse returns the embedded pulse table of the name.
func loadPulse(name string) (*pulseTable, error) {
	if t, ok := pulseCache.Load(name); ok {
		return t.(*pulseTable), nil
	}
	if name == "" || strings.ContainsAny(name, "/\\.") {
		return nil, fmt.Errorf("pulse table %q is not known", name)
	}
	data, err := pulseFiles.ReadFile("pulses/" + name + ".txt")
	if err != nil {
		return nil, fmt.Errorf("pulse table %q is not known", name)
	}
	t, err := parsePulse(string(data))
	if err != nil {
		return nil, fmt.Errorf("pulse table %s: %w", name, err)
	}
	pulseCache.Store(name, t)
	return t, nil
}

// parsePulse parses a pulse table file: # comments, one line per row with
// the word row, the RPM, the length in milliseconds and the values, and
// optionally one line per row with the word spread, the RPM of the row
// and the standard deviation of the firing gain.
func parsePulse(data string) (*pulseTable, error) {
	t := &pulseTable{}
	var spread [][2]float64
	sc := bufio.NewScanner(strings.NewReader(data))
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	for sc.Scan() {
		line, _, _ := strings.Cut(sc.Text(), "#")
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if f[0] == "spread" && len(f) == 3 {
			var s [2]float64
			for i := range s {
				v, err := strconv.ParseFloat(f[1+i], 64)
				if err != nil {
					return nil, err
				}
				s[i] = v
			}
			if s[1] < 0 || s[1] > 1 {
				return nil, errors.New("want a spread gain from 0 to 1")
			}
			spread = append(spread, s)
			continue
		}
		if f[0] != "row" || len(f) < 3+16 {
			return nil, fmt.Errorf("bad line %.40q", line)
		}
		var nums [2]float64
		for i := range nums {
			v, err := strconv.ParseFloat(f[1+i], 64)
			if err != nil {
				return nil, err
			}
			nums[i] = v
		}
		vals := make([]float32, len(f)-3)
		for i, s := range f[3:] {
			v, err := strconv.ParseFloat(s, 32)
			if err != nil {
				return nil, err
			}
			vals[i] = float32(v)
		}
		// The pulse ends at 0, so that a voice that reaches the end adds no
		// step.
		vals[len(vals)-1] = 0
		switch {
		case nums[0] <= 0 || nums[1] <= 0 || nums[1] > 200:
			return nil, errors.New("want an RPM above 0 and a length above 0 to 200 ms")
		case len(t.rpm) > 0 && (nums[0] <= t.rpm[len(t.rpm)-1] || len(vals) != len(t.v[0])):
			return nil, errors.New("want rows in rising RPM with the same number of values")
		}
		t.rpm = append(t.rpm, nums[0])
		t.lenSec = append(t.lenSec, nums[1]*1e-3)
		t.v = append(t.v, vals)
	}
	if len(t.rpm) == 0 {
		return nil, errors.New("no row")
	}
	t.gainSD = make([]float64, len(t.rpm))
	if len(spread) > 0 {
		if len(spread) != len(t.rpm) {
			return nil, fmt.Errorf("%d spread lines for %d rows", len(spread), len(t.rpm))
		}
		for i, s := range spread {
			if math.Abs(s[0]-t.rpm[i]) > 0.5 {
				return nil, fmt.Errorf("spread line %d is at %g RPM, its row at %g RPM", i+1, s[0], t.rpm[i])
			}
			t.gainSD[i] = s[1]
		}
	}
	t.blend = make([][][]float32, len(t.rpm)-1)
	for a := range t.blend {
		t.blend[a] = make([][]float32, blendSteps+1)
		ra, rb := t.v[a], t.v[a+1]
		for j := range t.blend[a] {
			w := float32(j) / blendSteps
			r := make([]float32, len(ra))
			for i := range r {
				r[i] = ra[i] + (rb[i]-ra[i])*w
			}
			t.blend[a][j] = r
		}
	}
	return t, nil
}

// at returns the blend of rows a and b at the weight w of row b, to the
// nearest step.
func (t *pulseTable) at(a, b int, w float64) []float32 {
	j := int(w*blendSteps + 0.5)
	switch {
	case a == b || j <= 0:
		return t.v[a]
	case j >= blendSteps:
		return t.v[b]
	}
	return t.blend[a][j]
}

// points returns the number of values of each row.
func (t *pulseTable) points() int {
	return len(t.v[0])
}

// bracket returns the rows j-1 and j around the RPM, and the weight of row
// j from 0 to 1 by the distance in RPM. Below the first row and above the
// last, both rows are that row.
func (t *pulseTable) bracket(rpm float64) (a, b int, w float64) {
	n := len(t.rpm)
	switch {
	case rpm <= t.rpm[0]:
		return 0, 0, 0
	case rpm >= t.rpm[n-1]:
		return n - 1, n - 1, 0
	}
	j := 1
	for t.rpm[j] < rpm {
		j++
	}
	return j - 1, j, (rpm - t.rpm[j-1]) / (t.rpm[j] - t.rpm[j-1])
}
