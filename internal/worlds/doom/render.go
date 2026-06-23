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
	m.drawWorld(pw, ph)
	m.drawSprites(pw, ph)
	m.drawWeapon(pw, ph)
	m.drawHUD(pw, ph)
	m.drawBanner(pw, ph)
	return m.place(left, top)
}

// drawWorld renders the ceiling/floor and raycasts the walls, filling the depth
// buffer used to occlude sprites.
func (m *model) drawWorld(pw, ph int) {
	g := m.g
	if cap(m.zbuf) < pw {
		m.zbuf = make([]float64, pw)
	}
	m.zbuf = m.zbuf[:pw]
	horizon := ph / 2

	// Ceiling fades to black overhead; floor brightens toward the camera.
	for y := 0; y < horizon; y++ {
		k := float64(horizon-y) / float64(horizon)
		v := uint8(34 * (1 - k))
		m.scr.HLine(0, pw-1, y, canvas.RGB(v, v, v))
	}
	for y := horizon; y < ph; y++ {
		k := float64(y-horizon) / float64(ph-horizon)
		v := uint8(18 + 46*k)
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
		base := 0.82 * fog
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

		for y := start; y < end; y++ {
			l := base
			ty := float64(y-top) / float64(h) // 0..1 down the wall
			if math.Mod(ty*6, 1.0) < 0.10 {   // mortar courses
				l *= 0.72
			}
			if seam {
				l *= 0.72
			}
			v := uint8(clamp01(l) * 255)
			m.scr.Set(x, y, canvas.RGB(v, v, v))
		}
	}
}

// drawSprites billboards the imps, occluded by the wall depth buffer.
func (m *model) drawSprites(pw, ph int) {
	g := m.g
	type sp struct {
		i    int
		dist float64
	}
	var order []sp
	for i := range g.enemies {
		if !g.enemies[i].alive {
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
		screenX := int(float64(pw) / 2 * (1 + tx/depth))
		size := int(float64(ph) / depth)
		if size < 3 {
			continue
		}
		sx0, sy0 := screenX-size/2, horizon-size/2
		fog := 1.0 / (1.0 + depth*depth*0.045)
		for x := sx0; x < sx0+size; x++ {
			if x < 0 || x >= pw || depth >= m.zbuf[x] {
				continue
			}
			u := float64(x-sx0) / float64(size)
			for y := sy0; y < sy0+size; y++ {
				if y < 0 || y >= ph {
					continue
				}
				v := float64(y-sy0) / float64(size)
				if col, ok := m.impPixel(u, v, fog, e.hurt > 0); ok {
					m.scr.Set(x, y, col)
				}
			}
		}
	}
}

// impPixel is the imp silhouette: a dark hunched body with horns and eyes that
// glow in the portal's hue; a hit flashes it white.
func (m *model) impPixel(u, v, fog float64, hurt bool) (canvas.Color, bool) {
	// Horns near the top.
	if v < 0.22 && (math.Abs(u-0.32) < 0.05 || math.Abs(u-0.68) < 0.05) {
		if hurt {
			return canvas.RGB(230, 230, 230), true
		}
		return canvas.RGB(40, 40, 40), true
	}
	ex, ey := (u-0.5)/0.30, (v-0.55)/0.42
	if ex*ex+ey*ey > 1 {
		return 0, false
	}
	if hurt {
		return canvas.RGB(235, 235, 235), true
	}
	if v > 0.40 && v < 0.50 && (math.Abs(u-0.40) < 0.045 || math.Abs(u-0.60) < 0.045) {
		return m.accent(0.62), true // glowing eyes
	}
	g := uint8(clamp01(0.16+0.12*fog) * 255)
	return canvas.RGB(g, g, g), true
}

// drawWeapon paints the gun at the bottom center, kicking up when it fires.
func (m *model) drawWeapon(pw, ph int) {
	kick := 0
	if m.g.muzzle > 0 {
		kick = 7
	}
	cx, by := pw/2, ph-kick
	m.scr.FillRect(cx-11, by-44, 22, 44, 0x2C2C2C)
	m.scr.FillRect(cx-7, by-58, 14, 16, 0x3C3C3C)
	m.scr.Rect(cx-11, by-44, 22, 44, 0x0E0E0E)
	m.scr.FillRect(cx-3, by-44, 6, 44, 0x202020)
	if m.g.muzzle > 0 {
		m.scr.FillCircle(cx, by-60, 8, m.accent(0.62))
		m.scr.FillCircle(cx, by-60, 4, canvas.RGB(245, 245, 245))
	}
}

func (m *model) drawHUD(pw, ph int) {
	g := m.g
	cx, cy := pw/2, ph/2
	col := m.accent(0.6)
	m.scr.HLine(cx-7, cx-3, cy, col)
	m.scr.HLine(cx+3, cx+7, cy, col)
	m.scr.VLine(cx, cy-7, cy-3, col)
	m.scr.VLine(cx, cy+3, cy+7, col)

	m.scr.DrawTextShadow(6, 5, "DOOM", 0xF2F2F2, 0x000000)
	stat := fmt.Sprintf("health %d   imps %d/%d", g.health, g.kills, g.total)
	m.scr.DrawTextShadow(6, 5+canvas.LineH, stat, 0xC8C8C8, 0x000000)
	const bw = 130
	by := 5 + 2*canvas.LineH
	m.scr.FillRect(6, by, bw, 6, 0x202020)
	m.scr.FillRect(6, by, bw*g.health/startHealth, 6, m.accent(0.55))

	hint := "WASD move · ←/→ turn · space fire · Esc leave"
	m.scr.DrawText(pw-canvas.TextWidth(hint)-6, ph-canvas.LineH-4, hint, 0x6E6E6E)

	// A hit pulses an accent vignette around the edges.
	if g.hurtCool > hurtCoolTicks-3 && g.state == playing {
		m.vignette(pw, ph, m.accent(0.5))
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
