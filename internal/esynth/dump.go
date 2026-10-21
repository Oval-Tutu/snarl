package esynth

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/oval-tutu/snarl/internal/wav"
)

// Dump is a synthesiser record of engine-sim-render --dump-synth
// (reference/engine-sim/README.md): the input of each channel per
// simulation step, the parameters, the frames, and the whole output.
type Dump struct {
	Dir                 string          `json:"-"`
	Engine              string          `json:"engine"`
	Script              string          `json:"script"`
	NoNoise             bool            `json:"no_noise"`
	SimulationFrequency float64         `json:"simulation_frequency_hz"`
	InputSampleRate     float64         `json:"input_sample_rate_hz"`
	AudioSampleRate     float64         `json:"audio_sample_rate_hz"`
	InputChannelCount   int             `json:"input_channel_count"`
	InputBufferSize     int             `json:"input_buffer_size"`
	AudioBufferSize     int             `json:"audio_buffer_size"`
	Audio               AudioParameters `json:"audio_parameters"`
	Channels            []DumpChannel   `json:"channels"`
	Output              string          `json:"output"`
	OutputSamples       int             `json:"output_samples"`
	SettleSamples       int             `json:"settle_samples"`
	KeptSamples         int             `json:"kept_samples"`
	Frames              []Frame         `json:"frames"`
	// Inputs holds the input of each channel, one sample per step.
	Inputs [][]float64 `json:"-"`
}

// DumpChannel is one exhaust channel of a Dump.
type DumpChannel struct {
	Input string `json:"input"`
	// ImpulseResponse is the impulse response path relative to es/.
	ImpulseResponse string `json:"impulse_response"`
	// ImpulseResponseCopy is the copy of the file in the dump directory.
	ImpulseResponseCopy string  `json:"impulse_response_copy"`
	FileSamples         int     `json:"impulse_response_file_samples"`
	Taps                int     `json:"impulse_response_taps"`
	Volume              float64 `json:"impulse_response_volume"`
}

// Frame is one renderer frame: its simulation steps, then its render
// blocks. Each block is the samples that Synthesizer::renderAudio()
// rendered and the rand() calls outside the synthesiser before the block.
type Frame struct {
	Steps  int        `json:"steps"`
	Blocks [][2]int64 `json:"blocks"`
}

// LoadDump reads the record in dir.
func LoadDump(dir string) (*Dump, error) {
	b, err := os.ReadFile(filepath.Join(dir, "synth.json")) //nolint:gosec // The dump directory is a user-supplied input.
	if err != nil {
		return nil, err
	}
	d := &Dump{Dir: dir}
	if err := json.Unmarshal(b, d); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	if d.InputChannelCount < 1 || len(d.Channels) != d.InputChannelCount {
		return nil, fmt.Errorf("%s: %d channels listed for %d input channels", dir, len(d.Channels), d.InputChannelCount)
	}
	steps := 0
	for _, f := range d.Frames {
		steps += f.Steps
	}
	for _, c := range d.Channels {
		in, err := readFloat64s(filepath.Join(dir, c.Input))
		if err != nil {
			return nil, err
		}
		if len(in) != steps {
			return nil, fmt.Errorf("%s: %d samples for %d steps", c.Input, len(in), steps)
		}
		d.Inputs = append(d.Inputs, in)
	}
	return d, nil
}

func readFloat64s(path string) ([]float64, error) {
	b, err := os.ReadFile(path) //nolint:gosec // The dump directory is a user-supplied input.
	if err != nil {
		return nil, err
	}
	if len(b)%8 != 0 {
		return nil, fmt.Errorf("%s: size %d is not a multiple of 8", path, len(b))
	}
	out := make([]float64, len(b)/8)
	for i := range out {
		out[i] = math.Float64frombits(binary.LittleEndian.Uint64(b[i*8:]))
	}
	return out, nil
}

