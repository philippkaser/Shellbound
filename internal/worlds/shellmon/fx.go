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

// hpFX eases a combatant's displayed HP toward its true value and tracks hit
// reactions (shake) and fainting (sink), so bars tick and bodies flinch.
type hpFX struct {
	id      string
	shown   float64
	lastHP  int
	hurtAt  time.Time
	sinkAt  time.Time
	fainted bool
	init    bool
}

// sync advances the easing one frame for the active creature identified by id
// (resetting cleanly when a switch swaps in a different creature).
func (f *hpFX) sync(id string, real, max int) {
	if !f.init || id != f.id {
		f.id, f.shown, f.lastHP, f.init = id, float64(real), real, true
		f.fainted = real <= 0
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
		drawImpact(c, foeX, foeY, impact)
	}
	if a.foeCast && impact >= 0 {
		drawImpact(c, youX, youY, impact)
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

// castMote draws one particle shaped by elemental type (monochrome, so the
// shape carries the identity: sparks flicker, droplets fall, leaves dash).
func castMote(c *canvas.Canvas, x, y int, typ mon.Type, i int) {
	switch typ {
	case mon.Spark:
		c.Set(x, y, fxBright)
		c.Set(x+1, y, fxMid)
		c.Set(x, y-1, fxMid)
	case mon.Tide:
		c.Set(x, y, fxBright)
		c.Set(x, y+1, fxMid)
		c.Set(x, y+2, fxMid)
	case mon.Bramble:
		c.Set(x, y, fxBright)
		c.Set(x+1, y-1, fxMid)
		c.Set(x-1, y+1, fxMid)
	default:
		c.Set(x, y, fxBright)
		c.Set(x+1, y, fxMid)
	}
}

// drawImpact bursts an expanding ring and radial sparks at a hit, fading out.
func drawImpact(c *canvas.Canvas, cx, cy int, p float64) {
	if p < 0 || p > 1 {
		return
	}
	k := 1 - p
	// Early core flash.
	if p < 0.25 {
		r := int(2 + p*36)
		add(c, cx, cy, r, fxBright.Scale(0.5*(1-p/0.25)))
	}
	// Expanding 2:1 ring.
	rad := 6 + p*46
	const steps = 28
	for s := 0; s < steps; s++ {
		ang := float64(s) / steps * 2 * math.Pi
		x := cx + int(math.Cos(ang)*rad)
		y := cy + int(math.Sin(ang)*rad*0.6)
		set(c, x, y, fxBright.Scale(0.8*k))
	}
	// Radial sparks shooting outward.
	for i := 0; i < 8; i++ {
		ang := float64(i)/8*2*math.Pi + 0.4
		d := rad * (0.5 + 0.5*p)
		x := cx + int(math.Cos(ang)*d)
		y := cy + int(math.Sin(ang)*d*0.6)
		set(c, x, y, fxBright.Scale(k))
		set(c, x, y-1, fxMid.Scale(k))
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
