// Package toast renders transient top-center notifications ("Coming
// soon", "Friend added", DM hints). Toasts queue; each shows for a fixed
// duration and expires on the overworld's animation tick.
package toast

import "time"

// duration is how long one toast stays visible.
const duration = 3 * time.Second

// Model is a small FIFO of pending toast messages.
type Model struct {
	queue []entry
}

type entry struct {
	text  string
	since time.Time // zero until the toast reaches the front
}

// New creates an empty toast queue.
func New() Model {
	return Model{}
}

// Show enqueues a toast.
func (m *Model) Show(text string) {
	m.queue = append(m.queue, entry{text: text})
}

// Tick advances the queue; call it from the overworld animation tick.
func (m *Model) Tick(now time.Time) {
	for len(m.queue) > 0 {
		head := &m.queue[0]
		if head.since.IsZero() {
			head.since = now
			return
		}
		if now.Sub(head.since) < duration {
			return
		}
		m.queue = m.queue[1:]
	}
}

// Message returns the visible toast's text, or "" when idle. The pixel
// renderer bakes this into the frame as an inverse banner.
func (m *Model) Message() string {
	if len(m.queue) == 0 {
		return ""
	}
	return m.queue[0].text
}
