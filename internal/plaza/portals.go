package plaza

import (
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
	"github.com/shellbound/shellbound/internal/render/shimmer"
)

// Portal arch dimensions: footprint in cells, billboard in pixels.
const (
	PortalW = 10
	PortalH = 6

	archW = 64 // billboard width in pixels
	archH = 92 // billboard height in pixels
)

// Portal is one gateway in the plaza referencing a world by key. X, Y is the
// top-left cell of its footprint. The shimmer interior is the only saturated
// color the plaza emits.
type Portal struct {
	Key  string // world registry key
	Name string // pedestal label
	X, Y int
}

// Portals is the 1.0 set, standing against the north wall. Starfall is the
// first real world; the other two still resolve to the "coming soon"
// placeholder.
var Portals = []Portal{
	{Key: "starfall", Name: "Starfall", X: 12, Y: 3},
	{Key: "chess", Name: "Chess", X: 45, Y: 3},
	{Key: "doom", Name: "Doom", X: 78, Y: 3},
}

// TriggerContains reports whether feet at cell (cx, cy) are inside the portal
// mouth (the central region of the footprint).
func (p Portal) TriggerContains(cx, cy int) bool {
	return cx >= p.X+2 && cx <= p.X+PortalW-3 &&
		cy >= p.Y+2 && cy <= p.Y+PortalH-1
}

// Center returns the portal's footprint center cell (fractional).
func (p Portal) Center() (float64, float64) {
	return float64(p.X) + float64(PortalW)/2, float64(p.Y) + float64(PortalH)/2
}

// RenderIso draws the portal as an upright arched gateway: a grey frame around
// the animated rainbow interior, a pedestal lip, and the name baked beneath.
// (originSx, originSy) is the screen-space point at the canvas's top-left.
func (p Portal) RenderIso(c *canvas.Canvas, f *shimmer.Field, t, originSx, originSy float64) {
	cgx, cgy := p.Center()
	sx, sy := iso.Project(cgx, cgy)
	ax := int(sx - originSx)
	groundY := int(sy-originSy) + iso.HH
	topY := groundY - archH
	half := archW / 2

	for dy := 0; dy < archH; dy++ {
		y := topY + dy
		// Interior half-width: a semicircular arch over straight jambs.
		hw := half
		if dy < half {
			d := half - dy
			hw = int(math.Sqrt(float64(half*half - d*d)))
		}
		for dx := -hw; dx <= hw; dx++ {
			c.Set(ax+dx, y, f.At(dx+half, dy, t))
		}
		// Grey frame hugging the interior, brighter where the arch catches light.
		frame := canvas.Color(0x737373)
		if dy < archH/3 {
			frame = 0xA1A1A1
		}
		for w := 1; w <= 2; w++ {
			c.Set(ax-hw-w, y, frame)
			c.Set(ax+hw+w, y, frame)
		}
	}

	// Pedestal lip under the arch.
	c.FillRect(ax-half-2, groundY, archW+4, 2, 0x404040)

	// Colored glow pool spilling onto the floor — a reflection of the portal's
	// light, and a sanctioned splash of color.
	pool := f.At(half, archH/2, t)
	glowRX, glowRY := half+10, 12
	for dy := -glowRY; dy <= glowRY; dy++ {
		for dx := -glowRX; dx <= glowRX; dx++ {
			fx, fy := float64(dx)/float64(glowRX), float64(dy)/float64(glowRY)
			d := fx*fx + fy*fy
			if d >= 1 {
				continue
			}
			gx, gy := ax+dx, groundY+5+dy
			c.Set(gx, gy, c.At(gx, gy).Lighten(pool.Scale((1-d)*0.5)))
		}
	}

	// Sparks rising through the arch, looping — bright, shimmer-tinted motes.
	for i := 0; i < 6; i++ {
		rise := math.Mod(t*0.35+float64(i)*0.17, 1.0) // 0 (ground) .. 1 (top)
		sy := groundY - int(rise*float64(archH))
		drift := int(float64(half-8) * math.Sin(t*1.4+float64(i)*2.1))
		spark := f.At(drift+half, sy-topY, t).Lerp(0xFFFFFF, 0.5)
		c.Set(ax+drift, sy, spark)
		c.Set(ax+drift, sy-1, spark.Scale(0.6))
	}

	// Name label, centered beneath, white with a black shadow for legibility.
	lw := canvas.TextWidth(p.Name)
	c.DrawTextShadow(ax-lw/2, groundY+18, p.Name, 0xFFFFFF, 0x000000)
}

// PortalAt returns the portal whose trigger zone contains feet cell (cx, cy).
func PortalAt(cx, cy int) (Portal, bool) {
	for _, p := range Portals {
		if p.TriggerContains(cx, cy) {
			return p, true
		}
	}
	return Portal{}, false
}
