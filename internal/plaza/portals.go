package plaza

import (
	"github.com/shellbound/shellbound/internal/render/halfblock"
	"github.com/shellbound/shellbound/internal/render/shimmer"
)

// Portal arch dimensions in cells.
const (
	PortalW = 10
	PortalH = 6
)

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
// portal mouth (the central region of the oval).
func (p Portal) TriggerContains(cx, cy int) bool {
	return cx >= p.X+2 && cx <= p.X+PortalW-3 &&
		cy >= p.Y+2 && cy <= p.Y+PortalH-1
}

// Render draws the portal into a screen canvas: a grey oval frame, the
// animated rainbow interior, a pedestal lip and the name label beneath.
// (ox, oy) is the world cell at the canvas's top-left; t is seconds since
// server start.
func (p Portal) Render(c *halfblock.Canvas, f *shimmer.Field, t float64, ox, oy int) {
	// Pixel-space oval: PortalW columns × PortalH*2 pixel rows.
	const pw, ph = float64(PortalW), float64(PortalH * 2)
	cx, cy := (pw-1)/2, (ph-1)/2
	for py := 0; py < PortalH*2; py++ {
		for px := 0; px < PortalW; px++ {
			dx := (float64(px) - cx) / (pw / 2)
			dy := (float64(py) - cy) / (ph / 2)
			e := dx*dx + dy*dy
			sx, sy := p.X+px-ox, p.Y*2+py-oy*2
			switch {
			case e <= 0.62:
				// Interior: the hue band sweeps diagonally through the oval.
				c.SetPx(sx, sy, f.At(p.X+px, p.Y*2+py, t))
			case e <= 1.0:
				// Frame: brighter on top where the light catches the arch.
				tone := halfblock.Color(0x737373)
				if py < PortalH {
					tone = 0xA1A1A1
				}
				c.SetPx(sx, sy, tone)
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
