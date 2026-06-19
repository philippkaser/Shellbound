// Package starfall is Shellbound's first real portal world: a small, juicy
// arcade game rendered with the same Sixel pixel pipeline as the plaza. Stars
// rain down out of a twinkling sky; you fly a glowing vessel to catch them,
// each catch bursting into a shower of colored sparks. Your best score
// persists through the world save pipeline.
//
// Unlike the strict black-and-white plaza, a portal world is its own little
// universe — so Starfall lets color run wild (the stars sweep the whole hue
// ring of the shared palette).
package starfall

import (
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/frame"
	"github.com/shellbound/shellbound/internal/render/light"
	"github.com/shellbound/shellbound/internal/world"
)

// Frame caps, matching the plaza so the world fills the same bounded area.
const (
	maxCanvasW = 896
	maxCanvasH = 560
	tickEvery  = 33 * time.Millisecond
	heldWindow = 140 * time.Millisecond
)

// World is the registrable portal entry.
type World struct{}

// New creates the Starfall world.
func New() *World { return &World{} }

// Key implements world.World.
func (w *World) Key() string { return "starfall" }

// Name implements world.World.
func (w *World) Name() string { return "Starfall" }

// Init implements world.World.
func (w *World) Init(ctx world.Context) tea.Model { return newModel(ctx) }

type tickMsg time.Time
type frameDoneMsg struct{}

// star is a falling collectible; coordinates are fractions of the play field
// so the game is resolution-independent.
type star struct {
	fx, fy, vy, hue float64
}

// spark is a burst particle.
type spark struct {
	fx, fy, vx, vy, life, hue float64
}

// bgStar is a fixed twinkling background point.
type bgStar struct {
	fx, fy, phase float64
}

type model struct {
	ctx          world.Context
	screen       *canvas.Canvas
	sb           *strings.Builder
	rng          *rand.Rand
	termW, termH int
	inFlight     bool
	start        time.Time
	lastTick     time.Time

	held        map[string]time.Time
	vfx, vfy    float64 // vessel position (fractions)
	stars       []star
	sparks      []spark
	spawnIn     float64
	score, best int
	bg          []bgStar
}

func newModel(ctx world.Context) *model {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	m := &model{
		ctx:      ctx,
		screen:   canvas.New(1, 1),
		sb:       &strings.Builder{},
		rng:      rng,
		start:    time.Now(),
		lastTick: time.Now(),
		held:     make(map[string]time.Time),
		vfx:      0.5,
		vfy:      0.82,
		spawnIn:  0.5,
		best:     loadBest(ctx.Save),
	}
	m.bg = make([]bgStar, 70)
	for i := range m.bg {
		m.bg[i] = bgStar{fx: rng.Float64(), fy: rng.Float64(), phase: rng.Float64() * 6.28}
	}
	return m
}

// Init implements tea.Model.
func (m *model) Init() tea.Cmd { return tickCmd() }

// View returns a constant sentinel — the world ships Sixel frames itself,
// keeping bubbletea's renderer quiescent (see the plaza for the rationale).
func (m *model) View() string { return " " }

// Update implements tea.Model.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termW, m.termH = msg.Width, msg.Height
		return m, m.maybeRender()

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			if m.ctx.Exit != nil {
				m.ctx.Exit()
			}
			return m, nil
		case "up", "down", "left", "right", "w", "a", "s", "d":
			m.held[normalizeKey(msg.String())] = time.Now()
		}
		return m, nil

	case tickMsg:
		now := time.Time(msg)
		dt := now.Sub(m.lastTick).Seconds()
		if dt <= 0 || dt > 0.25 {
			dt = 1.0 / 30
		}
		m.lastTick = now
		m.step(dt)
		return m, tea.Batch(tickCmd(), m.maybeRender())

	case frameDoneMsg:
		m.inFlight = false
		return m, nil
	}
	return m, nil
}

func tickCmd() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func normalizeKey(k string) string {
	switch k {
	case "w":
		return "up"
	case "s":
		return "down"
	case "a":
		return "left"
	case "d":
		return "right"
	}
	return k
}

// step advances the simulation by dt seconds.
func (m *model) step(dt float64) {
	now := time.Now()
	held := func(name string) bool {
		t, ok := m.held[name]
		return ok && now.Sub(t) <= heldWindow
	}
	dirX, dirY := 0.0, 0.0
	if held("left") {
		dirX -= 1
	}
	if held("right") {
		dirX += 1
	}
	if held("up") {
		dirY -= 1
	}
	if held("down") {
		dirY += 1
	}
	const speed = 0.85
	m.vfx = clampf(m.vfx+dirX*speed*dt, 0.04, 0.96)
	m.vfy = clampf(m.vfy+dirY*speed*dt, 0.10, 0.94)

	// Spawn stars, faster as the score climbs.
	m.spawnIn -= dt
	if m.spawnIn <= 0 {
		m.stars = append(m.stars, star{
			fx:  0.05 + m.rng.Float64()*0.9,
			fy:  -0.04,
			vy:  0.16 + m.rng.Float64()*0.26,
			hue: m.rng.Float64() * 360,
		})
		base := 0.72 - float64(m.score)*0.004
		if base < 0.30 {
			base = 0.30
		}
		m.spawnIn = base + m.rng.Float64()*0.28
	}

	// Move stars; catch or drop them.
	kept := m.stars[:0]
	for _, s := range m.stars {
		s.fy += s.vy * dt
		dx, dy := s.fx-m.vfx, s.fy-m.vfy
		if dx*dx < 0.0026 && dy*dy < 0.005 {
			m.score++
			if m.score > m.best {
				m.best = m.score
				m.saveBest()
			}
			m.burst(s.fx, s.fy, s.hue)
			continue
		}
		if s.fy > 1.06 {
			continue // missed
		}
		kept = append(kept, s)
	}
	m.stars = kept

	// Advance sparks with a little gravity; cull dead ones.
	sk := m.sparks[:0]
	for _, p := range m.sparks {
		p.life -= dt * 1.5
		if p.life <= 0 {
			continue
		}
		p.fx += p.vx * dt
		p.fy += p.vy * dt
		p.vy += 0.35 * dt // gentle gravity
		sk = append(sk, p)
	}
	m.sparks = sk
}

