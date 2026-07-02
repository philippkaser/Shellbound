package doom

import (
	"fmt"
	"math"
	"sort"

	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/screen"
)

// accent returns an on-palette color in the portal's hue at lightness l.
func (m *model) accent(l float64) canvas.Color { return plaza.AccentColor(m.key, l) }

// build composes one frame and returns the placement + Sixel bytes.
func (m *model) build(t float64) string {
	pw, ph, left, top := screen.Dims(m.termW, m.termH, m.cellW, m.cellH)
	m.scr.Resize(pw, ph)
	m.scr.Clear(canvas.Black)
	if m.termW < minTermW || m.termH < minTermH {
		msg := "please resize your terminal to at least 60x20"
		m.scr.DrawText(pw/2-canvas.TextWidth(msg)/2, ph/2, msg, 0xA1A1A1)
		return m.place(left, top)
	}
	m.drawWorld(pw, ph, t)
	m.drawSprites(pw, ph, t)
	m.drawWeapon(pw, ph, t)
	m.drawHUD(pw, ph, t)
	m.drawBanner(pw, ph)
	return m.place(left, top)
}

// hashf is a cheap deterministic 0..1 jitter from a few integers — used for
// per-brick tone variation so walls aren't perfectly flat.
func hashf(a, b, c int) float64 {
	h := uint32(a*73856093 ^ b*19349663 ^ c*83492791)
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	return float64(h&0xffff) / 65535.0
}

// drawWorld renders the ceiling/floor and raycasts the walls, filling the depth
// buffer used to occlude sprites.
func (m *model) drawWorld(pw, ph int, t float64) {
	g := m.g
	if cap(m.zbuf) < pw {
		m.zbuf = make([]float64, pw)
	}
	m.zbuf = m.zbuf[:pw]
	horizon := ph / 2

	// A muzzle flash briefly floods the hall with light.
	flash := 0.0
	if g.muzzle > 0 {
		flash = 0.4 * float64(g.muzzle) / muzzleTicks
	}
	// Torchlight gives the whole scene a gentle, restless flicker.
	flicker := 1 + 0.05*math.Sin(t*6.3) + 0.03*math.Sin(t*11.7)

	// Ceiling fades to black overhead; floor brightens toward the camera.
	// The muzzle flash pools near the camera (rows far from the horizon)
	// instead of flooding the whole hall uniformly.
	for y := 0; y < horizon; y++ {
		k := float64(horizon-y) / float64(horizon)
		v := uint8(clamp01((34*(1-k))/255*flicker+flash*k*k) * 255)
		m.scr.HLine(0, pw-1, y, canvas.RGB(v, v, v))
	}
	for y := horizon; y < ph; y++ {
		k := float64(y-horizon) / float64(ph-horizon)
		v := uint8(clamp01((18+46*k)/255*flicker+flash*0.8*k*k) * 255)
		m.scr.HLine(0, pw-1, y, canvas.RGB(v, v, v))
	}

	for x := 0; x < pw; x++ {
		cameraX := 2*float64(x)/float64(pw) - 1
		rdx := g.dirX + g.planeX*cameraX
		rdy := g.dirY + g.planeY*cameraX
		mapX, mapY := int(g.posX), int(g.posY)

		ddx, ddy := math.Inf(1), math.Inf(1)
		if rdx != 0 {
			ddx = math.Abs(1 / rdx)
		}
		if rdy != 0 {
			ddy = math.Abs(1 / rdy)
		}
		var stepX, stepY int
		var sideDistX, sideDistY float64
		if rdx < 0 {
			stepX = -1
			sideDistX = (g.posX - float64(mapX)) * ddx
		} else {
			stepX = 1
			sideDistX = (float64(mapX) + 1 - g.posX) * ddx
		}
		if rdy < 0 {
			stepY = -1
			sideDistY = (g.posY - float64(mapY)) * ddy
		} else {
			stepY = 1
			sideDistY = (float64(mapY) + 1 - g.posY) * ddy
		}

		side, hit := 0, false
		for i := 0; i < 80 && !hit; i++ {
			if sideDistX < sideDistY {
				sideDistX += ddx
				mapX += stepX
				side = 0
			} else {
				sideDistY += ddy
				mapY += stepY
				side = 1
			}
			if !inb(mapX, mapY) {
				break
			}
			if g.walls[mapY][mapX] {
				hit = true
			}
		}

		perp := sideDistX - ddx
		if side == 1 {
			perp = sideDistY - ddy
		}
		if perp < 1e-4 {
			perp = 1e-4
		}
		m.zbuf[x] = perp

		h := int(float64(ph) / perp)
		top := horizon - h/2
		start, end := top, top+h
		if start < 0 {
			start = 0
		}
		if end > ph {
			end = ph
		}

		fog := 1.0 / (1.0 + perp*perp*0.045)
		base := 0.82 * fog * flicker
		if side == 1 {
			base *= 0.6 // shade one axis of faces for depth
		}
		var wallX float64
		if side == 0 {
			wallX = g.posY + perp*rdy
		} else {
			wallX = g.posX + perp*rdx
		}
		wallX -= math.Floor(wallX)
		seam := wallX < 0.045 || wallX > 0.955

		torch := inb(mapX, mapY) && g.torches[mapY][mapX]
		tflick := 0.75 + 0.25*math.Sin(t*9+float64(mapX*7+mapY*13))
		accent := m.accent(0.6)

		for y := start; y < end; y++ {
			l := base
			ty := float64(y-top) / float64(h) // 0..1 down the wall
			// Running-bond brickwork: horizontal courses plus vertical joints
			// offset every other row.
			course := ty * 6
			off := 0.0
			if int(course)%2 == 1 {
				off = 0.5
			}
			brickU := math.Mod(wallX*3+off, 1.0)
			if math.Mod(course, 1.0) < 0.12 || brickU < 0.06 {
				l *= 0.66 // mortar
			} else {
				l *= 0.90 + 0.14*hashf(mapX, mapY, int(course)) // per-brick tone
			}
			if seam {
				l *= 0.7
			}
			l += flash * fog // the flash fades with wall distance
			v := uint8(clamp01(l) * 255)
			col := canvas.RGB(v, v, v)
			if torch {
				// Warm the whole face and blaze a bracketed flame mid-height.
				glow := (1 - math.Abs(ty-0.44)*1.6) * tflick
				if glow < 0 {
					glow = 0
				}
				col = col.Lerp(accent, 0.22+0.4*glow)
				if wallX > 0.40 && wallX < 0.60 && ty > 0.30 && ty < 0.50 {
					flame := accent
					if ty < 0.36 {
						flame = canvas.RGB(250, 250, 245) // hot core near the top
					}
					col = flame.Scale(tflick)
				}
			}
			m.scr.Set(x, y, col)
		}
	}
}

