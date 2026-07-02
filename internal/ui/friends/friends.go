// Package friends implements the friends panel: a list of friends and DM
// partners with presence dots. Messaging itself lives in the chat console
// (/w <user> <msg>); selecting someone here pre-fills that command.
package friends

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/storage"
	"github.com/shellbound/shellbound/internal/style"
	"github.com/shellbound/shellbound/internal/ui/listpanel"
)

// maxListRows caps how many rows the panel shows before eliding.
const maxListRows = 12

// row is one list entry: a friend, a DM partner, or both.
type row struct {
	player   storage.Player
	isFriend bool
	hasDMs   bool
}

// Model is the friends panel.
type Model struct {
	theme style.Theme
	repos *storage.Repos
	self  storage.Player

	open   bool
	rows   []row
	cursor int
	online map[int64]bool
	err    error

	// selected is set when the player presses Enter on a row; the overworld
	// drains it via TakeSelected to start a whisper.
	selected *storage.Player
}

// New creates a closed friends panel.
func New(theme style.Theme, repos *storage.Repos, self storage.Player) Model {
	return Model{theme: theme, repos: repos, self: self}
}

// IsOpen reports whether the panel is showing.
func (m *Model) IsOpen() bool { return m.open }

// Open shows the panel, refreshing friends, DM partners and presence.
func (m *Model) Open(online map[int64]bool) {
	m.open = true
	m.online = online
	m.cursor = 0
	m.refresh()
}

// Close hides the panel.
func (m *Model) Close() { m.open = false }

// TakeSelected returns and clears the player chosen with Enter, so the
// caller can seed a /w to them. ok is false when nothing is pending.
func (m *Model) TakeSelected() (storage.Player, bool) {
	if m.selected == nil {
		return storage.Player{}, false
	}
	p := *m.selected
	m.selected = nil
	return p, true
}

// refresh reloads the list rows from storage.
func (m *Model) refresh() {
	m.err = nil
	m.rows = nil
	seen := map[int64]int{} // player id -> index in rows

	friends, err := m.repos.Friends.List(m.self.ID)
	if err != nil {
		m.err = err
		return
	}
	for _, f := range friends {
		seen[f.ID] = len(m.rows)
		m.rows = append(m.rows, row{player: f, isFriend: true})
	}
	partners, err := m.repos.DMs.Partners(m.self.ID)
	if err != nil {
		m.err = err
		return
	}
	for _, p := range partners {
		if i, ok := seen[p.Player.ID]; ok {
			m.rows[i].hasDMs = true
			continue
		}
		seen[p.Player.ID] = len(m.rows)
		m.rows = append(m.rows, row{player: p.Player, hasDMs: true})
	}
	if m.cursor >= len(m.rows) {
		m.cursor = 0
	}
}

// Update handles list navigation while the panel is open. Enter records the
// highlighted player and closes the panel.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	if !m.open {
		return nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	s := key.String()
	if c, ok := listpanel.Nav(s, m.cursor, len(m.rows)); ok {
		m.cursor = c
		return nil
	}
	switch s {
	case "esc", "f":
		m.Close()
	case "enter":
		if m.cursor < len(m.rows) {
			sel := m.rows[m.cursor].player
			m.selected = &sel
			m.Close()
		}
	}
	return nil
}

// Content returns the panel for the pixel renderer. "*" marks who's online
// and "(msg)" tags DM partners who aren't friends yet.
func (m *Model) Content() listpanel.Content {
	c := listpanel.Content{
		Title:  "Friends & Messages",
		Cursor: -1,
		Footer: "up/down select  Enter message  Esc close",
	}
	switch {
	case m.err != nil:
		c.Lines = []string{"Could not load your people."}
	case len(m.rows) == 0:
		c.Lines = []string{"No friends yet.", "Try /friend add <name>."}
	default:
		for i, r := range m.rows {
			if i >= maxListRows {
				c.Lines = append(c.Lines, fmt.Sprintf("... and %d more", len(m.rows)-i))
				break
			}
			dot := "-"
			if m.online[r.player.ID] {
				dot = "*"
			}
			tag := ""
			if r.hasDMs && !r.isFriend {
				tag = " (msg)"
			}
			if i == m.cursor {
				c.Cursor = len(c.Lines)
			}
			c.Lines = append(c.Lines, dot+" "+r.player.Username+tag)
		}
	}
	return c
}
