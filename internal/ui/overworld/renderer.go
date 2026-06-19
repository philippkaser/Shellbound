package overworld

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shellbound/shellbound/internal/anim"
	"github.com/shellbound/shellbound/internal/hub"
	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
	"github.com/shellbound/shellbound/internal/render/light"
	"github.com/shellbound/shellbound/internal/render/sprites"
	"github.com/shellbound/shellbound/internal/render/syncwriter"
	"github.com/shellbound/shellbound/internal/ui/chat"
)

// Render cadence and framing. The play area is capped so a huge terminal shows
// the same world view as a modest one (and to bound per-frame Sixel bandwidth);
// the image is centered in the terminal.
const (
	// 20 fps is plenty: wall-clock interpolation keeps motion fluid, while the
	// lower rate eases per-frame Sixel CPU/bandwidth so the render loop never
	// falls behind a slow link.
	renderFPS    = 20
	maxWorldW    = 960  // max play-area width in pixels
	maxWorldH    = 600  // max play-area height in pixels
	moveLerpTau  = 0.09 // seconds; avatar/camera easing time constant
	ambientLight = 0.5
)

// playerSnapshot is the minimal per-player data the renderer needs.
type playerSnapshot struct {
	id     int64
	name   string
	color  string
	x, y   int // grid feet target (x = cell column, y = half-rows)
	dir    hub.Dir
	moving bool
}

// frameSnapshot is everything the render loop reads. Built cheaply on the
// bubbletea goroutine and handed across under a mutex; all slices are fresh
// copies, so the render loop can read them without further locking.
type frameSnapshot struct {
	termW, termH     int
	termPxW, termPxH int // drawable pixels, 0 if unknown
	players          []playerSnapshot
	selfID           int64

	chat       []chat.Entry
	toast      string
	panelLines []string
	chatInput  string
	chatOpen   bool
	unreadName string
	unreadN    int
}

// entity is a render-side interpolated avatar.
type entity struct {
	fx, fy float64 // current smoothed grid position
	tx, ty float64 // target grid position
	dir    hub.Dir
	moving bool
	name   string
	color  string
	seen   bool
}

