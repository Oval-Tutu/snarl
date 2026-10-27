package cycles

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/oval-tutu/snarl/internal/esynth"
)

// SpeedPoints is the number of points of the crank speed profile of a
// table point, and Sectors the number of crank angle sectors of the
// residual envelopes.
const (
	SpeedPoints = 64
	Sectors     = 64
)

// Tables are the cycle tables of one engine.
type Tables struct {
	// Engine is the engine name in the script, Script the script file.
	Engine string `json:"engine"`
	Script string `json:"script"`
	// Rate is the synthesiser input rate: the simulation frequency.
	Rate  float64                `json:"input_rate_hz"`
	Audio esynth.AudioParameters `json:"audio_parameters"`
	// Channels are the exhaust channels.
	Channels []Channel `json:"channels"`
	// ThrottleLag is the intake lag of the playback, measured from a
	// throttle step.
	ThrottleLag ThrottleLag `json:"throttle_lag"`
	// ModulationSlots is the number of equal parts of each cycle in which
	// the average cycle takes one random gain, the modulation of
	// Point.Modulation, or 0 with no modulation.
	ModulationSlots int `json:"modulation_slots,omitempty"`
	// Points are the grid points.
	Points []Point `json:"points"`
}

// Channel is one exhaust channel: its impulse response (path relative to
// the engine-sim es/ directory), its volume, and its exhaust delay in
// steps, the weighted mean of the delay filters of its cylinders.
type Channel struct {
	ImpulseResponse string  `json:"impulse_response"`
	Volume          float64 `json:"volume"`
	DelaySteps      float64 `json:"delay_steps"`
}

// Point is one grid point: a hold at an RPM and a throttle, with the
// ignition on or off.
type Point struct {
	RPM      float64 `json:"rpm"`
	Throttle float64 `json:"throttle"`
	Ignition bool    `json:"ignition"`
	// Since is 0 for a steady hold. For a slice of a closing transient
	// (CloseSlices), it is the time in seconds from the close to the middle
	// of the slice.
	Since float64 `json:"since_s,omitempty"`
	// MeasuredRPM is the mean crank speed of the hold, and Cycles the whole
	// cycles that the tables average.
	MeasuredRPM float64 `json:"measured_rpm"`
	Cycles      int     `json:"cycles"`
	// Harmonics is the highest harmonic of the cycle in Wave. The harmonics
	// above the Nyquist frequency of the steps are those that the steps fold
	// below it.
	Harmonics int `json:"harmonics"`
	// Modulation holds per channel the RMS of the random gain of the average
	// cycle without its mean in each of Tables.ModulationSlots parts of each
	// cycle, the part of the residual that follows the cycle. Noise holds
	// the rest of the residual.
	Modulation []float64 `json:"modulation,omitempty"`
	// ModulationRho holds per channel the correlation of the gains of
	// neighbouring parts of the modulation.
	ModulationRho []float64 `json:"modulation_rho,omitempty"`
	// Speed is the crank speed over the cycle relative to its mean, at
	// SpeedPoints crank angles from 0. The time of one cycle at the mean
	// RPM is the integral of 1/Speed over the cycle.
	Speed []float64 `json:"-"`
	// Wave holds per channel the Fourier series of the average cycle
	// over the emission angle (the crank phase less the channel delay at
	// MeasuredRPM): the mean, then the real and imaginary parts of each
	// harmonic from 1 to Harmonics, so that the input at emission phase
	// psi in cycles is Wave[0] + 2 sum_h (re_h cos 2 pi h psi - im_h sin 2 pi h psi).
	Wave [][]float64 `json:"-"`
	// Noise holds per channel the RMS of the residual in each band and
	// sector of the emission angle, band by band: Noise[c][b*Sectors+s].
	// The band noise is unit-power noise through the band filter, so the
	// value is its gain.
	Noise [][]float64 `json:"-"`
}

const (
	magic   = "SNARLCYC"
	version = 1
)

// The binary format: the magic, the version (uint32), the length of the
// JSON header (uint32), the JSON header (Tables without the arrays), then
// per point: Speed as float16, then per channel the Wave scale (float32)
// and Wave over the scale as float16, then per channel the Noise scale and
// Noise over the scale as float16. All values are little endian.

// Write encodes t to w.
func (t *Tables) Write(w io.Writer) error {
	head, err := json.Marshal(t)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	b.WriteString(magic)
	le := binary.LittleEndian
	b.Write(le.AppendUint32(nil, version))
	b.Write(le.AppendUint32(nil, uint32(len(head)))) //nolint:gosec // The header is far below 4 GB.
	b.Write(head)
	for i := range t.Points {
		p := &t.Points[i]
		if len(p.Speed) != SpeedPoints || len(p.Wave) != len(t.Channels) || len(p.Noise) != len(t.Channels) {
			return fmt.Errorf("point %d: wrong array sizes", i)
		}
		writeHalfs(&b, p.Speed)
		for c := range t.Channels {
			if len(p.Wave[c]) != 2*p.Harmonics+1 || len(p.Noise[c]) != BandCount*Sectors {
				return fmt.Errorf("point %d channel %d: wrong array sizes", i, c)
			}
			writeScaled(&b, p.Wave[c])
		}
		for c := range t.Channels {
			writeScaled(&b, p.Noise[c])
		}
	}
	_, err = w.Write(b.Bytes())
	return err
}

