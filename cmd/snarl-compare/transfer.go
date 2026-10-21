package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

const (
	// atmospherePa is the pressure that engine-sim subtracts from the
	// exhaust runner pressure, units::pressure(1.0, units::atm).
	atmospherePa = 101325
	// speedOfSound is the speed in m/s of the engine-sim exhaust delay.
	speedOfSound = 343
	// delayRate is the step rate in hertz that the engine-sim exhaust
	// delay filters assume for every engine.
	delayRate = 10000
	// soundTapGain is the gain of the engine-sim sound tap.
	soundTapGain = 1600
	// transferFFT is the frame size of the transfer measurement.
	transferFFT = 8192
	// transferMaxLag is the largest delay in seconds between the input
	// and the output that the transfer measurement searches.
	transferMaxLag = 0.04
)

// engineDump is the part of the engine-sim-render --dump-json output that
// the synthesiser input needs.
type engineDump struct {
	SimulationFrequency float64 `json:"simulation_frequency_hz"`
	ExhaustSystems      []struct {
		Length                float64 `json:"length_m"`
		AudioVolume           float64 `json:"audio_volume"`
		ImpulseResponse       string  `json:"impulse_response"`
		ImpulseResponseVolume float64 `json:"impulse_response_volume"`
	} `json:"exhaust_systems"`
	Cylinders []struct {
		ExhaustSystem    int     `json:"exhaust_system"`
		HeaderPrimary    float64 `json:"header_primary_length_m"`
		SoundAttenuation float64 `json:"sound_attenuation"`
	} `json:"cylinders"`
}

// tap is the gain and the delay in steps of one cylinder in the engine-sim
// sound tap.
type tap struct {
	gain  float64
	delay int
}

// readDump reads an engine dump and returns the sound tap of each
// cylinder and the simulation rate. The gain holds the sound attenuation,
// the audio volume of the exhaust system over the number of cylinders, the
// inverse square of the exhaust length and the impulse response volume.
// The exhaust systems must share one impulse response, so that the sum of
// the taps is the input of one linear system.
func readDump(path string) ([]tap, float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	var d engineDump
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, 0, fmt.Errorf("%s: %w", path, err)
	}
	if d.SimulationFrequency <= 0 || len(d.Cylinders) == 0 || len(d.ExhaustSystems) == 0 {
		return nil, 0, fmt.Errorf("%s: no simulation frequency, cylinders or exhaust systems", path)
	}
	for _, e := range d.ExhaustSystems[1:] {
		if e.ImpulseResponse != d.ExhaustSystems[0].ImpulseResponse {
			return nil, 0, fmt.Errorf("%s: the exhaust systems use different impulse responses", path)
		}
	}
	taps := make([]tap, len(d.Cylinders))
	n := float64(len(d.Cylinders))
	for i, c := range d.Cylinders {
		if c.ExhaustSystem < 0 || c.ExhaustSystem >= len(d.ExhaustSystems) {
			return nil, 0, fmt.Errorf("%s: cylinder %d has no exhaust system %d", path, i, c.ExhaustSystem)
		}
		e := d.ExhaustSystems[c.ExhaustSystem]
		l := c.HeaderPrimary + e.Length
		if l <= 0 {
			return nil, 0, fmt.Errorf("%s: cylinder %d has no exhaust length", path, i)
		}
		taps[i] = tap{
			gain:  soundTapGain * c.SoundAttenuation * e.AudioVolume / n / (l * l) * e.ImpulseResponseVolume,
			delay: int(math.Round(l / speedOfSound * delayRate)),
		}
	}
	return taps, d.SimulationFrequency, nil
}

