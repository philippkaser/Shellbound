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
	"github.com/shellbound/shellbound/internal/cosmetic"
	"github.com/shellbound/shellbound/internal/emote"
	"github.com/shellbound/shellbound/internal/hub"
	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
	"github.com/shellbound/shellbound/internal/render/light"
	"github.com/shellbound/shellbound/internal/render/screen"
	"github.com/shellbound/shellbound/internal/render/sprites"
	"github.com/shellbound/shellbound/internal/render/syncwriter"
	"github.com/shellbound/shellbound/internal/ui/chat"
)

// Render cadence and framing. The image is the shared fixed viewport (see
// internal/render/screen), letterboxed and centered, so every player sees the
// same slice of world regardless of terminal size.
const (
	// 20 fps is the steady cadence; wall-clock interpolation keeps motion fluid
	// at that rate while keeping per-frame Sixel CPU/bandwidth low so the loop
	// never falls behind a slow link. Input additionally Kick()s an out-of-band
	// frame so a keypress shows immediately instead of waiting up to a tick.
	renderFPS    = 20
	moveLerpTau  = 0.05 // seconds; avatar/camera easing — short so input feels tight and snappy
	ambientLight = 0.5
	// kickMinGap rate-limits out-of-band (input-driven) frames so a burst of
	// keys can't outrun the encoder; the steady tick covers anything skipped.
	kickMinGap = 22 * time.Millisecond
)

// playerSnapshot is the minimal per-player data the renderer needs.
type playerSnapshot struct {
	id       int64
	name     string
	color    string
	x, y     int // grid feet target (x = cell column, y = half-rows)
	dir      hub.Dir
	moving   bool
	cosmetic string // equipped headwear key
	emote    string // in-flight gesture key ("" = none)
}

// frameSnapshot is everything the render loop reads. Built cheaply on the
// bubbletea goroutine and handed across under a mutex; all slices are fresh
// copies, so the render loop can read them without further locking.
type frameSnapshot struct {
	termW, termH int
	cellW, cellH int // pixels per cell (best known estimate)
	players      []playerSnapshot
	selfID       int64

	chat           []chat.Entry
	toast          string
	panelLines     []string
	chatInput      string
	chatOpen       bool
	unreadName     string
	unreadN        int
	coins          int
	shopPrompt     bool
	interactPrompt string

	portalActive   bool
	portalProgress float64
	portalExiting  bool
	portalColor    canvas.Color
}

// entity is a render-side interpolated avatar.
type entity struct {
	fx, fy   float64 // current smoothed grid position
	tx, ty   float64 // target grid position
	dir      hub.Dir
	moving   bool
	name     string
	color    string
	cosmetic string
	emote    string
	seen     bool
}

// Renderer produces plaza frames on a dedicated goroutine so the heavy Sixel
// encode never blocks bubbletea's event loop. It interpolates player and
// camera motion at the render rate, so movement stays smooth no matter how the
// logic ticks.
type Renderer struct {
	pal   *canvas.Palette
	out   *syncwriter.Writer
	world *plaza.Map

	screen   *canvas.Canvas
	sb       *strings.Builder
	lights   *light.Field
	sortBuf  []*entity
	lightBuf []light.Light // reused each frame to avoid per-frame allocation

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
	kickCh   chan struct{} // input asks for an immediate, out-of-band frame
}

// NewRenderer builds a renderer for one session (not yet running). The cell
// size arrives per-frame in the snapshot, so it isn't needed here.
func NewRenderer(pal *canvas.Palette, out *syncwriter.Writer, world *plaza.Map) *Renderer {
	r := &Renderer{
		pal:    pal,
		out:    out,
		world:  world,
		screen: canvas.New(1, 1),
		sb:     &strings.Builder{},
		lights: light.NewField(),
		ents:   make(map[int64]*entity),
		stopCh: make(chan struct{}),
		kickCh: make(chan struct{}, 1),
	}
	r.active.Store(true)
	return r
}

