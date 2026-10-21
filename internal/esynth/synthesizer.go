// Package esynth ports the engine-sim synthesiser (src/synthesizer.cpp and
// its filters at engine-sim commit 85f7c3b) to Go. It keeps the order of
// operations, the constants, the buffering and the random number
// generators of engine-sim. In Float64 precision it computes in float64
// where engine-sim computes in float. Constants that engine-sim writes as
// float literals, and the parameters that it stores as float, keep their
// float values. In Float32 precision it rounds as engine-sim does.
package esynth

import "math"

// AudioParameters ports Synthesizer::AudioParameters.
type AudioParameters struct {
	Volume                          float64 `json:"volume"`
	Convolution                     float64 `json:"convolution"`
	DFFMix                          float64 `json:"dF_F_mix"`
	InputSampleNoise                float64 `json:"inputSampleNoise"`
	InputSampleNoiseFrequencyCutoff float64 `json:"inputSampleNoiseFrequencyCutoff"`
	AirNoise                        float64 `json:"airNoise"`
	AirNoiseFrequencyCutoff         float64 `json:"airNoiseFrequencyCutoff"`
	LevelerTarget                   float64 `json:"levelerTarget"`
	LevelerMaxGain                  float64 `json:"levelerMaxGain"`
	LevelerMinGain                  float64 `json:"levelerMinGain"`
}

// DefaultAudioParameters returns the default member values of
// Synthesizer::AudioParameters (include/synthesizer.h).
func DefaultAudioParameters() AudioParameters {
	return AudioParameters{
		Volume:                          float64(float32(1.0)),
		Convolution:                     float64(float32(1.0)),
		DFFMix:                          float64(float32(0.01)),
		InputSampleNoise:                float64(float32(0.5)),
		InputSampleNoiseFrequencyCutoff: float64(float32(10000.0)),
		AirNoise:                        float64(float32(1.0)),
		AirNoiseFrequencyCutoff:         float64(float32(2000.0)),
		LevelerTarget:                   float64(float32(30000.0)),
		LevelerMaxGain:                  float64(float32(1.9)),
		LevelerMinGain:                  float64(float32(0.00001)),
	}
}

// Parameters ports Synthesizer::Parameters.
type Parameters struct {
	InputChannelCount      int
	InputBufferSize        int
	AudioBufferSize        int
	InputSampleRate        float64
	AudioSampleRate        float64
	InitialAudioParameters AudioParameters
	// Precision is not an engine-sim parameter: it selects the arithmetic
	// of the port.
	Precision Precision
}

// SimulatorParameters ports Simulator::initializeSynthesizer
// (src/simulator.cpp): the parameters that the simulator gives the
// synthesiser of an engine with the given exhaust systems and simulation
// frequency.
func SimulatorParameters(channels int, simulationFrequency float64) Parameters {
	return Parameters{
		InputChannelCount:      channels,
		InputBufferSize:        44100,
		AudioBufferSize:        44100,
		InputSampleRate:        float64(float32(simulationFrequency)),
		AudioSampleRate:        44100,
		InitialAudioParameters: DefaultAudioParameters(),
	}
}

// inputChannel ports Synthesizer::InputChannel.
type inputChannel struct {
	data            *ringBuffer[float64]
	transferBuffer  []float64
	lastInputSample float64
}

// processingFilters ports Synthesizer::ProcessingFilters.
type processingFilters struct {
	convolution     convolution
	derivative      derivative
	jitterFilter    *jitter
	airNoiseLowPass *butterworthLowPass
	inputDcFilter   *lowPass
	antialiasing    *butterworthLowPass
}

// Synthesizer ports Synthesizer (src/synthesizer.cpp) without its render
// thread: the caller calls RenderAudio where the engine-sim thread wakes.
type Synthesizer struct {
	antialiasing     *butterworthLowPass
	levelingFilter   *leveling
	inputChannels    []inputChannel
	audioParameters  AudioParameters
	inputBufferSize  int
	inputSamplesRead int
	latency          int
	inputWriteOffset float64
	lastInputOffset  float64
	audioBuffer      *ringBuffer[int16]
	inputSampleRate  float64
	audioSampleRate  float64
	processed        bool
	filters          []processingFilters
	rand             *Random
	p                Precision
}

