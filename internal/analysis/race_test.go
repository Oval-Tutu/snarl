//go:build race

package analysis

// raceEnabled is true under the race detector. The residual tests skip
// there: they measure seconds of audio in one goroutine, which takes
// minutes under the race detector and has no shared state to check.
const raceEnabled = true
