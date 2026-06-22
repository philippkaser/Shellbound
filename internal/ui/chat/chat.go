// Package chat implements the plaza chat: a borderless fading history
// drawn straight onto the world canvas (bottom-left) and a Charm-style
// rounded input bar that appears on Enter.
package chat

import (
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/style"
)

// Fade timing: full brightness, then dimmed, then gone.
const (
	fadeAfter = 30 * time.Second
	dropAfter = 60 * time.Second
)

// maxVisible is how many history lines render at once.
const maxVisible = 12

// maxKept bounds the in-memory history.
const maxKept = 64

// Kind classifies a history entry, which controls its prefix and colors.
type Kind int

// Entry kinds.
const (
	KindChat Kind = iota
	KindEmote
	KindWhisperIn
	KindWhisperOut
	KindSystem
)

// Entry is one chat history line.
type Entry struct {
	Kind  Kind
	Name  string // counterpart name (sender, or recipient for KindWhisperOut)
	Color string // their personal hex color
	Text  string
	At    time.Time
}

// Model holds chat history and the input bar.
type Model struct {
	theme   style.Theme
	input   textinput.Model
	open    bool
	entries []Entry
}

// New creates a closed chat with empty history.
func New(theme style.Theme) Model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.PromptStyle = theme.Text
	ti.TextStyle = theme.Text
	ti.CharLimit = 240
	ti.Placeholder = "say something…"
	return Model{theme: theme, input: ti}
}

// Add appends an entry, stamping it with now.
func (m *Model) Add(e Entry) {
	e.At = time.Now()
	m.entries = append(m.entries, e)
	if len(m.entries) > maxKept {
		m.entries = m.entries[len(m.entries)-maxKept:]
	}
}

// AddSystem appends a grey system line (command output, errors).
func (m *Model) AddSystem(text string) {
	m.Add(Entry{Kind: KindSystem, Text: text})
}

// IsOpen reports whether the input bar is showing.
func (m *Model) IsOpen() bool { return m.open }

// Open reveals and focuses the input bar.
func (m *Model) Open() tea.Cmd {
	m.open = true
	m.input.SetValue("")
	m.input.Focus()
	return textinput.Blink
}

// OpenWith reveals the input bar pre-filled with text and the cursor at the
// end — used to seed a /w command when the player picks someone in the
// friends panel.
func (m *Model) OpenWith(text string) tea.Cmd {
	m.open = true
	m.input.SetValue(text)
	m.input.CursorEnd()
	m.input.Focus()
	return textinput.Blink
}

// Close hides the input bar without sending.
func (m *Model) Close() {
	m.open = false
	m.input.Blur()
	m.input.SetValue("")
}

// Update feeds a message to the open input bar. It returns the text to
// send when the user pressed Enter (empty string otherwise). Esc closes
// the bar. The caller should ignore everything while IsOpen() is false.
func (m *Model) Update(msg tea.Msg) (tea.Cmd, string) {
	if !m.open {
		return nil, ""
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			text := m.input.Value()
			m.Close()
			return nil, text
		case "esc":
			m.Close()
			return nil, ""
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd, ""
}

// InputLine returns the plain prompt+value the pixel renderer bakes into the
// input bar (the textinput's own styled View is not used in the Sixel path).
func (m *Model) InputLine() string {
	return m.input.Prompt + m.input.Value()
}

// ViewInput renders the input bar at the given total width (the bar takes
// ~60% of it, minimum 24 cells).
func (m *Model) ViewInput(totalWidth int) string {
	w := totalWidth * 6 / 10
	if w < 24 {
		w = 24
	}
	if w > totalWidth-2 {
		w = totalWidth - 2
	}
	// Interior width: subtract border (2) and padding (2).
	m.input.Width = w - 6
	return m.theme.InputBar.Width(w).Render(m.input.View())
}

// History returns the in-memory chat entries. Callers that hold the slice
// across goroutines should copy it first.
func (m *Model) History() []Entry { return m.entries }

// RenderEntries bakes up to maxVisible non-expired lines into the canvas,
// bottom-anchored so the baseline of the newest line sits at pixel (x,
// yBottom) and older lines stack upward, clipped to maxWPx pixels wide. Fresh
// lines are white with the sender's colored name (a sanctioned color pop);
// lines older than fadeAfter dim to grey; lines older than dropAfter vanish.
func RenderEntries(c *canvas.Canvas, entries []Entry, now time.Time, x, yBottom, maxWPx int) {
	maxW := maxWPx / canvas.AdvanceX
	if maxW < 8 {
		return
	}
	y := yBottom
	drawn := 0
	for i := len(entries) - 1; i >= 0 && drawn < maxVisible && y >= 0; i-- {
		e := entries[i]
		age := now.Sub(e.At)
		if age >= dropAfter {
			break // older entries are older still
		}
		faded := age >= fadeAfter

		nameCol := canvas.Hex(e.Color)
		textCol := canvas.Color(0xFFFFFF)
		if faded {
			nameCol = 0x737373
			textCol = 0x737373
		}
		if e.Kind == KindSystem {
			nameCol = 0x737373
			textCol = 0xA1A1A1
			if faded {
				textCol = 0x737373
			}
		}

		var prefix, body string
		switch e.Kind {
		case KindEmote:
			prefix = "* " + e.Name + " "
			body = e.Text
		case KindWhisperIn:
			prefix = "✉ " + e.Name + ": "
			body = e.Text
		case KindWhisperOut:
			prefix = "✉ → " + e.Name + ": "
			body = e.Text
		case KindSystem:
			prefix = "· "
			body = e.Text
		default:
			prefix = e.Name + ": "
			body = e.Text
		}

		pl := utf8.RuneCountInString(prefix)
		body = truncateRunes(body, maxW-pl)
		shadow := canvas.Color(0x000000)
		c.DrawTextShadow(x, y, prefix, nameCol, shadow)
		c.DrawTextShadow(x+pl*canvas.AdvanceX, y, body, textCol, shadow)
		y -= canvas.LineH
		drawn++
	}
}

// truncateRunes clips s to at most n runes, with a trailing ellipsis when
// clipped.
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	rs := []rune(s)
	if n == 1 {
		return "…"
	}
	return string(rs[:n-1]) + "…"
}