// New ports Synthesizer::Synthesizer and Synthesizer::initialize. rand is
// the process-wide rand() generator that renderAudio(int) draws from.
func New(p Parameters, rand *Random) *Synthesizer {
	s := &Synthesizer{
		inputBufferSize: p.InputBufferSize,
		inputSampleRate: p.InputSampleRate,
		audioSampleRate: p.AudioSampleRate,
		audioParameters: p.InitialAudioParameters,
		processed:       true,
		audioBuffer:     newRingBuffer[int16](p.AudioBufferSize),
		inputChannels:   make([]inputChannel, p.InputChannelCount),
		filters:         make([]processingFilters, p.InputChannelCount),
		antialiasing:    newButterworthLowPass(true, p.Precision),
		levelingFilter:  newLeveling(p.Precision),
		rand:            rand,
		p:               p.Precision,
	}
	for i := range s.inputChannels {
		s.inputChannels[i].transferBuffer = make([]float64, p.InputBufferSize)
		s.inputChannels[i].data = newRingBuffer[float64](p.InputBufferSize)
	}

	dt := float64(float32(1) / float32(s.audioSampleRate))
	for i := range s.filters {
		f := &s.filters[i]
		f.airNoiseLowPass = newButterworthLowPass(true, s.p)
		f.airNoiseLowPass.setCutoffFrequency(s.audioParameters.AirNoiseFrequencyCutoff, s.audioSampleRate)

		f.derivative.dt = dt
		f.derivative.p = s.p

		f.inputDcFilter = newLowPass(s.p)
		f.inputDcFilter.setCutoffFrequency(10.0)
		f.inputDcFilter.dt = dt

		f.jitterFilter = newJitter(10, s.audioParameters.InputSampleNoiseFrequencyCutoff, s.audioSampleRate, s.p)

		f.convolution.p = s.p

		// ButterworthLowPassFilter<double>: double in both precisions.
		f.antialiasing = newButterworthLowPass(false, s.p)
		f.antialiasing.setCutoffFrequency(float64(float32(1900.0)), s.audioSampleRate)
	}

	s.levelingFilter.target = s.audioParameters.LevelerTarget
	s.levelingFilter.maxLevel = s.audioParameters.LevelerMaxGain
	s.levelingFilter.minLevel = s.audioParameters.LevelerMinGain
	s.antialiasing.setCutoffFrequency(float64(float32(s.audioSampleRate)*float32(0.45)), s.audioSampleRate)

	for range p.AudioBufferSize {
		s.audioBuffer.write(0)
	}
	return s
}

// InitializeImpulseResponse ports Synthesizer::initializeImpulseResponse:
// it trims the response after its last sample above 100 in magnitude, caps
// it at 10000 taps and scales it by volume / INT16_MAX. It returns the tap
// count.
func (s *Synthesizer) InitializeImpulseResponse(impulseResponse []int16, volume float64, index int) int {
	clippedLength := 0
	for i, v := range impulseResponse {
		if abs16(v) > 100 {
			clippedLength = i + 1
		}
	}

	sampleCount := min(10000, clippedLength)
	c := &s.filters[index].convolution
	c.initialize(sampleCount)
	for i := range sampleCount {
		c.impulseResponse[i] = s.p.r(s.p.r(volume*float64(impulseResponse[i])) / math.MaxInt16)
	}
	return sampleCount
}

func abs16(v int16) int {
	if v < 0 {
		return -int(v)
	}
	return int(v)
}

// ReadAudioOutput ports Synthesizer::readAudioOutput: it moves up to
// len(buffer) samples to buffer, fills the rest with zeros and returns the
// samples that it moved.
func (s *Synthesizer) ReadAudioOutput(buffer []int16) int {
	newDataLength := s.audioBuffer.size()
	if newDataLength >= len(buffer) {
		s.audioBuffer.readAndRemove(buffer)
	} else {
		s.audioBuffer.readAndRemove(buffer[:newDataLength])
		clear(buffer[newDataLength:])
	}
	return min(len(buffer), newDataLength)
}

// AudioBufferSize returns m_audioBuffer.size(), the rendered samples that
// ReadAudioOutput has not moved.
func (s *Synthesizer) AudioBufferSize() int {
	return s.audioBuffer.size()
}

// InputSize returns m_inputChannels[0].data.size(), the input samples at
// the audio rate that the synthesiser has not rendered.
func (s *Synthesizer) InputSize() int {
	if len(s.inputChannels) == 0 {
		return 0
	}
	return s.inputChannels[0].data.size()
}

// WriteInput ports Synthesizer::writeInput: one input sample per channel at
// the input rate. It interpolates linearly to the audio rate and low-passes
// each channel at 1900 Hz.
func (s *Synthesizer) WriteInput(data []float64) {
	s.inputWriteOffset += s.audioSampleRate / s.inputSampleRate
	if s.inputWriteOffset >= float64(s.inputBufferSize) {
		s.inputWriteOffset -= float64(s.inputBufferSize)
	}

	for i := range s.inputChannels {
		ch := &s.inputChannels[i]
		buffer := ch.data
		lastInputSample := ch.lastInputSample
		baseIndex := float64(buffer.writeIndex)
		distance := s.inputDistance(s.inputWriteOffset, s.lastInputOffset)
		t := s.inputDistance(baseIndex, s.lastInputOffset)
		for ; t <= distance; t += 1.0 {
			if t >= float64(s.inputBufferSize) {
				t -= float64(s.inputBufferSize)
			}

			f := t / distance
			sample := lastInputSample*(1-f) + data[i]*f

			// The filter takes a float, computes in double, and the buffer stores floats.
			buffer.write(s.p.r(s.filters[i].antialiasing.fastF(s.p.r(sample))))
		}

		ch.lastInputSample = data[i]
	}

	s.lastInputOffset = s.inputWriteOffset
}