// drawSprites billboards the imps — live and collapsing — occluded by the wall
// depth buffer. Each imp bobs while idle, rears up when it closes to strike, and
// dissolves upward into embers when killed.
func (m *model) drawSprites(pw, ph int, t float64) {
	g := m.g
	type sp struct {
		i    int
		dist float64
	}
	var order []sp
	for i := range g.enemies {
		if !g.enemies[i].alive && g.enemies[i].dying == 0 {
			continue
		}
		dx, dy := g.enemies[i].x-g.posX, g.enemies[i].y-g.posY
		order = append(order, sp{i, dx*dx + dy*dy})
	}
	sort.Slice(order, func(a, b int) bool { return order[a].dist > order[b].dist })

	invDet := 1.0 / (g.planeX*g.dirY - g.dirX*g.planeY)
	horizon := ph / 2
	for _, o := range order {
		e := g.enemies[o.i]
		dx, dy := e.x-g.posX, e.y-g.posY
		tx := invDet * (g.dirY*dx - g.dirX*dy)
		depth := invDet * (-g.planeY*dx + g.planeX*dy)
		if depth <= 0.2 {
			continue
		}
		fog := 1.0 / (1.0 + depth*depth*0.045)
		base := int(float64(ph) / depth)
		if base < 3 {
			continue
		}

		// Idle bob, a rear-up lunge when near, and the death dissolve.
		dist := math.Hypot(dx, dy)
		bob, lunge, dissolve := 0.0, 0.0, 0.0
		if e.alive {
			bob = math.Sin(t*3+e.phase) * 0.03
			if dist < lungeRange {
				lunge = (lungeRange - dist) / lungeRange
			}
		} else {
			dissolve = 1 - float64(e.dying)/float64(deathTicks)
		}

		sizeX := base
		sizeY := int(float64(base) * (1 + 0.28*lunge))
		screenX := int(float64(pw) / 2 * (1 + tx/depth))
		sx0 := screenX - sizeX/2
		// Feet stay planted; bob/lunge grow upward, and a dying imp sinks.
		footY := horizon + base/2
		sy0 := footY - sizeY + int(bob*float64(base)) + int(dissolve*float64(base)*0.35)
		for x := sx0; x < sx0+sizeX; x++ {
			if x < 0 || x >= pw || depth >= m.zbuf[x] {
				continue
			}
			u := float64(x-sx0) / float64(sizeX)
			for y := sy0; y < sy0+sizeY; y++ {
				if y < 0 || y >= ph {
					continue
				}
				v := float64(y-sy0) / float64(sizeY)
				if col, ok := m.impPixel(u, v, fog, e.hurt > 0, dissolve, x, y); ok {
					m.scr.Set(x, y, col)
				}
			}
		}
	}
}

