package shellmon

import (
	"math"
	"time"

	"github.com/shellbound/shellbound/internal/render/canvas"
	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// Effect tones (bright, for additive glints over the dim battle backdrop).
const (
	fxBright = canvas.Color(0xF8F8F8)
	fxMid    = canvas.Color(0xC0C0C0)
)

// typeColor is a Shellmon type's signature hue — used sparingly as the only
// colour pops on the otherwise black-and-white battle stage (creature names and
// attack effects), the same discipline the plaza uses for names and portals.
func typeColor(t mon.Type) canvas.Color {
	switch t {
	case mon.Spark:
		return canvas.RGB(0xE6, 0xA0, 0x3C) // ember amber
	case mon.Tide:
		return canvas.RGB(0x52, 0xA6, 0xE6) // sea blue
	case mon.Bramble:
		return canvas.RGB(0x6F, 0xC4, 0x5A) // leaf green
	default:
		return canvas.RGB(0xD0, 0xD0, 0xD0) // plain: neutral
	}
}

// hpFX eases a combatant's displayed HP toward its true value and tracks hit
// reactions (shake) and fainting (sink), so bars tick and bodies flinch.
type hpFX struct {
	id       string
	shown    float64
	lastHP   int
	hurtAt   time.Time
	sinkAt   time.Time
	appearAt time.Time
	fainted  bool
	init     bool
}

// sync advances the easing one frame for the active creature identified by id
// (resetting cleanly when a switch swaps in a different creature).
func (f *hpFX) sync(id string, real, max int) {
	if !f.init || id != f.id {
		f.id, f.shown, f.lastHP, f.init = id, float64(real), real, true
		f.fainted = real <= 0
		f.appearAt, f.sinkAt = time.Now(), time.Time{}
		return
	}
	if real < f.lastHP {
		f.hurtAt = time.Now()
	}
	f.lastHP = real
	f.shown += (float64(real) - f.shown) * 0.3
	if real <= 0 && !f.fainted {
		f.fainted, f.sinkAt = true, time.Now()
	} else if real > 0 {
		f.fainted = false
	}
}

// shownHP is the eased bar value.
func (f *hpFX) shownHP() int { return int(f.shown + 0.5) }

// shakeX is a quick decaying horizontal jitter right after taking damage.
func (f *hpFX) shakeX() int {
	el := time.Since(f.hurtAt).Seconds()
	if f.hurtAt.IsZero() || el > 0.3 {
		return 0
	}
	return int(math.Sin(el*70) * 4 * (1 - el/0.3))
}

// sinkY drops a fainted creature into the ground as it disappears.
func (f *hpFX) sinkY() int {
	if !f.fainted || f.sinkAt.IsZero() {
		return 0
	}
	el := time.Since(f.sinkAt).Seconds()
	if el > 0.6 {
		return 40
	}
	return int(el / 0.6 * 40)
}

// gone reports whether a fainted creature has fully sunk (skip drawing it).
func (f *hpFX) gone() bool {
	return f.fainted && !f.sinkAt.IsZero() && time.Since(f.sinkAt).Seconds() > 0.6
}

// appearP is the send-out progress (0..1 over ~0.35s, then 1) used to flash a
// creature in as it takes the field.
func (f *hpFX) appearP() float64 {
	if f.appearAt.IsZero() {
		return 1
	}
	el := time.Since(f.appearAt).Seconds()
	if el >= 0.35 {
		return 1
	}
	return el / 0.35
}

// koP is the faint progress (0..1 over 0.6s) for the KO burst, or -1 if not
// currently fainting.
func (f *hpFX) koP() float64 {
	if !f.fainted || f.sinkAt.IsZero() {
		return -1
	}
	el := time.Since(f.sinkAt).Seconds()
	if el > 0.6 {
		return -1
	}
	return el / 0.6
}

// drawSendFlash bursts a bright ring where a creature takes the field.
func drawSendFlash(c *canvas.Canvas, cx, cy int, p float64, col canvas.Color) {
	k := 1 - p
	add(c, cx, cy, int(6+14*k), fxBright.Scale(0.5*k))
	rad := 8 + p*30
	const steps = 24
	for s := 0; s < steps; s++ {
		ang := float64(s) / steps * 2 * math.Pi
		x := cx + int(math.Cos(ang)*rad)
		y := cy + int(math.Sin(ang)*rad*0.6)
		set(c, x, y, col.Scale(0.9*k))
	}
}

// drawKOBurst puffs motes up and outward as a fainted creature drops.
func drawKOBurst(c *canvas.Canvas, cx, cy int, p float64, col canvas.Color) {
	k := 1 - p
	for i := 0; i < 10; i++ {
		ang := float64(i)/10*2*math.Pi + 0.3
		d := 6 + p*30
		x := cx + int(math.Cos(ang)*d)
		y := cy + int(math.Sin(ang)*d*0.5) - int(p*10) // drift upward
		set(c, x, y, fxBright.Scale(k))
		set(c, x, y+1, col.Scale(0.7*k))
	}
}

// battleAnim sequences the per-turn move effects: when the turn counter ticks,
// it records which move type each side threw and replays casts + impacts.
type battleAnim struct {
	seq     int
	start   time.Time
	youType mon.Type
	foeType mon.Type
	youCast bool // you threw a damaging-ish move worth animating
	foeCast bool
}

const animDur = 0.6 // seconds a turn's effects play

// trigger starts a fresh animation when seq advances.
func (a *battleAnim) trigger(seq int, youType mon.Type, youCast bool, foeType mon.Type, foeCast bool) {
	if seq == a.seq {
		return
	}
	a.seq = seq
	a.start = time.Now()
	a.youType, a.youCast = youType, youCast
	a.foeType, a.foeCast = foeType, foeCast
}

// draw plays the active turn's effects over the two combatant centers.
func (a *battleAnim) draw(c *canvas.Canvas, youX, youY, foeX, foeY int) {
	if a.start.IsZero() {
		return
	}
	el := time.Since(a.start).Seconds()
	if el > animDur {
		return
	}
	// Casts travel out over the first ~0.3s; impacts land 0.22..0.6s.
	castP := el / 0.32
	if a.youCast && castP <= 1 {
		drawCast(c, youX, youY, foeX, foeY, a.youType, castP)
	}
	if a.foeCast && castP <= 1 {
		drawCast(c, foeX, foeY, youX, youY, a.foeType, castP)
	}
	impact := (el - 0.22) / (animDur - 0.22)
	if a.youCast && impact >= 0 {
		drawImpact(c, foeX, foeY, impact, typeColor(a.youType))
	}
	if a.foeCast && impact >= 0 {
		drawImpact(c, youX, youY, impact, typeColor(a.foeType))
	}
}

// drawCast streams a few type-styled motes from a source toward a target.
func drawCast(c *canvas.Canvas, fx, fy, tx, ty int, typ mon.Type, prog float64) {
	for i := 0; i < 7; i++ {
		p := prog - float64(i)*0.05
		if p < 0 || p > 1 {
			continue
		}
		jx := int(math.Sin(p*9+float64(i)) * 5)
		jy := int(math.Cos(p*7+float64(i)*2) * 4)
		x := lerpI(fx, tx, p) + jx
		y := lerpI(fy, ty, p) + jy
		castMote(c, x, y, typ, i)
	}
}

// castMote draws one particle: a bright white core with a type-coloured trail,
// shaped by element (sparks flicker, droplets fall, leaves dash).
func castMote(c *canvas.Canvas, x, y int, typ mon.Type, i int) {
	col := typeColor(typ)
	c.Set(x, y, fxBright)
	switch typ {
	case mon.Spark:
		c.Set(x+1, y, col)
		c.Set(x, y-1, col)
	case mon.Tide:
		c.Set(x, y+1, col)
		c.Set(x, y+2, col)
	case mon.Bramble:
		c.Set(x+1, y-1, col)
		c.Set(x-1, y+1, col)
	default:
		c.Set(x+1, y, col)
	}
}

// drawImpact bursts an expanding ring and radial sparks at a hit, fading out.
// The ring carries the move's type colour (the pop); core and sparks stay white.
func drawImpact(c *canvas.Canvas, cx, cy int, p float64, col canvas.Color) {
	if p < 0 || p > 1 {
		return
	}
	k := 1 - p
	// Early white core flash.
	if p < 0.25 {
		r := int(2 + p*36)
		add(c, cx, cy, r, fxBright.Scale(0.5*(1-p/0.25)))
	}
	// Expanding 2:1 ring, tinted by element.
	rad := 6 + p*46
	const steps = 28
	for s := 0; s < steps; s++ {
		ang := float64(s) / steps * 2 * math.Pi
		x := cx + int(math.Cos(ang)*rad)
		y := cy + int(math.Sin(ang)*rad*0.6)
		set(c, x, y, col.Scale(0.95*k))
	}
	// Radial sparks shooting outward (white core, coloured trail).
	for i := 0; i < 8; i++ {
		ang := float64(i)/8*2*math.Pi + 0.4
		d := rad * (0.5 + 0.5*p)
		x := cx + int(math.Cos(ang)*d)
		y := cy + int(math.Sin(ang)*d*0.6)
		set(c, x, y, fxBright.Scale(k))
		set(c, x, y-1, col.Scale(0.8*k))
	}
}

// add lightens a filled disc additively (a soft glow).
func add(c *canvas.Canvas, cx, cy, r int, col canvas.Color) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy > r*r {
				continue
			}
			c.Set(cx+dx, cy+dy, c.At(cx+dx, cy+dy).Lighten(col))
		}
	}
}

func set(c *canvas.Canvas, x, y int, col canvas.Color) {
	c.Set(x, y, c.At(x, y).Lighten(col))
}

func lerpI(a, b int, t float64) int { return a + int(float64(b-a)*t) }

// moveType returns the elemental type of a creature's move index and whether it
// is worth animating a cast for (a damaging move).
func moveType(c *mon.Creature, idx int) (mon.Type, bool) {
	if idx < 0 || idx >= len(c.Moves) {
		return mon.Plain, false
	}
	mv, ok := mon.MoveByKey(c.Moves[idx])
	if !ok {
		return mon.Plain, false
	}
	return mv.Type, mv.Power > 0
}
