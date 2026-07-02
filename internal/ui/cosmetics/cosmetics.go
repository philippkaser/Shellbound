// Package cosmetics is the wardrobe panel: a list of the headwear the player
// owns (starters plus anything unlocked from the worlds), with the equipped
// piece marked. Selecting one equips it; the overworld persists and broadcasts
// the choice.
package cosmetics

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/cosmetic"
	"github.com/shellbound/shellbound/internal/style"
	"github.com/shellbound/shellbound/internal/ui/listpanel"
)

const maxListRows = 12

// Model is the wardrobe panel.
type Model struct {
	theme    style.Theme
	open     bool
	items    []cosmetic.Cosmetic
	cursor   int
	equipped string

	// pending is set when the player presses Enter; the overworld drains it
	// via TakeEquipped to persist and broadcast the change.
	pending *string
}

// New creates a closed wardrobe panel.
func New(theme style.Theme) Model { return Model{theme: theme} }

// IsOpen reports whether the panel is showing.
func (m *Model) IsOpen() bool { return m.open }

// Open shows the wardrobe. owned is the set of unlocked cosmetic keys (starters
// are always shown); equipped is the currently worn key.
func (m *Model) Open(owned map[string]bool, equipped string) {
	m.open = true
	m.equipped = equipped
	m.items = m.items[:0]
	for _, c := range cosmetic.All() {
		if c.Starter || owned[c.Key] {
			m.items = append(m.items, c)
		}
	}
	m.cursor = 0
	for i, c := range m.items {
		if c.Key == equipped {
			m.cursor = i
		}
	}
}

// Close hides the panel.
func (m *Model) Close() { m.open = false }

// TakeEquipped returns and clears a newly chosen cosmetic key.
func (m *Model) TakeEquipped() (string, bool) {
	if m.pending == nil {
		return "", false
	}
	k := *m.pending
	m.pending = nil
	return k, true
}

// Update handles navigation; Enter equips the highlighted cosmetic.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	if !m.open {
		return nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	s := key.String()
	if c, ok := listpanel.Nav(s, m.cursor, len(m.items)); ok {
		m.cursor = c
		return nil
	}
	switch s {
	case "esc", "c":
		m.Close()
	case "enter":
		if m.cursor < len(m.items) {
			k := m.items[m.cursor].Key
			m.equipped = k
			m.pending = &k
		}
	}
	return nil
}

// Content returns the panel for the pixel renderer; the worn piece is marked
// with "*" and the cursor row gets the renderer's selection bar.
func (m *Model) Content() listpanel.Content {
	c := listpanel.Content{
		Title:  "Wardrobe",
		Cursor: -1,
		Footer: "up/down select  Enter wear  Esc close",
	}
	if len(m.items) == 0 {
		c.Lines = []string{"Nothing to wear yet."}
		return c
	}
	for i, it := range m.items {
		if i >= maxListRows {
			c.Lines = append(c.Lines, "...")
			break
		}
		mark := " "
		if it.Key == m.equipped {
			mark = "*"
		}
		if i == m.cursor {
			c.Cursor = len(c.Lines)
		}
		c.Lines = append(c.Lines, mark+" "+listpanel.Pad(it.Name, 18)+it.Rarity.Label())
	}
	return c
}
