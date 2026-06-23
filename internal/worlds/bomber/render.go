package bomber

import (
	"fmt"
	"math"
	"sort"

	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
	"github.com/shellbound/shellbound/internal/render/light"
	"github.com/shellbound/shellbound/internal/render/screen"
	"github.com/shellbound/shellbound/internal/render/sprites"
)

// Monochrome tones for the vault (color is reserved for the world's hue accents).
const (
	floorA   = canvas.Color(0x141414)
	floorB   = canvas.Color(0x0E0E0E)
	floorEdg = canvas.Color(0x1E1E1E)

	wallTop = canvas.Color(0x9A9A9A)
	wallL   = canvas.Color(0x363636)
	wallR   = canvas.Color(0x6A6A6A)

	crateTop = canvas.Color(0xC9C9C9)
	crateL   = canvas.Color(0x5A5A5A)
	crateR   = canvas.Color(0x8E8E8E)
	crateLid = canvas.Color(0x3A3A3A)

	wallH  = 22
	crateH = 16

	ambient = 0.4
)

// accent returns an on-palette color in the world's hue at lightness l.
func (m *model) accent(l float64) canvas.Color { return plaza.AccentColor(m.key, l) }

// projection of a (fractional) cell to its ground-diamond top vertex on screen.
func proj(gx, gy, ox, oy float64) (int, int) {
	sx, sy := iso.Project(gx, gy)
	return int(sx - ox), int(sy - oy)
}

// build composes one frame into the model's canvas and returns the bytes to
// write (cursor placement + Sixel), or "" if there's nothing to draw.
func (m *model) build(t, dt float64) string {
	pw, ph, left, top := screen.Dims(m.termW, m.termH, m.cellW, m.cellH)
	m.scr.Resize(pw, ph)
	m.scr.Clear(canvas.Black)
	if m.termW < minTermW || m.termH < minTermH {
		msg := "please resize your terminal to at least 60x20"
		m.scr.DrawText(pw/2-canvas.TextWidth(msg)/2, ph/2, msg, 0xA1A1A1)
		return m.place(left, top)
	}

	m.interpolate(dt)

	// Center the arena in the viewport, nudged down for cube headroom at top.
	csx, csy := iso.Project(float64(cols-1)/2, float64(rows-1)/2)
	ox := csx - float64(pw)/2
	oy := csy - float64(ph)/2 - 18

	m.drawGround(ox, oy)
	m.drawPowerups(ox, oy, t)
	m.drawScene(ox, oy, t)
	m.drawBlasts(ox, oy, t)
	m.applyLighting(ox, oy, t)
	m.drawHUD(pw, ph)
	m.drawBanner(pw, ph)
	return m.place(left, top)
}

// interpolate eases the player and wisps toward their grid cells so motion
// glides between tiles regardless of the logic tick.
func (m *model) interpolate(dt float64) {
	f := 1.0
	if dt > 0 {
		f = 1 - math.Exp(-dt/0.06)
	}
	g := m.g
	g.pfx += (float64(g.px) - g.pfx) * f
	g.pfy += (float64(g.py) - g.pfy) * f
	for i := range g.enemies {
		e := &g.enemies[i]
		e.fx += (float64(e.x) - e.fx) * f
		e.fy += (float64(e.y) - e.fy) * f
	}
}

func (m *model) drawGround(ox, oy float64) {
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			if m.g.grid[y][x] == wall {
				continue // walls draw their own base
			}
			px, py := proj(float64(x), float64(y), ox, oy)
			col := floorA
			if (x+y)&1 == 1 {
				col = floorB
			}
			iso.DrawDiamond(m.scr, px, py, col, floorEdg)
		}
	}
}

func (m *model) drawPowerups(ox, oy, t float64) {
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			if m.g.power[y][x] == powNone || m.g.grid[y][x] != floor {
				continue
			}
			px, py := proj(float64(x), float64(y), ox, oy)
			cx, cy := px, py+iso.HH-int(3*math.Sin(t*3+float64(x)))
			col := m.accent(0.62)
			if m.g.power[y][x] == powReach {
				// a small plus
				m.scr.FillRect(cx-3, cy-1, 7, 3, col)
				m.scr.FillRect(cx-1, cy-3, 3, 7, col)
			} else {
				m.scr.FillCircle(cx, cy, 3, col)
				m.scr.FillCircle(cx-1, cy-1, 1, m.accent(0.5))
			}
		}
	}
}

