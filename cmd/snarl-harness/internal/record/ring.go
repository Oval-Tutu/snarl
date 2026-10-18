package record

import "sync/atomic"

// Ring is a single-producer single-consumer queue of fixed-size blocks of
// samples. All memory is allocated by NewRing, so Push and Pop do not
// allocate, and neither waits for the other.
type Ring struct {
	buf        []float32
	size       uint64 // Samples per block.
	slots      uint64
	head, tail atomic.Uint64 // Blocks pushed and popped.
	dropped    atomic.Uint64
}

// NewRing returns a ring of slots blocks of size samples.
func NewRing(slots, size int) *Ring {
	return &Ring{buf: make([]float32, slots*size), size: uint64(size), slots: uint64(slots)} //nolint:gosec // Sizes are positive.
}

// Push copies one block into the ring. The producer calls it. When the
// ring is full, it drops the block, counts the drop and returns false.
func (r *Ring) Push(block []float32) bool {
	h := r.head.Load()
	if h-r.tail.Load() >= r.slots {
		r.dropped.Add(1)
		return false
	}
	i := h % r.slots * r.size
	slot := r.buf[i : i+r.size]
	clear(slot[copy(slot, block):])
	r.head.Store(h + 1)
	return true
}

// Pop copies the oldest block into dst. The consumer calls it. It returns
// false when the ring is empty.
func (r *Ring) Pop(dst []float32) bool {
	t := r.tail.Load()
	if t == r.head.Load() {
		return false
	}
	i := t % r.slots * r.size
	copy(dst, r.buf[i:i+r.size])
	r.tail.Store(t + 1)
	return true
}

// Dropped returns the number of blocks that Push dropped.
func (r *Ring) Dropped() uint64 { return r.dropped.Load() }