// ImpulseResponses reads the impulse response of each channel as 16-bit
// samples of its first channel, as the renderer reads it. With an empty
// esDir it reads the copies in the dump, otherwise the files under esDir,
// the es/ directory of the engine-sim package.
func (d *Dump) ImpulseResponses(esDir string) ([][]int16, error) {
	out := make([][]int16, len(d.Channels))
	for i, c := range d.Channels {
		path := filepath.Join(d.Dir, c.ImpulseResponseCopy)
		if esDir != "" {
			path = filepath.Join(esDir, c.ImpulseResponse)
		}
		ir, err := ReadImpulseResponse(path)
		if err != nil {
			return nil, err
		}
		if len(ir) != c.FileSamples {
			return nil, fmt.Errorf("%s: %d samples, the renderer read %d", path, len(ir), c.FileSamples)
		}
		out[i] = ir
	}
	return out, nil
}

// ReadImpulseResponse reads the first channel of a 16-bit PCM WAV file.
func ReadImpulseResponse(path string) ([]int16, error) {
	a, err := wav.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return toInt16(a.Channels[0], path)
}

// ReadOutput reads the engine-sim synthesiser output of the dump.
func (d *Dump) ReadOutput() ([]int16, error) {
	path := filepath.Join(d.Dir, d.Output)
	a, err := wav.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(a.Channels) != 1 || a.SampleRate != int(d.AudioSampleRate) {
		return nil, fmt.Errorf("%s: want mono at %g Hz", path, d.AudioSampleRate)
	}
	return toInt16(a.Channels[0], path)
}

// toInt16 converts samples that wav.Read scaled by 1/32768 back to
// integers. It rejects samples that are not 16-bit values.
func toInt16(x []float64, path string) ([]int16, error) {
	out := make([]int16, len(x))
	for i, v := range x {
		n := v * 32768
		if n != math.Trunc(n) || n < math.MinInt16 || n > math.MaxInt16 {
			return nil, fmt.Errorf("%s: sample %d is not a 16-bit value", path, i)
		}
		out[i] = int16(n)
	}
	return out, nil
}

// errBlock reports a render block that differs from the record.
var errBlock = errors.New("render block differs from the record")

// Render runs the port on the dump input in the order of the renderer
// (reference/engine-sim/engine_sim_render.cpp): per frame, one WriteInput
// per step, Simulator::endFrame, then render blocks until the input is
// empty. Before each block it draws the rand() calls that engine-sim made
// outside the synthesiser. It returns the whole output.
func Render(d *Dump, irs [][]int16, precision Precision) ([]int16, error) {
	rand := NewRandom(1)
	p := SimulatorParameters(d.InputChannelCount, d.SimulationFrequency)
	p.Precision = precision
	s := New(p, rand)
	s.SetAudioParameters(d.Audio)
	for i, ir := range irs {
		if taps := s.InitializeImpulseResponse(ir, d.Channels[i].Volume, i); taps != d.Channels[i].Taps {
			return nil, fmt.Errorf("channel %d: %d impulse response taps, the renderer used %d", i, taps, d.Channels[i].Taps)
		}
	}
	s.ReadAudioOutput(make([]int16, s.AudioBufferSize()))

	out := make([]int16, 0, d.OutputSamples)
	chunk := make([]int16, 4096)
	data := make([]float64, d.InputChannelCount)
	step := 0
	for fi, f := range d.Frames {
		s.SetInputSampleRate(d.InputSampleRate)
		for range f.Steps {
			for c := range data {
				data[c] = d.Inputs[c][step]
			}
			s.WriteInput(data)
			step++
		}
		s.EndInputBlock()

		bi := 0
		for s.InputSize() > 0 {
			if bi >= len(f.Blocks) {
				return nil, fmt.Errorf("frame %d: %w: more blocks than %d", fi, errBlock, len(f.Blocks))
			}
			rand.Skip(f.Blocks[bi][1])
			n := s.RenderAudio()
			if int64(n) != f.Blocks[bi][0] {
				return nil, fmt.Errorf("frame %d block %d: %w: %d samples, the renderer rendered %d", fi, bi, errBlock, n, f.Blocks[bi][0])
			}
			if n > len(chunk) {
				chunk = make([]int16, n)
			}
			s.ReadAudioOutput(chunk[:n])
			out = append(out, chunk[:n]...)
			s.EndInputBlock()
			bi++
		}
		if bi != len(f.Blocks) {
			return nil, fmt.Errorf("frame %d: %w: %d blocks, the renderer rendered %d", fi, errBlock, bi, len(f.Blocks))
		}
	}
	return out, nil
}