// burst scatters sparks from a caught star.
func (m *model) burst(fx, fy, hue float64) {
	if len(m.sparks) > 600 {
		return // safety cap
	}
	for i := 0; i < 18; i++ {
		ang := m.rng.Float64() * 2 * math.Pi
		spd := 0.18 + m.rng.Float64()*0.4
		m.sparks = append(m.sparks, spark{
			fx: fx, fy: fy,
			vx:   math.Cos(ang) * spd,
			vy:   math.Sin(ang) * spd,
			life: 1,
			hue:  hue + m.rng.Float64()*40 - 20,
		})
	}
}

// maybeRender draws and ships one frame, paced so only one is ever in flight
// (the same off-thread pattern the plaza uses).
func (m *model) maybeRender() tea.Cmd {
	if m.inFlight || m.termW <= 0 || m.ctx.Screen.Out == nil {
		return nil
	}
	prefix := m.draw()
	m.inFlight = true
	screen, sb, pal, out := m.screen, m.sb, m.ctx.Screen.Pal, m.ctx.Screen.Out
	return func() tea.Msg {
		sb.Reset()
		sb.WriteString(prefix)
		screen.EncodeSixel(sb, pal)
		_, _ = out.WriteString(sb.String())
		return frameDoneMsg{}
	}
}

// draw bakes the current frame into the canvas and returns the centering
// prefix.
func (m *model) draw() string {
	pw, ph, prefix := frame.Geometry(m.termW, m.termH, m.ctx.Screen.CellW, m.ctx.Screen.CellH, maxCanvasW, maxCanvasH)
	m.screen.Resize(pw, ph)
	// Pure grey so it quantizes to the dark end of the grey ramp (a non-grey
	// near-black would snap to the nearest *saturated* register and flood the
	// sky with color).
	m.screen.Clear(0x080808)
	t := time.Since(m.start).Seconds()
	fw, fh := float64(pw), float64(ph)

	// Twinkling sky.
	for _, s := range m.bg {
		b := 0.35 + 0.65*0.5*(1+math.Sin(t*2+s.phase))
		v := uint8(180 * b)
		m.screen.Set(int(s.fx*fw), int(s.fy*fh), canvas.RGB(v, v, v))
	}

	// Falling stars, with a short fading trail and a colored glow.
	for _, s := range m.stars {
		x, y := int(s.fx*fw), int(s.fy*fh)
		for k := 1; k <= 4; k++ {
			m.screen.Set(x, y-k*3, canvas.HSL(s.hue, 0.9, 0.5).Scale(1-float64(k)*0.2))
		}
		light.Glow(m.screen, x, y, 12, canvas.HSL(s.hue, 0.9, 0.55))
		m.screen.FillCircle(x, y, 3, canvas.HSL(s.hue, 0.95, 0.72))
	}

	// Sparks.
	for _, p := range m.sparks {
		x, y := int(p.fx*fw), int(p.fy*fh)
		c := canvas.HSL(p.hue, 0.9, 0.6).Scale(clampf(p.life, 0, 1))
		m.screen.Set(x, y, c)
		m.screen.Set(x+1, y, c.Scale(0.6))
	}

	// The vessel: a bright core under a cyan glow, with a soft trail.
	vx, vy := int(m.vfx*fw), int(m.vfy*fh)
	light.Glow(m.screen, vx, vy, 18, canvas.HSL(190, 0.85, 0.6))
	m.screen.FillCircle(vx, vy, 4, canvas.RGB(0xEA, 0xFF, 0xFF))
	m.screen.FillEllipse(vx, vy+6, 5, 2, canvas.HSL(190, 0.7, 0.4))

	light.Vignette(m.screen, 0.4)

	// HUD.
	m.screen.DrawTextShadow(6, 5, "STARFALL", canvas.HSL(285, 0.8, 0.72), 0x000000)
	score := "SCORE " + strconv.Itoa(m.score) + "   BEST " + strconv.Itoa(m.best)
	m.screen.DrawTextShadow(pw-canvas.TextWidth(score)-6, 5, score, 0xFFFFFF, 0x000000)
	hint := "arrows / wasd to fly  -  Esc to return"
	m.screen.DrawTextShadow(pw/2-canvas.TextWidth(hint)/2, ph-canvas.LineH-4, hint, 0xA0A6AD, 0x000000)

	return prefix
}

// saveBest persists the high score (best-effort; the score is cosmetic).
func (m *model) saveBest() {
	if m.ctx.Save == nil {
		return
	}
	_ = m.ctx.Save.Save([]byte(strconv.Itoa(m.best)))
}

func loadBest(store world.SaveStore) int {
	if store == nil {
		return 0
	}
	data, err := store.Load()
	if err != nil || len(data) == 0 {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return n
}

func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