// impPixel samples the imp silhouette at (u, v) in its sprite box: a hunched
// body, a horned head with eyes glowing in the portal's hue, arms and legs. A
// hit flashes it white; while dying it burns to accent embers and dithers away
// from the top down.
func (m *model) impPixel(u, v, fog float64, hurt bool, dissolve float64, px, py int) (canvas.Color, bool) {
	part := impShape(u, v)
	if part == partNone {
		return 0, false
	}
	// Death: dither out (top first) and glow as embers.
	if dissolve > 0 {
		threshold := dissolve*1.3 - (v * 0.5) // upper pixels vanish sooner
		if hashf(px, py, 1) < threshold {
			return 0, false
		}
	}
	if hurt {
		return canvas.RGB(235, 235, 235), true
	}
	switch part {
	case partEye:
		return m.accent(0.66), true // glowing eyes stay lit
	case partHorn:
		g := uint8(clamp01(0.22+0.10*fog) * 255)
		c := canvas.RGB(g, g, g)
		if dissolve > 0 {
			c = c.Lerp(m.accent(0.55), dissolve)
		}
		return c, true
	default: // body / head / limbs
		shade := 0.16 + 0.14*fog
		if part == partRim {
			shade += 0.14 // top-left rim light
		}
		g := uint8(clamp01(shade) * 255)
		c := canvas.RGB(g, g, g)
		if dissolve > 0 {
			c = c.Lerp(m.accent(0.5), 0.4+0.5*dissolve) // burning embers
		}
		return c, true
	}
}

// imp silhouette parts.
const (
	partNone = iota
	partBody
	partHorn
	partEye
	partRim
)

// impShape returns which part of the hunched imp covers normalized (u, v).
func impShape(u, v float64) int {
	// Horns rising from the head.
	if v < 0.20 && (math.Abs(u-0.34) < 0.055 || math.Abs(u-0.66) < 0.055) {
		return partHorn
	}
	// Head.
	hx, hy := (u-0.5)/0.20, (v-0.28)/0.17
	if hx*hx+hy*hy <= 1 {
		if v > 0.24 && v < 0.32 && (math.Abs(u-0.42) < 0.05 || math.Abs(u-0.58) < 0.05) {
			return partEye
		}
		if u < 0.5 && v < 0.28 {
			return partRim
		}
		return partBody
	}
	// Arms flanking the torso.
	for _, ax := range []float64{0.22, 0.78} {
		ex, ey := (u-ax)/0.10, (v-0.58)/0.20
		if ex*ex+ey*ey <= 1 {
			return partBody
		}
	}
	// Legs.
	if v > 0.78 && v < 1.0 && (math.Abs(u-0.40) < 0.09 || math.Abs(u-0.60) < 0.09) {
		return partBody
	}
	// Hunched torso.
	tx, ty := (u-0.5)/0.27, (v-0.60)/0.24
	if tx*tx+ty*ty <= 1 {
		if u < 0.5 && v < 0.55 {
			return partRim
		}
		return partBody
	}
	return partNone
}

