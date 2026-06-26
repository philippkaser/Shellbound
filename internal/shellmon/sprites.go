package shellmon

import (
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// Shellmon are drawn as procedural monochrome pixel art in the same
// overhead-lit grey ramp as the rest of Shellbound, but with richer, multi-part
// silhouettes than the avatars: each is built from outlined volumes (a dark rim
// makes parts read against each other and the world) plus appendages — horns,
// fins, leaves, claws, tails — and detail passes (eyes with glints, mouths,
// markings). A sprite is described in "design pixels" around a center and scaled
// by an integer unit u (u=3 ≈ 66px tall battle portraits; u=1 tiny encounters).

// Overhead-lit grey ramp (top catches light, belly falls into shadow) plus an
// outline and a bright specular.
const (
	sTop     = canvas.Color(0xEDEDED)
	sUp      = canvas.Color(0xC4C4C4)
	sMid     = canvas.Color(0x9A9A9A)
	sLow     = canvas.Color(0x6E6E6E)
	sDark    = canvas.Color(0x3A3A3A) // shadowed detail
	sOutline = canvas.Color(0x202020) // silhouette rim
	sGlint   = canvas.Color(0xFBFBFB) // specular highlight / eye shine
)

// spriteCtx carries the draw target, the creature center and the scale unit.
type spriteCtx struct {
	c      *canvas.Canvas
	cx, cy int
	u      int
}

// DrawCreature paints a species centered at (cx, cy) at scale u. The center is
// the body's middle; sprites extend roughly ±11·u in each direction.
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
	case v < 0.22:
		return sTop
	case v < 0.48:
		return sUp
	case v < 0.74:
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

// --- low-level fills (actual-pixel coordinates) ---

func (s *spriteCtx) oval(cx, cy, rx, ry int, col canvas.Color) {
	if rx < 1 || ry < 1 {
		return
	}
	for dy := -ry; dy <= ry; dy++ {
		w := int(float64(rx) * math.Sqrt(math.Max(0, 1-float64(dy*dy)/float64(ry*ry))))
		for dx := -w; dx <= w; dx++ {
			s.c.Set(cx+dx, cy+dy, col)
		}
	}
}

func (s *spriteCtx) ovalLit(cx, cy, rx, ry int) {
	if rx < 1 || ry < 1 {
		return
	}
	for dy := -ry; dy <= ry; dy++ {
		w := int(float64(rx) * math.Sqrt(math.Max(0, 1-float64(dy*dy)/float64(ry*ry))))
		v := float64(dy+ry) / float64(2*ry)
		tone := sramp(v)
		for dx := -w; dx <= w; dx++ {
			col := tone
			if dx < -w/3 && v > 0.25 {
				col = sdarker(tone) // shade the lower-left for roundness
			}
			s.c.Set(cx+dx, cy+dy, col)
		}
	}
}

// triAt fills a triangle (tip up, or down) at actual-pixel (cx, cy) base.
func (s *spriteCtx) triAt(cx, cy, halfW, h int, down bool, col canvas.Color) {
	if h < 1 {
		h = 1
	}
	for i := 0; i <= h; i++ {
		w := halfW * (h - i) / h
		yy := cy - i
		if down {
			yy = cy + i
		}
		for dx := -w; dx <= w; dx++ {
			s.c.Set(cx+dx, yy, col)
		}
	}
}

func (s *spriteCtx) disc(x, y, d int, col canvas.Color) {
	if d <= 1 {
		s.c.Set(x, y, col)
		return
	}
	s.c.FillCircle(x, y, d/2, col)
}

// --- design-space building blocks (offsets/sizes in design px, scaled by u) ---

// mass is an outlined, top-lit volume: the dark rim separates it from neighbors
// and the world, which is what makes the creatures read as solid and complex.
func (s *spriteCtx) mass(ox, oy, rx, ry int) {
	cx, cy := s.cx+ox*s.u, s.cy+oy*s.u
	s.oval(cx, cy, rx*s.u+1, ry*s.u+1, sOutline)
	s.ovalLit(cx, cy, rx*s.u, ry*s.u)
}

// blade is an outlined triangle appendage (fin, horn, leaf, flame) pointing up
// or down, shaded lighter toward the lit right side.
func (s *spriteCtx) blade(ox, oy, halfW, h int, down bool, tone canvas.Color) {
	cx, cy := s.cx+ox*s.u, s.cy+oy*s.u
	s.triAt(cx, cy, halfW*s.u+1, h*s.u+1, down, sOutline)
	s.triAt(cx, cy, halfW*s.u, h*s.u, down, tone)
}

// glint adds a bright specular highlight near the top-right of a volume.
func (s *spriteCtx) glint(ox, oy int) {
	s.disc(s.cx+ox*s.u, s.cy+oy*s.u, s.u, sGlint)
}

// eye draws a small glossy eye — a dark bead with a thin rim and a single
// shine, so it reads as a creature's eye rather than a googly cartoon one.
// fierce adds a thin slanted brow for a sharper expression.
func (s *spriteCtx) eye(ox, oy int, fierce bool) {
	cx, cy := s.cx+ox*s.u, s.cy+oy*s.u
	r := s.u / 2
	if r < 1 {
		r = 1
	}
	s.c.FillCircle(cx, cy, r+1, sOutline) // thin rim
	s.c.FillCircle(cx, cy, r, sDark)      // dark bead
	s.c.Set(cx-r/2, cy-r/2, sGlint)       // shine
	if fierce {
		for i := 0; i <= r+1; i++ {
			s.c.Set(cx-r-1+i, cy-r-2-i/2, sOutline) // slanted brow
		}
	}
}

// stroke draws a thick line between two design points (limbs, vines, lures).
func (s *spriteCtx) stroke(ox0, oy0, ox1, oy1, thick int, col canvas.Color) {
	x0, y0 := s.cx+ox0*s.u, s.cy+oy0*s.u
	x1, y1 := s.cx+ox1*s.u, s.cy+oy1*s.u
	dx, dy := absI(x1-x0), -absI(y1-y0)
	sx, sy := sgn(x1-x0), sgn(y1-y0)
	err := dx + dy
	for {
		s.disc(x0, y0, thick, col)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// arc draws a darker curved seam across a body (shell segments, ripples).
func (s *spriteCtx) arc(ox, oy, rx, ry int, col canvas.Color) {
	cx, cy := s.cx+ox*s.u, s.cy+oy*s.u
	rxu, ryu := rx*s.u, ry*s.u
	for dx := -rxu; dx <= rxu; dx++ {
		yy := cy - int(float64(ryu)*math.Sqrt(math.Max(0, 1-float64(dx*dx)/float64(rxu*rxu))))
		s.c.Set(cx+dx, yy, col)
	}
}

// teeth draws a row of small triangles (a maw) between ±span at oy.
func (s *spriteCtx) teeth(ox, oy, span, count int, down bool) {
	cx, cy := s.cx+ox*s.u, s.cy+oy*s.u
	sp := span * s.u
	for i := 0; i < count; i++ {
		tx := cx - sp + 2*sp*i/maxI(1, count-1)
		s.triAt(tx, cy, max1(s.u/2), s.u, down, sGlint)
	}
}

// legs drops a pair (or more) of short stubby limbs under a body.
func (s *spriteCtx) legs(oy, span, h int, n int) {
	for i := 0; i < n; i++ {
		ox := -span + 2*span*i/maxI(1, n-1)
		s.stroke(ox, oy, ox, oy+h, max1(s.u/2)*2, sLow)
	}
}

// speckle scatters a few darker flecks over a body for texture (moss, scales).
func (s *spriteCtx) speckle(ox, oy, rx, ry int, seed int) {
	pts := []int{3, 7, 1, 5, 2, 6, 4, 0}
	for i := 0; i+1 < len(pts); i += 2 {
		dx := (pts[(i+seed)%len(pts)] - 4) * rx / 4
		dy := (pts[(i+1+seed)%len(pts)] - 4) * ry / 4
		s.c.Set(s.cx+(ox+dx)*s.u, s.cy+(oy+dy)*s.u, sLow)
	}
}

// === Spark line — embers, flame and heat: sharp crests, fierce eyes ===

func spriteCindle(s *spriteCtx) {
	// A living flame: a broad ember base narrowing through a tongue to a tip,
	// with side licks and a glowing core — a flame silhouette, not an egg.
	s.blade(-5, 1, 2, 5, false, sMid) // side licks
	s.blade(5, 1, 2, 5, false, sMid)
	s.mass(0, 6, 6, 4)               // ember base
	s.mass(0, 1, 4, 5)               // flame body
	s.blade(0, -4, 4, 8, false, sUp) // flame tip
	s.arc(0, 7, 4, 2, sLow)          // base seam
	s.glint(1, -2)
	s.eye(-2, 4, true)
	s.eye(2, 4, true)
	s.dot(0, 7)                          // ember mouth
	s.c.Set(s.cx-7*s.u, s.cy-2*s.u, sUp) // floating sparks
	s.c.Set(s.cx+7*s.u, s.cy-5*s.u, sUp)
}

func spriteFlickit(s *spriteCtx) {
	// A sparky imp: horned head, little arms, a forked tail.
	s.blade(-3, -4, 1, 4, false, sUp) // horns
	s.blade(3, -4, 1, 4, false, sUp)
	s.stroke(6, 6, 9, 3, max1(s.u/2)*2, sMid) // forked tail
	s.stroke(9, 3, 11, 4, max1(s.u/2), sMid)
	s.stroke(9, 3, 11, 1, max1(s.u/2), sMid)
	s.mass(0, 5, 5, 5)                          // body
	s.stroke(-5, 4, -7, 6, max1(s.u/2)*2, sMid) // arms
	s.stroke(5, 4, 7, 6, max1(s.u/2)*2, sMid)
	s.mass(0, -1, 4, 4)               // head
	s.stroke(-2, 5, 2, 3, s.u, sDark) // lightning marking
	s.stroke(2, 3, -1, 7, s.u, sDark)
	s.eye(-2, -1, true)
	s.eye(2, -1, true)
	s.glint(2, -3)
}

func spriteCindershell(s *spriteCtx) {
	// A molten snail: a spiral shell glowing along its seams, head poking out
	// front, stubby feet — read by the concentric spiral, not a plain dome.
	s.legs(9, 5, 3, 3)
	s.mass(2, 3, 8, 7) // shell
	// Spiral: concentric arcs tightening toward an off-center eye of the shell.
	s.arc(2, 6, 6, 6, sLow)
	s.arc(3, 5, 4, 4, sLow)
	s.arc(3, 4, 2, 2, sLow)
	s.stroke(0, -3, 3, 0, max1(s.u/2), sUp) // glowing magma seam
	s.blade(2, -5, 1, 3, false, sUp)        // heat vent
	s.mass(-7, 5, 3, 3)                     // head
	s.stroke(-9, 6, -10, 7, s.u, sLow)      // foot/snout
	s.eye(-7, 4, true)
	s.glint(4, -1)
}

func spriteAshfin(s *spriteCtx) {
	// A fire salamander: jagged dorsal crest, flame tail, low slung body.
	s.blade(-2, -1, 2, 5, false, sMid) // crest spines
	s.blade(1, -2, 2, 6, false, sMid)
	s.blade(4, -1, 2, 5, false, sMid)
	s.mass(0, 4, 8, 4)              // body
	s.blade(9, 2, 2, 5, false, sUp) // flame tail
	s.mass(-7, 4, 3, 3)             // head
	s.legs(7, 6, 3, 4)
	s.stroke(-9, 4, -7, 5, s.u, sDark) // jaw line
	s.eye(-7, 3, true)
	s.glint(0, 2)
}

// === Bramble line — seeds, vines, thorns and moss: soft eyes, leafy crowns ===

func spriteSprigling(s *spriteCtx) {
	// A sprouting seed: paired leaves on a stem, little root feet.
	s.stroke(0, -1, 0, -6, max1(s.u/2)*2, sLow) // stem
	s.leaf(-3, -6, 4, false)
	s.leaf(3, -7, 4, true)
	s.mass(0, 5, 6, 5) // seed body
	s.arc(0, 6, 4, 2, sLow)
	s.stroke(-3, 9, -4, 11, s.u, sLow) // roots
	s.stroke(3, 9, 4, 11, s.u, sLow)
	s.eye(-2, 5, false)
	s.eye(2, 5, false)
	s.glint(2, 3)
}

func spriteThornpod(s *spriteCtx) {
	// A spiny pod: layered scales, thorns all round, a sprig on top.
	for _, a := range []float64{0.5, 1.2, 1.9, 2.6, 3.3, 4.0, 4.7, 5.4, 6.1} {
		dx := int(math.Round(math.Cos(a) * 7))
		dy := int(math.Round(math.Sin(a)*9)) + 2
		s.stroke(dx*6/7, dy*6/9, dx, dy, s.u, sMid) // radiating thorns
	}
	s.mass(0, 2, 6, 8) // pod
	s.arc(0, 0, 5, 3, sLow)
	s.arc(0, 3, 5, 3, sLow)
	s.arc(0, 6, 4, 2, sLow)
	s.stroke(0, -6, 1, -9, s.u, sLow) // sprig
	s.leaf(2, -9, 3, true)
	s.eye(-2, 2, true)
	s.eye(2, 2, true)
}

func spriteMossmaw(s *spriteCtx) {
	// A mossy beast on four legs with a wide toothy maw and leafy back.
	s.leaf(-5, -4, 3, false) // back tufts
	s.leaf(0, -6, 4, false)
	s.leaf(5, -4, 3, true)
	s.mass(0, 3, 9, 6) // bulk
	s.legs(9, 6, 3, 4)
	s.speckle(0, 2, 7, 4, 1)                            // moss flecks
	s.oval(s.cx-3*s.u, s.cy+5*s.u, 5*s.u, 2*s.u, sDark) // maw
	s.teeth(-3, 4, 4, 5, true)
	s.eye(-5, -1, true)
	s.eye(0, -1, true)
	s.glint(4, 0)
}

func spriteFernling(s *spriteCtx) {
	// A slender fern sprite: a frond crown, vine arms, a bud.
	for i, a := range []float64{-1.1, -0.55, 0, 0.55, 1.1} {
		tipX := int(math.Round(math.Sin(a) * 8))
		s.stroke(0, -2, tipX, -10, max1(s.u/2)*2, sLow)
		s.leaf(tipX, -10, 3, i%2 == 0)
	}
	s.mass(0, 5, 4, 6)                          // slim body
	s.stroke(-4, 4, -8, 6, max1(s.u/2)*2, sLow) // vine arms
	s.leaf(-8, 6, 2, false)
	s.stroke(4, 4, 8, 5, max1(s.u/2)*2, sLow)
	s.leaf(8, 5, 2, true)
	s.eye(-1, 5, false)
	s.eye(2, 5, false)
	s.glint(2, 3)
}

// === Tide line — droplets, shells and waves: glossy eyes, fins ===

func spriteDripling(s *spriteCtx) {
	// A living dew drop with fin ears, a finned tail and a big glossy eye.
	s.blade(0, -4, 3, 6, false, sUp)  // droplet point
	s.blade(-6, 1, 2, 4, false, sMid) // fin ears
	s.blade(6, 1, 2, 4, false, sMid)
	s.mass(0, 4, 6, 6)
	s.blade(4, 9, 3, 3, true, sUp) // tail fin
	s.arc(0, 5, 4, 2, sUp)         // ripple shine
	s.eye(0, 3, false)
	s.c.FillCircle(s.cx+1*s.u, s.cy+2*s.u, max1(s.u/3), sGlint) // big shine
	s.glint(3, 0)
}

func spriteBrineback(s *spriteCtx) {
	// An armored crab: segmented carapace, eye stalks, big pincers, many legs.
	s.legs(7, 7, 3, 6)
	s.mass(0, 3, 9, 5) // carapace
	s.arc(0, 1, 7, 3, sLow)
	s.arc(0, 4, 6, 2, sLow)
	// pincers on arms
	s.stroke(-8, 3, -11, 1, max1(s.u/2)*2, sMid)
	s.mass(-11, 0, 2, 2)
	s.blade(-12, -1, 1, 2, false, sUp)
	s.blade(-10, -1, 1, 2, false, sUp)
	s.stroke(8, 3, 11, 1, max1(s.u/2)*2, sMid)
	s.mass(11, 0, 2, 2)
	s.blade(12, -1, 1, 2, false, sUp)
	s.blade(10, -1, 1, 2, false, sUp)
	// eye stalks
	s.stroke(-2, 0, -2, -3, s.u, sMid)
	s.stroke(2, 0, 2, -3, s.u, sMid)
	s.eye(-2, -4, false)
	s.eye(2, -4, false)
}

func spriteTidecoil(s *spriteCtx) {
	// A sea serpent: a coiling stack of finned segments rising to a crested head.
	s.mass(0, 8, 6, 3)
	s.blade(0, 5, 2, 3, false, sUp)
	s.mass(3, 4, 5, 3)
	s.blade(3, 1, 2, 3, false, sUp)
	s.mass(-2, 0, 5, 3)
	s.blade(-2, -3, 2, 3, false, sUp)
	s.mass(1, -5, 4, 4)                  // head
	s.blade(1, -9, 3, 4, false, sUp)     // crest
	s.stroke(-3, -5, -5, -4, s.u, sDark) // snout
	s.eye(0, -6, true)
	s.glint(2, -7)
}

func spriteGulper(s *spriteCtx) {
	// A deep-sea angler: a round body, a vast toothy mouth, a glowing lure.
	s.stroke(-1, -6, 2, -11, max1(s.u/2)*2, sLow) // lure stalk
	s.c.FillCircle(s.cx+2*s.u, s.cy-11*s.u, max1(s.u/2)+1, sGlint)
	s.mass(1, 3, 8, 7)
	s.blade(8, 1, 2, 4, false, sMid)           // dorsal fin
	s.stroke(9, 5, 11, 7, max1(s.u/2)*2, sMid) // tail
	// the maw: a big dark mouth low and forward with two rows of teeth
	s.oval(s.cx-3*s.u, s.cy+5*s.u, 6*s.u, 3*s.u, sDark)
	s.teeth(-3, 3, 5, 6, true)
	s.teeth(-3, 7, 5, 5, false)
	s.eye(0, -2, true)
	s.glint(4, -3)
}

// dot is a small dark feature (mouths, embers).
func (s *spriteCtx) dot(ox, oy int) {
	s.c.FillCircle(s.cx+ox*s.u, s.cy+oy*s.u, max1(s.u/2), sDark)
}

// leaf draws a slanted, veined leaf at design (ox, oy); flip mirrors it.
func (s *spriteCtx) leaf(ox, oy, length int, flip bool) {
	dir := 1
	if flip {
		dir = -1
	}
	cx, cy := s.cx+ox*s.u, s.cy+oy*s.u
	lu := length * s.u
	for i := 0; i <= lu; i++ {
		// width swells in the middle then tapers to a tip
		t := float64(i) / float64(lu)
		w := int(float64(length*s.u/2) * math.Sin(t*math.Pi))
		x := cx + dir*(i-lu/2)
		y := cy - (lu/2 - i) // slight upward slant
		for dy := -w; dy <= w; dy++ {
			tone := sUp
			if dy > w/3 {
				tone = sMid
			}
			s.c.Set(x, y+dy, tone)
		}
		s.c.Set(x, y-w-1, sOutline)
		s.c.Set(x, y+w+1, sOutline)
	}
	s.stroke(ox-dir*length/2, oy, ox+dir*length/2, oy, s.u, sLow) // vein
}

func absI(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sgn(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

func maxI(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}
