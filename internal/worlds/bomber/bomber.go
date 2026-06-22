package bomber

import (
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/light"
	"github.com/shellbound/shellbound/internal/render/screen"
	"github.com/shellbound/shellbound/internal/world"
)

// Minimum playable terminal, and timing for the model.
const (
	minTermW = 60
	minTermH = 20

	tickEvery = 50 * time.Millisecond  // logic + steady render cadence (20 Hz)
	moveEvery = 120 * time.Millisecond // player step throttle (independent of tick)
	idleAfter = 240 * time.Millisecond // revert to idle walk pose this long after a step
)

// World is the Bomberman portal's destination: the bomb arena "The Vault".
type World struct {
	key, name string
}

// New builds the world for a given portal key and display name.
func New(key, name string) *World { return &World{key: key, name: name} }

// Key implements world.World.
func (w *World) Key() string { return w.key }

// Name implements world.World.
func (w *World) Name() string { return w.name }

// Init implements world.World: it builds a fresh arena and the renderer that
// ships Sixel frames to the session (the same pipeline the plaza uses).
func (w *World) Init(ctx world.Context) tea.Model {
	seed := time.Now().UnixNano() ^ (ctx.Player.ID * 2654435761)
	m := &model{
		ctx:    ctx,
		key:    w.key,
		name:   w.name,
		g:      newGame(seed),
		scr:    canvas.New(1, 1),
		sb:     &strings.Builder{},
		lights: light.NewField(),
		pal:    ctx.Render.Palette,
		out:    ctx.Render.Out,
		cellW:  ctx.Render.CellW,
		cellH:  ctx.Render.CellH,
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

// model is the per-session game: state plus its Sixel renderer.
type model struct {
	ctx       world.Context
	key, name string
	g         *game
	scr       *canvas.Canvas
	sb        *strings.Builder
	lights    *light.Field
	pal       *canvas.Palette
	out       io.Writer
	cellW     int
	cellH     int
	termW     int
	termH     int
	start     time.Time
	last      time.Time
	lastMove  time.Time
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
				_ = m.ctx.Inventory.Grant("bomberman.spark_core", "Spark Core", 1)
			}
		}
		m.render()
		return m, tick()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// View implements tea.Model: a constant sentinel, since frames ship as Sixel
// straight to the session (bubbletea's renderer stays quiescent).
func (m *model) View() string { return " " }

func (m *model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "q", "ctrl+c":
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
	case " ", "space":
		if m.g.plant() {
			m.render()
		}
		return m, nil
	}
	if dx, dy, ok := dir(key.String()); ok {
		if m.g.state == playing && time.Since(m.lastMove) >= moveEvery {
			if m.g.move(dx, dy) {
				m.lastMove = time.Now()
			}
			m.render()
		}
	}
	return m, nil
}

// dir maps a key to a 4-way step (no diagonals in the arena).
func dir(s string) (dx, dy int, ok bool) {
	switch strings.ToLower(s) {
	case "up", "w":
		return 0, -1, true
	case "down", "s":
		return 0, 1, true
	case "left", "a":
		return -1, 0, true
	case "right", "d":
		return 1, 0, true
	}
	return 0, 0, false
}

// render composes a frame and ships it to the session.
func (m *model) render() {
	if m.exiting || m.out == nil {
		return
	}
	now := time.Now()
	if m.start.IsZero() {
		m.start, m.last = now, now
	}
	dt := now.Sub(m.last).Seconds()
	if dt < 0 || dt > 0.25 {
		dt = 1.0 / 20
	}
	m.last = now
	t := now.Sub(m.start).Seconds()
	if out := m.build(t, dt); out != "" {
		_, _ = io.WriteString(m.out, out)
	}
}

// place centers the image at the given cell offset and returns the bytes.
func (m *model) place(left, top int) string {
	m.sb.Reset()
	screen.Place(m.sb, m.scr, m.pal, left, top)
	return m.sb.String()
}

// sinceMove reports how long since the last successful step (for the walk pose).
func (m *model) sinceMove() time.Duration { return time.Since(m.lastMove) }
