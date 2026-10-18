// Package record writes what the harness sends to the engine and what the
// player plays: a CSV log of every snapshot and a 32-bit float mono WAV
// file of the audio. snarl-render --replay renders the log offline.
package record

import (
	"bufio"
	"encoding/binary"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/oval-tutu/snarl"
)

// Header is the header row of the snapshot log. snarl-render --replay
// (cmd/snarl-render) reads the same columns. t is the time in seconds
// from the start of the recording.
const Header = "t,profile,tier,rpm,throttle,load,speed,gear,flags,shift_up,shift_down,limiter"

// Log writes snapshot rows.
type Log struct {
	w           *csv.Writer
	last        snarl.Snapshot
	lastProfile string
	lastTier    snarl.Tier
	rows        int
}

// NewLog writes the header row to w.
func NewLog(w io.Writer) (*Log, error) {
	l := &Log{w: csv.NewWriter(w)}
	if _, err := io.WriteString(w, Header+"\n"); err != nil {
		return nil, err
	}
	return l, nil
}

// Write adds the row of one Engine.Set call at time t.
func (l *Log) Write(t time.Duration, profile string, tier snarl.Tier, s snarl.Snapshot) error {
	f := func(v float32) string { return strconv.FormatFloat(float64(v), 'g', -1, 32) }
	u := func(v uint8) string { return strconv.Itoa(int(v)) }
	l.last, l.lastProfile, l.lastTier = s, profile, tier
	l.rows++
	return l.w.Write([]string{
		strconv.FormatFloat(t.Seconds(), 'g', -1, 64), profile, u(uint8(tier)),
		f(s.RPM), f(s.Throttle), f(s.Load), f(s.Speed),
		u(s.Gear), u(uint8(s.Flags)), u(s.ShiftUp), u(s.ShiftDown), u(s.Limiter),
	})
}

// Close repeats the last row at time end, so that the log lasts until
// end, and flushes the rows.
func (l *Log) Close(end time.Duration) error {
	if l.rows > 0 {
		if err := l.Write(end, l.lastProfile, l.lastTier, l.last); err != nil {
			return err
		}
	}
	return l.Flush()
}

// Flush writes the buffered rows.
func (l *Log) Flush() error {
	l.w.Flush()
	return l.w.Error()
}

// The ring holds this many blocks, and the writer drains it at this
// interval. 512 blocks of 256 frames are 2.7 seconds at 48 kHz. Both files
// are complete up to the last sync, so that a crash keeps the recording.
const (
	ringSlots = 512
	drainTime = 10 * time.Millisecond
	syncTime  = time.Second
)

// Recorder writes the snapshot log and the audio of one recording.
// Capture runs on the audio goroutine. The other methods run on one other
// goroutine, the game goroutine in the harness.
type Recorder struct {
	CSVPath, WAVPath string

	start   time.Time
	csvFile *os.File
	log     *Log
	logErr  error
	synced  time.Time

	ring    *Ring
	rate    int
	wavFile *os.File
	quit    chan struct{}
	done    chan struct{}
	frames  atomic.Int64
	wavErr  error // The writer goroutine owns it until done closes.
}

// Result is the outcome of a recording.
type Result struct {
	CSVPath, WAVPath string
	Seconds          float64 // Length of the snapshot log.
	Frames           int64   // Audio frames written.
	Dropped          uint64  // Audio blocks that the writer did not take in time.
}

