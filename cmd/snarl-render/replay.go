package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/oval-tutu/snarl"
)

// logHeader is the header row of the snapshot log that snarl-harness
// writes (cmd/snarl-harness/internal/record). Each row is one Engine.Set
// call. t is the time in seconds from the start of the recording. The last
// row is at the stop time, so the log lasts until its last row.
const logHeader = "t,profile,tier,rpm,throttle,load,speed,gear,flags,shift_up,shift_down,limiter"

// logRow is one row of a snapshot log.
type logRow struct {
	t       float64
	profile string
	tier    snarl.Tier
	snap    snarl.Snapshot
}

// readLog reads a snapshot log. The times must not fall.
func readLog(path string) ([]logRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := parseLog(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return rows, nil
}

func parseLog(r io.Reader) ([]logRow, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = len(strings.Split(logHeader, ","))
	head, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("header: %w", err)
	}
	if strings.Join(head, ",") != logHeader {
		return nil, fmt.Errorf("header is not %q", logHeader)
	}
	var rows []logRow
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		line, _ := cr.FieldPos(0)
		row, err := parseRow(rec)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if n := len(rows); n > 0 && row.t < rows[n-1].t {
			return nil, fmt.Errorf("line %d: time %v falls", line, row.t)
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, errors.New("log has no snapshots")
	}
	return rows, nil
}

// parseRow parses one row. The fields after the profile are numbers, and
// the tier and the fields from gear on are bytes.
func parseRow(rec []string) (logRow, error) {
	var nums [11]float64
	for i, f := range slices.Concat(rec[:1], rec[2:]) {
		x, err := strconv.ParseFloat(f, 64)
		if err != nil || math.IsNaN(x) || math.IsInf(x, 0) {
			return logRow{}, fmt.Errorf("%q is not a number", f)
		}
		nums[i] = x
	}
	var b [6]uint8
	for i, x := range slices.Concat(nums[1:2], nums[6:]) {
		if x < 0 || x > math.MaxUint8 || x != math.Trunc(x) {
			return logRow{}, fmt.Errorf("%v is not a whole number from 0 to 255", x)
		}
		b[i] = uint8(x)
	}
	if nums[0] < 0 || (b[0] != uint8(snarl.Tier1) && b[0] != uint8(snarl.Tier2)) {
		return logRow{}, errors.New("time is negative or tier is not 1 or 2")
	}
	return logRow{
		t:       nums[0],
		profile: rec[1],
		tier:    snarl.Tier(b[0]),
		snap: snarl.Snapshot{
			RPM:       float32(nums[2]),
			Throttle:  float32(nums[3]),
			Load:      float32(nums[4]),
			Speed:     float32(nums[5]),
			Gear:      b[1],
			Flags:     snarl.Flags(b[2]),
			ShiftUp:   b[3],
			ShiftDown: b[4],
			Limiter:   b[5],
		},
	}, nil
}

// replayer applies the snapshots of a log at their times. It builds a new
// engine when the profile or the tier changes, as snarl-harness does.
type replayer struct {
	rows []logRow
	i    int
	rate int
	sets []string

	eng     snarl.Engine
	profile string
	tier    snarl.Tier
	last    snarl.Snapshot
}

// at returns the engine and the latest snapshot at time t. Before the
// first row, it returns the first row.
func (r *replayer) at(t float64) (snarl.Engine, snarl.Snapshot, error) {
	for r.i+1 < len(r.rows) && r.rows[r.i+1].t <= t {
		r.i++
	}
	row := r.rows[r.i]
	if r.eng == nil || row.profile != r.profile || row.tier != r.tier {
		p, err := snarl.LoadProfileWith(row.profile, r.sets...)
		if err != nil {
			return nil, snarl.Snapshot{}, err
		}
		e, err := snarl.New(p, r.rate, row.tier)
		if err != nil {
			return nil, snarl.Snapshot{}, err
		}
		if r.eng != nil {
			e.Set(r.last)
		}
		r.eng, r.profile, r.tier = e, row.profile, row.tier
	}
	r.last = row.snap
	return r.eng, row.snap, nil
}
