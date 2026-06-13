// Package comingsoon is the 1.0 placeholder world wired to every portal:
// a full-screen "coming soon" card with an Esc-to-return flow. It
// deliberately exercises the real save pipeline by persisting a visit
// counter, proving per-(player, world) isolation end to end.
package comingsoon

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/world"
)

// World is a placeholder world registered under a configurable key so the
// same implementation can stand in for "bomberman", "chess" and "doom".
type World struct {
	key  string
	name string
}

// New creates a placeholder world with the given stable key and display
// name.
func New(key, name string) *World {
	return &World{key: key, name: name}
}

// Key implements world.World.
func (w *World) Key() string { return w.key }

// Name implements world.World.
func (w *World) Name() string { return w.name }

// Init implements world.World.
func (w *World) Init(ctx world.Context) tea.Model {
	m := model{ctx: ctx, name: w.name}
	m.visits = bumpVisits(ctx.Save)
	return m
}

// bumpVisits loads, increments and stores the visit counter. Any storage
// error degrades to 0 (the counter is cosmetic).
func bumpVisits(store world.SaveStore) int {
	if store == nil {
		return 0
	}
	data, err := store.Load()
	if err != nil {
		return 0
	}
	n := 0
	if len(data) > 0 {
		if parsed, perr := strconv.Atoi(strings.TrimSpace(string(data))); perr == nil {
			n = parsed
		}
	}
	n++
	if err := store.Save([]byte(strconv.Itoa(n))); err != nil {
		return 0
	}
	return n
}

type model struct {
	ctx    world.Context
	name   string
	visits int
	w, h   int
}

// Init implements tea.Model.
func (m model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "enter":
			if m.ctx.Exit != nil {
				m.ctx.Exit()
			}
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m model) View() string {
	lines := []string{
		"╭──────────────────────────────────╮",
		fmt.Sprintf("│ %s │", center(m.name, 32)),
		"│                                  │",
		fmt.Sprintf("│ %s │", center("✦ Coming soon ✦", 32)),
		"│                                  │",
		fmt.Sprintf("│ %s │", center(visitLine(m.visits), 32)),
		"│                                  │",
		fmt.Sprintf("│ %s │", center("press Esc to return", 32)),
		"╰──────────────────────────────────╯",
	}
	return place(lines, m.w, m.h)
}

func visitLine(n int) string {
	switch n {
	case 0:
		return "The portal hums quietly."
	case 1:
		return "First time peeking through."
	default:
		return fmt.Sprintf("You have peeked %d times.", n)
	}
}

// center pads s with spaces to width w (best effort for plain ASCII plus
// the ✦ sparkles).
func center(s string, w int) string {
	n := len([]rune(s))
	if n >= w {
		return s
	}
	left := (w - n) / 2
	right := w - n - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// place centers the card in the terminal with blank padding.
func place(lines []string, termW, termH int) string {
	var sb strings.Builder
	topPad := 0
	if termH > len(lines) {
		topPad = (termH - len(lines)) / 2
	}
	for i := 0; i < topPad; i++ {
		sb.WriteByte('\n')
	}
	for i, line := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		pad := 0
		if w := len([]rune(line)); termW > w {
			pad = (termW - w) / 2
		}
		sb.WriteString(strings.Repeat(" ", pad))
		sb.WriteString(line)
	}
	return sb.String()
}