// Start creates dir and two new files in it, named by the time now, and
// starts the audio writer. block is the frame count of each Capture call.
func Start(dir string, now time.Time, rate, block int) (*Recorder, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	base := filepath.Join(dir, "snarl-"+now.Format("20060102-150405.000"))
	r := &Recorder{
		CSVPath: base + ".csv",
		WAVPath: base + ".wav",
		start:   now,
		ring:    NewRing(ringSlots, block),
		rate:    rate,
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	var err error
	if r.csvFile, err = os.OpenFile(r.CSVPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err != nil {
		return nil, err
	}
	if r.wavFile, err = os.OpenFile(r.WAVPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err != nil {
		_ = r.csvFile.Close()
		return nil, err
	}
	if r.log, err = NewLog(r.csvFile); err == nil {
		_, err = r.wavFile.Write(wavHeader(rate, 0))
	}
	if err != nil {
		_ = r.csvFile.Close()
		_ = r.wavFile.Close()
		return nil, err
	}
	go r.writeAudio()
	return r, nil
}

// Capture queues one block of the played audio. It does not allocate or
// wait. When the queue is full, it drops the block and counts it.
func (r *Recorder) Capture(mono []float32) { r.ring.Push(mono) }

// Snapshot adds the snapshot of one Engine.Set call at the time of the
// call.
func (r *Recorder) Snapshot(profile string, tier snarl.Tier, s snarl.Snapshot) {
	if r.logErr != nil {
		return
	}
	now := time.Now()
	r.logErr = r.log.Write(now.Sub(r.start), profile, tier, s)
	if r.logErr == nil && now.Sub(r.synced) >= syncTime {
		r.synced = now
		r.logErr = r.log.Flush()
	}
}

// Elapsed returns the recording time.
func (r *Recorder) Elapsed() time.Duration { return time.Since(r.start) }

// Dropped returns the number of audio blocks that were dropped.
func (r *Recorder) Dropped() uint64 { return r.ring.Dropped() }

// Err returns the first write error of the log.
func (r *Recorder) Err() error { return r.logErr }

// Stop ends the recording: it writes the last log row, writes the queued
// audio, completes the WAV header and closes both files. Remove the
// recorder from the audio source before Stop.
func (r *Recorder) Stop() (Result, error) {
	end := time.Since(r.start)
	close(r.quit)
	<-r.done
	res := Result{
		CSVPath: r.CSVPath, WAVPath: r.WAVPath, Seconds: end.Seconds(),
		Frames: r.frames.Load(), Dropped: r.ring.Dropped(),
	}
	errs := []error{r.logErr, r.wavErr}
	if r.logErr == nil {
		errs = append(errs, r.log.Close(end))
	}
	if r.wavErr == nil {
		_, err := r.wavFile.WriteAt(wavHeader(r.rate, res.Frames), 0)
		errs = append(errs, err)
	}
	errs = append(errs, r.csvFile.Close(), r.wavFile.Close())
	return res, errors.Join(errs...)
}

// writeAudio drains the ring into the WAV file until Stop.
func (r *Recorder) writeAudio() {
	defer close(r.done)
	block := make([]float32, r.ring.size)
	raw := make([]byte, 4*len(block))
	bw := bufio.NewWriterSize(r.wavFile, 1<<16)
	drain := func() {
		for r.ring.Pop(block) {
			if r.wavErr != nil {
				continue
			}
			for i, v := range block {
				binary.LittleEndian.PutUint32(raw[4*i:], math.Float32bits(v))
			}
			_, r.wavErr = bw.Write(raw)
			r.frames.Add(int64(len(block)))
		}
	}
	tick := time.NewTicker(drainTime)
	defer tick.Stop()
	for n := 1; ; n++ {
		drain()
		if n%int(syncTime/drainTime) == 0 && r.wavErr == nil {
			if r.wavErr = bw.Flush(); r.wavErr == nil {
				_, r.wavErr = r.wavFile.WriteAt(wavHeader(r.rate, r.frames.Load()), 0)
			}
		}
		select {
		case <-tick.C:
		case <-r.quit:
			drain()
			if r.wavErr == nil {
				r.wavErr = bw.Flush()
			}
			return
		}
	}
}

// wavHeader returns the header of a mono 32-bit float WAV file, with the
// cbSize field and the fact chunk of a non-PCM format.
func wavHeader(rate int, frames int64) []byte {
	data := uint32(min(frames*4, math.MaxUint32-58)) //nolint:gosec // Clamped.
	h := make([]byte, 0, 58)
	h = append(h, "RIFF"...)
	h = binary.LittleEndian.AppendUint32(h, 50+data)
	h = append(h, "WAVEfmt "...)
	h = binary.LittleEndian.AppendUint32(h, 18)
	h = binary.LittleEndian.AppendUint16(h, 3) // IEEE float.
	h = binary.LittleEndian.AppendUint16(h, 1)
	h = binary.LittleEndian.AppendUint32(h, uint32(rate))   //nolint:gosec // The rate is 48000.
	h = binary.LittleEndian.AppendUint32(h, uint32(rate)*4) //nolint:gosec // The rate is 48000.
	h = binary.LittleEndian.AppendUint16(h, 4)
	h = binary.LittleEndian.AppendUint16(h, 32)
	h = binary.LittleEndian.AppendUint16(h, 0)
	h = append(h, "fact"...)
	h = binary.LittleEndian.AppendUint32(h, 4)
	h = binary.LittleEndian.AppendUint32(h, data/4)
	h = append(h, "data"...)
	h = binary.LittleEndian.AppendUint32(h, data)
	return h
}

// String describes the result for the screen.
func (r Result) String() string {
	return fmt.Sprintf("%.1f s, %d frames, %d dropped blocks", r.Seconds, r.Frames, r.Dropped)
}
