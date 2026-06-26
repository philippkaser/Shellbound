package shellmon

import (
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// Shellmon are drawn as procedural monochrome pixel art in the same
// overhead-lit grey ramp as the avatars, so they sit in Shellbound's world. A
// sprite is described in "design pixels" around a center and scaled by an
// integer unit u (u=2 ≈ 36px tall for battle portraits, u=1 for small overworld
// encounters).

// Overhead-lit grey ramp (top catches light, belly falls into shadow).
const (
	sTop  = canvas.Color(0xEDEDED)
	sUp   = canvas.Color(0xC4C4C4)
	sMid  = canvas.Color(0x9A9A9A)
	sLow  = canvas.Color(0x6E6E6E)
	sDark = canvas.Color(0x363636) // eyes / features
)

// spriteCtx carries the draw target, the creature center and the scale unit.
type spriteCtx struct {
	c      *canvas.Canvas
	cx, cy int
	u      int
}

// DrawCreature paints a species centered at (cx, cy) at scale u. The center is
// the body's middle; sprites extend roughly ±9·u vertically.
func DrawCreature(c *canvas.Canvas, cx, cy, u int, speciesKey string) {
	sp, ok := bySpecies[speciesKey]
	if !ok || sp.sprite == nil {
		return
	}
	if u < 1 {
		u = 1
	}
	sp.sprite(&spriteCtx{c: c, cx: cx, cy: cy, u: u})
}

func sramp(v float64) canvas.Color {
	switch {
	case v < 0.25:
		return sTop
	case v < 0.5:
		return sUp
	case v < 0.75:
		return sMid
	default:
		return sLow
	}
}

func sdarker(c canvas.Color) canvas.Color {
	switch c {
	case sTop:
		return sUp
	case sUp:
		return sMid
	case sMid:
		return sLow
	default:
		return sDark
	}
}

// ball draws a top-lit filled ellipse centered at design offset (ox, oy) with
// design radii (rx, ry); the lower-left is shaded a notch darker for volume.
func (s *spriteCtx) ball(ox, oy, rx, ry int) {
	cx, cy := s.cx+ox*s.u, s.cy+oy*s.u
	rxu, ryu := rx*s.u, ry*s.u
	for dy := -ryu; dy <= ryu; dy++ {
		w := int(float64(rxu) * math.Sqrt(math.Max(0, 1-float64(dy*dy)/float64(ryu*ryu))))
		v := float64(dy+ryu) / float64(2*ryu)
		tone := sramp(v)
		for dx := -w; dx <= w; dx++ {
			col := tone
			if dx < -w/3 && v > 0.3 {
				col = sdarker(tone)
			}
			s.c.Set(cx+dx, cy+dy, col)
		}
	}
}

// triUp draws a top-lit triangle pointing up: a fin, flame tongue or leaf.
func (s *spriteCtx) triUp(ox, oy, halfW, h int, tone canvas.Color) {
	cx, cy := s.cx+ox*s.u, s.cy+oy*s.u
	hu := h * s.u
	for i := 0; i < hu; i++ {
		w := (halfW * s.u) * (hu - i) / hu
		yy := cy - i
		for dx := -w; dx <= w; dx++ {
			s.c.Set(cx+dx, yy, tone)
		}
	}
}

// spike draws a short triangle pointing away along (dirx, diry) from (ox, oy).
func (s *spriteCtx) spike(ox, oy, dirx, diry, length int, tone canvas.Color) {
	x, y := s.cx+ox*s.u, s.cy+oy*s.u
	for i := 0; i <= length*s.u; i++ {
		s.c.Set(x+dirx*i, y+diry*i, tone)
		s.c.Set(x+dirx*i, y+diry*i+1, tone)
	}
}

// eyes draws two dark eyes at design offset (oy) spread ±sp about the center.
func (s *spriteCtx) eyes(oy, sp int) {
	r := s.u / 2
	if r < 1 {
		r = 1
	}
	for _, ox := range []int{-sp, sp} {
		s.c.FillCircle(s.cx+ox*s.u, s.cy+oy*s.u, r, sDark)
	}
}

// belly darkens a small patch low on the body for grounding.
func (s *spriteCtx) belly(ox, oy, rx, ry int) {
	cx, cy := s.cx+ox*s.u, s.cy+oy*s.u
	for dy := -ry * s.u; dy <= ry*s.u; dy++ {
		w := int(float64(rx*s.u) * math.Sqrt(math.Max(0, 1-float64(dy*dy)/float64(ry*ry*s.u*s.u))))
		for dx := -w; dx <= w; dx++ {
			s.c.Set(cx+dx, cy+dy, sLow)
		}
	}
}

func (s *spriteCtx) dot(ox, oy int, tone canvas.Color) {
	s.c.FillCircle(s.cx+ox*s.u, s.cy+oy*s.u, max1(s.u/2), tone)
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

// --- Spark line: wisps and embers, with flame tongues and sharp fins ---

func spriteCindle(s *spriteCtx) {
	s.triUp(0, -6, 4, 8, sUp) // flame crest
	s.ball(0, 1, 6, 7)
	s.eyes(-1, 2)
	s.dot(0, 4, sDark) // ember mouth
}

func spriteFlickit(s *spriteCtx) {
	s.triUp(-4, -6, 2, 5, sUp) // ear tufts
	s.triUp(4, -6, 2, 5, sUp)
	s.ball(0, 1, 6, 6)
	s.eyes(0, 2)
}

func spriteCindershell(s *spriteCtx) {
	s.ball(0, 3, 8, 5) // bulky shell
	s.belly(0, 5, 5, 2)
	s.triUp(-3, -2, 2, 4, sMid) // vents
	s.triUp(3, -2, 2, 4, sMid)
	s.ball(0, -3, 4, 4) // head poking up
	s.eyes(-3, 2)
}

func spriteAshfin(s *spriteCtx) {
	s.triUp(0, -4, 3, 7, sMid) // dorsal fin
	s.ball(0, 2, 7, 5)
	s.spike(6, 2, 1, 0, 3, sUp) // tail fin
	s.eyes(1, 3)
}

// --- Bramble line: seeds, pods and mossy maws, with leaves and thorns ---

func spriteSprigling(s *spriteCtx) {
	s.triUp(0, -7, 2, 6, sUp) // sprout
	s.ball(0, 2, 6, 6)
	s.belly(0, 5, 4, 2)
	s.eyes(1, 2)
}

func spriteThornpod(s *spriteCtx) {
	s.ball(0, 1, 6, 7)
	for _, a := range []float64{0.4, 1.1, 2.0, 2.7, 3.6, 4.3, 5.2, 5.9} {
		dx := int(math.Round(math.Cos(a) * 2))
		dy := int(math.Round(math.Sin(a) * 2))
		s.spike(dx*3, dy*3+1, dx, dy, 2, sMid)
	}
	s.eyes(1, 2)
}

func spriteMossmaw(s *spriteCtx) {
	s.ball(0, 1, 9, 7)
	s.belly(0, 4, 6, 3) // wide mouth
	s.eyes(-3, 1)
	s.triUp(-6, -4, 2, 3, sUp) // mossy tufts
	s.triUp(6, -4, 2, 3, sUp)
}

func spriteFernling(s *spriteCtx) {
	s.triUp(-3, -6, 2, 6, sUp) // fronds
	s.triUp(0, -7, 2, 7, sUp)
	s.triUp(3, -6, 2, 6, sUp)
	s.ball(0, 3, 5, 5)
	s.eyes(2, 2)
}

// --- Tide line: droplets, shells and coils ---

func spriteDripling(s *spriteCtx) {
	s.triUp(0, -7, 3, 5, sUp) // droplet point
	s.ball(0, 2, 6, 6)
	s.eyes(1, 2)
}

func spriteBrineback(s *spriteCtx) {
	s.ball(0, 2, 9, 5) // wide carapace
	s.belly(0, 4, 6, 2)
	s.spike(-9, 1, -1, 0, 3, sUp) // claws
	s.spike(9, 1, 1, 0, 3, sUp)
	s.eyes(-2, 2)
	s.eyes(2, 2)
}

func spriteTidecoil(s *spriteCtx) {
	s.ball(0, 6, 6, 3)   // lower coil
	s.ball(2, 2, 5, 3)   // mid coil
	s.ball(-1, -2, 5, 4) // upper coil
	s.ball(0, -6, 4, 4)  // head
	s.eyes(-6, 2)
}

func spriteGulper(s *spriteCtx) {
	s.ball(0, 2, 8, 7)
	s.belly(0, 5, 6, 3) // huge mouth
	s.eyes(0, 3)
}
