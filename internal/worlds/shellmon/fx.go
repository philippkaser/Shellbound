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

// fxID is the identity an hpFX tracks: the team slot plus species. Display
// names alone collide for duplicate unnicknamed creatures of one species.
func fxID(slot int, c *mon.Creature) string {
	return itoa(slot) + ":" + c.Species
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

// ghostHold is how long the just-lost HP chunk lingers before draining, and
// ghostEase how fast it then eases toward the real value per frame.
const (
	ghostHold = 0.35
	ghostEase = 0.18
)

// sync advances the easing one frame for the active creature identified by id
// (resetting cleanly when a switch swaps in a different creature). The solid
// bar snaps to the real HP; `shown` is the trailing ghost value that lingers
// for a beat after a hit, then drains.
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
	if float64(real) >= f.shown || time.Since(f.hurtAt).Seconds() > ghostHold {
		f.shown += (float64(real) - f.shown) * ghostEase
	}
	if real <= 0 && !f.fainted {
		f.fainted, f.sinkAt = true, time.Now()
	} else if real > 0 {
		f.fainted = false
	}
}

// realHP is the solid bar value (the creature's actual HP).
func (f *hpFX) realHP() int { return f.lastHP }

// shownHP is the trailing ghost value for the bar's damage trail.
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
		drawImpact(c, foeX, foeY, impact, a.youType)
	}
	if a.foeCast && impact >= 0 {
		drawImpact(c, youX, youY, impact, a.foeType)
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

// drawImpact bursts a hit effect at the target, fading out. Each element has
// its own silhouette so Spark, Tide and Bramble hits read differently even
// in a glance: bolts, ripples, or a spray of leaves. The white core flash is
// shared; the type colour carries the shape (the sanctioned colour pop).
func drawImpact(c *canvas.Canvas, cx, cy int, p float64, typ mon.Type) {
	if p < 0 || p > 1 {
		return
	}
	col := typeColor(typ)
	k := 1 - p
	// Early white core flash, shared by every element.
	if p < 0.25 {
		r := int(2 + p*36)
		add(c, cx, cy, r, fxBright.Scale(0.5*(1-p/0.25)))
	}
	switch typ {
	case mon.Spark:
		// Jagged bolts crackling outward: short polylines with hard elbows.
		for i := 0; i < 5; i++ {
			ang := float64(i)/5*2*math.Pi + 0.55
			d0 := 4 + p*20
			d1 := d0 + 10 + p*16
			x0 := cx + int(math.Cos(ang)*d0)
			y0 := cy + int(math.Sin(ang)*d0*0.6)
			// Elbow: kink the bolt sideways halfway along.
			mx := (x0 + cx + int(math.Cos(ang)*d1)) / 2
			my := (y0+cy+int(math.Sin(ang)*d1*0.6))/2 + boltKink(i)
			x1 := cx + int(math.Cos(ang)*d1)
			y1 := cy + int(math.Sin(ang)*d1*0.6)
			addLine(c, x0, y0, mx, my, col.Scale(0.95*k))
			addLine(c, mx, my, x1, y1, fxBright.Scale(0.8*k))
		}
	case mon.Tide:
		// Concentric 2:1 ripples widening like a splash.
		for ring := 0; ring < 3; ring++ {
			rp := p - float64(ring)*0.16
			if rp < 0 {
				continue
			}
			rad := 6 + rp*46
			fade := (1 - rp) * (1 - float64(ring)*0.25)
			const steps = 30
			for s := 0; s < steps; s++ {
				ang := float64(s) / steps * 2 * math.Pi
				x := cx + int(math.Cos(ang)*rad)
				y := cy + int(math.Sin(ang)*rad*0.45)
				set(c, x, y, col.Scale(0.95*fade))
			}
		}
		// A few droplets thrown upward.
		for i := 0; i < 4; i++ {
			dx := (i*2 - 3) * 6
			dy := -int(p*26) + i*3
			set(c, cx+dx, cy+dy, fxBright.Scale(0.7*k))
		}
	case mon.Bramble:
		// A spray of leaf chevrons tumbling outward.
		for i := 0; i < 9; i++ {
			ang := float64(i)/9*2*math.Pi + 0.2
			d := (8 + p*38) * (0.7 + 0.3*float64(i%3)/2)
			x := cx + int(math.Cos(ang)*d)
			y := cy + int(math.Sin(ang)*d*0.6) + int(p*8) // leaves settle downward
			// A 3px "V" chevron oriented by particle index.
			set(c, x, y, col.Scale(0.95*k))
			if i%2 == 0 {
				set(c, x-1, y-1, col.Scale(0.8*k))
				set(c, x+1, y-1, fxBright.Scale(0.5*k))
			} else {
				set(c, x-1, y+1, fxBright.Scale(0.5*k))
				set(c, x+1, y-1, col.Scale(0.8*k))
			}
		}
	default:
		// Plain: the classic expanding ring plus radial sparks.
		rad := 6 + p*46
		const steps = 28
		for s := 0; s < steps; s++ {
			ang := float64(s) / steps * 2 * math.Pi
			x := cx + int(math.Cos(ang)*rad)
			y := cy + int(math.Sin(ang)*rad*0.6)
			set(c, x, y, col.Scale(0.95*k))
		}
		for i := 0; i < 8; i++ {
			ang := float64(i)/8*2*math.Pi + 0.4
			d := rad * (0.5 + 0.5*p)
			x := cx + int(math.Cos(ang)*d)
			y := cy + int(math.Sin(ang)*d*0.6)
			set(c, x, y, fxBright.Scale(k))
			set(c, x, y-1, col.Scale(0.8*k))
		}
	}
}

// boltKink offsets a lightning elbow so the five bolts kink differently.
func boltKink(i int) int { return (i%3 - 1) * 4 }

// addLine draws an additive 1px line (for bolts).
func addLine(c *canvas.Canvas, x0, y0, x1, y1 int, col canvas.Color) {
	dx, dy := x1-x0, y1-y0
	steps := absInt(dx)
	if absInt(dy) > steps {
		steps = absInt(dy)
	}
	if steps == 0 {
		set(c, x0, y0, col)
		return
	}
	for s := 0; s <= steps; s++ {
		t := float64(s) / float64(steps)
		set(c, x0+int(float64(dx)*t+0.5), y0+int(float64(dy)*t+0.5), col)
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
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
