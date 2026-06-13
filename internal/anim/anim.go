// Package anim wraps harmonica springs for the camera and provides small
// deterministic helpers for ambient idle animations (lamp flicker, water
// phase). Nothing here uses wall-clock randomness; everything is a pure
// function of time so renders stay reproducible.
package anim

import (
	"math"

	"github.com/charmbracelet/harmonica"
)

// Camera smooth-follows a target point using a critically-damped-ish
// spring per axis, giving the view a slight, pleasant lag.
type Camera struct {
	X, Y   float64 // current position
	vx, vy float64
	spring harmonica.Spring
}

// NewCamera creates a camera tuned for the plaza's 30 FPS update tick,
// starting snapped to (x, y).
func NewCamera(x, y float64) *Camera {
	return &Camera{
		X: x, Y: y,
		// Angular frequency 5.5 and damping 1.0 settle quickly with no
		// overshoot — a follow-cam, not a bouncy one.
		spring: harmonica.NewSpring(harmonica.FPS(30), 5.5, 1.0),
	}
}

// Update advances the spring one tick toward the target.
func (c *Camera) Update(targetX, targetY float64) {
	c.X, c.vx = c.spring.Update(c.X, c.vx, targetX)
	c.Y, c.vy = c.spring.Update(c.Y, c.vy, targetY)
}

// Snap teleports the camera to (x, y) and kills velocity. Used on join and
// when returning from a portal world.
func (c *Camera) Snap(x, y float64) {
	c.X, c.Y = x, y
	c.vx, c.vy = 0, 0
}

// Flicker returns a brightness multiplier in [0.7, 1.0] for time t and an
// element seed, layering two incommensurate sine waves so the result feels
// organic rather than strobing.
func Flicker(t float64, seed int) float64 {
	s := float64(seed)
	v := 0.5*math.Sin(t*7.3+s*1.7) + 0.5*math.Sin(t*3.1+s*4.2)
	// v is in [-1, 1]; map to [0.7, 1.0].
	return 0.85 + 0.15*v*0.5
}

// Phase returns an integer animation phase in [0, n) for time t advancing
// at `rate` phases per second, offset by seed. Used to pick water ripple
// glyphs and cloud drift offsets.
func Phase(t float64, rate float64, n int, seed int) int {
	if n <= 0 {
		return 0
	}
	p := int(t*rate) + seed
	p %= n
	if p < 0 {
		p += n
	}
	return p
}

// Scale multiplies a 0xRRGGBB grey/color by k (clamped to [0,1]),
// darkening it. Handy for flicker on monochrome decorations.
func Scale(rgb uint32, k float64) uint32 {
	if k < 0 {
		k = 0
	}
	if k > 1 {
		k = 1
	}
	r := uint32(float64(rgb>>16&0xFF) * k)
	g := uint32(float64(rgb>>8&0xFF) * k)
	b := uint32(float64(rgb&0xFF) * k)
	return r<<16 | g<<8 | b
}
