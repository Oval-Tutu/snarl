package esynth

// ringBuffer ports RingBuffer<T_Data> (include/ring_buffer.h). As in
// engine-sim, a buffer that holds capacity samples reports size 0.
type ringBuffer[T any] struct {
	buf        []T
	writeIndex int
	start      int
}

// newRingBuffer ports RingBuffer::initialize.
func newRingBuffer[T any](capacity int) *ringBuffer[T] {
	return &ringBuffer[T]{buf: make([]T, capacity)}
}

// write ports RingBuffer::write.
func (r *ringBuffer[T]) write(v T) {
	r.buf[r.writeIndex] = v
	r.writeIndex++
	if r.writeIndex >= len(r.buf) {
		r.writeIndex = 0
	}
}

// at ports RingBuffer::read(size_t index).
func (r *ringBuffer[T]) at(index int) T {
	if r.start+index >= len(r.buf) {
		return r.buf[r.start+index-len(r.buf)]
	}
	return r.buf[r.start+index]
}

// read ports RingBuffer::read(size_t n, T_Data *target): it copies
// len(target) samples from the start and keeps them.
func (r *ringBuffer[T]) read(target []T) {
	n := len(target)
	if r.start+n < len(r.buf) {
		copy(target, r.buf[r.start:r.start+n])
		return
	}
	head := copy(target, r.buf[r.start:])
	copy(target[head:], r.buf[:n-head])
}

// readAndRemove ports RingBuffer::readAndRemove.
func (r *ringBuffer[T]) readAndRemove(target []T) {
	r.read(target)
	r.removeBeginning(len(target))
}

// removeBeginning ports RingBuffer::removeBeginning.
func (r *ringBuffer[T]) removeBeginning(n int) {
	r.start += n
	if r.start >= len(r.buf) {
		r.start -= len(r.buf)
	}
}

// size ports RingBuffer::size.
func (r *ringBuffer[T]) size() int {
	if r.writeIndex < r.start {
		return r.writeIndex + (len(r.buf) - r.start)
	}
	return r.writeIndex - r.start
}
