package plaza

import (
	"crypto/sha256"
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
	"github.com/shellbound/shellbound/internal/render/light"
)

// Portal geometry: footprint in cells, glowing disc in pixels. The portal is a
// round vortex that floats above a faint ground ring.
const (
	PortalW = 10
	PortalH = 6

	orbRadius  = 30 // vortex disc radius in pixels
	orbFloat   = 12 // gap between the disc bottom and the ground ring
	haloRadius = 50 // colored bloom that bleeds onto the surrounding floor
)

// Portal color discipline: each portal has a signature hue derived from its
// world key, and the vortex swirls between that hue and a single nearby accent
// — one or two colors, never the full rainbow.
const (
	portalSat         = 0.85
	portalAccentDelta = 26.0 // degrees from the base hue to the accent hue
	portalRampLevels  = 12   // lightness steps baked per hue into the palette
	portalLightMin    = 0.16 // darkest baked shimmer level
	portalLightStep   = 0.07 // lightness gap between baked levels
	portalLightMax    = 0.94 // brightest shimmer; below 1 so the core stays hued, not white
)

// Portal is one gateway in the plaza referencing a world by key. X, Y is the
// top-left cell of its footprint.
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

// PortalHue maps a world key to a stable base hue in [0, 360). A given world
// always shimmers in the same color.
func PortalHue(key string) float64 {
	sum := sha256.Sum256([]byte(key))
	return float64(int(sum[0])<<8|int(sum[1])) * 360 / 65536
}

// PaletteAccents returns every saturated color the portals can emit, so the
// canvas palette can quantize shimmer pixels exactly onto a register.
func PaletteAccents() []canvas.Color {
	var out []canvas.Color
	for _, p := range Portals {
		base := PortalHue(p.Key)
		for _, hue := range []float64{base, base + portalAccentDelta} {
			for i := 0; i < portalRampLevels; i++ {
				l := portalLightMin + float64(i)*portalLightStep
				out = append(out, canvas.HSL(hue, portalSat, l))
			}
		}
	}
	return out
}

// orbShade returns the vortex color for a disc pixel at normalized radius nd
// (0 at the core, 1 at the rim) and angle ang, at time t. The look is a
// high-energy spiral — a pulsing near-white core, rotating arms that wash
// between the portal's two hues, and rings of light travelling outward to a
// crisp rim — the plaza's take on a "max effort" burst.
func orbShade(baseHue, nd, ang, t float64) canvas.Color {
	// Rotating spiral arms: the phase twists with radius so the bands curl.
	swirl := math.Sin(ang*3 + nd*6.5 - t*3.2)
	hue := baseHue
	if swirl > 0.15 {
		hue = baseHue + portalAccentDelta
	}
	// Body: bright at the core, fading toward the rim, with the arms glinting.
	l := 0.58 - 0.34*nd + 0.12*swirl
	// Pulsing near-white core.
	l += 0.34 * math.Exp(-nd*nd/0.05) * (0.75 + 0.25*math.Sin(t*4.5))
	// A crisp rim plus a ring of light travelling out from the core.
	l += 0.26 * math.Exp(-sq(nd-0.86)/0.004)
	l += 0.22 * math.Exp(-sq(nd-math.Mod(t*0.5, 1.1))/0.006)
	if l > portalLightMax {
		l = portalLightMax
	}
	return canvas.HSL(hue, portalSat, l)
}

func sq(x float64) float64 { return x * x }

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

// RenderIso draws the portal as a round, glowing vortex floating over a faint
// ground ring, with the name baked beneath. (originSx, originSy) is the
// screen-space point at the canvas's top-left.
func (p Portal) RenderIso(c *canvas.Canvas, t, originSx, originSy float64) {
	baseHue := PortalHue(p.Key)
	cgx, cgy := p.Center()
	sx, sy := iso.Project(cgx, cgy)
	ax := int(sx - originSx)
	groundY := int(sy-originSy) + iso.HH

	// The disc bobs gently and floats above the ground ring.
	bob := int(math.Round(2 * math.Sin(t*1.6)))
	cy := groundY - orbRadius - orbFloat + bob

	drawGroundRing(c, ax, groundY, baseHue, t)

	// Colored bloom that bleeds onto the (otherwise dimmed) floor around the
	// gateway, so the portal glows into the plaza like the lamps do.
	light.Glow(c, ax, cy, haloRadius, canvas.HSL(baseHue, portalSat, 0.5))

	// The vortex disc itself.
	for dy := -orbRadius; dy <= orbRadius; dy++ {
		for dx := -orbRadius; dx <= orbRadius; dx++ {
			d2 := dx*dx + dy*dy
			if d2 > orbRadius*orbRadius {
				continue
			}
			nd := math.Sqrt(float64(d2)) / float64(orbRadius)
			ang := math.Atan2(float64(dy), float64(dx))
			c.Set(ax+dx, cy+dy, orbShade(baseHue, nd, ang, t))
		}
	}

	// Name label, centered beneath, white with a black shadow for legibility.
	lw := canvas.TextWidth(p.Name)
	c.DrawTextShadow(ax-lw/2, groundY+5, p.Name, 0xFFFFFF, 0x000000)
}

// drawGroundRing paints the faint, breathing iso-ellipse the vortex hovers
// over — a 2:1 ring in the portal's hue that anchors it to the floor.
func drawGroundRing(c *canvas.Canvas, ax, groundY int, baseHue, t float64) {
	rx := float64(orbRadius + 5)
	ry := rx / 2
	l := 0.34 + 0.06*math.Sin(t*2)
	col := canvas.HSL(baseHue, portalSat, l)
	const steps = 160
	for i := 0; i < steps; i++ {
		a := float64(i) / steps * 2 * math.Pi
		x := ax + int(rx*math.Cos(a))
		y := groundY + int(ry*math.Sin(a))
		c.Set(x, y, col)
		c.Set(x, y+1, col.Scale(0.6))
	}
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
