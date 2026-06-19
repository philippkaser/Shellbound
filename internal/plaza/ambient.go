package plaza

import (
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
)

// RenderAmbient draws the plaza's living detail — drifting fireflies and the
// occasional bird overhead. It is a pure function of time, drawn after the
// lighting pass so the motes glow regardless of how dim their surroundings
// are. Everything stays monochrome to preserve the color discipline.
func (m *Map) RenderAmbient(c *canvas.Canvas, originSx, originSy, t float64) {
	m.renderFireflies(c, originSx, originSy, t)
	m.renderBirds(c, originSx, originSy, t)
}

// renderFireflies scatters slow-wandering, twinkling motes across the plaza
// interior, hovering a little above the ground plane.
func (m *Map) renderFireflies(c *canvas.Canvas, originSx, originSy, t float64) {
	const n = 18
	for i := 0; i < n; i++ {
		fi := float64(i)
		gx := 6 + math.Mod(fi*13.7, float64(m.W-12))
		gy := 5 + math.Mod(fi*7.3, float64(m.H-10))
		sx, sy := iso.Project(gx, gy)
		px := int(sx - originSx + 22*math.Sin(t*0.5+fi*1.3))
		py := int(sy-originSy+12*math.Cos(t*0.4+fi*2.1)) - 28
		if px < -2 || px > c.W+2 || py < -2 || py > c.H+2 {
			continue
		}
		// Twinkle: blink fully off for part of the cycle, then glow.
		tw := 0.5 + 0.5*math.Sin(t*3+fi*2.0)
		if tw < 0.35 {
			continue
		}
		v := uint8(120 + tw*135)
		col := canvas.RGB(v, v, v)
		c.Set(px, py, col)
		if tw > 0.72 { // bright: a tiny plus glints
			dim := canvas.RGB(v/2, v/2, v/2)
			c.Set(px-1, py, dim)
			c.Set(px+1, py, dim)
			c.Set(px, py-1, dim)
			c.Set(px, py+1, dim)
		}
	}
}

// renderBirds drifts a few chevron silhouettes high above the plaza, wings
// flapping, wrapping around as they cross.
func (m *Map) renderBirds(c *canvas.Canvas, originSx, originSy, t float64) {
	const n = 3
	span := float64(m.W) + 24
	for i := 0; i < n; i++ {
		fi := float64(i)
		gx := math.Mod(t*(2.4+fi*0.7)+fi*43, span) - 12
		gy := 8 + fi*9
		sx, sy := iso.Project(gx, gy)
		px := int(sx - originSx)
		py := int(sy-originSy) - 92 - int(8*math.Sin(t*0.7+fi))
		if px < -8 || px > c.W+8 || py < -8 || py > c.H+8 {
			continue
		}
		up := math.Sin(t*7+fi*2) > 0
		drawBird(c, px, py, up)
	}
}

// drawBird draws a 5px chevron; up raises the peak for a wing-flap frame.
func drawBird(c *canvas.Canvas, x, y int, up bool) {
	const col = canvas.Color(0x8A8A8A)
	peak := y - 1
	if up {
		peak = y - 2
	}
	c.Set(x-2, y, col)
	c.Set(x-1, y-1, col)
	c.Set(x, peak, col)
	c.Set(x+1, y-1, col)
	c.Set(x+2, y, col)
}
