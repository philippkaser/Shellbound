// Package cosmetic is Shellbound's avatar customization: a catalog of headwear
// players can wear, drawn over the avatar's head in the same monochrome,
// overhead-lit style as the figure itself. A few pieces are owned by everyone;
// some are bought at the plaza shop for coins; the rest are unlocked by
// clearing the portal worlds. Every non-starter piece a player owns is recorded
// as an inventory item keyed "cosmetic.<key>". Equipping is persisted per
// player and broadcast so other players see it.
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
//
// Ownership comes from one of three places: Starter pieces belong to everyone;
// Price > 0 pieces are bought at the plaza shop for coins; the rest (Starter
// false, Price 0) are unlocked by clearing the portal worlds. In every case a
// non-starter the player owns is recorded as a "cosmetic.<key>" inventory item,
// which is what the wardrobe reads.
type Cosmetic struct {
	Key     string
	Name    string
	Starter bool // owned by everyone from the start
	Price   int  // coin cost at the shop; 0 means not for sale
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

	// Shop stock — bought with coins earned by being online.
	{Key: "beanie", Name: "Wool Beanie", Price: 30, draw: drawBeanie},
	{Key: "bow", Name: "Ribbon Bow", Price: 45, draw: drawBow},
	{Key: "visor", Name: "Sun Visor", Price: 60, draw: drawVisor},
	{Key: "flower", Name: "Flower Crown", Price: 90, draw: drawFlower},
	{Key: "wizard", Name: "Wizard Hat", Price: 120, draw: drawWizard},
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

// Shop returns the buyable cosmetics (those with a price), in display order.
func Shop() []Cosmetic {
	var out []Cosmetic
	for _, c := range catalog {
		if c.Price > 0 {
			out = append(out, c)
		}
	}
	return out
}

// Price returns a cosmetic's coin cost (0 if unknown or not for sale).
func Price(key string) int { return byKey[key].Price }

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

// drawBeanie: a snug knit dome with a rolled brim band and a pom-pom — shop.
func drawBeanie(c *canvas.Canvas, hx, hy, hr int, f sprites.Facing, t float64) {
	for dy := -hr - 2; dy <= -hr/4; dy++ {
		w := int(math.Sqrt(math.Max(0, float64((hr+2)*(hr+2)-dy*dy))))
		tone := mid
		if dy < -hr/2 {
			tone = hi
		}
		c.HLine(hx-w, hx+w, hy+dy, tone)
	}
	c.FillRect(hx-hr-1, hy-hr/4, 2*hr+3, 3, low) // rolled brim
	c.FillCircle(hx, hy-hr-3, 2, hi)             // pom-pom
}

// drawBow: a ribbon bow perched on top of the head — shop.
func drawBow(c *canvas.Canvas, hx, hy, hr int, f sprites.Facing, t float64) {
	by := hy - hr - 1
	for dx := 1; dx <= 5; dx++ {
		h := dx + 1 // loops grow taller toward the outer edge
		c.FillRect(hx-2-dx, by-h/2, 1, h, mid)
		c.FillRect(hx+2+dx, by-h/2, 1, h, mid)
	}
	c.HLine(hx-7, hx-3, by-3, hi) // loop catch-lights
	c.HLine(hx+3, hx+7, by-3, hi)
	c.FillRect(hx-2, by-2, 5, 4, low) // knot
	c.FillRect(hx-1, by-2, 1, 4, hi)
}

// drawVisor: a brow band with a bright brim jutting toward the facing — shop.
func drawVisor(c *canvas.Canvas, hx, hy, hr int, f sprites.Facing, t float64) {
	c.FillRect(hx-hr, hy-hr/2-1, 2*hr+1, 3, low)
	c.HLine(hx-hr, hx+hr, hy-hr/2-1, mid)
	bw := hr + 3
	bxStart := hx - hr + 1
	switch f {
	case sprites.FaceLeft:
		bxStart = hx - hr - 3
	case sprites.FaceRight:
		bxStart = hx + 1
	}
	c.FillRect(bxStart, hy-hr/2+1, bw, 2, mid)
	c.HLine(bxStart, bxStart+bw-1, hy-hr/2+2, hi)
}

// drawFlower: a garland of little blossoms arching over the brow — shop.
func drawFlower(c *canvas.Canvas, hx, hy, hr int, f sprites.Facing, t float64) {
	const n = 7
	r := hr + 1
	for i := 0; i < n; i++ {
		frac := float64(i) / float64(n-1)
		px := hx - r + int(frac*float64(2*r))
		py := hy - hr/2 - int(math.Sin(frac*math.Pi)*3)
		c.Set(px, py, hi) // blossom center
		c.Set(px-1, py, mid)
		c.Set(px+1, py, mid)
		c.Set(px, py-1, mid)
		c.Set(px, py+1, low)
	}
}

// drawWizard: a tall leaning cone on a wide brim, a star at the tip — shop.
func drawWizard(c *canvas.Canvas, hx, hy, hr int, f sprites.Facing, t float64) {
	brimY := hy - hr/2
	steps := hr * 3
	for i := 0; i <= steps; i++ {
		yy := brimY - i
		frac := float64(i) / float64(steps)
		w := int(float64(hr) * (1 - frac))
		cx := hx + int(frac*frac*3) // the tip curls gently to one side
		for dx := -w; dx <= w; dx++ {
			tone := mid
			switch {
			case dx < -w/2:
				tone = low
			case dx > w/2:
				tone = hi
			}
			c.Set(cx+dx, yy, tone)
		}
	}
	c.FillRect(hx-hr-2, brimY, 2*hr+5, 2, low) // brim
	c.HLine(hx-hr-2, hx+hr+2, brimY, mid)
	c.FillCircle(hx+3, brimY-steps+1, 1, hi) // tip star
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