// ritem is one depth-sorted thing to paint (walls, crates, bombs, wisps, you).
type ritem struct {
	depth  float64
	kind   int // 0 wall, 1 crate, 2 bomb, 3 enemy, 4 player
	x, y   int
	fx, fy float64
	idx    int
}

const (
	itWall = iota
	itCrate
	itBomb
	itEnemy
	itPlayer
)

func (m *model) drawScene(ox, oy, t float64) {
	g := m.g
	items := make([]ritem, 0, 64)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			switch g.grid[y][x] {
			case wall:
				items = append(items, ritem{depth: float64(x + y), kind: itWall, x: x, y: y})
			case crate:
				items = append(items, ritem{depth: float64(x + y), kind: itCrate, x: x, y: y})
			}
		}
	}
	for i := range g.bombs {
		b := g.bombs[i]
		items = append(items, ritem{depth: float64(b.x+b.y) + 0.1, kind: itBomb, x: b.x, y: b.y, idx: i})
	}
	for i := range g.enemies {
		e := g.enemies[i]
		items = append(items, ritem{depth: e.fx + e.fy + 0.2, kind: itEnemy, fx: e.fx, fy: e.fy, idx: i})
	}
	items = append(items, ritem{depth: g.pfx + g.pfy + 0.2, kind: itPlayer, fx: g.pfx, fy: g.pfy})

	sort.SliceStable(items, func(i, j int) bool { return items[i].depth < items[j].depth })
	for _, it := range items {
		switch it.kind {
		case itWall:
			px, py := proj(float64(it.x), float64(it.y), ox, oy)
			iso.DrawCube(m.scr, px, py, wallH, wallTop, wallL, wallR)
		case itCrate:
			px, py := proj(float64(it.x), float64(it.y), ox, oy)
			iso.DrawCube(m.scr, px, py, crateH, crateTop, crateL, crateR)
			// A lid marking so crates read as breakable, not as walls.
			m.scr.FillRect(px-4, py-crateH+iso.HH-2, 8, 4, crateLid)
		case itBomb:
			px, py := proj(float64(it.x), float64(it.y), ox, oy)
			m.drawBomb(px, py+iso.HH, g.bombs[it.idx], t)
		case itEnemy:
			px, py := proj(it.fx, it.fy, ox, oy)
			m.drawWisp(px, py+iso.HH, t, it.idx)
		case itPlayer:
			px, py := proj(it.fx, it.fy, ox, oy)
			m.drawPlayer(px, py+iso.HH, t)
		}
	}
}

func (m *model) drawPlayer(footX, footY int, t float64) {
	g := m.g
	// Blink while briefly invulnerable after a respawn.
	if g.invuln > 0 && int(t*12)%2 == 0 {
		return
	}
	moving := m.sinceMove() < idleAfter
	frame := 0
	if moving {
		frame = int(t * 8)
	}
	sprites.Draw(m.scr, footX, footY, sprites.Facing(g.face), frame, moving)
}

// drawBomb is a chunky dark sphere sitting on the tile: a ground shadow, a
// black outline for contrast against the floor, a sheen, and a fuse spark that
// quickens and brightens as the fuse runs down. It pulses so it's easy to spot.
func (m *model) drawBomb(footX, footY int, b bomb, t float64) {
	frac := float64(b.fuse) / fuseTicks
	pulse := 1 + 0.14*math.Sin(t*(7+(1-frac)*16))
	r := int(10 * pulse)
	cy := footY - r - 1

	m.scr.FillCircle(footX, footY-1, r-2, 0x070707)  // ground shadow
	m.scr.FillCircle(footX, cy, r+1, 0x000000)       // outline
	m.scr.FillCircle(footX, cy, r, 0x303030)         // body
	m.scr.FillCircle(footX, cy, r-3, 0x1E1E1E)       // shaded belly
	m.scr.FillCircle(footX-r/3, cy-r/3, 2, 0xB0B0B0) // sheen
	m.scr.FillRect(footX-1, cy-r-2, 3, 3, 0x6A6A6A)  // fuse nub

	// Spark: blinks faster as detonation nears, in the world's hue.
	speed := 8 + (1-frac)*24
	if math.Sin(t*speed) > -0.2 {
		sx := footX + int(2*math.Sin(t*13))
		m.scr.FillCircle(sx, cy-r-5, 2, m.accent(0.62))
		m.scr.Set(sx, cy-r-7, m.accent(0.5))
	}
}