// Kick requests an immediate frame (e.g. right after a keypress) so input shows
// without waiting for the next steady tick. Non-blocking and coalescing: a kick
// already pending is enough.
func (r *Renderer) Kick() {
	select {
	case r.kickCh <- struct{}{}:
	default:
	}
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
		case <-r.kickCh:
			// Out-of-band frame for input; skip if we just drew so a key burst
			// can't outrun the encoder (the steady tick still covers it).
			if r.active.Load() && time.Since(r.last) >= kickMinGap {
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

	out := r.build(snap, dt, t, now)
	// Re-check active after the (slow) encode: if a portal world took over while
	// we were building, drop this frame so it can't paint over the world's text.
	if out != "" && r.active.Load() {
		_, _ = r.out.WriteString(out)
	}
}

func (r *Renderer) build(snap frameSnapshot, dt, t float64, now time.Time) string {
	pw, ph, left, top := screen.Dims(snap.termW, snap.termH, snap.cellW, snap.cellH)
	r.screen.Resize(pw, ph)
	r.screen.Clear(canvas.Black)

	if snap.termW < minTermW || snap.termH < minTermH {
		msg := "please resize your terminal to at least 60x20"
		r.screen.DrawText(pw/2-canvas.TextWidth(msg)/2, ph/2, msg, 0xA1A1A1)
		return r.place(left, top)
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
	r.drawPortalFX(snap, pw, ph)
	return r.place(left, top)
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
		e.cosmetic = p.cosmetic
		e.emote = p.emote
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
	for i, e := range r.sortBuf {
		sx, sy := iso.Project(e.fx, e.fy)
		footX := int(sx - originSx)
		footY := int(sy-originSy) + iso.HH
		// An emote adds a body gesture: sway shifts the feet (and shadow), while
		// hop/crouch lift or settle the figure off its (stationary) shadow.
		edx, edy := 0, 0
		if e.emote != "" {
			edx, edy = emote.Offset(e.emote, t)
		}
		footX += edx
		// A soft oval contact shadow grounds the avatar (stays on the ground).
		drawShadow(r.screen, footX, footY)
		frame, bob := sprites.Pose(t, e.moving, float64(i)*1.3)
		drawY := footY + bob + edy
		sprites.Draw(r.screen, footX, drawY, sprites.Facing(e.dir), frame, e.moving)
		cosmetic.Draw(r.screen, footX, drawY, sprites.Facing(e.dir), e.cosmetic, t)
		nameW := canvas.TextWidth(e.name)
		r.screen.DrawTextShadow(footX-nameW/2, drawY-sprites.Height-canvas.LineH, e.name, canvas.Hex(e.color), 0x000000)
		if e.emote != "" {
			_, hcy := sprites.HeadCenter(footX, drawY)
			emote.DrawBubble(r.screen, footX, hcy-sprites.HeadRadius(), e.emote, t)
		}
	}
}

// drawShadow darkens the floor under an avatar's feet into a soft 2:1 oval so
// the figure feels grounded rather than floating.
func drawShadow(c *canvas.Canvas, footX, footY int) {
	const rx, ry = 8, 4
	for dy := -ry; dy <= ry; dy++ {
		w := float64(rx) * math.Sqrt(math.Max(0, 1-float64(dy*dy)/float64(ry*ry)))
		iw := int(w)
		yy := footY + dy - 1
		for dx := -iw; dx <= iw; dx++ {
			c.Set(footX+dx, yy, c.At(footX+dx, yy).Scale(0.55))
		}
	}
}

func (r *Renderer) applyLighting(snap frameSnapshot, originSx, originSy, t float64, pw, ph int) {
	lights := r.lightBuf[:0]
	if self := r.ents[snap.selfID]; self != nil {
		sx, sy := iso.Project(self.fx, self.fy)
		// The lantern you carry breathes a little so the world feels alive.
		pulse := 0.9 + 0.1*math.Sin(t*1.7)
		lights = append(lights, light.Light{
			X: int(sx - originSx), Y: int(sy-originSy) + iso.HH - 18,
			Radius: 165, Power: 0.85 * pulse,
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
		lights = append(lights, light.Light{X: hx, Y: hy, Radius: 165, Power: 0.65 * k})
		glows = append(glows, glowSpec{hx, hy, k})
	}

	// Each portal sheds a soft, steady light onto the floor around it so it
	// glows into the environment (the colored bloom is painted in RenderIso).
	for _, p := range plaza.Portals {
		gx, gy := p.GlowCenter(originSx, originSy)
		if gx < -150 || gx > pw+150 || gy < -150 || gy > ph+150 {
			continue
		}
		lights = append(lights, light.Light{X: gx, Y: gy, Radius: 140, Power: 0.45})
	}

	// The shop stall glows softly under its awning so it reads as a welcoming
	// spot even when the plaza is dimmed.
	for _, s := range r.world.Shops {
		sx, sy := plaza.ShopLight(s.X, s.Y, originSx, originSy)
		if sx < -150 || sx > pw+150 || sy < -150 || sy > ph+150 {
			continue
		}
		lights = append(lights, light.Light{X: sx, Y: sy, Radius: 120, Power: 0.5})
	}
	r.lightBuf = lights // retain backing array for reuse next frame

	r.lights.Apply(r.screen, lights, ambientLight)

	// The player carries light (added above) but no glowing disc — only the
	// lamps flare.
	for _, g := range glows {
		light.Glow(r.screen, g.x, g.y, 46, canvas.RGB(255, 255, 255).Scale(0.7+0.3*g.k))
	}
}

func (r *Renderer) drawHUD(snap frameSnapshot, pw, ph int) {
	hint := "Enter chat · g emote · i inventory · c wardrobe · f friends · q quit"
	r.screen.DrawText(pw-canvas.TextWidth(hint)-6, ph-canvas.LineH-4, hint, 0x6E6E6E)

	// Coin purse, top-left, so the player always knows their balance.
	purse := "✦ " + strconv.Itoa(snap.coins)
	r.screen.DrawTextShadow(6, 4, purse, 0xF2F2F2, 0x000000)

	// Contextual nudges, stacked above the hint line.
	if snap.shopPrompt {
		prompt := "press e to shop"
		r.screen.DrawTextShadow(pw/2-canvas.TextWidth(prompt)/2, ph-2*canvas.LineH-10, prompt, 0xFFFFFF, 0x000000)
	}
	if snap.interactPrompt != "" {
		r.screen.DrawTextShadow(pw/2-canvas.TextWidth(snap.interactPrompt)/2, ph-3*canvas.LineH-12, snap.interactPrompt, 0xFFFFFF, 0x000000)
	}

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

// drawPortalFX overlays the portal transition: a colored 2:1 diamond that
// swallows the screen on entry (and shrinks back, revealing the plaza, on
// exit), with a bright glowing rim — matching the iso motif.
func (r *Renderer) drawPortalFX(snap frameSnapshot, pw, ph int) {
	if !snap.portalActive {
		return
	}
	col := snap.portalColor
	rim := col.Lighten(canvas.RGB(140, 140, 150))
	cx, cy := pw/2, ph/2
	maxR := pw/2 + ph + 8
	prog := snap.portalProgress
	if snap.portalExiting {
		prog = 1 - prog
	}
	rad := int(float64(maxR) * prog)
	for y := 0; y < ph; y++ {
		dy2 := abs(y-cy) * 2
		for x := 0; x < pw; x++ {
			d := abs(x-cx) + dy2
			switch {
			case d <= rad-3:
				r.screen.Set(x, y, col)
			case d <= rad:
				r.screen.Set(x, y, rim) // glowing edge
			}
		}
	}
}

// place centers the image at the given cell offset and appends the Sixel.
func (r *Renderer) place(left, top int) string {
	r.sb.Reset()
	screen.Place(r.sb, r.screen, r.pal, left, top)
	return r.sb.String()
}