// WriteFile writes t to path.
func (t *Tables) WriteFile(path string) error {
	var b bytes.Buffer
	if err := t.Write(&b); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// ReadFile reads the tables at path.
func ReadFile(path string) (*Tables, error) {
	b, err := os.ReadFile(path) //nolint:gosec // The tables path is a user-supplied input.
	if err != nil {
		return nil, err
	}
	t, err := Read(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
}

// Read decodes tables from b.
func Read(b []byte) (*Tables, error) {
	le := binary.LittleEndian
	if len(b) < 16 || string(b[:8]) != magic {
		return nil, errors.New("not a cycle table file")
	}
	if v := le.Uint32(b[8:]); v != version {
		return nil, fmt.Errorf("version %d, want %d", v, version)
	}
	n := int(le.Uint32(b[12:]))
	if 16+n > len(b) {
		return nil, errors.New("truncated header")
	}
	t := &Tables{}
	if err := json.Unmarshal(b[16:16+n], t); err != nil {
		return nil, err
	}
	r := reader{b: b[16+n:]}
	for i := range t.Points {
		p := &t.Points[i]
		p.Speed = r.halfs(SpeedPoints)
		p.Wave = make([][]float64, len(t.Channels))
		p.Noise = make([][]float64, len(t.Channels))
		for c := range t.Channels {
			p.Wave[c] = r.scaled(2*p.Harmonics + 1)
		}
		for c := range t.Channels {
			p.Noise[c] = r.scaled(BandCount * Sectors)
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	if len(r.b) != 0 {
		return nil, fmt.Errorf("%d bytes after the last point", len(r.b))
	}
	return t, nil
}

func writeHalfs(b *bytes.Buffer, x []float64) {
	for _, v := range x {
		b.Write(binary.LittleEndian.AppendUint16(nil, toHalf(v)))
	}
}

// writeScaled writes the peak magnitude of x as float32 and x over it as
// float16, which keeps a relative precision of 2^-11 of the peak.
func writeScaled(b *bytes.Buffer, x []float64) {
	var peak float64
	for _, v := range x {
		peak = max(peak, math.Abs(v))
	}
	scale := float32(peak)
	if scale == 0 {
		scale = 1
	}
	b.Write(binary.LittleEndian.AppendUint32(nil, math.Float32bits(scale)))
	for _, v := range x {
		b.Write(binary.LittleEndian.AppendUint16(nil, toHalf(v/float64(scale))))
	}
}

type reader struct {
	b   []byte
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if len(r.b) < n {
		r.err = errors.New("truncated point data")
		return nil
	}
	out := r.b[:n]
	r.b = r.b[n:]
	return out
}

func (r *reader) halfs(n int) []float64 {
	b := r.take(2 * n)
	if b == nil {
		return make([]float64, n)
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = fromHalf(binary.LittleEndian.Uint16(b[2*i:]))
	}
	return out
}

func (r *reader) scaled(n int) []float64 {
	b := r.take(4)
	if b == nil {
		return make([]float64, n)
	}
	scale := float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
	out := r.halfs(n)
	for i := range out {
		out[i] *= scale
	}
	return out
}

// toHalf rounds v to the nearest IEEE 754 binary16 value, ties to even.
// Values beyond the range become infinities.
func toHalf(v float64) uint16 {
	f := math.Float32bits(float32(v))
	sign := uint16(f>>16) & 0x8000
	exp := int((f>>23)&0xff) - 127 + 15
	mant := f & 0x7fffff
	switch {
	case (f>>23)&0xff == 0xff:
		if mant != 0 {
			return sign | 0x7e00
		}
		return sign | 0x7c00
	case exp >= 0x1f:
		return sign | 0x7c00
	case exp <= 0:
		if exp < -10 {
			return sign
		}
		mant |= 0x800000
		shift := uint(14 - exp)
		half := mant >> shift
		rem := mant & (1<<shift - 1)
		mid := uint32(1) << (shift - 1)
		if rem > mid || (rem == mid && half&1 == 1) {
			half++
		}
		return sign | uint16(half) //nolint:gosec // half fits in 11 bits.
	}
	half := uint32(exp)<<10 | mant>>13 //nolint:gosec // exp is in 1..30.
	rem := mant & 0x1fff
	if rem > 0x1000 || (rem == 0x1000 && half&1 == 1) {
		half++
	}
	return sign | uint16(half) //nolint:gosec // The carry into the exponent is the correct rounding.
}

// fromHalf returns the value of an IEEE 754 binary16.
func fromHalf(h uint16) float64 {
	sign := 1.0
	if h&0x8000 != 0 {
		sign = -1
	}
	exp := int(h>>10) & 0x1f
	mant := float64(h & 0x3ff)
	switch exp {
	case 0:
		return sign * mant * math.Exp2(-24)
	case 0x1f:
		if mant != 0 {
			return math.NaN()
		}
		return sign * math.Inf(1)
	}
	return sign * (1 + mant/1024) * math.Exp2(float64(exp-15))
}
