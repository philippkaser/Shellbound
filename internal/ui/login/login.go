// Package login implements the first-connection username picker. Returning
// players never see it — their fingerprint logs them in automatically.
package login

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/shellbound/shellbound/internal/auth"
	"github.com/shellbound/shellbound/internal/storage"
	"github.com/shellbound/shellbound/internal/style"
)

// DoneMsg is emitted when the player has been created; the session app
// switches to the plaza on receipt.
type DoneMsg struct {
	Player *storage.Player
}

// QuitMsg is emitted when the user backs out of registration.
type QuitMsg struct{}

// Model is the username picker.
type Model struct {
	theme       style.Theme
	players     *storage.Players
	fingerprint string
	color       string
	input       textinput.Model
	errText     string
	w, h        int
}

// New creates the picker for a fingerprint that has no player yet.
func New(theme style.Theme, players *storage.Players, fingerprint string) Model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.PromptStyle = theme.Text
	ti.TextStyle = theme.Text
	ti.CharLimit = 16
	ti.Placeholder = "username"
	ti.Focus()
	return Model{
		theme:       theme,
		players:     players,
		fingerprint: fingerprint,
		color:       style.PlayerColor(fingerprint),
		input:       ti,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return textinput.Blink }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, func() tea.Msg { return QuitMsg{} }
		case "enter":
			return m.submit()
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// submit validates and persists the chosen username.
func (m Model) submit() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.input.Value())
	if !auth.ValidUsername(name) {
		m.errText = "3–16 characters: letters, digits, _ or -"
		return m, nil
	}
	player, err := m.players.Create(m.fingerprint, name, m.color)
	if err != nil {
		if errors.Is(err, storage.ErrUsernameTaken) {
			m.errText = fmt.Sprintf("%q is taken — try another", name)
		} else {
			m.errText = "could not save — try again"
		}
		return m, nil
	}
	return m, func() tea.Msg { return DoneMsg{Player: player} }
}

// View implements tea.Model.
func (m Model) View() string {
	title := m.theme.PanelTitle.Render("S H E L L B O U N D")
	sub := m.theme.Dim.Render("a plaza at the end of an ssh pipe")

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(sub)
	b.WriteString("\n\n")
	b.WriteString(m.theme.Text.Render("Choose a username:"))
	b.WriteString("\n")
	m.input.Width = 24
	b.WriteString(m.input.View())
	b.WriteString("\n")
	if m.errText != "" {
		b.WriteString(m.theme.Error.Render(m.errText))
	} else {
		b.WriteString(m.theme.Faded.Render("your color: "))
		b.WriteString(m.theme.Colored(m.color).Render("██████"))
	}
	b.WriteString("\n\n")
	b.WriteString(m.theme.Faded.Render("key " + shortFP(m.fingerprint)))
	b.WriteString("\n")
	b.WriteString(m.theme.Faded.Render("Enter confirm · Esc leave"))

	card := m.theme.PanelBorder.Width(46).Render(b.String())
	if m.w > 0 && m.h > 0 {
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, card)
	}
	return card
}

// shortFP abbreviates a fingerprint for display.
func shortFP(fp string) string {
	if len(fp) <= 24 {
		return fp
	}
	return fp[:24] + "…"
}