// readInput reads an engine-sim-render CSV with every step and returns the
// synthesiser input from time 0: the sum over the cylinders of the runner
// pressure above one atmosphere, delayed and scaled by the taps. Before
// the first row, the delayed pressure is 0, as in the engine-sim delay
// filter. The CSV must have one row per simulation step at rate.
func readInput(r io.Reader, taps []tap, rate float64) ([]float64, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	if !sc.Scan() {
		return nil, errors.New("empty CSV")
	}
	cols := map[string]int{}
	for i, h := range strings.Split(sc.Text(), ",") {
		cols[strings.TrimSpace(h)] = i
	}
	timeCol, ok := cols["time_s"]
	if !ok {
		return nil, errors.New("CSV has no time_s column")
	}
	pcol := make([]int, len(taps))
	for i := range taps {
		c, ok := cols["runner_pressure_pa_"+strconv.Itoa(i)]
		if !ok {
			return nil, fmt.Errorf("CSV has no runner_pressure_pa_%d column", i)
		}
		pcol[i] = c
	}
	var times []float64
	p := make([][]float64, len(taps))
	line := 1
	for sc.Scan() {
		line++
		f := strings.Split(sc.Text(), ",")
		t, err := strconv.ParseFloat(f[timeCol], 64)
		if err != nil {
			return nil, fmt.Errorf("CSV line %d: %w", line, err)
		}
		times = append(times, t)
		for i, c := range pcol {
			if c >= len(f) {
				return nil, fmt.Errorf("CSV line %d: too few columns", line)
			}
			v, err := strconv.ParseFloat(f[c], 64)
			if err != nil {
				return nil, fmt.Errorf("CSV line %d: %w", line, err)
			}
			p[i] = append(p[i], v-atmospherePa)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(times) < 2 {
		return nil, errors.New("CSV has fewer than two rows")
	}
	step := (times[len(times)-1] - times[0]) / float64(len(times)-1)
	if math.Abs(step*rate-1) > 0.01 {
		return nil, fmt.Errorf("CSV rows are %.6g s apart, not one step of %.6g s: render it with --csv-every 1", step, 1/rate)
	}
	s0 := 0
	for s0 < len(times) && times[s0] < -0.5/rate {
		s0++
	}
	x := make([]float64, len(times)-s0)
	for i, tp := range taps {
		for s := range x {
			if j := s0 + s - tp.delay; j >= 0 {
				x[s] += tp.gain * p[i][j]
			}
		}
	}
	return x, nil
}

// transfers writes Markdown tables of the transfer from the engine-sim
// synthesiser input, which the CSV and the engine dump give, to the WAV
// output, for each hold of the scenario.
func transfers(w io.Writer, paths []string, cfg *config) error {
	if cfg.scenario == "" || cfg.engine == "" || len(paths) != 1 {
		return errors.New("--transfer needs --scenario, --engine and one WAV file")
	}
	if cfg.settle < 0 {
		return errors.New("--settle must not be negative")
	}
	sc, err := scenario.ParseFile(cfg.scenario)
	if err != nil {
		return err
	}
	taps, simRate, err := readDump(cfg.engine)
	if err != nil {
		return err
	}
	f, err := os.Open(cfg.transfer)
	if err != nil {
		return err
	}
	in, err := readInput(f, taps, simRate)
	f.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", cfg.transfer, err)
	}
	audio, err := wav.ReadFile(paths[0])
	if err != nil {
		return err
	}
	y, rate := audio.Mono(), audio.SampleRate
	x := analysis.Resample(in, int(math.Round(simRate)), rate)
	// The latency of the synthesiser is constant, so one lag over the
	// whole file, in steps of 0.5 ms, serves every hold.
	lag, ok := analysis.Lag(x, y, rate, transferFFT, int(transferMaxLag*float64(rate)), rate/2000)
	if !ok {
		return errors.New("the input or the output is shorter than four frames")
	}
	bands := analysis.ThirdOctaveBands(rate)
	windows := analysis.Windows(sc, float64(min(len(x), len(y)))/float64(rate), analysis.Options{Settle: cfg.settle, Window: 1})
	fmt.Fprintf(w, "## %s\n\nInput: `%s` with `%s`, %g Hz, resampled to %d Hz. The output lags the input by %.2f ms.\n\n",
		paths[0], cfg.transfer, cfg.engine, simRate, rate, 1000*float64(lag)/float64(rate))
	var labels []string
	var rs []analysis.TransferResult
	for _, win := range windows {
		if win.Kind != "hold" {
			continue
		}
		i0, i1 := int(win.Start*float64(rate)), min(int(win.End*float64(rate)), len(x), len(y))
		r, ok := analysis.Transfer(x[i0:i1], y[i0:i1], rate, transferFFT, lag, bands)
		if !ok {
			fmt.Fprintf(w, "%s: too short or silent, no transfer.\n\n", win.Label)
			continue
		}
		labels, rs = append(labels, fmt.Sprintf("%s | %.2f", win.Label, win.Throttle)), append(rs, r)
	}
	if len(rs) == 0 {
		fmt.Fprint(w, "No hold has a transfer.\n\n")
		return nil
	}
	transferTable(w, labels, rs)
	return nil
}

func transferTable(w io.Writer, labels []string, rs []analysis.TransferResult) {
	fmt.Fprint(w, "| Hold | Throttle | Input (dB) | Output (dBFS) | Gain (dB) | Noise (dB) | Frames |\n|"+strings.Repeat(" --- |", 7)+"\n")
	for i, r := range rs {
		fmt.Fprintf(w, "| %s | %.1f | %.1f | %.1f | %.1f | %d |\n", labels[i], r.InputDB, r.OutputDB, r.GainDB, r.NoiseDB, r.Frames)
	}
	fmt.Fprint(w, "\nGain: the output power over the input power. Noise: the output power that the input does not explain (1 minus the coherence, power-weighted) over the output power. With F frames, an output with no relation to the input still gives about 10*log10(1-1/F) dB, and a fully coherent output gives the floor of the estimate.\n\n")
	for _, part := range []struct {
		title string
		value func(b analysis.TransferBand) float64
	}{
		{"Gain per third-octave band: the coherent output power over the input power in the band, in dB.", func(b analysis.TransferBand) float64 { return b.GainDB }},
		{"Noise per third-octave band: the output power that the input does not explain over the output power in the band, in dB.", func(b analysis.TransferBand) float64 { return b.NoiseDB }},
		{"Output per third-octave band: the output power in the band over the whole output power, in dB.", func(b analysis.TransferBand) float64 { return b.OutputDB }},
	} {
		fmt.Fprintf(w, "%s\n\n| Band (Hz) |", part.title)
		for _, l := range labels {
			name, _, _ := strings.Cut(l, " |")
			fmt.Fprintf(w, " %s |", name)
		}
		fmt.Fprint(w, "\n|"+strings.Repeat(" --- |", 1+len(rs))+"\n")
		for j, b := range rs[0].Bands {
			fmt.Fprintf(w, "| %.0f |", math.Sqrt(b.Lo*b.Hi))
			for _, r := range rs {
				if r.Bands[j].Bins == 0 {
					fmt.Fprint(w, " - |")
					continue
				}
				fmt.Fprintf(w, " %.1f |", part.value(r.Bands[j]))
			}
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w)
	}
}
