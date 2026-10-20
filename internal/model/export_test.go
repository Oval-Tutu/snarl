package model

// SetControlBlock sets the control period of New and Render to n samples,
// from 1 to chunk, and returns a function that restores it. A test that
// calls it must not run in parallel with other tests of the package.
func SetControlBlock(n int) (restore func()) {
	was := controlBlock
	controlBlock = min(chunk, max(1, n))
	return func() { controlBlock = was }
}

// SetWholeSampleStart makes each pulse start on the whole sample at or
// after its position, with no band-limited corner and no end at zero, and
// returns a function that restores the fractional start. A test that
// calls it must not run in parallel with other tests of the package.
func SetWholeSampleStart() (restore func()) {
	wholeSampleStart = true
	return func() { wholeSampleStart = false }
}
