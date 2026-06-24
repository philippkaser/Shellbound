// Package cosmetic is Shellbound's avatar customization: a catalog of headwear
// players can wear, drawn over the avatar's head in the same monochrome,
// overhead-lit style as the figure itself. A few pieces are owned by everyone;
// the rest are unlocked by clearing the portal worlds (granted as inventory
// items keyed "cosmetic.<key>"). Equipping is persisted per player and
// broadcast so other players see it.
package cosmetic

import (
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/sprites"
)

// InventoryPrefix is the item-key prefix a granted cosmetic carries in the
// inventory, e.g. "cosmetic.horns" unlocks the "horns" cosmetic.
const InventoryPrefix = "cosmetic."

// Cosmetic is one wearable. draw is nil for the bare-headed "none".
type Cosmetic struct {
	Key     string
	Name    string
	Starter bool // owned by everyone from the start
	draw    func(c *canvas.Canvas, hx, hy, hr int, f sprites.Facing, t float64)
}

// Monochrome tones, overhead-lit to match the avatar.
const (
	hi   = canvas.Color(0xF2F2F2)
	mid  = canvas.Color(0xC2C2C2)
	low  = canvas.Color(0x8A8A8A)
	dark = canvas.Color(0x444444)
)

// catalog is the full set, in display order.
var catalog = []Cosmetic{
	{Key: "none", Name: "Bare-headed", Starter: true},
	{Key: "cap", Name: "Flat Cap", Starter: true, draw: drawCap},
	{Key: "band", Name: "Headband", Starter: true, draw: drawBand},
	{Key: "tophat", Name: "Top Hat", Starter: true, draw: drawTopHat},
	{Key: "antenna", Name: "Antenna", Starter: true, draw: drawAntenna},
	{Key: "crown", Name: "Sparkforged Crown", draw: drawCrown}, // Bomberman reward
	{Key: "horns", Name: "Hellbreaker Horns", draw: drawHorns}, // Doom reward
	{Key: "halo", Name: "Wanderer's Halo", draw: drawHalo},     // (future reward)
}

var byKey = func() map[string]Cosmetic {
	m := make(map[string]Cosmetic, len(catalog))
	for _, c := range catalog {
		m[c.Key] = c
	}
	return m
}()

// All returns the full catalog in display order.
func All() []Cosmetic { return catalog }

// Name returns a cosmetic's display name (or "Bare-headed" if unknown).
func Name(key string) string {
	if c, ok := byKey[key]; ok {
		return c.Name
	}
	return "Bare-headed"
}

// IsStarter reports whether everyone owns the cosmetic by default.
func IsStarter(key string) bool {
	c, ok := byKey[key]
	return ok && c.Starter
}

// Valid reports whether key names a real cosmetic.
func Valid(key string) bool { _, ok := byKey[key]; return ok }

// Draw paints the equipped cosmetic over the avatar whose feet are at
// (footX, footY). Unknown keys and "none" draw nothing.
func Draw(c *canvas.Canvas, footX, footY int, f sprites.Facing, key string, t float64) {
	cos, ok := byKey[key]
	if !ok || cos.draw == nil {
		return
	}
	hx, hy := sprites.HeadCenter(footX, footY)
	cos.draw(c, hx, hy, sprites.HeadRadius(), f, t)
}

// --- the pieces ---

// drawCap: a rounded cap hugging the crown with a short brim toward the facing.
func drawCap(c *canvas.Canvas, hx, hy, hr int, f sprites.Facing, t float64) {
	for dy := -hr - 1; dy <= -hr/3; dy++ {
		w := int(math.Sqrt(math.Max(0, float64((hr+1)*(hr+1)-dy*dy))))
		tone := mid
		if dy < -hr/2 {
			tone = hi
		}
		c.HLine(hx-w, hx+w, hy+dy, tone)
	}
	bx := hx
	switch f {
	case sprites.FaceLeft:
		bx = hx - hr - 2
	case sprites.FaceRight:
		bx = hx + hr - 1
	}
	c.FillRect(min2(bx, hx-hr+1), hy-hr/3, hr+3, 2, low) // brim
}

// drawBand: a simple headband stripe across the brow.
func drawBand(c *canvas.Canvas, hx, hy, hr int, f sprites.Facing, t float64) {
	c.FillRect(hx-hr, hy-hr/2-1, 2*hr+1, 2, low)
	c.HLine(hx-hr, hx+hr, hy-hr/2-1, mid)
}

// drawTopHat: a short cylinder on a thin brim.
func drawTopHat(c *canvas.Canvas, hx, hy, hr int, f sprites.Facing, t float64) {
	brimY := hy - hr/2
	c.FillRect(hx-hr-1, brimY, 2*hr+3, 2, dark)  // brim
	c.FillRect(hx-hr+2, brimY-9, 2*hr-3, 9, mid) // crown
	c.FillRect(hx-hr+2, brimY-9, 2, 9, low)      // shadow side
	c.HLine(hx-hr+2, hx+hr-2, brimY-9, hi)       // top catch-light
}

// drawAntenna: a stalk topped by a bobbing dot.
func drawAntenna(c *canvas.Canvas, hx, hy, hr int, f sprites.Facing, t float64) {
	topY := hy - hr - 6 + int(math.Round(math.Sin(t*4)))
	c.VLine(hx, hy-hr, topY, low)
	c.FillCircle(hx, topY-1, 2, hi)
}

// drawCrown: a band with five points — the Bomberman reward.
func drawCrown(c *canvas.Canvas, hx, hy, hr int, f sprites.Facing, t float64) {
	baseY := hy - hr
	c.FillRect(hx-hr, baseY, 2*hr+1, 3, mid)
	c.HLine(hx-hr, hx+hr, baseY+2, low)
	for i := -2; i <= 2; i++ {
		px := hx + i*((hr)/2)
		h := 4
		if i%2 == 0 {
			h = 6
		}
		c.VLine(px, baseY-h, baseY, hi)
		c.FillCircle(px, baseY-h-1, 1, hi)
	}
}

// drawHorns: two curved horns sweeping up from the temples — the Doom reward.
func drawHorns(c *canvas.Canvas, hx, hy, hr int, f sprites.Facing, t float64) {
	for i := 0; i < 7; i++ {
		dy := -i
		dx := i / 2
		w := 2 - i/4
		c.FillRect(hx-hr-dx, hy-hr/2+dy-3, w+1, 2, mid)
		c.FillRect(hx+hr+dx-1, hy-hr/2+dy-3, w+1, 2, mid)
	}
	c.FillCircle(hx-hr-3, hy-hr/2-9, 1, hi)
	c.FillCircle(hx+hr+2, hy-hr/2-9, 1, hi)
}

// drawHalo: a thin ring floating above the head, bobbing gently.
func drawHalo(c *canvas.Canvas, hx, hy, hr int, f sprites.Facing, t float64) {
	cy := hy - hr - 6 + int(math.Round(math.Sin(t*2)*1.5))
	rx, ry := hr+1, 3
	const steps = 40
	for i := 0; i < steps; i++ {
		a := float64(i) / steps * 2 * math.Pi
		x := hx + int(float64(rx)*math.Cos(a))
		y := cy + int(float64(ry)*math.Sin(a))
		c.Set(x, y, hi)
	}
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
