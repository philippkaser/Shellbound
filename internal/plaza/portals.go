package plaza

import (
	"crypto/sha256"
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
)

// Portal geometry: footprint in cells, glowing disc in pixels. The portal lies
// in the isometric ground plane as a 2:1 ellipse (half-height is half the
// half-width, matching the tile diamonds) so it sits in the world rather than
// facing the camera as a flat circle.
const (
	PortalW = 10
	PortalH = 6

	orbHalfW = 30 // disc half-width in pixels
	orbHalfH = 15 // disc half-height (2:1 iso ellipse)
	orbPixel = 3  // size of one chunky "pixel" block (bigger = lower-res, calmer)

	// Soft colored bloom on the floor around the gateway (also a 2:1 ellipse).
	haloHalfW = 72
	haloHalfH = 36
)

// Surface shaping: drifting caustics make the pool shimmer like the plaza
// fountain's water, brightest at the center and fading to the rim.
const (
	orbLightMid   = 0.50 // base lightness at the center of the pool
	orbRadialFade = 0.20 // lightness lost from center to rim
)

// Portal color discipline: each portal has a signature hue derived from its
// world key, and the vortex breathes across that hue and a single nearby accent
// — one or two soft colors, never the full rainbow. Saturation is kept low so
// the glow is gentle, not neon. The hue is baked at several intermediate steps
// and the lightness in a fine ramp, so the wave's color transitions read smooth
// rather than as two hard bands.
const (
	portalSat         = 0.55
	portalAccentDelta = 18.0 // degrees of gentle hue drift across the wave
	portalHueSteps    = 4    // baked hue stops between base and accent
	portalRampLevels  = 10   // lightness steps baked per hue into the palette
	portalLightMin    = 0.18 // darkest baked shimmer level
	portalLightStep   = 0.05 // lightness gap between baked levels
)

// Portal is one gateway in the plaza referencing a world by key. X, Y is the
// top-left cell of its footprint.
type Portal struct {
	Key  string // world registry key
	Name string // pedestal label
	X, Y int
}

// Portals is the 1.0 set, standing evenly across the north end of the plaza.
var Portals = []Portal{
	{Key: "bomberman", Name: "Bomberman", X: 15, Y: 3},
	{Key: "chess", Name: "Chess", X: 33, Y: 3},
	{Key: "doom", Name: "Doom", X: 51, Y: 3},
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
		for s := 0; s < portalHueSteps; s++ {
			hue := base + portalAccentDelta*float64(s)/float64(portalHueSteps-1)
			for i := 0; i < portalRampLevels; i++ {
				l := portalLightMin + float64(i)*portalLightStep
				out = append(out, canvas.HSL(hue, portalSat, l))
			}
		}
	}
	return out
}

