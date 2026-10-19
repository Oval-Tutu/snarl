package main

import (
	"bytes"
	"encoding/csv"
	"strconv"
	"testing"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/record"
)

func TestScriptLog(t *testing.T) {
	ps, i, _, err := loadProfiles("v8", "")
	if err != nil {
		t.Fatal(err)
	}
	var a, b bytes.Buffer
	if err := runScript(&a, ps[i], driveScript); err != nil {
		t.Fatal(err)
	}
	if err := runScript(&b, ps[i], driveScript); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Error("two scripted logs differ")
	}
	rows, err := csv.NewReader(&a).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	// One row per tick for 27.5 seconds at 60 ticks per second, the
	// header, and the end row.
	if len(rows) != 27.5*tps+2 || rows[0][0] != "t" || rows[len(rows)-1][0] != "27.5" {
		t.Fatalf("%d rows, end %v", len(rows), rows[len(rows)-1])
	}
	// The first lift-off is at 6 seconds, in second gear.
	at := func(sec float64, col int) float64 {
		v, err := strconv.ParseFloat(rows[1+int(sec*tps)][col], 64)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	const rpm, throttle, gear, flags, request, strength = 3, 4, 7, 8, 12, 13
	before, after := at(5.99, rpm), at(6.5, rpm)
	if at(5.99, throttle) != 1 || at(6, throttle) != 0 || at(6, gear) != 1 || after >= before ||
		snarl.Flags(at(6.5, flags))&snarl.FlagOverrun == 0 {
		t.Errorf("lift-off: RPM %v then %v, row %v", before, after, rows[1+int(6.5*tps)])
	}
	// Four upshifts at full or part throttle and three downshifts on the
	// coast request pops. The part-throttle upshift is weak, and the
	// downshift into first gear at speed is strong.
	if n := at(27.49, request); n != 7 || at(20, strength) > 1.0/3 || at(16.5, strength) < 2.0/3 {
		t.Errorf("%v pop requests, strengths %v and %v", n, at(20, strength), at(16.5, strength))
	}
	if rows[0][len(rows[0])-1] != "pop_character" || len(rows[0]) != len(splitHeader()) {
		t.Errorf("header %v", rows[0])
	}
}

func splitHeader() []string {
	r, _ := csv.NewReader(bytes.NewBufferString(record.Header)).Read()
	return r
}
