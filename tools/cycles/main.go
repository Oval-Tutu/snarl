// Command cycles builds and plays the cycle tables of the engine-sim
// engines (internal/cycles, docs/research/cycle-tables.md).
//
// Usage:
//
//	go run ./tools/cycles scenarios --holds HOLDS.scn --name ENGINE --out DIR [--step RPM]
//	go run ./tools/cycles build --out TABLES.bin [--settle S] [--thin N] RECORD...
//	go run ./tools/cycles play --tables TABLES.bin --es ES --out-dir DIR SCENARIO...
//	go run ./tools/cycles spectrum --record DIR --rpm RPM [--cut] [--channel N]
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "cycles:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("need a command: scenarios, build, play or spectrum")
	}
	switch args[0] {
	case "scenarios":
		return runScenarios(args[1:], stdout)
	case "build":
		return runBuild(args[1:], stdout)
	case "play":
		return runPlay(args[1:], stdout)
	case "spectrum":
		return runSpectrum(args[1:], stdout)
	}
	return fmt.Errorf("unknown command %q, want scenarios, build, play or spectrum", args[0])
}

func newFlags(name string) *flag.FlagSet {
	return flag.NewFlagSet("cycles "+name, flag.ContinueOnError)
}
