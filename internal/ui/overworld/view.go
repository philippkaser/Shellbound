package overworld

import (
	"sort"
	"time"

	"github.com/shellbound/shellbound/internal/anim"
	"github.com/shellbound/shellbound/internal/hub"
	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
	"github.com/shellbound/shellbound/internal/render/light"
	"github.com/shellbound/shellbound/internal/render/sprites"
)

// Viewport caps in pixels. Bounding the image in *pixels* (not cells) means
// everyone sees the same slice of the world regardless of their cell size or
// window: bigger terminals just get more letterbox around a centered image.
// At the current tile size this is ~16 tiles wide, with the avatar a clear
// ~1/13 of the frame height.
const (
	maxCanvasW = 896
	maxCanvasH = 560
)

// View returns a constant sentinel. The plaza does not render through
// bubbletea: it bakes a full frame into a pixel canvas and ships it as one
// Sixel image straight to the session (see maybeRender). Returning the same
// string every call keeps bubbletea's renderer quiescent so it never clobbers
// our graphics.
func (m Model) View() string { return " " }

// computeGeom returns the frame's pixel size (a whole number of terminal
// cells, capped) and the cursor-positioning prefix that centers it in the
// window.
func (m *Model) computeGeom() (pw, ph int, prefix string) {
	cw, ch := m.cellW, m.cellH
	if cw <= 0 {
		cw = 8
	}
	if ch <= 0 {
		ch = 16
	}
	pw, ph = m.termW*cw, m.termH*ch
	if pw > maxCanvasW {
		pw = maxCanvasW
	}
	if ph > maxCanvasH {
		ph = maxCanvasH
	}
	// Snap to whole cells so centering is exact.
	pw = (pw / cw) * cw
	ph = (ph / ch) * ch
	if pw < cw {
		pw = cw
	}
	if ph < ch {
		ph = ch
	}

	imgCols, imgRows := pw/cw, ph/ch
	leftCols := (m.termW - imgCols) / 2
	topRows := (m.termH - imgRows) / 2
	if leftCols < 0 {
		leftCols = 0
	}
	if topRows < 0 {
		topRows = 0
	}
	prefix = "\x1b[?25l\x1b[" + itoa(topRows+1) + ";" + itoa(leftCols+1) + "H"
	return pw, ph, prefix
}

// drawScene bakes one full frame into m.screen and returns the cursor prefix
// that positions it. It runs on the bubbletea update goroutine, so reading
// model state here is race-free; encoding the canvas to Sixel then happens
// off-thread (see maybeRender), keeping the heavy work off the event loop.
func (m *Model) drawScene() string {
	pw, ph, prefix := m.computeGeom()
	m.screen.Resize(pw, ph)
	m.screen.Clear(canvas.Black)

	if m.termW < minTermW || m.termH < minTermH {
		msg := "please resize your terminal to at least 60x20"
		m.screen.DrawText(pw/2-canvas.TextWidth(msg)/2, ph/2, msg, 0xA1A1A1)
		return prefix
	}

	t := time.Since(m.start).Seconds()
	now := time.Now()

	// Camera: project the smoothed grid position and center it.
	csx, csy := iso.Project(m.cam.X, m.cam.Y)
	originSx := csx - float64(pw)/2
	originSy := csy - float64(ph)/2

	// World, portals (the only color on the map), then players on top.
	m.world.RenderIso(m.screen, originSx, originSy, t)
	for _, p := range plaza.Portals {
		p.RenderIso(m.screen, m.field, t, originSx, originSy)
	}
	m.drawPlayers(originSx, originSy, t)

	// Interactive lighting: dim the plaza and let the player and lamps reveal
	// it, with blocky glow halos. Applied before the UI so text stays readable.
	m.applyLighting(originSx, originSy, t, pw, ph)

	// Text overlays, baked into the same image at full brightness.
	m.chat.RenderHistory(m.screen, now, 4, ph-3*canvas.LineH, pw*2/3)
	m.drawHUD(pw, ph)
	m.drawToast(pw)

	switch {
	case m.friends.IsOpen():
		m.drawPanel(pw, ph, m.friends.Lines())
	case m.inv.IsOpen():
		m.drawPanel(pw, ph, m.inv.Lines())
	}
	if m.chat.IsOpen() {
		m.drawInputBar(pw, ph)
	}

	return prefix
}

