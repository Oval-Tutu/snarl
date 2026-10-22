// Package crackle is a real-time model of a short exterior exhaust crackle:
// a random train of shocks over a bed of crackle noise and a low body
// (docs/research/crackle-model.md). The shocks come at random gaps with
// random levels from the measured tables. Each shock is a crack with a
// linear rise and a fast decay, and a ring with a slow decay, each through
// a bank of octave band filters. The bed is a third bank, and the activity
// of the shocks plays the ring bank as a slow ring. The body is white
// noise through a resonant band pass, independent of the shocks, and at
// the end of a train a damped sine is the tail. NewBank renders the shocks
// as a table of grains and the bed, the ring and the body as loops once,
// so that a voice only adds grains and reads loops. The class-level
// parameters in crackle.toml are statistics and fits of the set of clips
// that tools/crackle measures, with their sources in crackle.sources.toml.
package crackle

import (
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// Bands is the number of octave bands of the filter banks.
const Bands = 11

// BandEdges are the edges of the bands of the filter banks in hertz, the
// bands of the measures of tools/crackle.
var BandEdges = [Bands][2]float64{
	{15.625, 31.25}, {31.25, 62.5}, {62.5, 125}, {125, 250}, {250, 500}, {500, 1000},
	{1000, 2000}, {2000, 4000}, {4000, 8000}, {8000, 16000}, {16000, 20000},
}

// Quantiles is the number of values of a quantile table of the gaps and
// the levels. Value i is the quantile at the probability (i + 0.5) /
// Quantiles, so that the table holds no extreme value of the set.
const Quantiles = 21

// Params are the class-level parameters of the model.
type Params struct {
	// LengthMS holds the minimum, the quartiles and the maximum of the
	// active length of a train. LengthOffsetMS is the part of the active
	// length after the span in which shocks start.
	LengthMS       [5]float64
	LengthOffsetMS float64
	// ShocksMin and ShocksMax are the range of the count of shocks of a
	// train for the length control.
	ShocksMin, ShocksMax int
	// GapMS is the quantile table of the gaps between the shocks, and
	// LevelDB that of the shock levels in dB. A gap is a value of GapMS
	// times GapScale, and a level is the top of LevelDB plus LevelScale
	// times the distance of a value of LevelDB below it.
	GapMS, LevelDB       [Quantiles]float64
	GapScale, LevelScale float64
	// LevelCorr is the correlation of the levels of consecutive shocks
	// (Voice.level).
	LevelCorr float64
	// PlaceDB is the level offset of a shock in each tenth of the train,
	// and PlaceWeight the part of it that the top level does not take
	// (Voice.level).
	PlaceDB     [10]float64
	PlaceWeight float64
	// AttackMS is the linear rise of the shock envelope. From the peak, the
	// crack falls with the time constant DecayMS and the ring with RingMS.
	AttackMS, DecayMS, RingMS float64
	// ShockDB, RingDB and BedDB are the gains of the bands of the banks of
	// the crack, the ring and the bed, for a shock at 0 dB. TopHz is the
	// corner of the eighth-order Butterworth low pass after the banks.
	ShockDB, RingDB, BedDB [Bands]float64
	TopHz                  float64
	// BodyHz and BodyQ set the resonant band pass of the body, and
	// BodyGainDB its gain.
	BodyHz, BodyQ, BodyGainDB float64
	// FadeDelayMS is the time from the onset of the last shock to the
	// start of the fade at the end, and FadeDBPerMS the rate of the fade.
	// FadeInMS is the linear fade at the start.
	FadeDelayMS, FadeDBPerMS, FadeInMS float64
	// The activity of the shocks plays the ring bank as a slow ring: each
	// shock adds its amplitude, which decays with the time constant ActMS,
	// at the gain ActDB.
	ActMS, ActDB float64
	// At the fade, the tail starts: a damped sine at TailHz that falls
	// 20 dB in TailDecayMS, with the amplitude TailGainDB.
	TailHz, TailDecayMS, TailGainDB float64
	// OutDB is the output gain at level 0 dB.
	OutDB float64
}

// key is one key of the parameter file and the values that it sets. An
// array key has its values in brackets, also when it has one.
type key struct {
	name  string
	v     []*float64
	array bool
}

func one(name string, x *float64) key { return key{name: name, v: []*float64{x}} }

func array(name string, a []float64) key {
	k := key{name: name, array: true}
	for i := range a {
		k.v = append(k.v, &a[i])
	}
	return k
}

// keys names the values of the parameter file, in the order of the file.
// shocks holds the shock range, which the file has as numbers.
func (p *Params) keys(shocks *[2]float64) []key {
	return []key{
		array("length_ms", p.LengthMS[:]), one("length_offset_ms", &p.LengthOffsetMS),
		array("shocks_range", shocks[:]),
		array("gap_ms", p.GapMS[:]), one("gap_scale", &p.GapScale),
		array("level_db", p.LevelDB[:]), one("level_scale", &p.LevelScale), one("level_corr", &p.LevelCorr), array("place_db", p.PlaceDB[:]), one("place_weight", &p.PlaceWeight),
		one("attack_ms", &p.AttackMS), one("decay_ms", &p.DecayMS), one("ring_ms", &p.RingMS),
		array("shock_db", p.ShockDB[:]), array("ring_db", p.RingDB[:]), array("bed_db", p.BedDB[:]), one("top_hz", &p.TopHz),
		one("body_hz", &p.BodyHz), one("body_q", &p.BodyQ), one("body_gain_db", &p.BodyGainDB),
		one("fade_delay_ms", &p.FadeDelayMS), one("fade_db_per_ms", &p.FadeDBPerMS),
		one("fade_in_ms", &p.FadeInMS), one("act_ms", &p.ActMS), one("act_db", &p.ActDB),
		one("tail_hz", &p.TailHz), one("tail_decay_ms", &p.TailDecayMS), one("tail_gain_db", &p.TailGainDB),
		one("out_db", &p.OutDB),
	}
}

// Keys returns the names of the values of the parameter file, in the
// order of the file.
func (p *Params) Keys() []string {
	var out []string
	for _, k := range p.keys(&[2]float64{}) {
		out = append(out, k.name)
	}
	return out
}

// Marshal returns the parameter file: a header of comment lines, then one
// line for each key, a number or an array of numbers.
func (p *Params) Marshal(header string) []byte {
	var b strings.Builder
	for _, l := range strings.Split(strings.TrimSpace(header), "\n") {
		b.WriteString(strings.TrimRight("# "+l, " ") + "\n")
	}
	b.WriteString("\n")
	for _, k := range p.keys(&[2]float64{float64(p.ShocksMin), float64(p.ShocksMax)}) {
		var s []string
		for _, v := range k.v {
			s = append(s, strconv.FormatFloat(*v, 'g', 6, 64))
		}
		if !k.array {
			fmt.Fprintf(&b, "%s = %s\n", k.name, s[0])
		} else {
			fmt.Fprintf(&b, "%s = [%s]\n", k.name, strings.Join(s, ", "))
		}
	}
	return []byte(b.String())
}

// Parse reads a parameter file. Each key must be in the file once, with
// the count of values of its field. Unknown keys are errors.
func Parse(data []byte) (*Params, error) {
	p := &Params{}
	var shocks [2]float64
	keys := p.keys(&shocks)
	want := map[string][]*float64{}
	for _, k := range keys {
		want[k.name] = k.v
	}
	seen := map[string]bool{}
	for n, line := range strings.Split(string(data), "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		name, val, ok := strings.Cut(line, "=")
		name, val = strings.TrimSpace(name), strings.TrimSpace(val)
		dst, known := want[name]
		switch {
		case !ok:
			return nil, fmt.Errorf("line %d: no = in %q", n+1, line)
		case !known:
			return nil, fmt.Errorf("line %d: unknown key %q", n+1, name)
		case seen[name]:
			return nil, fmt.Errorf("line %d: key %q again", n+1, name)
		}
		seen[name] = true
		var fields []string
		if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
			fields = strings.Split(val[1:len(val)-1], ",")
		} else {
			fields = []string{val}
		}
		if len(fields) != len(dst) {
			return nil, fmt.Errorf("line %d: %q has %d values, want %d", n+1, name, len(fields), len(dst))
		}
		for i, f := range fields {
			v, err := strconv.ParseFloat(strings.TrimSpace(f), 64)
			if err != nil {
				return nil, fmt.Errorf("line %d: %q: %w", n+1, name, err)
			}
			*dst[i] = v
		}
	}
	for _, k := range keys {
		if !seen[k.name] {
			return nil, fmt.Errorf("no key %q", k.name)
		}
	}
	p.ShocksMin, p.ShocksMax = int(shocks[0]), int(shocks[1])
	return p, p.check()
}

