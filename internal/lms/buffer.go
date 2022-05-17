package lms

import (
	"sync"
)

type sliceBuffer struct {
	mu  sync.Mutex
	buf []*Message
}

var _ MessageBuffer = (*sliceBuffer)(nil)

func (sb *sliceBuffer) Lock() {
	sb.mu.Lock()
}
func (sb *sliceBuffer) Unlock() {
	sb.mu.Unlock()
}

func (sb *sliceBuffer) Append(msg Message) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	sb.buf = append(sb.buf, &msg)
	return len(sb.buf), nil
}

func (sb *sliceBuffer) Len() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	return len(sb.buf)
}

// NewSliceBuffer initializes new message buffer with slice as backend
func NewSliceBuffer() *sliceBuffer {
	return &sliceBuffer{
		mu:  sync.Mutex{},
		buf: []*Message{},
	}
}
