// Package shellmon is the Shellmon portal world (it replaces Chess): a
// single-player creature collector. You pick a starter, roam a wild route to
// catch and level creatures, and battle them with the 6v6 engine in
// internal/shellmon. The party persists through the per-world SaveStore as a
// JSON roster, so no database schema is involved.
package shellmon

import (
	"io"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/screen"
	mon "github.com/shellbound/shellbound/internal/shellmon"
	"github.com/shellbound/shellbound/internal/world"
)

const (
	minTermW  = 60
	minTermH  = 20
	tickEvery = 50 * time.Millisecond // steady render + animation cadence (20 Hz)
	maxParty  = 6
)

// state is the world's top-level screen.
type state int

const (
	stateStarter state = iota // choosing a first partner
	stateRoute                // roaming the wild route
	stateParty                // viewing the party
	stateBattle               // in a battle
	stateBadge                // the badge-award animation
)

// World is the Shellmon portal destination.
type World struct{ key, name string }

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
		scr:   canvas.New(1, 1),
		sb:    &strings.Builder{},
		pal:   ctx.Render.Palette,
		out:   ctx.Render.Out,
		cellW: ctx.Render.CellW,
		cellH: ctx.Render.CellH,
		rng:   rand.New(rand.NewSource(time.Now().UnixNano() ^ (ctx.Player.ID * 2654435761))),
	}
	if m.cellW <= 0 {
		m.cellW = 8
	}
	if m.cellH <= 0 {
		m.cellH = 16
	}
	m.loadRoster()
	m.healParty() // each visit starts rested
	if len(m.roster) == 0 {
		m.state = stateStarter
		m.starters = mon.Starters()
	} else {
		m.enterArea(startArea, -1, -1)
	}
	m.trans.begin(0.55) // wipe in as the portal opens into the world
	return m
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// model is the per-session Shellmon world.
type model struct {
	ctx       world.Context
	key, name string

	scr          *canvas.Canvas
	sb           *strings.Builder
	pal          *canvas.Palette
	out          io.Writer
	cellW, cellH int
	termW, termH int
	start        time.Time
	rng          *rand.Rand

	state   state
	roster  []*mon.Creature
	exiting bool

	// starter selection
	starters      []mon.Species
	starterCursor int

	// overworld
	route      *routeState
	partyCur   int             // cursor in the party view
	routeMsg   string          // a transient line shown on the overworld
	routeMsgAt time.Time       // when routeMsg was set
	defeated   map[string]bool // beaten trainer ids (persisted)
	found      map[string]bool // collected secret/item ids (persisted)
	badges     map[string]bool // earned gym badge ids (persisted)

	// battle
	bt      *battleUI
	youFX   hpFX
	foeFX   hpFX
	anim    battleAnim
	animSeq int

	// badge award animation (stateBadge)
	awardBadge string    // badge id being presented
	awardAt    time.Time // when the award animation started

	trans transition // screen wipe between Shellmon screens
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
		m.render()
		return m, tick()
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// View implements tea.Model: a constant sentinel; frames ship as Sixel.
func (m *model) View() string { return " " }

// handleKey dispatches input by the current screen.
func (m *model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.String() == "ctrl+c" {
		return m.leave()
	}
	k := key.String()
	switch m.state {
	case stateStarter:
		if m.keyStarter(k) {
			return m.leave()
		}
	case stateRoute:
		if m.keyRoute(k) {
			return m.leave()
		}
	case stateParty:
		m.keyParty(k)
	case stateBattle:
		m.keyBattle(k)
	case stateBadge:
		m.keyBadge(k)
	}
	m.render()
	return m, nil
}

// leave exits the world back to the plaza, saving progress first.
func (m *model) leave() (tea.Model, tea.Cmd) {
	if !m.exiting {
		m.exiting = true
		m.saveRoster()
		if m.ctx.Exit != nil {
			m.ctx.Exit()
		}
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

	pw, ph, left, top := screen.Dims(m.termW, m.termH, m.cellW, m.cellH)
	if pw <= 0 || ph <= 0 {
		return
	}
	m.scr.Resize(pw, ph)
	m.scr.Clear(canvas.Black)

	if m.termW < minTermW || m.termH < minTermH {
		msg := "please resize your terminal to at least 60x20"
		m.scr.DrawText(pw/2-canvas.TextWidth(msg)/2, ph/2, msg, 0xA1A1A1)
		m.ship(left, top)
		return
	}

	switch m.state {
	case stateStarter:
		m.drawStarter(pw, ph, t)
	case stateRoute:
		m.drawRoute(pw, ph, t)
	case stateParty:
		m.drawParty(pw, ph, t)
	case stateBattle:
		m.drawBattle(pw, ph, t)
	case stateBadge:
		m.drawBadgeAward(pw, ph, t)
	}
	m.trans.overlay(m.scr, pw, ph)
	m.ship(left, top)
}

func (m *model) ship(left, top int) {
	m.sb.Reset()
	screen.Place(m.sb, m.scr, m.pal, left, top)
	if s := m.sb.String(); s != "" {
		_, _ = io.WriteString(m.out, s)
	}
}
