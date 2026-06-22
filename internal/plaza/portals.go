package plaza

import (
	"crypto/sha256"
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
	"github.com/shellbound/shellbound/internal/render/light"
)

// Portal geometry: footprint in cells, glowing disc in pixels. The portal is a
// round vortex of chunky pixels that floats above a faint ground ring.
const (
	PortalW = 10
	PortalH = 6

	orbRadius  = 30 // vortex disc radius in pixels
	orbFloat   = 12 // gap between the disc bottom and the ground ring
	haloRadius = 46 // colored bloom that bleeds onto the surrounding floor
	orbPixel   = 3  // size of one chunky "pixel" block, for a pixel-art look
)

// Wave shaping: rings of light ripple outward from the core. The portal reads
// as soft concentric pulses rather than a harsh spinning vortex.
const (
	waveFreq      = 13.0 // ring count across the radius (radians)
	waveSpeed     = 3.6  // how fast rings travel outward
	waveAmp       = 0.13 // lightness swing between ring crest and trough
	orbLightMid   = 0.50 // base lightness at the core
	orbRadialFade = 0.20 // lightness lost from core to rim
)

// Portal color discipline: each portal has a signature hue derived from its
// world key, and the vortex breathes between that hue and a single nearby
// accent — one or two soft colors, never the full rainbow. Saturation is kept
// low so the glow is gentle, not neon.
const (
	portalSat         = 0.55
	portalAccentDelta = 22.0  // degrees from the base hue to the accent hue
	portalRampLevels  = 12    // lightness steps baked per hue into the palette
	portalLightMin    = 0.18  // darkest baked shimmer level
	portalLightStep   = 0.045 // lightness gap between baked levels
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

// orbShade returns the vortex color for a disc block at normalized radius nd
// (0 at the core, 1 at the rim) at time t. A single sine wave travels outward
// from the core, so the portal reads as soft concentric ripples; alternate
// ring bands tint toward the accent hue, and the body fades gently to the rim.
func orbShade(baseHue, nd, t float64) canvas.Color {
	// Concentric wave: the crest moves outward as t grows.
	wave := math.Sin(nd*waveFreq - t*waveSpeed)
	hue := baseHue
	if wave > 0 {
		hue = baseHue + portalAccentDelta
	}
	l := orbLightMid + waveAmp*wave - orbRadialFade*nd
	return canvas.HSL(hue, portalSat, clampLight(l))
}

// clampLight keeps a lightness within the baked shimmer range so every emitted
// color lands cleanly on a palette register (and never blows out to white).
func clampLight(l float64) float64 {
	const max = portalLightMin + (portalRampLevels-1)*portalLightStep
	if l < portalLightMin {
		return portalLightMin
	}
	if l > max {
		return max
	}
	return l
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

	// Soft colored bloom that bleeds onto the (otherwise dimmed) floor around
	// the gateway, so the portal glows into the plaza like the lamps do.
	light.Glow(c, ax, cy, haloRadius, canvas.HSL(baseHue, portalSat, 0.42))

	// The vortex disc, drawn as chunky orbPixel-sized blocks so it sits in the
	// same pixel-art register as the sprites and tiles rather than as a smooth
	// gradient. Each block is shaded by its center sample.
	r2 := orbRadius * orbRadius
	for by := -orbRadius; by <= orbRadius; by += orbPixel {
		for bx := -orbRadius; bx <= orbRadius; bx += orbPixel {
			scx, scy := bx+orbPixel/2, by+orbPixel/2 // block center
			d2 := scx*scx + scy*scy
			if d2 > r2 {
				continue
			}
			nd := math.Sqrt(float64(d2)) / float64(orbRadius)
			col := orbShade(baseHue, nd, t)
			for yy := 0; yy < orbPixel; yy++ {
				for xx := 0; xx < orbPixel; xx++ {
					c.Set(ax+bx+xx, cy+by+yy, col)
				}
			}
		}
	}

	// Name label, centered beneath, white with a black shadow for legibility.
	lw := canvas.TextWidth(p.Name)
	c.DrawTextShadow(ax-lw/2, groundY+5, p.Name, 0xFFFFFF, 0x000000)
}

// drawGroundRing paints the faint, breathing iso-ellipse the vortex hovers
// over — a 2:1 ring of chunky blocks in the portal's hue that anchors it to
// the floor.
func drawGroundRing(c *canvas.Canvas, ax, groundY int, baseHue, t float64) {
	rx := float64(orbRadius + 5)
	ry := rx / 2
	l := clampLight(0.30 + 0.05*math.Sin(t*2))
	col := canvas.HSL(baseHue, portalSat, l)
	const steps = 80
	for i := 0; i < steps; i++ {
		a := float64(i) / steps * 2 * math.Pi
		// Snap to the chunky pixel grid so the ring matches the disc's blocks.
		x := ax + (int(rx*math.Cos(a))/orbPixel)*orbPixel
		y := groundY + (int(ry*math.Sin(a))/orbPixel)*orbPixel
		c.FillRect(x, y, orbPixel, orbPixel, col)
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
