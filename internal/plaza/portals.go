package plaza

import (
	"crypto/sha256"
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
)

// Portal arch dimensions: footprint in cells, billboard in pixels.
const (
	PortalW = 10
	PortalH = 6

	archW = 56 // billboard width in pixels
	archH = 86 // billboard height in pixels
)

// Portal color discipline: each portal has a signature hue derived from its
// world key, and shimmers between that hue and a single nearby accent — one or
// two colors, never the full rainbow.
const (
	portalSat         = 0.85
	portalAccentDelta = 26.0 // degrees from the base hue to the accent hue
	portalRampLevels  = 6    // lightness steps baked per hue into the palette
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
				l := 0.30 + float64(i)*0.09
				out = append(out, canvas.HSL(hue, portalSat, l))
			}
		}
	}
	return out
}

// shade returns the shimmer color for a portal-local pixel at time t: the base
// hue or its accent, picked in soft diagonal bands, with a breathing
// lightness so the gateway glimmers.
func portalShade(baseHue, lx, ly, t float64) canvas.Color {
	hue := baseHue
	if math.Sin((lx+ly)*0.45+t*1.8) > 0.25 {
		hue = baseHue + portalAccentDelta
	}
	l := 0.50 + 0.17*math.Sin((lx-ly)*0.30+t*2.6)
	return canvas.HSL(hue, portalSat, l)
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
// the animated single-hue interior, a pedestal lip, and the name baked
// beneath. (originSx, originSy) is the screen-space point at the canvas's
// top-left.
func (p Portal) RenderIso(c *canvas.Canvas, t, originSx, originSy float64) {
	baseHue := PortalHue(p.Key)
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
			c.Set(ax+dx, y, portalShade(baseHue, float64(dx+half), float64(dy), t))
		}
		// Grey frame hugging the interior, brighter where the arch catches light.
		frame := canvas.Color(0x737373)
		if dy < archH/3 {
			frame = 0xA1A1A1
		}
		for w := 1; w <= 3; w++ {
			c.Set(ax-hw-w, y, frame)
			c.Set(ax+hw+w, y, frame)
		}
	}

	// Pedestal lip under the arch.
	c.FillRect(ax-half-3, groundY, archW+6, 3, 0x404040)

	// Name label, centered beneath, white with a black shadow for legibility.
	lw := canvas.TextWidth(p.Name)
	c.DrawTextShadow(ax-lw/2, groundY+5, p.Name, 0xFFFFFF, 0x000000)
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
