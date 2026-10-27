package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// scoreCell is one hold of the full-range matrix of tools/scorecard
// --matrix: its score and the worst band above 2 kHz.
type scoreCell struct {
	score, band float64
}

// passes reports whether the hold passes: a score of 95 or more and the
// worst band within 1.5 dB.
func (c scoreCell) passes() bool { return c.score >= 95 && c.band <= 1.5 && c.band >= -1.5 }

// runScoreDiff prints the holds of two full-range scorecards (tools/scorecard
// --matrix) that passed in the first and score lower in the second by more
// than --margin, or miss there, and counts the holds that pass in each.
func runScoreDiff(args []string, stdout io.Writer) error {
	fs := newFlags("scorediff")
	margin := fs.Float64("margin", 1, "the least loss of score that counts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return errors.New("need the scorecard before and after")
	}
	before, err := readMatrix(fs.Arg(0))
	if err != nil {
		return err
	}
	after, err := readMatrix(fs.Arg(1))
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, "| Hold | Before | After |\n| --- | --- | --- |\n")
	var passBefore, passAfter, n int
	for _, k := range before.order {
		b := before.cells[k]
		a, ok := after.cells[k]
		if !ok {
			continue
		}
		n++
		if b.passes() {
			passBefore++
		}
		if a.passes() {
			passAfter++
		}
		if b.passes() && (!a.passes() || a.score < b.score-*margin) {
			fmt.Fprintf(stdout, "| %s | %.1f %+.1f | %.1f %+.1f |\n", k, b.score, b.band, a.score, a.band)
		}
	}
	fmt.Fprintf(stdout, "\n%d holds in both: %d pass before, %d after\n", n, passBefore, passAfter)
	return nil
}

// matrix holds the cells of a scorecard by engine, column and RPM, in the
// order of the file.
type matrix struct {
	cells map[string]scoreCell
	order []string
}

// readMatrix reads the hold matrices of a tools/scorecard --matrix report:
// under each "### engine" heading, tables whose header row starts with
// "| RPM |" and whose cells start with the score and the worst band.
func readMatrix(path string) (*matrix, error) {
	f, err := os.Open(path) //nolint:gosec // The report is a user-supplied input.
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := &matrix{cells: map[string]scoreCell{}}
	var engine string
	var cols []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if e, ok := strings.CutPrefix(line, "### "); ok {
			engine, cols = e, nil
			continue
		}
		if !strings.HasPrefix(line, "| ") {
			cols = nil
			continue
		}
		fields := strings.Split(strings.Trim(line, "| "), "|")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		if fields[0] == "RPM" {
			cols = fields[1:]
			continue
		}
		if cols == nil || engine == "" {
			continue
		}
		if _, err := strconv.ParseFloat(fields[0], 64); err != nil {
			continue
		}
		for i, c := range fields[1:] {
			v := strings.Fields(c)
			if i >= len(cols) || len(v) < 2 {
				continue
			}
			score, err1 := strconv.ParseFloat(v[0], 64)
			band, err2 := strconv.ParseFloat(v[1], 64)
			if err1 != nil || err2 != nil {
				continue
			}
			k := fmt.Sprintf("%s %s %s rpm", engine, cols[i], fields[0])
			if _, ok := m.cells[k]; !ok {
				m.order = append(m.order, k)
			}
			m.cells[k] = scoreCell{score, band}
		}
	}
	return m, sc.Err()
}