// drawWisp is a floating dark orb with glowing eyes in the world's hue.
func (m *model) drawWisp(footX, footY int, t float64, idx int) {
	bob := int(2 * math.Sin(t*3+float64(idx)*1.7))
	cy := footY - 9 + bob
	// faint trailing motes
	for k := 1; k <= 2; k++ {
		m.scr.FillCircle(footX, cy+k*3, 3-k, 0x303030)
	}
	m.scr.FillCircle(footX, cy, 5, 0x252525)
	m.scr.FillCircle(footX, cy, 4, 0x555555)
	eye := m.accent(0.6)
	m.scr.FillCircle(footX-2, cy-1, 1, eye)
	m.scr.FillCircle(footX+2, cy-1, 1, eye)
}

// drawBlasts paints the explosion: a bright hued flash on each lethal cell plus
// a rising spark, echoing the portal/fountain spray so the world feels of a
// piece with its gateway.
func (m *model) drawBlasts(ox, oy, t float64) {
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			ttl := m.g.blast[y][x]
			if ttl <= 0 {
				continue
			}
			inten := float64(ttl) / blastTicks
			px, py := proj(float64(x), float64(y), ox, oy)
			flash := m.accent(0.30 + 0.33*inten)
			iso.DrawDiamond(m.scr, px, py, flash, flash)
			// Rising spark column at the center.
			cx, cyc := px, py+iso.HH
			h := int(18 * inten)
			for k := 0; k < h; k += 2 {
				l := 0.62 - 0.3*float64(k)/18
				m.scr.FillRect(cx-1, cyc-6-k, 2, 2, m.accent(l))
			}
		}
	}
}

func (m *model) applyLighting(ox, oy, t float64) {
	lights := make([]light.Light, 0, 8)
	// The player carries a warm light.
	pfx, pfy := proj(m.g.pfx, m.g.pfy, ox, oy)
	lights = append(lights, light.Light{X: pfx, Y: pfy + iso.HH - 16, Radius: 130, Power: 0.75})
	// Blasts light the room; bombs glimmer faintly.
	type glow struct {
		x, y int
		l    float64
	}
	var glows []glow
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			if ttl := m.g.blast[y][x]; ttl > 0 {
				inten := float64(ttl) / blastTicks
				px, py := proj(float64(x), float64(y), ox, oy)
				lights = append(lights, light.Light{X: px, Y: py + iso.HH, Radius: 80, Power: 0.6 * inten})
				glows = append(glows, glow{px, py + iso.HH, 0.45 + 0.3*inten})
			}
		}
	}
	m.lights.Apply(m.scr, lights, ambient)
	for _, gl := range glows {
		light.Glow(m.scr, gl.x, gl.y, 30, m.accent(gl.l))
	}
}

func (m *model) drawHUD(pw, ph int) {
	g := m.g
	m.scr.DrawTextShadow(6, 5, "THE VAULT", 0xF2F2F2, 0x000000)
	line := fmt.Sprintf("bombs %d/%d  reach %d  lives %d  wisps %d",
		g.activeBombs(), g.bombsMax, g.reach, g.lives, len(g.enemies))
	m.scr.DrawTextShadow(6, 5+canvas.LineH, line, 0xC8C8C8, 0x000000)

	hint := "WASD/arrows move · space bomb · Esc leave"
	m.scr.DrawText(pw-canvas.TextWidth(hint)-6, ph-canvas.LineH-4, hint, 0x6E6E6E)
}

func (m *model) drawBanner(pw, ph int) {
	var title, sub string
	switch m.g.state {
	case won:
		title, sub = "VAULT CLEARED", "✦ Spark Core obtained ✦   ·   Esc to return"
	case lost:
		title, sub = "OVERWHELMED", "R to try again   ·   Esc to return"
	default:
		return
	}
	bw := canvas.TextWidth(title)
	if w := canvas.TextWidth(sub); w > bw {
		bw = w
	}
	bw += 24
	bh := canvas.LineH*2 + 22
	x := pw/2 - bw/2
	y := ph/2 - bh/2
	// Dim the scene behind the card.
	px := m.scr.Pixels()
	for i := range px {
		px[i] = px[i].Scale(0.4)
	}
	m.scr.FillRect(x, y, bw, bh, 0x0A0A0A)
	m.scr.Rect(x, y, bw, bh, m.accent(0.6))
	m.scr.DrawTextShadow(pw/2-canvas.TextWidth(title)/2, y+8, title, 0xFFFFFF, 0x000000)
	m.scr.DrawText(pw/2-canvas.TextWidth(sub)/2, y+10+canvas.LineH, sub, 0xC8C8C8)
}
