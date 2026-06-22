package plaza

import (
	"math"

	"github.com/shellbound/shellbound/internal/render/halfblock"
	"github.com/shellbound/shellbound/internal/render/shimmer"
)

// Portal arch dimensions in cells.
const (
	PortalW = 10
	PortalH = 6
)

// Oval thresholds, expressed as normalized squared distance from the portal
// center (0 at the core, 1 at the rim of the stone frame). All of render,
// collision and the entry trigger derive from these same numbers so the
// shimmering mouth, the visible frame and the world hitbox stay aligned.
const (
	mouthFill = 0.60 // interior shimmer fills out to here
	frameRim  = 1.00 // stone frame runs from mouthFill to here
	glowRim   = 1.75 // outer floor glow fades from frameRim out to here
	mouthCell = 0.50 // cell-space trigger: feet inside this enter the world
)

// dist returns the normalized squared distance of a point (in oval-local
// units, where x is columns and y is half-block pixel rows) from the
// portal center. Working in pixel rows keeps the oval visually round on a
// terminal grid, where a cell is twice as tall as it is wide.
func ovalDist(px, py float64) float64 {
	cx, cy := (PortalW-1)/2.0, (PortalH*2-1)/2.0
	dx := (px - cx) / (PortalW / 2.0)
	dy := (py - cy) / (PortalH * 2 / 2.0)
	return dx*dx + dy*dy
}

// cellDist is the same ellipse measured in whole cells, used by the entry
// trigger so the hitbox tracks the round mouth rather than a loose box.
func (p Portal) cellDist(cx, cy int) float64 {
	x := (float64(cx-p.X) - (PortalW-1)/2.0) / (PortalW / 2.0)
	y := (float64(cy-p.Y) - (PortalH-1)/2.0) / (PortalH / 2.0)
	return x*x + y*y
}

// Portal is one pedestal in the plaza referencing a world by key. X, Y is
// the top-left cell of the arch. The shimmer interior is the only
// saturated color on the map.
type Portal struct {
	Key  string // world registry key
	Name string // pedestal label
	X, Y int
}

// Portals is the 1.0 set, standing against the north wall. All three keys
// resolve to the "coming soon" placeholder world for now.
var Portals = []Portal{
	{Key: "bomberman", Name: "Bomberman", X: 12, Y: 3},
	{Key: "chess", Name: "Chess", X: 45, Y: 3},
	{Key: "doom", Name: "Doom", X: 78, Y: 3},
}

// TriggerContains reports whether feet at cell (cx, cy) are inside the
// portal mouth — the round core of the oval, matching what the player sees.
func (p Portal) TriggerContains(cx, cy int) bool {
	return p.cellDist(cx, cy) <= mouthCell
}

// Render draws the portal into a screen canvas: a soft outer glow on the
// floor, a grey oval frame, the animated rainbow interior with a smooth
// radial gradient, a pedestal lip and the name label beneath. (ox, oy) is
// the world cell at the canvas's top-left; t is seconds since server start.
func (p Portal) Render(c *halfblock.Canvas, f *shimmer.Field, t float64, ox, oy int) {
	// Pixel-space oval, padded so the outer glow has room to bloom.
	const pad = 4
	for py := -pad; py < PortalH*2+pad; py++ {
		for px := -pad; px < PortalW+pad; px++ {
			e := ovalDist(float64(px), float64(py))
			sx, sy := p.X+px-ox, p.Y*2+py-oy*2
			switch {
			case e <= mouthFill:
				// Interior: rainbow hue sweep with a bright core fading to a
				// deeper rim — a smooth gradient from color to color.
				r := math.Sqrt(e / mouthFill)
				c.SetPx(sx, sy, f.Radial(p.X+px, p.Y*2+py, t, r))
			case e <= frameRim:
				// Stone frame, brighter along the top arc where light catches.
				tone := halfblock.Color(0x6E6E6E)
				if float64(py) < (PortalH*2-1)/2.0 {
					tone = 0xAEAEAE
				}
				c.SetPx(sx, sy, tone)
			case e <= glowRim:
				// Fancy outer glow: a shimmer-tinted halo blended over the
				// floor, fading to nothing at glowRim and pulsing gently.
				a := (glowRim - e) / (glowRim - frameRim)
				a *= 0.5 * (0.8 + 0.2*math.Sin(t*2.2))
				tint := f.At(p.X+px, p.Y*2+py, t)
				c.SetPx(sx, sy, halfblock.Lerp(c.PxAt(sx, sy), tint, a))
			}
		}
	}
	// Pedestal lip under the arch.
	for px := 1; px < PortalW-1; px++ {
		c.SetPx(p.X+px-ox, (p.Y+PortalH)*2-1-oy*2, 0x404040)
	}
	// Name label centered beneath the pedestal.
	lx := p.X + (PortalW-len(p.Name))/2 - ox
	c.WriteText(lx, p.Y+PortalH-oy, p.Name, 0xFFFFFF)
}

// PortalAt returns the portal whose trigger zone contains feet cell
// (cx, cy), if any.
func PortalAt(cx, cy int) (Portal, bool) {
	for _, p := range Portals {
		if p.TriggerContains(cx, cy) {
			return p, true
		}
	}
	return Portal{}, false
}
