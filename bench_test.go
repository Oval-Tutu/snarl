package snarl

import "testing"

// benchBlocks renders 256-frame blocks of e with the snapshot s and
// reports the cost as a share of real time.
func benchBlocks(b *testing.B, e Engine, s Snapshot) {
	b.Helper()
	buf := make([]float32, 256)
	e.Set(s)
	// Settle into the state, so that the start does not count.
	for range 200 {
		e.Render(buf)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		e.Set(s)
		e.Render(buf)
	}
	blockNs := 256.0 / testRate * 1e9
	nsPerBlock := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
	b.ReportMetric(100*nsPerBlock/blockNs, "%realtime")
}

// BenchmarkRender renders each profile at three quarters of its redline
// and throttle 0.7, in 256-frame blocks at 48 kHz.
func BenchmarkRender(b *testing.B) {
	for _, name := range Profiles() {
		b.Run(name, func(b *testing.B) {
			p, err := LoadProfile(name)
			if err != nil {
				b.Fatal(err)
			}
			benchBlocks(b, newEngine(b, p), Snapshot{RPM: 0.75 * p.RedlineRPM, Throttle: 0.7, Load: 0.7, Flags: FlagRunning})
		})
	}
}

// BenchmarkStates renders each profile in four states: full throttle at
// three quarters of the redline (wot), closed throttle at idle (idle),
// closed throttle on overrun at four fifths of the redline, after a lift
// from full throttle (overrun), and full throttle at the limiter RPM,
// bouncing (limiter).
func BenchmarkStates(b *testing.B) {
	for _, name := range Profiles() {
		p, err := LoadProfile(name)
		if err != nil {
			b.Fatal(err)
		}
		states := []struct {
			name string
			s    Snapshot
		}{
			{"wot", Snapshot{RPM: 0.75 * p.RedlineRPM, Throttle: 1, Load: 1, Flags: FlagRunning}},
			{"idle", Snapshot{RPM: p.IdleRPM, Flags: FlagRunning}},
			{"overrun", Snapshot{RPM: 0.8 * p.RedlineRPM, Flags: FlagRunning | FlagOverrun}},
			{"limiter", Snapshot{RPM: p.LimiterRPM, Throttle: 1, Load: 1, Gear: 3, Flags: FlagRunning | FlagRedZone}},
		}
		for _, st := range states {
			b.Run(name+"/"+st.name, func(b *testing.B) {
				e := newEngine(b, p)
				if st.name == "overrun" {
					// The lift from full throttle starts the closing transient.
					buf := make([]float32, 256)
					e.Set(Snapshot{RPM: st.s.RPM, Throttle: 1, Load: 1, Flags: FlagRunning})
					for range 100 {
						e.Render(buf)
					}
				}
				benchBlocks(b, e, st.s)
			})
		}
	}
}
