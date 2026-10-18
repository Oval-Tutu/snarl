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
	ps, i, err := loadProfiles("v8")
	if err != nil {
		t.Fatal(err)
	}
	var a, b bytes.Buffer
	if err := runScript(&a, ps[i], liftOffScript); err != nil {
		t.Fatal(err)
	}
	if err := runScript(&b, ps[i], liftOffScript); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Error("two scripted logs differ")
	}
	rows, err := csv.NewReader(&a).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	// One row per tick for 17 seconds at 60 ticks per second, the header,
	// and the end row.
	if len(rows) != 17*tps+2 || rows[0][0] != "t" || rows[len(rows)-1][0] != "17" {
		t.Fatalf("%d rows, end %v", len(rows), rows[len(rows)-1])
	}
	// The first lift-off is at 5 seconds, in second gear.
	at := func(sec float64, col int) float64 {
		v, err := strconv.ParseFloat(rows[1+int(sec*tps)][col], 64)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	const rpm, throttle, gear, flags = 3, 4, 7, 8
	before, after := at(4.99, rpm), at(5.5, rpm)
	if at(4.99, throttle) != 1 || at(5, throttle) != 0 || at(5, gear) != 1 || after >= before ||
		snarl.Flags(at(5.5, flags))&snarl.FlagOverrun == 0 {
		t.Errorf("lift-off: RPM %v then %v, row %v", before, after, rows[1+int(5.5*tps)])
	}
	if rows[0][len(rows[0])-1] != "limiter" || len(rows[0]) != len(splitHeader()) {
		t.Errorf("header %v", rows[0])
	}
}

func splitHeader() []string {
	r, _ := csv.NewReader(bytes.NewBufferString(record.Header)).Read()
	return r
}