// drawPlayers bakes every avatar (remote and local), painter-sorted by
// isometric depth, with a name tag in the player's color above the head.
func (m *Model) drawPlayers(originSx, originSy, t float64) {
	states := make([]hub.PlayerState, 0, len(m.remotes)+1)
	for _, st := range m.remotes {
		states = append(states, st)
	}
	states = append(states, hub.PlayerState{
		Info:   hub.PlayerInfo{ID: m.player.ID, Name: m.player.Username, Color: m.player.Color},
		Pos:    hub.Pos{X: m.px, Y: m.py},
		Dir:    m.dir,
		Moving: m.moving,
	})
	// Depth = gx + gy; nearer (larger) drawn later so it overlaps.
	sort.Slice(states, func(i, j int) bool {
		di := float64(states[i].Pos.X) + float64(states[i].Pos.Y)/2
		dj := float64(states[j].Pos.X) + float64(states[j].Pos.Y)/2
		return di < dj
	})

	for _, st := range states {
		frame := 0
		if st.Moving {
			if st.Info.ID == m.player.ID {
				frame = m.walkCount / 2
			} else {
				frame = int(t * 6)
			}
		}
		gx := float64(st.Pos.X)
		gy := float64(st.Pos.Y) / 2
		sx, sy := iso.Project(gx, gy)
		footX := int(sx - originSx)
		footY := int(sy-originSy) + iso.HH
		sprites.Draw(m.screen, footX, footY, sprites.Facing(st.Dir), frame, st.Moving)

		nameW := canvas.TextWidth(st.Info.Name)
		nameY := footY - sprites.Height - canvas.LineH
		m.screen.DrawTextShadow(footX-nameW/2, nameY, st.Info.Name, canvas.Hex(st.Info.Color), 0x000000)
	}
}

// applyLighting dims the plaza toward an ambient floor, then lights it from
// the player and every nearby lamp (with organic flicker), finishing with
// blocky glow halos at each source.
func (m *Model) applyLighting(originSx, originSy, t float64, pw, ph int) {
	const ambient = 0.5

	// Player's carried light, anchored at the avatar's torso.
	psx, psy := iso.Project(float64(m.px), float64(m.py)/2)
	pfx := int(psx - originSx)
	pfy := int(psy-originSy) + iso.HH - 14

	lights := []light.Light{{X: pfx, Y: pfy, Radius: 82, Power: 0.75}}

	type glowSpec struct {
		x, y int
		k    float64
	}
	var glows []glowSpec
	for _, p := range m.world.Lamps {
		hx, hy := plaza.LampHead(p.X, p.Y, originSx, originSy)
		if hx < -120 || hx > pw+120 || hy < -120 || hy > ph+120 {
			continue // off-screen lamp, skip
		}
		k := anim.Flicker(t, p.X*31+p.Y*7)
		lights = append(lights, light.Light{X: hx, Y: hy, Radius: 96, Power: 0.6 * k})
		glows = append(glows, glowSpec{hx, hy, k})
	}

	m.lights.Apply(m.screen, lights, ambient)

	light.Glow(m.screen, pfx, pfy, 24, 0x9A9A9A)
	for _, g := range glows {
		light.Glow(m.screen, g.x, g.y, 34, canvas.RGB(255, 255, 255).Scale(0.7+0.3*g.k))
	}
}

// drawHUD bakes the hint line (bottom-right) and the unread-DM badge.
func (m *Model) drawHUD(pw, ph int) {
	hint := "Enter chat · i inventory · f friends · q quit"
	m.screen.DrawText(pw-canvas.TextWidth(hint)-6, ph-canvas.LineH-4, hint, 0x6E6E6E)

	if len(m.unread) > 0 && !m.friends.IsOpen() {
		var name string
		for _, n := range m.unread {
			name = n
			break
		}
		ind := "✉ " + name
		if extra := len(m.unread) - 1; extra > 0 {
			ind += " +" + itoa(extra)
		}
		m.screen.DrawTextShadow(pw-canvas.TextWidth(ind)-6, 4, ind, 0xFFFFFF, 0x000000)
	}
}

// drawToast bakes the top-center notification banner as an inverse box.
func (m *Model) drawToast(pw int) {
	msg := m.toasts.Message()
	if msg == "" {
		return
	}
	w := canvas.TextWidth(msg) + 12
	x := pw/2 - w/2
	m.screen.FillRect(x, 4, w, canvas.LineH+6, 0xFFFFFF)
	m.screen.DrawText(x+6, 7, msg, 0x000000)
}

// drawPanel bakes a centered modal box (inventory, friends) over a dimmed
// scene.
func (m *Model) drawPanel(pw, ph int, lines []string) {
	// Dim the world behind the panel.
	px := m.screen.Pixels()
	for i := range px {
		px[i] = px[i].Scale(0.35)
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
	m.screen.FillRect(x, y, bw, bh, 0x0A0A0A)
	m.screen.Rect(x, y, bw, bh, 0xD4D4D4)
	ty := y + padY
	for i, l := range lines {
		col := canvas.Color(0xD4D4D4)
		if i == 0 {
			col = 0xFFFFFF // title
		}
		m.screen.DrawText(x+padX, ty, l, col)
		ty += canvas.LineH
	}
}

// drawInputBar bakes the chat input near the bottom with a solid caret.
func (m *Model) drawInputBar(pw, ph int) {
	line := m.chat.InputLine()
	bw := pw * 2 / 3
	if bw > pw-8 {
		bw = pw - 8
	}
	x := (pw - bw) / 2
	y := ph - canvas.LineH - 16
	m.screen.FillRect(x, y, bw, canvas.LineH+8, 0x101010)
	m.screen.Rect(x, y, bw, canvas.LineH+8, 0xA1A1A1)
	tx := m.screen.DrawText(x+6, y+4, line, 0xFFFFFF)
	m.screen.FillRect(tx, y+4, 2, canvas.GlyphH, 0xFFFFFF) // caret
}

// itoa is a tiny positive-int formatter to keep fmt out of the frame path.
func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 && i > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
