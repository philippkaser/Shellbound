// Package syncwriter serializes writes to a shared io.Writer behind a mutex.
//
// Shellbound's plaza ships full frames as Sixel graphics written directly to
// the SSH session, while bubbletea's own renderer also holds that session as
// its output. The two never paint at the same time (the plaza keeps
// bubbletea's renderer quiescent by returning a constant View), but a shared
// mutex guarantees their byte streams can never interleave even at the
// transitions where both might write.
package syncwriter

import (
	"io"
	"sync"
)

// Writer wraps an io.Writer with a mutex.
type Writer struct {
	mu sync.Mutex
	w  io.Writer
}

// New wraps w.
func New(w io.Writer) *Writer { return &Writer{w: w} }

// Write implements io.Writer.
func (s *Writer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// WriteString locks once and writes a whole frame string atomically.
func (s *Writer) WriteString(str string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return io.WriteString(s.w, str)
}
