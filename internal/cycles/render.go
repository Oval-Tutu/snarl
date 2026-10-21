package cycles

import (
	"errors"
	"fmt"
	"math"

	"github.com/oval-tutu/snarl/internal/esynth"
)

// Control returns the RPM, the throttle and the ignition state at time t
// in seconds from the end of the pre-roll. Before 0 it returns the first
// state.
type Control func(t float64) (rpm, throttle float64, ignition bool)

// RenderOptions set a render.
type RenderOptions struct {
	// Settle is the pre-roll in seconds at the first state, which the
	// output does not keep. Seconds is the kept length.
	Settle, Seconds float64
	// Volume overrides the synthesiser volume of the tables when positive.
	Volume float64
	// Precision is the arithmetic of the synthesiser port.
	Precision esynth.Precision
	// ImpulseResponses are the impulse responses of the channels, 16-bit
	// samples as engine-sim reads them.
	ImpulseResponses [][]int16
	Play             PlayOptions
}

// SynthRate is the output rate of the engine-sim synthesiser.
const SynthRate = 44100

// Render plays the tables under the control through the engine-sim
// synthesiser port, in the frame order of engine-sim-render: per frame of
// 1/200 s the steps of the frame, then render blocks until the input is
// empty. It returns the output after the pre-roll at 44.1 kHz.
func Render(t *Tables, ctl Control, opt RenderOptions) ([]int16, error) {
	if len(opt.ImpulseResponses) != len(t.Channels) {
		return nil, fmt.Errorf("%d impulse responses for %d channels", len(opt.ImpulseResponses), len(t.Channels))
	}
	if opt.Seconds <= 0 || opt.Settle < 0 {
		return nil, errors.New("the kept length must be positive and the pre-roll not negative")
	}
	params := esynth.SimulatorParameters(len(t.Channels), t.Rate)
	params.Precision = opt.Precision
	s := esynth.New(params, esynth.NewRandom(1))
	audio := t.Audio
	if opt.Volume > 0 {
		audio.Volume = float64(float32(opt.Volume))
	}
	s.SetAudioParameters(audio)
	for i, ir := range opt.ImpulseResponses {
		s.InitializeImpulseResponse(ir, t.Channels[i].Volume, i)
	}
	s.ReadAudioOutput(make([]int16, s.AudioBufferSize()))

	player := NewPlayer(t, opt.Play)
	dt := 1 / t.Rate
	stepsPerFrame := int(math.Round(t.Rate / 200))
	end := opt.Settle + opt.Seconds
	in := make([]float64, len(t.Channels))
	out := make([]int16, 0, int((end+1)*SynthRate))
	chunk := make([]int16, 4096)
	settleSamples := 0
	step := 0
	for now := 0.0; now < end; {
		s.SetInputSampleRate(t.Rate)
		for range stepsPerFrame {
			rpm, throttle, ignition := ctl(max(0, float64(step)*dt-opt.Settle))
			player.Next(rpm, throttle, ignition, in)
			s.WriteInput(in)
			step++
		}
		s.EndInputBlock()
		for s.InputSize() > 0 {
			n := s.RenderAudio()
			if n == 0 {
				break
			}
			if n > len(chunk) {
				chunk = make([]int16, n)
			}
			s.ReadAudioOutput(chunk[:n])
			out = append(out, chunk[:n]...)
			s.EndInputBlock()
		}
		now = float64(step) * dt
		if now <= opt.Settle {
			settleSamples = len(out)
		}
	}
	want := int(math.Round(opt.Seconds * SynthRate))
	start := min(settleSamples, len(out))
	return out[start:min(len(out), start+want)], nil
}