// poolShade returns the surface color for a pool block at (bx, by) pixels from
// the center, normalized radius nd, at time t. A few drifting sines layer into
// shifting caustics so the pool glints like water; the body fades gently to the
// rim and the hue drifts a touch for life. The caustics are sampled on a coarse
// grid so the shimmer reads as chunky water rather than fine noise.
func poolShade(baseHue, nd, bx, by, t float64) canvas.Color {
	const cell = 6 // caustic sample size in px (bigger = chunkier shimmer)
	qx := math.Round(bx/cell) * cell
	qy := math.Round(by/cell) * cell
	shim := 0.10*math.Sin(qx*0.16+t*1.6) +
		0.07*math.Sin(qy*0.20-t*1.3) +
		0.05*math.Sin((qx+qy)*0.10+t*2.1)
	l := orbLightMid - orbRadialFade*nd + shim
	if nd > 0.7 { // feather the outer rim so it dissolves into the glow
		l -= (nd - 0.7) * 1.1
	}
	hue := baseHue + portalAccentDelta*(0.4+0.3*math.Sin((qx-qy)*0.11+t*1.0))
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

// TriggerContains reports whether feet at cell (cx, cy) are on the portal — the
// 3×3 of cells centered on the footprint center, which is exactly where the
// glowing disc is drawn (RenderIso centers it on Center()). Keeping the hitbox
// aligned with the visible disc means you enter when you step onto it, not from
// a tile away.
func (p Portal) TriggerContains(cx, cy int) bool {
	ccx, ccy := p.X+PortalW/2, p.Y+PortalH/2
	dx, dy := cx-ccx, cy-ccy
	return dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1
}

// Center returns the portal's footprint center cell (fractional).
func (p Portal) Center() (float64, float64) {
	return float64(p.X) + float64(PortalW)/2, float64(p.Y) + float64(PortalH)/2
}

// RenderIso draws the portal as a glowing vortex lying in the isometric ground
// plane (a 2:1 ellipse), with a soft colored bloom spilling onto the floor and
// the name baked beneath. (originSx, originSy) is the screen-space point at the
// canvas's top-left.
func (p Portal) RenderIso(c *canvas.Canvas, t, originSx, originSy float64) {
	baseHue := PortalHue(p.Key)
	ax, cy := p.GlowCenter(originSx, originSy)

	// Soft colored bloom washing onto the surrounding floor — a 2:1 iso ellipse
	// with a smooth falloff, so the portal glows gently into the plaza along the
	// ground like the lamps do, with no hard edge.
	softGlow(c, ax, cy, haloHalfW, haloHalfH, canvas.HSL(baseHue, portalSat, 0.30))

	// The pool itself: an iso ground ellipse of chunky orbPixel blocks so it sits
	// in the same pixel-art register as the sprites and tiles. nd is the
	// elliptical (world-circular) distance; the surface shimmers like water, and
	// the rim feathers out so there's no hard edge (no outline ring).
	for by := -orbHalfH; by <= orbHalfH; by += orbPixel {
		for bx := -orbHalfW; bx <= orbHalfW; bx += orbPixel {
			nx := float64(bx+orbPixel/2) / orbHalfW
			ny := float64(by+orbPixel/2) / orbHalfH
			nd := math.Sqrt(nx*nx + ny*ny)
			if nd > 1 {
				continue
			}
			col := poolShade(baseHue, nd, float64(bx), float64(by), t)
			for yy := 0; yy < orbPixel; yy++ {
				for xx := 0; xx < orbPixel; xx++ {
					c.Set(ax+bx+xx, cy+by+yy, col)
				}
			}
		}
	}

	// A few fat motes of the portal's light drift up from the pool and fade — a
	// gentle particle effect that hints the gateway is alive.
	drawParticles(c, ax, cy, baseHue, t)

	// Name label, centered beneath, white with a black shadow for legibility.
	lw := canvas.TextWidth(p.Name)
	c.DrawTextShadow(ax-lw/2, cy+orbHalfH+6, p.Name, 0xFFFFFF, 0x000000)
}

// drawParticles lifts sparse, fat colored motes off the pool, swaying as they
// rise and fading out — each a soft additive disc so it glows over whatever is
// behind it.
func drawParticles(c *canvas.Canvas, ax, cy int, baseHue, t float64) {
	const n = 6
	for i := 0; i < n; i++ {
		fi := float64(i)
		prog := math.Mod(t*0.32+fi*0.41, 1.0) // 0..1 rise life
		if prog < 0.03 {
			continue
		}
		x := ax + int(8*math.Sin(t*0.8+fi*2.3)) + int((fi-2.5)*5)
		y := cy - 4 - int(prog*32)
		fade := 1 - prog
		core := canvas.HSL(baseHue, portalSat, clampLight(0.28+0.4*fade))
		// Soft 5px additive disc (radius 2) — fatter than a single pixel.
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				d := dx*dx + dy*dy
				if d > 4 {
					continue
				}
				k := 1 - float64(d)/5
				c.Set(x+dx, y+dy, c.At(x+dx, y+dy).Lighten(core.Scale(k)))
			}
		}
	}
}

// AccentColor returns an on-palette color in a world's signature hue at the
// given lightness. Portal worlds use it so their accents (blasts, pickups, exit
// runes) match the portal and quantize cleanly onto the baked color registers.
func AccentColor(key string, lightness float64) canvas.Color {
	return canvas.HSL(PortalHue(key), portalSat, clampLight(lightness))
}

// GlowCenter returns the portal disc's center in canvas pixel space for the
// given camera origin. The lighting pass uses it to softly illuminate the
// surrounding floor, so the portal reveals the plaza around it like a lamp.
func (p Portal) GlowCenter(originSx, originSy float64) (int, int) {
	cgx, cgy := p.Center()
	sx, sy := iso.Project(cgx, cgy)
	return int(sx - originSx), int(sy-originSy) + iso.HH
}

// softGlow adds a smooth additive bloom over a 2:1 ellipse around (cx, cy),
// brightest at the center and fading to nothing at the rim with a smoothstep
// falloff (no dither), so the portal's color washes gently onto the floor with
// no visible edge.
func softGlow(c *canvas.Canvas, cx, cy, halfW, halfH int, core canvas.Color) {
	for dy := -halfH; dy <= halfH; dy++ {
		ny := float64(dy) / float64(halfH)
		yy := cy + dy
		for dx := -halfW; dx <= halfW; dx++ {
			nx := float64(dx) / float64(halfW)
			d := nx*nx + ny*ny
			if d >= 1 {
				continue
			}
			s := 1 - math.Sqrt(d)
			k := s * s * (3 - 2*s) // smoothstep — soft shoulders, gentle gradient
			xx := cx + dx
			c.Set(xx, yy, c.At(xx, yy).Lighten(core.Scale(k)))
		}
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