// drawWeapon paints the gun at the bottom center: it sways with a breathing
// idle, bobs as you walk, and recoils with a muzzle flash when it fires.
func (m *model) drawWeapon(pw, ph int, t float64) {
	g := m.g
	// Idle sway + walk bob (driven by the step counter).
	swayX := int(4 * math.Sin(t*1.6))
	bobY := int(3 * math.Abs(math.Sin(float64(g.steps)*0.6)))
	kick := 0
	if g.muzzle > 0 {
		kick = 9 * g.muzzle / muzzleTicks
	}
	cx := pw/2 + swayX
	by := ph + bobY - kick

	shadow := canvas.Color(0x0E0E0E)
	// Grip and body.
	m.scr.FillRect(cx-13, by-46, 26, 46, 0x2A2A2A)
	m.scr.FillRect(cx-13, by-46, 4, 46, 0x1C1C1C) // shaded left
	m.scr.HLine(cx-13, cx+12, by-46, 0x3A3A3A)    // top catch-light
	m.scr.Rect(cx-13, by-46, 26, 46, shadow)
	// Barrel and sight.
	m.scr.FillRect(cx-4, by-64, 8, 20, 0x232323)
	m.scr.FillRect(cx-4, by-64, 2, 20, 0x121212)
	m.scr.Rect(cx-4, by-64, 8, 20, shadow)
	m.scr.FillRect(cx-1, by-67, 2, 4, 0x343434) // front sight
	// Muzzle flash lights the barrel in the portal hue.
	if g.muzzle > 0 {
		f := float64(g.muzzle) / muzzleTicks
		m.scr.FillCircle(cx, by-66, int(4+7*f), m.accent(0.6))
		m.scr.FillCircle(cx, by-66, int(2+3*f), canvas.RGB(248, 248, 244))
	}
}

func (m *model) drawHUD(pw, ph int, t float64) {
	g := m.g
	cx, cy := pw/2, ph/2
	col := m.accent(0.6)
	m.scr.HLine(cx-7, cx-3, cy, col)
	m.scr.HLine(cx+3, cx+7, cy, col)
	m.scr.VLine(cx, cy-7, cy-3, col)
	m.scr.VLine(cx, cy+3, cy+7, col)

	// A connecting shot flashes an X of white ticks around the crosshair.
	if g.hitMark > 0 {
		mk := canvas.RGB(240, 240, 240)
		for _, d := range [4][2]int{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
			m.scr.Line(cx+d[0]*4, cy+d[1]*4, cx+d[0]*7, cy+d[1]*7, mk)
		}
	}

	m.scr.DrawTextShadow(6, 5, "DOOM", 0xF2F2F2, 0x000000)
	stat := fmt.Sprintf("health %d   imps %d/%d", g.health, g.kills, g.total)
	m.scr.DrawTextShadow(6, 5+canvas.LineH, stat, 0xC8C8C8, 0x000000)
	const bw = 130
	by := 5 + 2*canvas.LineH
	m.scr.FillRect(6, by, bw, 6, 0x202020)
	m.scr.FillRect(6, by, bw*g.health/startHealth, 6, m.accent(0.55))

	hint := "WASD move · ←/→ turn · space fire · Esc leave"
	m.scr.DrawText(pw-canvas.TextWidth(hint)-6, ph-canvas.LineH-4, hint, 0x6E6E6E)

	// A hit pulses an accent vignette around the edges, and critical health
	// keeps a slow warning pulse breathing there.
	if g.hurtCool > hurtCoolTicks-3 && g.state == playing {
		m.vignette(pw, ph, m.accent(0.5))
	} else if g.health <= 30 && g.state == playing {
		pulse := 0.25 + 0.2*math.Sin(t*3.2)
		m.vignette(pw, ph, m.accent(0.5).Scale(pulse))
	}
}

func (m *model) vignette(pw, ph int, col canvas.Color) {
	for i := 0; i < 4; i++ {
		c := col.Scale(1 - float64(i)*0.22)
		m.scr.HLine(0, pw-1, i, c)
		m.scr.HLine(0, pw-1, ph-1-i, c)
		m.scr.VLine(i, 0, ph-1, c)
		m.scr.VLine(pw-1-i, 0, ph-1, c)
	}
}

func (m *model) drawBanner(pw, ph int) {
	var title, sub string
	switch m.g.state {
	case won:
		title, sub = "HALL CLEARED", "✦ Hellbreaker's Mark obtained ✦   ·   Esc to return"
	case lost:
		title, sub = "YOU DIED", "R to try again   ·   Esc to return"
	default:
		return
	}
	bw := canvas.TextWidth(title)
	if w := canvas.TextWidth(sub); w > bw {
		bw = w
	}
	bw += 24
	bh := canvas.LineH*2 + 22
	x, y := pw/2-bw/2, ph/2-bh/2
	px := m.scr.Pixels()
	for i := range px {
		px[i] = px[i].Scale(0.4)
	}
	m.scr.FillRect(x, y, bw, bh, 0x0A0A0A)
	m.scr.Rect(x, y, bw, bh, m.accent(0.6))
	m.scr.DrawTextShadow(pw/2-canvas.TextWidth(title)/2, y+8, title, 0xFFFFFF, 0x000000)
	m.scr.DrawText(pw/2-canvas.TextWidth(sub)/2, y+10+canvas.LineH, sub, 0xC8C8C8)
}
