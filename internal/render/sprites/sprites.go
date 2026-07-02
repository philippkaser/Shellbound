// Package sprites draws Shellbound's avatars as procedural pixel art directly
// into the canvas. The figure is built to sit in the isometric world: it is
// lit from above like the tiles and cubes (brightest on the head and shoulders,
// darkening toward the ground), has rounded shoulders and stocky, grounded
// proportions, and pairs with the soft iso contact shadow the renderer drops at
// its feet. The four facings plus a two-step walk cycle fall out of small
// parameter changes.
//
// Avatars are strictly monochrome — a grey volume. A player's identity color
// lives only in their name tag, keeping the world's three splashes of color
// (names, chat, portals) intact.
package sprites

import (
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// Facing is a 4-way direction, ordered to match hub.Dir.
type Facing int

// Facing values.
const (
	FaceDown Facing = iota
	FaceUp
	FaceLeft
	FaceRight
)

// Sprite extents in pixels. Height is measured up from the feet; callers place
// name tags Height+gap above the foot point.
const (
	Width  = 18
	Height = 36
)

// Overhead-lit grey ramp: top surfaces catch the light, lower ones fall into
// shadow — the same logic that shades the iso cubes, so the avatar reads as a
// volume in the scene rather than a flat billboard.
const (
	cTop  = canvas.Color(0xF0F0F0)
	cUp   = canvas.Color(0xCCCCCC)
	cMid  = canvas.Color(0xA2A2A2)
	cLow  = canvas.Color(0x7C7C7C)
	cDark = canvas.Color(0x4A4A4A) // features / belt
)

// vramp picks a tone for a vertical position v in [0,1] (0 = top, lit).
func vramp(v float64) canvas.Color {
	switch {
	case v < 0.20:
		return cTop
	case v < 0.46:
		return cUp
	case v < 0.74:
		return cMid
	default:
		return cLow
	}
}

// darker steps a tone one notch down the ramp, for the shadowed (left) side.
func darker(c canvas.Color) canvas.Color {
	switch c {
	case cTop:
		return cUp
	case cUp:
		return cMid
	case cMid:
		return cLow
	default:
		return cDark
	}
}

// limb draws a vertical limb (leg or arm) from topY to botY, top-lit, with the
// shadow side optionally one notch darker.
func limb(c *canvas.Canvas, cx, topY, botY, halfW int, leftShade bool) {
	if botY <= topY {
		botY = topY + 1
	}
	for y := topY; y < botY; y++ {
		v := float64(y-topY) / float64(botY-topY)
		base := vramp(0.45 + 0.5*v) // limbs are lower-body tones
		for dx := -halfW; dx <= halfW; dx++ {
			col := base
			if leftShade && dx < 0 {
				col = darker(base)
			}
			c.Set(cx+dx, y, col)
		}
	}
}

// Body proportions in pixels (feet at the origin). Promoted to package scope so
// cosmetics can be placed relative to the head.
const (
	legH   = 10
	torsoH = 14
	neckH  = 2
	headR  = 6
)

// HeadCenter returns the pixel center of the avatar's head for feet at
// (footX, footY); cosmetics (hats, halos) are drawn relative to it.
func HeadCenter(footX, footY int) (int, int) {
	return footX, footY - legH - torsoH - neckH - headR
}

// HeadRadius returns the head radius in pixels.
func HeadRadius() int { return headR }

// Pose picks the avatar's animation frame and vertical bob at time t: a walk
// frame while moving, otherwise a gentle idle breathing bob. phase offsets the
// idle bob so a crowd of avatars doesn't breathe in unison. It's the single
// source of avatar timing shared by the plaza and the Shellmon route.
//
// The walk is a 4-phase cycle (contact, pass, contact, pass) and the body
// dips one pixel on the passing frames, so a walking figure has a real gait
// instead of two alternating stills.
func Pose(t float64, moving bool, phase float64) (frame, bob int) {
	if moving {
		frame = int(t * 10)
		if frame%2 == 1 {
			bob = 1 // body dips as the legs pass each other
		}
		return frame, bob
	}
	return 0, int(math.Round(math.Sin(t*2.2+phase) * 0.8))
}

// Draw paints an avatar whose feet rest at pixel (footX, footY). frame selects
// the walk pose; facing orients the head.
func Draw(c *canvas.Canvas, footX, footY int, f Facing, frame int, moving bool) {
	const (
		shHalf = 8 // shoulder half-width
		waHalf = 5 // waist half-width
	)
	hipY := footY - legH
	shoulderY := hipY - torsoH
	neckY := shoulderY - neckH
	headCY := neckY - headR

	// Legs: a 4-phase gait. On the contact frames one leg is lifted high and
	// the other planted; on the passing frames both are nearly under the body
	// with a slight lift, so the cycle reads contact → pass → contact → pass
	// instead of flicking between two stills.
	lFoot, rFoot := footY, footY
	if moving {
		switch frame % 4 {
		case 0:
			lFoot = footY - 4
		case 1:
			lFoot, rFoot = footY-1, footY-1
		case 2:
			rFoot = footY - 4
		case 3:
			lFoot, rFoot = footY-1, footY-1
		}
	}
	limb(c, footX-3, hipY, lFoot, 2, true)
	limb(c, footX+3, hipY, rFoot, 2, false)

	// Arms swing opposite the legs, hanging from the shoulders, easing
	// through the passing frames.
	armTop := shoulderY + 1
	lH, rH := armTop+11, armTop+11
	if moving {
		switch frame % 4 {
		case 0:
			lH -= 3
			rH += 1
		case 2:
			lH += 1
			rH -= 3
		default: // passing frames: arms nearly neutral
			lH -= 1
			rH -= 1
		}
	}
	limb(c, footX-shHalf, armTop, lH, 1, true)
	limb(c, footX+shHalf, armTop, rH, 1, false)

	// Torso: a rounded trapezoid (wide sloped shoulders tapering to the waist),
	// shaded top-bright to bottom-dark, with the left side a notch darker.
	for y := shoulderY; y < hipY; y++ {
		v := float64(y-shoulderY) / float64(torsoH)
		half := int(float64(shHalf) - float64(shHalf-waHalf)*v + 0.5)
		base := vramp(v)
		for dx := -half; dx <= half; dx++ {
			col := base
			if dx < -half/3 {
				col = darker(base)
			} else if dx > half/2 && v < 0.35 {
				col = cTop // shoulder catch-light
			}
			c.Set(footX+dx, y, col)
		}
	}
	c.HLine(footX-waHalf, footX+waHalf, hipY-1, cDark) // belt

	// Neck.
	c.FillRect(footX-2, neckY, 4, neckH, cMid)

	// Head: a dome, top-lit, shaded on the lower-left.
	for dy := -headR; dy <= headR; dy++ {
		w := int(math.Sqrt(float64(headR*headR - dy*dy)))
		v := float64(dy+headR) / float64(2*headR)
		base := vramp(v * 0.85)
		for dx := -w; dx <= w; dx++ {
			col := base
			if dx < -w/3 && v > 0.3 {
				col = darker(base)
			}
			c.Set(footX+dx, headCY+dy, col)
		}
	}
	// Hood: a darker cowl outlining the top arc of the head and pooling at
	// the shoulders, so the figure reads as the hooded wanderer of the plaza
	// rather than a bare dome.
	for dy := -headR; dy <= 0; dy++ {
		w := int(math.Sqrt(float64(headR*headR - dy*dy)))
		c.Set(footX-w-1, headCY+dy, cLow)
		c.Set(footX+w+1, headCY+dy, cLow)
	}
	c.HLine(footX-1, footX+1, headCY-headR-1, cLow) // hood peak
	c.HLine(footX-4, footX-3, neckY, cLow)          // cowl folds at the shoulders
	c.HLine(footX+3, footX+4, neckY, cLow)
	c.HLine(footX-2, footX+2, headCY-headR+1, cTop) // crown catch-light

	// Eyes: two, shifted toward the facing; the back of the head has none.
	eyeY := headCY + 1
	eye := func(ex int) { c.Set(footX+ex, eyeY, cDark); c.Set(footX+ex, eyeY+1, cDark) }
	switch f {
	case FaceDown:
		eye(-2)
		eye(2)
	case FaceUp:
		c.FillCircle(footX, headCY-1, 3, cMid) // darker crown, no eyes
	case FaceLeft:
		eye(-3)
		eye(1)
	case FaceRight:
		eye(-1)
		eye(3)
	}
}
