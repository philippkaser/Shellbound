// Package inventory renders the inventory panel: the satchel of items the
// portal worlds grant, listed straight from storage.
package inventory

import (
	"fmt"

	"github.com/shellbound/shellbound/internal/storage"
	"github.com/shellbound/shellbound/internal/style"
	"github.com/shellbound/shellbound/internal/ui/listpanel"
)

// Model is the inventory panel.
type Model struct {
	theme style.Theme
	open  bool
	items []storage.Item
	err   error
}

// New creates a closed inventory panel.
func New(theme style.Theme) Model {
	return Model{theme: theme}
}

// Open shows the panel with the given items (load them first; pass err to
// surface a storage failure).
func (m *Model) Open(items []storage.Item, err error) {
	m.open = true
	m.items = items
	m.err = err
}

// Close hides the panel.
func (m *Model) Close() { m.open = false }

// IsOpen reports whether the panel is showing.
func (m *Model) IsOpen() bool { return m.open }

// Content returns the panel for the pixel renderer.
func (m *Model) Content() listpanel.Content {
	c := listpanel.Content{Title: "Inventory", Cursor: -1, Footer: "Esc to close"}
	switch {
	case m.err != nil:
		c.Lines = []string{"Could not load your satchel."}
	case len(m.items) == 0:
		c.Lines = []string{"Your satchel is empty.", "Worlds beyond the portals will fill it."}
	default:
		for i, it := range m.items {
			if i >= 10 {
				c.Lines = append(c.Lines, fmt.Sprintf("... and %d more", len(m.items)-i))
				break
			}
			c.Lines = append(c.Lines, fmt.Sprintf("%-24s x%d", it.Name, it.Qty), "  from "+it.WorldKey)
		}
	}
	return c
}