// check reports parameters that the voice cannot use.
func (p *Params) check() error {
	switch {
	case p.PlaceWeight < 0 || p.PlaceWeight > 1:
		return errors.New("place_weight must be from 0 to 1")
	case p.LevelCorr <= -1 || p.LevelCorr >= 1:
		return errors.New("level_corr must be inside -1 to 1")
	case p.ShocksMin < 1 || p.ShocksMax < p.ShocksMin:
		return errors.New("shocks_range must be 1 or more and in order")
	case p.LengthMS[0] <= 0 || p.GapMS[0] <= 0:
		return errors.New("length_ms and gap_ms must be above 0")
	case p.GapScale <= 0 || p.LevelScale <= 0 || p.AttackMS <= 0 || p.DecayMS <= 0 || p.RingMS <= 0 || p.FadeDBPerMS <= 0:
		return errors.New("gap_scale, level_scale, attack_ms, decay_ms, ring_ms and fade_db_per_ms must be above 0")
	case p.BodyHz <= 0 || p.BodyQ <= 0 || p.TopHz <= 0 || p.TailHz <= 0 || p.TailDecayMS <= 0 || p.ActMS <= 0:
		return errors.New("body_hz, body_q, top_hz, tail_hz, tail_decay_ms and act_ms must be above 0")
	case p.FadeDelayMS < 0 || p.FadeInMS < 0:
		return errors.New("fade_delay_ms and fade_in_ms must not be negative")
	}
	for _, t := range [][]float64{p.LengthMS[:], p.GapMS[:], p.LevelDB[:]} {
		for i := 1; i < len(t); i++ {
			if t[i] < t[i-1] {
				return errors.New("length_ms, gap_ms and level_db must be in order")
			}
		}
	}
	return nil
}

//go:embed crackle.toml
var paramFile []byte

var defaults = sync.OnceValues(func() (*Params, error) { return Parse(paramFile) })

// Fitted returns the fitted parameters of crackle.toml, or the error of
// the file.
func Fitted() (*Params, error) {
	p, err := defaults()
	if err != nil {
		return nil, fmt.Errorf("crackle: crackle.toml: %w", err)
	}
	c := *p
	return &c, nil
}

// Default returns the fitted parameters of crackle.toml, and panics on an
// error of the file.
func Default() *Params {
	p, err := Fitted()
	if err != nil {
		panic(err.Error())
	}
	return p
}
