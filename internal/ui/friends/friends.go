// Package friends implements the friends/DM panel: a list of friends and
// DM partners with presence dots, and a per-conversation view backed by
// persisted DMs.
package friends

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/storage"
	"github.com/shellbound/shellbound/internal/style"
)

// Panel geometry.
const (
	panelWidth  = 48
	convoHeight = 12
	maxListRows = 12
)

// SendFunc delivers and persists one outgoing DM; the overworld provides
// it (it owns the hub handle).
type SendFunc func(to storage.Player, text string) error

type mode int

const (
	modeList mode = iota
	modeConvo
)

// row is one list entry: a friend, a DM partner, or both.
type row struct {
	player   storage.Player
	isFriend bool
	hasDMs   bool
}

// Model is the friends/DM panel.
type Model struct {
	theme style.Theme
	repos *storage.Repos
	self  storage.Player
	send  SendFunc

	open   bool
	mode   mode
	rows   []row
	cursor int
	online map[int64]bool
	err    error

	// Conversation state.
	convoWith storage.Player
	convo     []storage.DM
	names     map[int64]storage.Player // id -> player for rendering
	vp        viewport.Model
	input     textinput.Model
}

// New creates a closed friends panel.
func New(theme style.Theme, repos *storage.Repos, self storage.Player, send SendFunc) Model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.PromptStyle = theme.Text
	ti.TextStyle = theme.Text
	ti.CharLimit = 240
	ti.Placeholder = "message…"
	vp := viewport.New(panelWidth-4, convoHeight)
	return Model{theme: theme, repos: repos, self: self, send: send, vp: vp, input: ti}
}

// IsOpen reports whether the panel is showing.
func (m *Model) IsOpen() bool { return m.open }

// Open shows the panel in list mode, refreshing friends, DM partners and
// presence.
func (m *Model) Open(online map[int64]bool) {
	m.open = true
	m.mode = modeList
	m.online = online
	m.cursor = 0
	m.refresh()
}

// Close hides the panel.
func (m *Model) Close() {
	m.open = false
	m.input.Blur()
}

// SetOnline updates presence dots while the panel is open.
func (m *Model) SetOnline(online map[int64]bool) { m.online = online }

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

// OpenConversation jumps straight into a conversation (used by /w and by
// unread-notification shortcuts as well as list selection).
func (m *Model) OpenConversation(with storage.Player) tea.Cmd {
	m.open = true
	m.mode = modeConvo
	m.convoWith = with
	m.loadConvo()
	m.input.SetValue("")
	return m.input.Focus()
}

// loadConvo (re)loads the message history with convoWith.
func (m *Model) loadConvo() {
	msgs, err := m.repos.DMs.Conversation(m.self.ID, m.convoWith.ID, 50)
	if err != nil {
		m.err = err
		return
	}
	m.convo = msgs
	m.names = map[int64]storage.Player{
		m.self.ID:      m.self,
		m.convoWith.ID: m.convoWith,
	}
	m.vp.SetContent(m.renderConvo())
	m.vp.GotoBottom()
}

// NotifyIncoming appends a just-received DM if its conversation is open.
// It returns true when the message was displayed (so the caller knows
// whether to count it as unread).
func (m *Model) NotifyIncoming(fromID int64, body string) bool {
	if !m.open || m.mode != modeConvo || m.convoWith.ID != fromID {
		return false
	}
	m.convo = append(m.convo, storage.DM{SenderID: fromID, RecipientID: m.self.ID, Body: body})
	m.vp.SetContent(m.renderConvo())
	m.vp.GotoBottom()
	return true
}

// Update handles input while the panel is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	if !m.open {
		return nil
	}
	if m.mode == modeList {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "esc", "f":
				m.Close()
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.rows)-1 {
					m.cursor++
				}
			case "enter":
				if m.cursor < len(m.rows) {
					return m.OpenConversation(m.rows[m.cursor].player)
				}
			}
		}
		return nil
	}

	// Conversation mode.
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.mode = modeList
			m.input.Blur()
			m.refresh()
			return nil
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			m.input.SetValue("")
			if text == "" {
				return nil
			}
			if m.send != nil {
				if err := m.send(m.convoWith, text); err != nil {
					m.err = err
					return nil
				}
			}
			m.convo = append(m.convo, storage.DM{SenderID: m.self.ID, RecipientID: m.convoWith.ID, Body: text})
			m.vp.SetContent(m.renderConvo())
			m.vp.GotoBottom()
			return nil
		case "pgup":
			m.vp.LineUp(3)
			return nil
		case "pgdown":
			m.vp.LineDown(3)
			return nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

// renderConvo formats the message history for the viewport.
func (m *Model) renderConvo() string {
	if len(m.convo) == 0 {
		return m.theme.Dim.Render("No messages yet. Say hi!")
	}
	var b strings.Builder
	for i, msg := range m.convo {
		if i > 0 {
			b.WriteString("\n")
		}
		sender, ok := m.names[msg.SenderID]
		name, color := "???", style.GreyDim
		if ok {
			name, color = sender.Username, sender.Color
		}
		b.WriteString(m.theme.Colored(color).Render(name))
		b.WriteString(m.theme.Text.Render(": " + msg.Body))
	}
	return b.String()
}

// View renders the panel box.
func (m *Model) View() string {
	if m.mode == modeConvo {
		return m.viewConvo()
	}
	return m.viewList()
}

func (m *Model) viewList() string {
	var b strings.Builder
	b.WriteString(m.theme.PanelTitle.Render("Friends & Messages"))
	b.WriteString("\n\n")
	switch {
	case m.err != nil:
		b.WriteString(m.theme.Dim.Render("Could not load your people."))
	case len(m.rows) == 0:
		b.WriteString(m.theme.Dim.Render("No friends yet."))
		b.WriteString("\n")
		b.WriteString(m.theme.Dim.Render("Try /friend add <name>."))
	default:
		for i, r := range m.rows {
			if i >= maxListRows {
				b.WriteString(m.theme.Dim.Render(fmt.Sprintf("… and %d more", len(m.rows)-i)))
				break
			}
			dot, dotStyle := "○", m.theme.Faded
			if m.online[r.player.ID] {
				dot, dotStyle = "●", m.theme.Text
			}
			cursor := "  "
			if i == m.cursor {
				cursor = "❯ "
			}
			tag := ""
			if r.hasDMs && !r.isFriend {
				tag = "  ✉"
			}
			b.WriteString(m.theme.Text.Render(cursor))
			b.WriteString(dotStyle.Render(dot))
			b.WriteString(" ")
			b.WriteString(m.theme.Colored(r.player.Color).Render(r.player.Username))
			b.WriteString(m.theme.Faded.Render(tag))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Faded.Render("↑/↓ select · Enter chat · Esc close"))
	return m.theme.PanelBorder.Width(panelWidth).Render(b.String())
}

func (m *Model) viewConvo() string {
	var b strings.Builder
	b.WriteString(m.theme.PanelTitle.Render("Chat with "))
	b.WriteString(m.theme.Colored(m.convoWith.Color).Render(m.convoWith.Username))
	b.WriteString("\n\n")
	b.WriteString(m.vp.View())
	b.WriteString("\n")
	m.input.Width = panelWidth - 10
	b.WriteString(m.input.View())
	b.WriteString("\n")
	b.WriteString(m.theme.Faded.Render("Enter send · PgUp/PgDn scroll · Esc back"))
	return m.theme.PanelBorder.Width(panelWidth).Render(b.String())
}
