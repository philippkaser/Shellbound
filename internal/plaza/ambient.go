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

// renderFireflies scatters slow-wandering, twinkling motes across the plaza,
// hovering above the ground plane. Each is a soft additive glow so it reads
// clearly even over the dim floor.
func (m *Map) renderFireflies(c *canvas.Canvas, originSx, originSy, t float64) {
	const n = 48
	for i := 0; i < n; i++ {
		fi := float64(i)
		gx := 6 + math.Mod(fi*11.3, float64(m.W-12))
		gy := 5 + math.Mod(fi*6.7, float64(m.H-10))
		sx, sy := iso.Project(gx, gy)
		px := int(sx - originSx + 26*math.Sin(t*0.5+fi*1.3))
		py := int(sy-originSy+16*math.Cos(t*0.4+fi*2.1)) - 34
		if px < -8 || px > c.W+8 || py < -8 || py > c.H+8 {
			continue
		}
		// Twinkle: blink fully off for part of the cycle, then glow bright.
		tw := 0.5 + 0.5*math.Sin(t*2.5+fi*2.0)
		if tw < 0.3 {
			continue
		}
		mote(c, px, py, 4, uint8(150+tw*105))
	}
}

// mote additively brightens a small soft disc of radius r at (x, y), so it
// glows over whatever is behind it rather than punching a hole.
func mote(c *canvas.Canvas, x, y, r int, v uint8) {
	r2 := float64(r*r + 1)
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			d2 := dx*dx + dy*dy
			if d2 > r*r {
				continue
			}
			f := 1 - float64(d2)/r2
			s := uint8(float64(v) * f)
			lit := canvas.RGB(s, s, s)
			c.Set(x+dx, y+dy, c.At(x+dx, y+dy).Lighten(lit))
		}
	}
}

// renderBirds drifts a few chevron silhouettes above the plaza, wings
// flapping, wrapping around as they cross.
func (m *Map) renderBirds(c *canvas.Canvas, originSx, originSy, t float64) {
	const n = 4
	span := float64(m.W) + 24
	for i := 0; i < n; i++ {
		fi := float64(i)
		gx := math.Mod(t*(2.4+fi*0.7)+fi*31, span) - 12
		gy := 8 + fi*7
		sx, sy := iso.Project(gx, gy)
		px := int(sx - originSx)
		py := int(sy-originSy) - 74 - int(10*math.Sin(t*0.7+fi))
		if px < -12 || px > c.W+12 || py < -12 || py > c.H+12 {
			continue
		}
		drawBird(c, px, py, math.Sin(t*7+fi*2) > 0)
	}
}

// drawBird draws a ~9px chevron; up raises the peak for a wing-flap frame.
func drawBird(c *canvas.Canvas, x, y int, up bool) {
	const col = canvas.Color(0x9A9A9A)
	dip := 3
	if up {
		dip = 5
	}
	// Two wings sloping down from a central peak; 2px thick for visibility.
	for i := 0; i <= 4; i++ {
		yy := y - dip + i
		c.Set(x-i, yy, col)
		c.Set(x-i, yy+1, col)
		c.Set(x+i, yy, col)
		c.Set(x+i, yy+1, col)
	}
}
