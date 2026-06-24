// Package cosmetics is the wardrobe panel: a list of the headwear the player
// owns (starters plus anything unlocked from the worlds), with the equipped
// piece marked. Selecting one equips it; the overworld persists and broadcasts
// the choice.
package cosmetics

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/cosmetic"
	"github.com/shellbound/shellbound/internal/style"
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
	switch key.String() {
	case "esc", "c":
		m.Close()
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "enter":
		if m.cursor < len(m.items) {
			k := m.items[m.cursor].Key
			m.equipped = k
			m.pending = &k
		}
	}
	return nil
}

// Lines returns the panel content for the pixel renderer. The highlighted row
// is marked "> " and the worn one with "*".
func (m *Model) Lines() []string {
	out := []string{"Wardrobe", ""}
	if len(m.items) == 0 {
		out = append(out, "Nothing to wear yet.")
	}
	for i, c := range m.items {
		if i >= maxListRows {
			out = append(out, "...")
			break
		}
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		mark := " "
		if c.Key == m.equipped {
			mark = "*"
		}
		out = append(out, cursor+mark+" "+c.Name)
	}
	return append(out, "", "up/down select  Enter wear  Esc close")
}

// View renders the panel box (unused on the Sixel path, kept for parity).
func (m *Model) View() string {
	var b strings.Builder
	b.WriteString(m.theme.PanelTitle.Render("Wardrobe"))
	b.WriteString("\n\n")
	for i, c := range m.items {
		line := "  " + c.Name
		if i == m.cursor {
			line = "> " + c.Name
		}
		if c.Key == m.equipped {
			line += " (worn)"
		}
		b.WriteString(m.theme.Text.Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Faded.Render("up/down select  Enter wear  Esc close"))
	return m.theme.PanelBorder.Width(44).Render(b.String())
}
