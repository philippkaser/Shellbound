// Package inventory renders the inventory panel. In 1.0 nothing grants
// items, so the panel mostly shows its empty state — but it lists real
// rows from storage so future worlds' grants appear with no UI changes.
package inventory

import (
	"fmt"
	"strings"

	"github.com/shellbound/shellbound/internal/storage"
	"github.com/shellbound/shellbound/internal/style"
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

// View renders the panel box.
func (m *Model) View() string {
	var b strings.Builder
	b.WriteString(m.theme.PanelTitle.Render("Inventory"))
	b.WriteString("\n\n")
	switch {
	case m.err != nil:
		b.WriteString(m.theme.Dim.Render("Could not load your satchel."))
	case len(m.items) == 0:
		b.WriteString(m.theme.Dim.Render("Your satchel is empty."))
		b.WriteString("\n")
		b.WriteString(m.theme.Dim.Render("Worlds beyond the portals will fill it."))
	default:
		for i, it := range m.items {
			if i >= 10 {
				b.WriteString(m.theme.Dim.Render(fmt.Sprintf("… and %d more", len(m.items)-i)))
				break
			}
			line := fmt.Sprintf("%-24s ×%d", it.Name, it.Qty)
			b.WriteString(m.theme.Text.Render(line))
			b.WriteString("\n")
			b.WriteString(m.theme.Faded.Render("  from " + it.WorldKey))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n\n")
	b.WriteString(m.theme.Faded.Render("Esc to close"))
	return m.theme.PanelBorder.Width(44).Render(b.String())
}