// Renderer produces plaza frames on a dedicated goroutine so the heavy Sixel
// encode never blocks bubbletea's event loop. It interpolates player and
// camera motion at the render rate, so movement stays smooth no matter how the
// logic ticks.
type Renderer struct {
	pal          *canvas.Palette
	out          *syncwriter.Writer
	cellW, cellH int
	world        *plaza.Map

	screen  *canvas.Canvas
	sb      *strings.Builder
	lights  *light.Field
	sortBuf []*entity

	mu   sync.Mutex
	snap frameSnapshot

	active  atomic.Bool
	reprime atomic.Bool

	// render-goroutine-owned interpolation state
	ents       map[int64]*entity
	camX, camY float64
	primed     bool
	start      time.Time
	last       time.Time

	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewRenderer builds a renderer for one session (not yet running).
func NewRenderer(env Env, world *plaza.Map) *Renderer {
	r := &Renderer{
		pal: env.Pal, out: env.Out, cellW: env.CellW, cellH: env.CellH,
		world:  world,
		screen: canvas.New(1, 1),
		sb:     &strings.Builder{},
		lights: light.NewField(),
		ents:   make(map[int64]*entity),
		stopCh: make(chan struct{}),
	}
	r.active.Store(true)
	return r
}

// Start launches the render goroutine.
func (r *Renderer) Start() { go r.loop() }

// Stop halts the render goroutine (idempotent).
func (r *Renderer) Stop() { r.stopOnce.Do(func() { close(r.stopCh) }) }

// SetActive gates frame output. Re-activating re-primes interpolation so the
// avatar snaps to its true position instead of gliding from a stale one.
func (r *Renderer) SetActive(b bool) {
	if b {
		r.reprime.Store(true)
	}
	r.active.Store(b)
}

// Submit hands the renderer the latest snapshot.
func (r *Renderer) Submit(s frameSnapshot) {
	r.mu.Lock()
	r.snap = s
	r.mu.Unlock()
}

func (r *Renderer) loop() {
	tick := time.NewTicker(time.Second / renderFPS)
	defer tick.Stop()
	for {
		select {
		case <-r.stopCh:
			return
		case <-tick.C:
			if r.active.Load() {
				r.frame()
			}
		}
	}
}

func (r *Renderer) frame() {
	r.mu.Lock()
	snap := r.snap
	r.mu.Unlock()
	if snap.termW <= 0 || snap.termH <= 0 {
		return
	}

	now := time.Now()
	if r.start.IsZero() {
		r.start, r.last = now, now
	}
	dt := now.Sub(r.last).Seconds()
	if dt <= 0 {
		dt = 1.0 / renderFPS
	} else if dt > 0.25 {
		dt = 0.25 // clamp after a stall so nothing teleports
	}
	r.last = now
	t := now.Sub(r.start).Seconds()

	if out := r.build(snap, dt, t, now); out != "" {
		_, _ = r.out.WriteString(out)
	}
}

// dims returns the frame's pixel size (capped to the play-area maximum) and
// the top-left cell offset that centers it in the terminal.
//
// When the client reports its drawable pixel size we derive the real cell
// size from it, make the image a whole number of cells, and center exactly.
// Otherwise we fall back to the assumed cell size (SHELLBOUND_CELL), which
// can leave centering slightly off if the guess is wrong.
func (r *Renderer) dims(snap frameSnapshot) (pw, ph, left, top int) {
	cols, rows := snap.termW, snap.termH

	cw, ch := r.cellW, r.cellH
	if cw <= 0 {
		cw = 8
	}
	if ch <= 0 {
		ch = 16
	}
	if snap.termPxW > 0 && snap.termPxH > 0 && cols > 0 && rows > 0 {
		cw, ch = snap.termPxW/cols, snap.termPxH/rows
		if cw < 1 {
			cw = 1
		}
		if ch < 1 {
			ch = 1
		}
	}

	// Image spans whole cells, capped to the play area.
	imgCols := cols
	if maxCols := maxWorldW / cw; imgCols > maxCols {
		imgCols = maxCols
	}
	imgRows := rows
	if maxRows := maxWorldH / ch; imgRows > maxRows {
		imgRows = maxRows
	}
	if imgCols < 1 {
		imgCols = 1
	}
	if imgRows < 1 {
		imgRows = 1
	}
	pw, ph = imgCols*cw, imgRows*ch
	if left = (cols - imgCols) / 2; left < 0 {
		left = 0
	}
	if top = (rows - imgRows) / 2; top < 0 {
		top = 0
	}
	return
}

func (r *Renderer) build(snap frameSnapshot, dt, t float64, now time.Time) string {
	pw, ph, left, top := r.dims(snap)
	r.screen.Resize(pw, ph)
	r.screen.Clear(canvas.Black)

	if snap.termW < minTermW || snap.termH < minTermH {
		msg := "please resize your terminal to at least 60x20"
		r.screen.DrawText(pw/2-canvas.TextWidth(msg)/2, ph/2, msg, 0xA1A1A1)
		return r.encode(left, top)
	}

	if r.reprime.Swap(false) {
		r.primed = false
	}
	r.updateEntities(snap)
	r.interpolate(dt)
	if self := r.ents[snap.selfID]; self != nil {
		r.camX, r.camY = self.fx, self.fy
	}

	csx, csy := iso.Project(r.camX, r.camY)
	originSx := csx - float64(pw)/2
	originSy := csy - float64(ph)/2

	r.world.RenderIso(r.screen, originSx, originSy, t)
	for _, p := range plaza.Portals {
		p.RenderIso(r.screen, t, originSx, originSy)
	}
	r.drawPlayers(originSx, originSy, t)
	r.applyLighting(snap, originSx, originSy, t, pw, ph)
	r.world.RenderAmbient(r.screen, originSx, originSy, t)

	chat.RenderEntries(r.screen, snap.chat, now, 4, ph-3*canvas.LineH, pw*2/3)
	r.drawHUD(snap, pw, ph)
	r.drawToast(snap, pw)
	if len(snap.panelLines) > 0 {
		r.drawPanel(snap.panelLines, pw, ph)
	}
	if snap.chatOpen {
		r.drawInputBar(snap.chatInput, pw, ph)
	}
	return r.encode(left, top)
}

// updateEntities reconciles the interpolation set with the snapshot and prunes
// players who have left.
func (r *Renderer) updateEntities(snap frameSnapshot) {
	for _, e := range r.ents {
		e.seen = false
	}
	for _, p := range snap.players {
		tx, ty := float64(p.x), float64(p.y)/2
		e := r.ents[p.id]
		if e == nil {
			e = &entity{fx: tx, fy: ty}
			r.ents[p.id] = e
		}
		e.tx, e.ty = tx, ty
		e.dir, e.moving = p.dir, p.moving
		e.name, e.color = p.name, p.color
		e.seen = true
	}
	for id, e := range r.ents {
		if !e.seen {
			delete(r.ents, id)
		}
	}
	if !r.primed {
		for _, e := range r.ents {
			e.fx, e.fy = e.tx, e.ty
		}
		if self := r.ents[snap.selfID]; self != nil {
			r.camX, r.camY = self.fx, self.fy
		}
		r.primed = true
	}
}

// interpolate eases every avatar toward its grid target with frame-rate-
// independent exponential smoothing.
func (r *Renderer) interpolate(dt float64) {
	f := 1 - math.Exp(-dt/moveLerpTau)
	for _, e := range r.ents {
		e.fx += (e.tx - e.fx) * f
		e.fy += (e.ty - e.fy) * f
	}
}

func (r *Renderer) drawPlayers(originSx, originSy, t float64) {
	r.sortBuf = r.sortBuf[:0]
	for _, e := range r.ents {
		r.sortBuf = append(r.sortBuf, e)
	}
	sort.Slice(r.sortBuf, func(i, j int) bool {
		return r.sortBuf[i].fx+r.sortBuf[i].fy < r.sortBuf[j].fx+r.sortBuf[j].fy
	})
	for _, e := range r.sortBuf {
		sx, sy := iso.Project(e.fx, e.fy)
		footX := int(sx - originSx)
		footY := int(sy-originSy) + iso.HH
		frame := 0
		if e.moving {
			frame = int(t * 8)
		}
		sprites.Draw(r.screen, footX, footY, sprites.Facing(e.dir), frame, e.moving)
		nameW := canvas.TextWidth(e.name)
		r.screen.DrawTextShadow(footX-nameW/2, footY-sprites.Height-canvas.LineH, e.name, canvas.Hex(e.color), 0x000000)
	}
}

func (r *Renderer) applyLighting(snap frameSnapshot, originSx, originSy, t float64, pw, ph int) {
	lights := make([]light.Light, 0, len(r.world.Lamps)+1)
	if self := r.ents[snap.selfID]; self != nil {
		sx, sy := iso.Project(self.fx, self.fy)
		lights = append(lights, light.Light{
			X: int(sx - originSx), Y: int(sy-originSy) + iso.HH - 18,
			Radius: 120, Power: 0.8,
		})
	}

	type glowSpec struct {
		x, y int
		k    float64
	}
	var glows []glowSpec
	for _, p := range r.world.Lamps {
		hx, hy := plaza.LampHead(p.X, p.Y, originSx, originSy)
		if hx < -150 || hx > pw+150 || hy < -150 || hy > ph+150 {
			continue
		}
		k := anim.Flicker(t, p.X*31+p.Y*7)
		lights = append(lights, light.Light{X: hx, Y: hy, Radius: 130, Power: 0.6 * k})
		glows = append(glows, glowSpec{hx, hy, k})
	}

	r.lights.Apply(r.screen, lights, ambientLight)

	// The player carries light (added above) but no glowing disc — only the
	// lamps flare.
	for _, g := range glows {
		light.Glow(r.screen, g.x, g.y, 46, canvas.RGB(255, 255, 255).Scale(0.7+0.3*g.k))
	}
}

func (r *Renderer) drawHUD(snap frameSnapshot, pw, ph int) {
	hint := "Enter chat · i inventory · f friends · q quit"
	r.screen.DrawText(pw-canvas.TextWidth(hint)-6, ph-canvas.LineH-4, hint, 0x6E6E6E)

	if snap.unreadName != "" {
		ind := "✉ " + snap.unreadName
		if snap.unreadN > 0 {
			ind += " +" + strconv.Itoa(snap.unreadN)
		}
		r.screen.DrawTextShadow(pw-canvas.TextWidth(ind)-6, 4, ind, 0xFFFFFF, 0x000000)
	}
}

func (r *Renderer) drawToast(snap frameSnapshot, pw int) {
	if snap.toast == "" {
		return
	}
	w := canvas.TextWidth(snap.toast) + 12
	x := pw/2 - w/2
	r.screen.FillRect(x, 4, w, canvas.LineH+6, 0xFFFFFF)
	r.screen.DrawText(x+6, 7, snap.toast, 0x000000)
}

func (r *Renderer) drawPanel(lines []string, pw, ph int) {
	px := r.screen.Pixels()
	for i := range px {
		px[i] = px[i].Scale(0.35) // dim the world behind the modal
	}
	maxw := 0
	for _, l := range lines {
		if w := canvas.TextWidth(l); w > maxw {
			maxw = w
		}
	}
	const padX, padY = 8, 8
	bw := maxw + padX*2
	bh := len(lines)*canvas.LineH + padY*2
	x := pw/2 - bw/2
	y := ph/2 - bh/2
	r.screen.FillRect(x, y, bw, bh, 0x0A0A0A)
	r.screen.Rect(x, y, bw, bh, 0xD4D4D4)
	ty := y + padY
	for i, l := range lines {
		col := canvas.Color(0xD4D4D4)
		if i == 0 {
			col = 0xFFFFFF // title
		}
		r.screen.DrawText(x+padX, ty, l, col)
		ty += canvas.LineH
	}
}

func (r *Renderer) drawInputBar(line string, pw, ph int) {
	bw := pw * 2 / 3
	if bw > pw-8 {
		bw = pw - 8
	}
	x := (pw - bw) / 2
	y := ph - canvas.LineH - 16
	r.screen.FillRect(x, y, bw, canvas.LineH+8, 0x101010)
	r.screen.Rect(x, y, bw, canvas.LineH+8, 0xA1A1A1)
	tx := r.screen.DrawText(x+6, y+4, line, 0xFFFFFF)
	r.screen.FillRect(tx, y+4, 2, canvas.GlyphH, 0xFFFFFF) // caret
}

// encode positions the cursor to center the image, then appends the Sixel.
func (r *Renderer) encode(left, top int) string {
	r.sb.Reset()
	r.sb.WriteString("\x1b[?25l\x1b[")
	r.sb.WriteString(strconv.Itoa(top + 1))
	r.sb.WriteByte(';')
	r.sb.WriteString(strconv.Itoa(left + 1))
	r.sb.WriteByte('H')
	r.screen.EncodeSixel(r.sb, r.pal)
	return r.sb.String()
}
