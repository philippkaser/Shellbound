// Package light adds Shellbound's interactive lighting: the plaza is rendered
// dim, and light sources (the player and every lamp) reveal the world's full
// brightness around them, falling off with distance. A separate glow pass
// paints a blocky bloom halo at each source for an ASCII-block-style flare.
//
// Lighting only modulates grey pixels; the saturated color pops — portal
// shimmer and player name tags — are left vivid by design.
package light

import "github.com/shellbound/shellbound/internal/render/canvas"

// Light is a point source in canvas pixel space.
type Light struct {
	X, Y   int
	Radius float64
	Power  float64 // peak brightness added to ambient at the center
}

// Field applies lighting using a reusable per-pixel multiplier buffer so no
// allocation happens per frame.
type Field struct {
	mult []float64
}

// NewField creates an empty lighting field.
func NewField() *Field { return &Field{} }

// Apply dims every grey pixel toward `ambient` (0..1), then reveals it toward
// its full tone near each light. Pixels brighter than 1× are clamped, so
// lights restore the art's original tones rather than overexposing it.
func (f *Field) Apply(c *canvas.Canvas, lights []Light, ambient float64) {
	w, h := c.W, c.H
	n := w * h
	if cap(f.mult) < n {
		f.mult = make([]float64, n)
	}
	f.mult = f.mult[:n]
	for i := range f.mult {
		f.mult[i] = ambient
	}

	for _, l := range lights {
		if l.Radius <= 0 || l.Power <= 0 {
			continue
		}
		r := int(l.Radius)
		r2 := l.Radius * l.Radius
		x0, x1 := clamp(l.X-r, 0, w-1), clamp(l.X+r, 0, w-1)
		y0, y1 := clamp(l.Y-r, 0, h-1), clamp(l.Y+r, 0, h-1)
		for y := y0; y <= y1; y++ {
			dy := float64(y - l.Y)
			row := y * w
			for x := x0; x <= x1; x++ {
				dx := float64(x - l.X)
				d2 := dx*dx + dy*dy
				if d2 >= r2 {
					continue
				}
				f.mult[row+x] += l.Power * (1 - d2/r2)
			}
		}
	}

	px := c.Pixels()
	for i, col := range px {
		if !col.IsGray() {
			continue // leave color pops vivid
		}
		m := f.mult[i]
		if m >= 1 {
			continue
		}
		px[i] = col.Scale(m)
	}
}

// Glow paints an additive bloom of radius r around (cx, cy): a bright core
// fading out, with the outer band rendered as coarse 2×2 blocks so it reads
// like a chunky ASCII-block flare rather than a smooth gradient.
func Glow(c *canvas.Canvas, cx, cy int, r float64, core canvas.Color) {
	if r <= 0 {
		return
	}
	ri := int(r)
	r2 := r * r
	for dy := -ri; dy <= ri; dy++ {
		for dx := -ri; dx <= ri; dx++ {
			d2 := float64(dx*dx + dy*dy)
			if d2 >= r2 {
				continue
			}
			t := d2 / r2
			// Blocky dither in the outer half: drop alternate 2×2 blocks.
			if t > 0.4 {
				bx, by := (cx+dx)/2, (cy+dy)/2
				if (bx+by)%2 != 0 {
					continue
				}
			}
			gx, gy := cx+dx, cy+dy
			lit := core.Scale(1 - t)
			c.Set(gx, gy, c.At(gx, gy).Lighten(lit))
		}
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
