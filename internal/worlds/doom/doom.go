package doom

import (
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/screen"
	"github.com/shellbound/shellbound/internal/world"
)

const (
	minTermW  = 60
	minTermH  = 20
	tickEvery = 50 * time.Millisecond // imp AI + steady render cadence (20 Hz)
)

// World is the Doom portal's destination: a first-person raycast shooter.
type World struct {
	key, name string
}

// New builds the world for a given portal key and display name.
func New(key, name string) *World { return &World{key: key, name: name} }

// Key implements world.World.
func (w *World) Key() string { return w.key }

// Name implements world.World.
func (w *World) Name() string { return w.name }

// Init implements world.World.
func (w *World) Init(ctx world.Context) tea.Model {
	m := &model{
		ctx:   ctx,
		key:   w.key,
		name:  w.name,
		g:     newGame(),
		scr:   canvas.New(1, 1),
		sb:    &strings.Builder{},
		pal:   ctx.Render.Palette,
		out:   ctx.Render.Out,
		cellW: ctx.Render.CellW,
		cellH: ctx.Render.CellH,
	}
	if m.cellW <= 0 {
		m.cellW = 8
	}
	if m.cellH <= 0 {
		m.cellH = 16
	}
	return m
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type model struct {
	ctx       world.Context
	key, name string
	g         *game
	scr       *canvas.Canvas
	sb        *strings.Builder
	pal       *canvas.Palette
	out       io.Writer
	zbuf      []float64
	cellW     int
	cellH     int
	termW     int
	termH     int
	start     time.Time
	granted   bool
	exiting   bool
}

// Init implements tea.Model.
func (m *model) Init() tea.Cmd { return tick() }

// Update implements tea.Model.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termW, m.termH = msg.Width, msg.Height
		m.render()
		return m, nil
	case tickMsg:
		m.g.tick()
		if m.g.state == won && !m.granted {
			m.granted = true
			if m.ctx.Inventory != nil {
				// Unlocks the Hellbreaker Horns cosmetic (and shows in the satchel).
				_ = m.ctx.Inventory.Grant("cosmetic.horns", "Hellbreaker Horns", 1)
			}
		}
		m.render()
		return m, tick()
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// View implements tea.Model: a constant sentinel; frames ship as Sixel.
func (m *model) View() string { return " " }

func (m *model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "ctrl+c":
		if !m.exiting {
			m.exiting = true
			if m.ctx.Exit != nil {
				m.ctx.Exit()
			}
		}
		return m, nil
	case "r":
		if m.g.state == lost {
			m.g.restart()
			m.render()
		}
		return m, nil
	}
	if m.g.state == playing {
		switch key.String() {
		case " ", "space":
			m.g.fire()
		case "w", "up":
			m.g.forward(moveStep)
		case "s", "down":
			m.g.forward(-moveStep)
		case "a":
			m.g.strafe(strafeStep)
		case "d":
			m.g.strafe(-strafeStep)
		case "left", "q":
			m.g.turn(-turnStep)
		case "right", "e":
			m.g.turn(turnStep)
		default:
			return m, nil
		}
		m.render()
	}
	return m, nil
}

// render composes a frame and ships it to the session.
func (m *model) render() {
	if m.exiting || m.out == nil {
		return
	}
	if m.start.IsZero() {
		m.start = time.Now()
	}
	t := time.Since(m.start).Seconds()
	if out := m.build(t); out != "" {
		_, _ = io.WriteString(m.out, out)
	}
}

// place centers the image at the given cell offset and returns the bytes.
func (m *model) place(left, top int) string {
	m.sb.Reset()
	screen.Place(m.sb, m.scr, m.pal, left, top)
	return m.sb.String()
}
