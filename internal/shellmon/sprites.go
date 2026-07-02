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
// blink closes the eyes for the current frame (a periodic idle tick).
type spriteCtx struct {
	c      *canvas.Canvas
	cx, cy int
	u      int
	blink  bool
}

// DrawCreature paints a species centered at (cx, cy) at scale u. The center is
// the body's middle; sprites extend roughly ±11·u in each direction.
func DrawCreature(c *canvas.Canvas, cx, cy, u int, speciesKey string) {
	drawCreature(c, cx, cy, u, speciesKey, false)
}

// DrawCreatureT paints a species with its idle animation at time t: the
// creature blinks every few seconds. phase staggers individuals so a pair on
// a battle stage never blinks in lockstep.
func DrawCreatureT(c *canvas.Canvas, cx, cy, u int, speciesKey string, t, phase float64) {
	// A 0.14s blink roughly every 3.7s, offset by phase.
	cycle := math.Mod(t+phase*1.31, 3.7)
	drawCreature(c, cx, cy, u, speciesKey, cycle < 0.14)
}

func drawCreature(c *canvas.Canvas, cx, cy, u int, speciesKey string, blink bool) {
	sp, ok := bySpecies[speciesKey]
	if !ok || sp.sprite == nil {
		return
	}
	if u < 1 {
		u = 1
	}
	sp.sprite(&spriteCtx{c: c, cx: cx, cy: cy, u: u, blink: blink})
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
// fierce adds a thin slanted brow for a sharper expression. During a blink
// the bead collapses to a closed lid line.
func (s *spriteCtx) eye(ox, oy int, fierce bool) {
	cx, cy := s.cx+ox*s.u, s.cy+oy*s.u
	r := s.u / 2
	if r < 1 {
		r = 1
	}
	if s.blink {
		s.c.HLine(cx-r-1, cx+r+1, cy, sOutline) // closed lid
	} else {
		s.c.FillCircle(cx, cy, r+1, sOutline) // thin rim
		s.c.FillCircle(cx, cy, r, sDark)      // dark bead
		s.c.Set(cx-r/2, cy-r/2, sGlint)       // shine
	}
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

// slighter steps a tone one notch up the ramp (rim lights, catch highlights).
func slighter(c canvas.Color) canvas.Color {
	switch c {
	case sLow:
		return sMid
	case sMid:
		return sUp
	case sUp:
		return sTop
	default:
		return sGlint
	}
}

// blob is a flat-shaded outlined volume: one base tone, a darker lower-left
// shadow wedge and a short top catch-light. Flat fills read as pixel-art
// bodies where the old 4-band ramp read as a striped egg.
func (s *spriteCtx) blob(ox, oy, rx, ry int, base canvas.Color) {
	cx, cy := s.cx+ox*s.u, s.cy+oy*s.u
	rxu, ryu := rx*s.u, ry*s.u
	s.oval(cx, cy, rxu+1, ryu+1, sOutline)
	for dy := -ryu; dy <= ryu; dy++ {
		w := int(float64(rxu) * math.Sqrt(math.Max(0, 1-float64(dy*dy)/float64(ryu*ryu))))
		v := float64(dy+ryu) / float64(2*ryu)
		for dx := -w; dx <= w; dx++ {
			col := base
			switch {
			case v > 0.45 && dx < -w/3:
				col = sdarker(base) // grounded shadow side
			case v < 0.18 && dx > -w/2:
				col = slighter(base) // crown catch-light
			}
			s.c.Set(cx+dx, cy+dy, col)
		}
	}
}

// patch is an un-outlined flat oval laid over a body — bellies, muzzles,
// inner ears, wing feathers.
func (s *spriteCtx) patch(ox, oy, rx, ry int, col canvas.Color) {
	s.oval(s.cx+ox*s.u, s.cy+oy*s.u, maxI(rx*s.u, 1), maxI(ry*s.u, 1), col)
}

// snout is an outlined horizontal triangle pointing dir=-1 (left) or +1
// (right): muzzles, beaks, nose tapers, fish tails.
func (s *spriteCtx) snout(ox, oy, halfH, length, dir int, col canvas.Color) {
	cx, cy := s.cx+ox*s.u, s.cy+oy*s.u
	s.triSidePx(cx, cy, halfH*s.u+1, length*s.u+1, dir, sOutline)
	s.triSidePx(cx, cy, halfH*s.u, length*s.u, dir, col)
}

// triSidePx fills a horizontal triangle whose base is a vertical line at
// (cx, cy) of half-height h, tapering to a tip `length` px away toward dir.
func (s *spriteCtx) triSidePx(cx, cy, h, length, dir int, col canvas.Color) {
	if length < 1 {
		length = 1
	}
	for i := 0; i <= length; i++ {
		w := h * (length - i) / length
		x := cx + dir*i
		for dy := -w; dy <= w; dy++ {
			s.c.Set(x, cy+dy, col)
		}
	}
}

// ringEye is a large glossy eye for front-facing faces (owls, frogs,
// octopuses): a bright ring, a dark pupil and a shine. Blinks like eye().
func (s *spriteCtx) ringEye(ox, oy, r int) {
	cx, cy := s.cx+ox*s.u, s.cy+oy*s.u
	ru := maxI(r*s.u/2, 2)
	if s.blink {
		s.c.HLine(cx-ru, cx+ru, cy, sOutline)
		return
	}
	s.c.FillCircle(cx, cy, ru+1, sOutline)
	s.c.FillCircle(cx, cy, ru, sUp)
	s.c.FillCircle(cx, cy, maxI(ru/2, 1), sDark)
	s.c.Set(cx-ru/3, cy-ru/3, sGlint)
}

// tailSeg chains a curved tail/tentacle from (x0,y0) through the given design
// points at the given thickness.
func (s *spriteCtx) tailSeg(pts [][2]int, thick int, col canvas.Color) {
	for i := 0; i+1 < len(pts); i++ {
		s.stroke(pts[i][0], pts[i][1], pts[i+1][0], pts[i+1][1], thick, col)
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

// The species sprites are drawn as readable animal archetypes — fox, mouse,
// snail, shark, owl, hippo, rabbit, hedgehog, bear, songbird, puppy,
// tortoise, frog, crab, seahorse, anglerfish, penguin, octopus — so a player
// recognizes what a creature IS at a glance, the way a Pokémon reads. All in
// design pixels around the center; feet land near +9..+11.

// === Spark line ===

func spriteCindle(s *spriteCtx) {
	// A fox pup, three-quarter view: tall pointed ears, a sharp muzzle and a
	// bushy tail whose tip burns bright.
	// Tail sweeping up on the right, flame tip.
	s.tailSeg([][2]int{{5, 6}, {9, 4}, {10, 0}}, max1(s.u/2)*3, sMid)
	s.blade(10, -1, 2, 3, false, sGlint) // burning tail tip
	// Haunches + body.
	s.blob(2, 6, 5, 4, sMid)
	// Front legs.
	s.stroke(-2, 7, -2, 10, max1(s.u/2)*2, sLow)
	s.stroke(1, 7, 1, 10, max1(s.u/2)*2, sLow)
	// Head with big pointed ears.
	s.blade(-5, -7, 2, 4, false, sMid) // left ear
	s.blade(0, -8, 2, 4, false, sMid)  // right ear
	s.patch(-5, -8, 1, 2, sDark)       // inner ears
	s.patch(0, -9, 1, 2, sDark)
	s.blob(-2, -3, 4, 4, sMid)
	// Muzzle pointing left, nose, mouth.
	s.snout(-6, -2, 2, 3, -1, sUp)
	s.dot(-9, -2) // nose
	// Chest ruff.
	s.patch(-2, 2, 2, 2, sUp)
	s.eye(-3, -4, true)
	s.eye(1, -4, true)
}

func spriteFlickit(s *spriteCtx) {
	// A mouse: two huge round ears, a plump pear body, whiskers and a
	// zigzag lightning tail.
	// Lightning tail, right side.
	s.tailSeg([][2]int{{5, 5}, {8, 3}, {7, 0}, {10, -2}}, max1(s.u/2)*2, sUp)
	s.dot2(10, -2, sGlint)
	// Ears: outlined discs with dark centers.
	for _, e := range [][2]int{{-4, -7}, {4, -7}} {
		s.c.FillCircle(s.cx+e[0]*s.u, s.cy+e[1]*s.u, 3*s.u+1, sOutline)
		s.c.FillCircle(s.cx+e[0]*s.u, s.cy+e[1]*s.u, 3*s.u, sMid)
		s.c.FillCircle(s.cx+e[0]*s.u, s.cy+e[1]*s.u, 3*s.u/2, sDark)
	}
	// Pear body (head merges into it).
	s.blob(0, 3, 5, 6, sMid)
	s.patch(0, 6, 3, 3, sUp) // belly
	// Feet.
	s.patch(-3, 10, 2, 1, sLow)
	s.patch(3, 10, 2, 1, sLow)
	// Whiskers.
	s.stroke(-4, 2, -8, 1, max1(s.u/3), sUp)
	s.stroke(-4, 3, -8, 4, max1(s.u/3), sUp)
	s.stroke(4, 2, 8, 1, max1(s.u/3), sUp)
	s.stroke(4, 3, 8, 4, max1(s.u/3), sUp)
	// Face: round eyes, tiny nose, cheek sparks.
	s.eye(-2, 0, false)
	s.eye(2, 0, false)
	s.dot(0, 2) // nose
	s.dot2(-4, 2, sGlint)
	s.dot2(4, 2, sGlint)
}

func spriteCindershell(s *spriteCtx) {
	// A snail: a slug body stretched along the ground, eye stalks up front,
	// and a big spiral shell riding its back — the spiral glows at the seam.
	// Slug body: low and long, head rising at the left.
	s.blob(-3, 8, 8, 2, sUp)
	s.blob(-8, 5, 3, 4, sUp) // raised head/neck
	// Eye stalks with bead eyes — the defining snail feature.
	s.stroke(-9, 2, -11, -2, max1(s.u/2), sUp)
	s.stroke(-7, 2, -6, -2, max1(s.u/2), sUp)
	s.eye(-11, -3, false)
	s.eye(-6, -3, false)
	// Shell: big disc with a spiral.
	s.c.FillCircle(s.cx+3*s.u, s.cy-1*s.u, 6*s.u+1, sOutline)
	s.c.FillCircle(s.cx+3*s.u, s.cy-1*s.u, 6*s.u, sMid)
	s.arc(3, 3, 5, 5, sDark)
	s.arc(4, 2, 3, 3, sDark)
	s.arc(4, 1, 1, 1, sDark)
	s.stroke(6, -4, 8, -6, s.u, sGlint) // glowing seam vent
	s.patch(1, -4, 2, 1, slighter(sMid))
	s.dot(-10, 1) // little mouth
}

func spriteAshfin(s *spriteCtx) {
	// A shark in profile, swimming left: spindle body, the classic dorsal
	// fin, a two-lobed tail and gill slits — the tail lobes burn like coals.
	// Tail (right): two lobes.
	s.snout(9, 0, 1, 4, 1, sUp)
	s.blade(11, -2, 1, 3, false, sUp)
	s.blade(11, 4, 1, 3, true, sUp)
	s.dot2(12, -3, sGlint) // ember tips
	s.dot2(12, 5, sGlint)
	// Body: horizontal spindle with a pale belly.
	s.blob(0, 2, 9, 4, sMid)
	s.patch(-2, 4, 6, 2, sUp) // belly
	// Nose taper.
	s.snout(-9, 1, 3, 3, -1, sMid)
	// Dorsal fin.
	s.blade(1, -2, 3, 5, false, sMid)
	// Pectoral fin.
	s.blade(-2, 8, 2, 3, true, sLow)
	// Gills: three slits.
	s.stroke(-4, 0, -4, 3, max1(s.u/3), sDark)
	s.stroke(-3, 0, -3, 3, max1(s.u/3), sDark)
	s.stroke(-2, 0, -2, 3, max1(s.u/3), sDark)
	// Mouth underslung, a hint of teeth, fierce eye.
	s.stroke(-10, 4, -6, 5, max1(s.u/3), sOutline)
	s.c.Set(s.cx-8*s.u, s.cy+4*s.u+1, sGlint)
	s.eye(-8, 0, true)
}

func spriteVoltun(s *spriteCtx) {
	// An owl, front-on: upright egg body, two huge ringed eyes in a facial
	// disc, ear tufts, folded wings and little talons — charged with static.
	// Ear tufts.
	s.blade(-4, -9, 1, 3, false, sMid)
	s.blade(4, -9, 1, 3, false, sMid)
	// Body.
	s.blob(0, 0, 6, 8, sMid)
	// Folded wings: darker side patches.
	s.patch(-5, 2, 2, 5, sLow)
	s.patch(5, 2, 2, 5, sLow)
	// Chest chevrons.
	s.stroke(-2, 4, 0, 5, max1(s.u/3), sLow)
	s.stroke(0, 5, 2, 4, max1(s.u/3), sLow)
	s.stroke(-2, 6, 0, 7, max1(s.u/3), sLow)
	s.stroke(0, 7, 2, 6, max1(s.u/3), sLow)
	// Facial disc + big ring eyes + beak.
	s.patch(0, -4, 5, 3, sUp)
	s.ringEye(-2, -4, 3)
	s.ringEye(2, -4, 3)
	s.triAt(s.cx, s.cy, max1(s.u/2), s.u+1, true, sDark) // small down beak
	// Static sparks off the tufts.
	s.dot2(-6, -11, sGlint)
	s.dot2(6, -11, sGlint)
	// Talons.
	s.stroke(-2, 8, -2, 10, max1(s.u/2), sLow)
	s.stroke(2, 8, 2, 10, max1(s.u/2), sLow)
}

func spriteMagmaw(s *spriteCtx) {
	// A hippo: a massive rounded muzzle in front of a barrel body, tiny round
	// ears, nostril bumps and tusk nubs in the jaw — magma glows in the seams.
	// Barrel body behind.
	s.blob(4, 3, 6, 5, sMid)
	s.legs(8, 6, 3, 4)
	// Glowing back cracks.
	s.stroke(4, -1, 6, 1, max1(s.u/2), sGlint)
	s.stroke(7, 0, 8, 2, max1(s.u/3), sGlint)
	// Head: big dome + huge muzzle.
	s.blob(-4, -1, 5, 4, sMid)
	s.blob(-6, 4, 6, 4, sUp) // muzzle
	// Tiny ears.
	s.dot2(-7, -5, sMid)
	s.dot2(-1, -6, sMid)
	// Nostrils on the muzzle top.
	s.dot(-9, 2)
	s.dot(-5, 2)
	// Mouth line + tusk nubs poking up.
	s.stroke(-11, 6, -1, 6, max1(s.u/3), sOutline)
	s.triAt(s.cx-9*s.u, s.cy+6*s.u, max1(s.u/2), s.u, false, sGlint)
	s.triAt(s.cx-3*s.u, s.cy+6*s.u, max1(s.u/2), s.u, false, sGlint)
	s.eye(-6, -2, true)
	s.eye(-2, -3, true)
}

// === Bramble line ===

func spriteSprigling(s *spriteCtx) {
	// A rabbit: two long leaf-bladed ears, a crouched round body, cheeks,
	// a puff tail and big hind feet.
	// Ears: tall, slightly splayed, leafy inner.
	s.blade(-3, -6, 2, 7, false, sMid)
	s.blade(3, -7, 2, 7, false, sMid)
	s.patch(-3, -9, 1, 2, sLow)
	s.patch(3, -10, 1, 2, sLow)
	// Puff tail.
	s.dot2(7, 5, sGlint)
	// Body crouched.
	s.blob(0, 4, 6, 5, sMid)
	// Hind haunch + big hind foot.
	s.patch(4, 6, 3, 3, sLow)
	s.patch(4, 9, 3, 1, sUp)
	// Front paws.
	s.stroke(-3, 8, -3, 10, max1(s.u/2), sLow)
	s.stroke(-1, 8, -1, 10, max1(s.u/2), sLow)
	// Face: soft eyes, Y nose, whisker dots.
	s.eye(-3, 2, false)
	s.eye(1, 2, false)
	s.dot(-1, 4) // nose
	s.c.Set(s.cx-4*s.u, s.cy+4*s.u, sUp)
	s.c.Set(s.cx+2*s.u, s.cy+4*s.u, sUp)
}

func spriteThornpod(s *spriteCtx) {
	// A hedgehog in profile, nosing left: a dome of thorny quills over a
	// pale face wedge that tapers to a pointed snout.
	// Quill dome: blades following the back's curve.
	s.blob(1, 3, 8, 6, sLow) // quill mass base
	for _, q := range [][3]int{{-4, -3, 4}, {-1, -5, 5}, {3, -4, 5}, {6, -1, 4}, {8, 2, 3}} {
		s.blade(q[0], q[1], 1, q[2], false, sMid)
	}
	// A couple of thorns flank low.
	s.blade(8, 6, 1, 3, true, sMid)
	// Face wedge + snout.
	s.patch(-5, 4, 4, 3, sUp)
	s.snout(-8, 4, 2, 4, -1, sUp)
	s.dot(-12, 4) // nose
	// Feet stubs.
	s.stroke(-4, 8, -4, 10, max1(s.u/2), sLow)
	s.stroke(1, 9, 1, 11, max1(s.u/2), sLow)
	s.stroke(5, 8, 5, 10, max1(s.u/2), sLow)
	s.eye(-6, 3, false)
}

func spriteMossmaw(s *spriteCtx) {
	// A bear: a humped mossy back, a big round head with round ears and a
	// short snout, sitting up on heavy forelegs — unmistakably ursine next
	// to Magmaw's long low hippo.
	// Humped body, sitting: tall at the shoulder.
	s.blob(3, 3, 6, 6, sMid)
	// Heavy forelegs + haunch.
	s.stroke(-1, 6, -1, 10, max1(s.u/2)*3, sLow)
	s.stroke(6, 7, 6, 10, max1(s.u/2)*3, sLow)
	s.patch(6, 6, 3, 3, sLow)
	// Moss on the hump.
	s.leaf(4, -4, 3, false)
	s.leaf(7, -2, 2, true)
	s.speckle(4, 2, 4, 3, 1)
	// Big round head with round outlined ears.
	for _, e := range [][2]int{{-7, -8}, {-1, -9}} {
		s.c.FillCircle(s.cx+e[0]*s.u, s.cy+e[1]*s.u, s.u+s.u/2+1, sOutline)
		s.c.FillCircle(s.cx+e[0]*s.u, s.cy+e[1]*s.u, s.u+s.u/2, sMid)
	}
	s.blob(-4, -4, 4, 4, sMid)
	// Short snout: a small pale muzzle with a big nose, jaw slightly open.
	s.patch(-6, -2, 2, 1, sUp)
	s.dot(-8, -3)
	s.stroke(-7, -1, -4, 0, max1(s.u/3), sOutline)
	s.c.Set(s.cx-6*s.u, s.cy-1*s.u+1, sGlint) // one tooth
	s.eye(-6, -6, true)
	s.eye(-2, -6, true)
}

func spriteFernling(s *spriteCtx) {
	// A songbird perched on stick legs: round head and breast, a wing of
	// feathers, a fern-frond tail fanning behind and a tiny beak.
	// Frond tail: three leaves fanning up-right.
	s.stroke(4, 2, 8, -1, max1(s.u/2), sLow)
	s.leaf(8, -2, 3, true)
	s.stroke(4, 3, 9, 3, max1(s.u/2), sLow)
	s.leaf(9, 2, 3, true)
	s.stroke(4, 4, 8, 6, max1(s.u/2), sLow)
	s.leaf(9, 6, 2, true)
	// Body + head (one soft pear).
	s.blob(0, 3, 4, 5, sMid)
	s.blob(-2, -4, 3, 3, sMid)
	// Breast.
	s.patch(-1, 4, 2, 3, sUp)
	// Wing: darker patch with feather lines.
	s.patch(2, 3, 2, 3, sLow)
	s.stroke(1, 2, 3, 4, max1(s.u/3), sOutline)
	s.stroke(1, 4, 3, 6, max1(s.u/3), sOutline)
	// Beak + crest leaf.
	s.snout(-5, -4, 1, 2, -1, sGlint)
	s.stroke(-2, -7, -1, -9, max1(s.u/3), sLow)
	s.leaf(0, -9, 2, true)
	// Stick legs.
	s.stroke(-1, 8, -1, 11, max1(s.u/3), sLow)
	s.stroke(1, 8, 1, 11, max1(s.u/3), sLow)
	s.stroke(-1, 11, -2, 11, max1(s.u/3), sLow)
	s.stroke(1, 11, 0, 11, max1(s.u/3), sLow)
	s.eye(-3, -5, false)
}

func spritePricklepup(s *spriteCtx) {
	// A puppy, three-quarter view: floppy ears, a blunt muzzle, a wagging
	// leaf-tipped tail and a bristle of cactus spines down its back.
	// Wagging tail with a leaf tip.
	s.stroke(6, 2, 9, -1, max1(s.u/2)*2, sMid)
	s.leaf(10, -2, 2, true)
	// Body.
	s.blob(2, 4, 6, 4, sMid)
	s.legs(8, 5, 3, 4)
	// Back spines.
	for _, dx := range []int{0, 3, 6} {
		s.blade(dx, -1, 1, 2, false, sLow)
	}
	// Head: round, floppy ears hanging at the sides.
	s.blob(-5, -3, 4, 4, sMid)
	s.blade(-9, -3, 1, 4, true, sLow) // floppy ears point DOWN
	s.blade(-1, -3, 1, 4, true, sLow)
	// Muzzle + nose + happy mouth.
	s.patch(-6, 0, 2, 2, sUp)
	s.dot(-8, -1)
	s.stroke(-8, 1, -6, 2, max1(s.u/3), sOutline)
	s.eye(-6, -4, false)
	s.eye(-3, -4, false)
}

func spriteBloomback(s *spriteCtx) {
	// A tortoise in profile, head out to the left: a high dome shell with
	// plate seams and a blossom growing from its crown.
	// Legs first (behind the shell rim): stout columns.
	s.stroke(-5, 8, -5, 11, max1(s.u/2)*3, sLow)
	s.stroke(-1, 9, -1, 11, max1(s.u/2)*3, sLow)
	s.stroke(3, 9, 3, 11, max1(s.u/2)*3, sLow)
	s.stroke(6, 8, 6, 11, max1(s.u/2)*3, sLow)
	// Head on a short neck, big enough to carry a face.
	s.stroke(-7, 5, -9, 4, max1(s.u/2)*3, sUp) // neck
	s.blob(-10, 3, 3, 3, sUp)
	s.eye(-11, 2, false)
	s.stroke(-13, 5, -11, 5, max1(s.u/3), sOutline) // mouth
	// Dome shell with plate seams.
	s.blob(1, 2, 8, 6, sMid)
	s.arc(1, 6, 6, 5, sDark)
	s.arc(1, 6, 3, 3, sDark)
	s.stroke(-3, 0, -5, 3, max1(s.u/3), sDark)
	s.stroke(5, 0, 7, 3, max1(s.u/3), sDark)
	// Shell rim.
	s.stroke(-7, 6, 9, 6, max1(s.u/3), sOutline)
	// The blossom on top.
	for _, a := range []float64{0, 1.05, 2.1, 3.14, 4.19, 5.24} {
		px := int(math.Round(math.Cos(a) * 2))
		py := int(math.Round(math.Sin(a)*1.5)) - 6
		s.dot2(1+px, py, sUp)
	}
	s.dot2(1, -6, sGlint)
}

// === Tide line ===

func spriteDripling(s *spriteCtx) {
	// A frog, front-on and crouched: eye bumps on top of a wide squat body,
	// a broad smile, splayed front feet and a dewdrop on its brow.
	// Hind haunches poking out the sides.
	s.patch(-7, 6, 2, 3, sLow)
	s.patch(7, 6, 2, 3, sLow)
	// Body: wide and squat.
	s.blob(0, 3, 7, 5, sMid)
	s.patch(0, 6, 4, 2, sUp) // pale belly
	// Eye bumps on top.
	s.c.FillCircle(s.cx-4*s.u, s.cy-3*s.u, 2*s.u+1, sOutline)
	s.c.FillCircle(s.cx-4*s.u, s.cy-3*s.u, 2*s.u, sMid)
	s.c.FillCircle(s.cx+4*s.u, s.cy-3*s.u, 2*s.u+1, sOutline)
	s.c.FillCircle(s.cx+4*s.u, s.cy-3*s.u, 2*s.u, sMid)
	s.ringEye(-4, -3, 2)
	s.ringEye(4, -3, 2)
	// The wide mouth.
	s.arc(0, 1, 5, -2, sOutline) // inverted arc = smile
	// Front feet splayed.
	s.patch(-4, 9, 2, 1, sUp)
	s.patch(4, 9, 2, 1, sUp)
	// Dewdrop on the brow.
	s.blade(0, -7, 1, 2, false, sGlint)
}

func spriteBrineback(s *spriteCtx) {
	// A crab: a wide flat carapace, two BIG claws held up front, stalk eyes
	// and three angled legs per side.
	// Legs: three per side, angled down-out.
	for i, l := range [][4]int{{-6, 4, -10, 8}, {-5, 5, -9, 10}, {-4, 6, -7, 11}} {
		_ = i
		s.stroke(l[0], l[1], l[2], l[3], max1(s.u/2), sLow)
		s.stroke(-l[0], l[1], -l[2], l[3], max1(s.u/2), sLow)
	}
	// Carapace: wide and flat, with a seam.
	s.blob(0, 3, 8, 4, sMid)
	s.arc(0, 5, 6, 3, sDark)
	// Claws: big outlined discs with a wedge notch.
	for _, side := range []int{-1, 1} {
		cxp, cyp := side*9, 0
		s.stroke(side*6, 3, cxp, cyp+1, max1(s.u/2)*2, sMid)
		s.c.FillCircle(s.cx+cxp*s.u, s.cy+cyp*s.u, 3*s.u+1, sOutline)
		s.c.FillCircle(s.cx+cxp*s.u, s.cy+cyp*s.u, 3*s.u, sUp)
		// the notch: a dark wedge opening outward
		s.triSidePx(s.cx+(cxp+side*3)*s.u, s.cy+cyp*s.u, s.u, 2*s.u, side, sOutline)
	}
	// Stalk eyes.
	s.stroke(-2, 1, -2, -2, max1(s.u/2), sMid)
	s.stroke(2, 1, 2, -2, max1(s.u/2), sMid)
	s.eye(-2, -3, false)
	s.eye(2, -3, false)
	// Bubbles.
	s.dot2(6, -3, sGlint)
}

func spriteTidecoil(s *spriteCtx) {
	// A seahorse in profile facing left: tube snout, coronet crest, a ridged
	// belly and a tail that curls under it — the coil.
	// Curled tail: spirals under the body.
	s.tailSeg([][2]int{{2, 5}, {4, 8}, {2, 10}, {-1, 9}, {0, 7}}, max1(s.u/2)*2, sMid)
	// Body: upright with a ridged belly.
	s.blob(0, 1, 4, 6, sMid)
	s.arc(-1, 3, 3, 2, sDark) // belly ridges
	s.arc(-1, 5, 3, 2, sDark)
	s.arc(-1, 1, 3, 2, sDark)
	// Dorsal fin on the back.
	s.blade(4, 0, 1, 3, false, sUp)
	// Head angled left with a tube snout.
	s.blob(-2, -6, 3, 3, sMid)
	s.stroke(-5, -6, -9, -5, max1(s.u/2)*2, sUp) // tube snout
	// Coronet crest.
	s.blade(-2, -9, 1, 2, false, sUp)
	s.blade(0, -10, 1, 2, false, sUp)
	s.eye(-3, -7, false)
}

func spriteGulper(s *spriteCtx) {
	// An anglerfish in profile facing left: a huge head that IS the body, a
	// gaping underbite jaw full of teeth, a glowing lure dangling ahead.
	// Tail fin (right).
	s.snout(8, 1, 1, 3, 1, sMid)
	s.blade(10, -1, 1, 3, false, sMid)
	s.blade(10, 3, 1, 3, true, sMid)
	// Body/head.
	s.blob(1, 1, 8, 6, sMid)
	// The gaping mouth: dark wedge opening left, teeth both rows.
	s.oval(s.cx-5*s.u, s.cy+3*s.u, 5*s.u, 3*s.u, sOutline)
	s.oval(s.cx-5*s.u, s.cy+3*s.u, 5*s.u-1, 3*s.u-1, sDark)
	s.teeth(-6, 1, 3, 4, true)
	s.teeth(-6, 6, 3, 3, false)
	// Lure: stalk arcing forward from the forehead, glowing bulb.
	s.tailSeg([][2]int{{0, -6}, {-4, -9}, {-8, -7}}, max1(s.u/2), sLow)
	s.c.FillCircle(s.cx-8*s.u, s.cy-6*s.u, max1(s.u/2)+1, sGlint)
	// Pectoral fin + speckle.
	s.blade(3, 8, 2, 3, true, sLow)
	s.speckle(3, -1, 4, 2, 3)
	s.eye(-4, -2, true)
}

func spriteFrostnip(s *spriteCtx) {
	// A penguin: upright egg, white belly, flipper wings, an orange-less
	// little beak and webbed feet — frost sparkles at its crown.
	// Body.
	s.blob(0, 2, 5, 8, sMid)
	// Belly: big pale oval.
	s.patch(0, 4, 3, 5, sTop)
	// Flippers.
	s.blade(-6, 1, 1, 5, true, sLow)
	s.blade(6, 1, 1, 5, true, sLow)
	// Face: eyes high on the dark hood, beak between.
	s.eye(-2, -3, false)
	s.eye(2, -3, false)
	s.triAt(s.cx, s.cy-1*s.u, s.u, s.u+1, true, sUp) // beak
	// Frost crystals at the crown.
	s.blade(0, -8, 1, 2, false, sGlint)
	s.dot2(-3, -8, sGlint)
	s.dot2(3, -8, sGlint)
	// Webbed feet.
	s.patch(-2, 10, 2, 1, sUp)
	s.patch(2, 10, 2, 1, sUp)
}

func spriteAnchora(s *spriteCtx) {
	// An octopus: a tall dome mantle with big ring eyes and four curling
	// tentacles — an anchor emblem marks the dome.
	// Tentacles: curls spreading from under the mantle.
	s.tailSeg([][2]int{{-5, 4}, {-8, 7}, {-10, 6}}, max1(s.u/2)*2, sMid)
	s.tailSeg([][2]int{{-2, 5}, {-3, 9}, {-5, 10}}, max1(s.u/2)*2, sMid)
	s.tailSeg([][2]int{{2, 5}, {3, 9}, {5, 10}}, max1(s.u/2)*2, sMid)
	s.tailSeg([][2]int{{5, 4}, {8, 7}, {10, 6}}, max1(s.u/2)*2, sMid)
	// Curl tips.
	s.dot2(-10, 5, sUp)
	s.dot2(10, 5, sUp)
	// Mantle: tall dome.
	s.blob(0, -1, 6, 6, sMid)
	// Anchor emblem on the dome.
	s.stroke(0, -6, 0, -1, max1(s.u/3), sUp)
	s.stroke(-2, -5, 2, -5, max1(s.u/3), sUp)
	s.stroke(0, -1, -2, -3, max1(s.u/3), sUp)
	s.stroke(0, -1, 2, -3, max1(s.u/3), sUp)
	// Big ring eyes low on the mantle.
	s.ringEye(-3, 2, 2)
	s.ringEye(3, 2, 2)
	// A little siphon mouth.
	s.dot(0, 4)
}

// dot2 sets a small filled disc at a design offset (a petal, spark or rivet).
func (s *spriteCtx) dot2(ox, oy int, tone canvas.Color) {
	s.c.FillCircle(s.cx+ox*s.u, s.cy+oy*s.u, max1(s.u/2), tone)
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