// EndInputBlock ports Synthesizer::endInputBlock: it drops the input that
// the last RenderAudio rendered and records the latency.
func (s *Synthesizer) EndInputBlock() {
	for i := range s.inputChannels {
		s.inputChannels[i].data.removeBeginning(s.inputSamplesRead)
	}

	if len(s.inputChannels) != 0 {
		s.latency = s.inputChannels[0].data.size()
	}

	s.inputSamplesRead = 0
	s.processed = false
}

// RenderAudio ports Synthesizer::renderAudio(): it renders the available
// input until the output buffer holds 2000 samples, and returns the samples
// that it rendered. The engine-sim thread waits until input is available,
// the output holds fewer than 2000 samples and EndInputBlock ran since the
// last block. Where the thread would wait, RenderAudio returns 0.
func (s *Synthesizer) RenderAudio() int {
	inputAvailable := s.InputSize() > 0 && s.audioBuffer.size() < 2000
	if !inputAvailable || s.processed {
		return 0
	}

	n := min(max(0, 2000-s.audioBuffer.size()), s.InputSize())

	for i := range s.inputChannels {
		s.inputChannels[i].data.read(s.inputChannels[i].transferBuffer[:n])
	}

	s.inputSamplesRead = n
	s.processed = true

	for i := range s.filters {
		s.filters[i].airNoiseLowPass.setCutoffFrequency(float64(float32(s.audioParameters.AirNoiseFrequencyCutoff)), s.audioSampleRate)
		s.filters[i].jitterFilter.jitterScale = s.audioParameters.InputSampleNoise
	}

	for i := range n {
		s.audioBuffer.write(s.renderSample(i))
	}
	return n
}

// Latency ports Synthesizer::getLatency.
func (s *Synthesizer) Latency() float64 {
	return float64(s.latency) / s.audioSampleRate
}

// inputDistance ports Synthesizer::inputDistance.
func (s *Synthesizer) inputDistance(s1, s0 float64) float64 {
	if s1 < s0 {
		return float64(s.inputBufferSize) - s0 + s1
	}
	return s1 - s0
}

// SetInputSampleRate ports Synthesizer::setInputSampleRate, which stores the
// rate as float.
func (s *Synthesizer) SetInputSampleRate(sampleRate float64) {
	s.inputSampleRate = float64(float32(sampleRate))
}

// random ports the float 2.0 * ((double)rand() / RAND_MAX) - 1.0.
func (s *Synthesizer) random() float64 {
	return s.p.r(2.0*(float64(s.rand.Rand())/RandMax) - 1.0)
}

// renderSample ports Synthesizer::renderAudio(int inputSample): one output
// sample from the transfer buffers, in float.
func (s *Synthesizer) renderSample(inputSample int) int16 {
	r := s.p.r
	airNoise := s.audioParameters.AirNoise
	dFFMix := s.audioParameters.DFFMix
	convAmount := s.audioParameters.Convolution

	signal := 0.0
	for i := range s.inputChannels {
		f := &s.filters[i]
		// r_0: engine-sim draws it and does not use it.
		_ = s.random()

		jitteredSample := f.jitterFilter.fastF(s.inputChannels[i].transferBuffer[inputSample])

		fIn := jitteredSample
		fDc := f.inputDcFilter.fastF(fIn)
		fAc := r(fIn - fDc)
		fP := f.derivative.f(fIn)

		noise := s.random()
		// engine-sim uses m_filters->airNoiseLowPass, the filter of
		// channel 0, for every channel.
		rNoise := s.filters[0].airNoiseLowPass.fastF(noise)
		rMixed := r(r(airNoise*rNoise) + r(1-airNoise))

		vIn := r(r(fP*dFFMix) + r(r(fAc*rMixed)*r(1-dFFMix)))
		if float32Subnormal(vIn) {
			vIn = 0
		}

		v := r(r(convAmount*f.convolution.f(vIn)) + r(r(1-convAmount)*vIn))

		signal = r(signal + v)
	}

	signal = s.antialiasing.fastF(signal)

	s.levelingFilter.target = s.audioParameters.LevelerTarget
	vLeveled := r(s.levelingFilter.f(signal) * s.audioParameters.Volume)
	rInt := math.Round(vLeveled)
	switch {
	case rInt > math.MaxInt16:
		rInt = math.MaxInt16
	case rInt < math.MinInt16:
		rInt = math.MinInt16
	}
	return int16(rInt)
}

// AudioParameters ports Synthesizer::getAudioParameters.
func (s *Synthesizer) AudioParameters() AudioParameters {
	return s.audioParameters
}

// SetAudioParameters ports Synthesizer::setAudioParameters.
func (s *Synthesizer) SetAudioParameters(p AudioParameters) {
	s.audioParameters = p
}
