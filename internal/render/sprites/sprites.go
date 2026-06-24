// Package sprites draws Shellbound's avatars as higher-resolution pixel art
// directly into the canvas. The figure is built procedurally from canvas
// primitives (head disc, torso, legs) rather than baked masks: it reads
// cleanly at the isometric scale, and the four facings plus a two-step walk
// cycle fall out of small parameter changes.
//
// Avatars are strictly monochrome — white body with grey shading. A player's
// identity color lives only in their name tag, keeping the world's three
// splashes of color (names, chat, portals) intact.
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

// Sprite extents in pixels. Height is measured up from the feet; callers
// place name tags Height+gap above the foot point.
const (
	Width  = 20
	Height = 42
)

// Monochrome shades for the avatar's volume.
const (
	body  = canvas.Color(0xF2F2F2) // lit white
	shade = canvas.Color(0xA1A1A1) // mid grey (shadowed side)
	dark  = canvas.Color(0x404040) // facial features / outline
)

// Draw paints an avatar whose feet rest at pixel (footX, footY). frame selects
// the walk pose; facing orients the head. The figure has swinging arms, a
// little hooded cloak and soft shading so it reads as a character, not a blob.
func Draw(c *canvas.Canvas, footX, footY int, f Facing, frame int, moving bool) {
	const (
		legH   = 13
		legW   = 5
		torsoH = 16
		torsoW = 14
		headR  = 6
		armW   = 4
		armH   = 12
		cloakW = 18
		cloakH = 9
	)
	hipY := footY - legH
	shoulderY := hipY - torsoH
	headCY := shoulderY - headR
	tx := footX - torsoW/2

	// A short cloak flares behind the hips for silhouette (drawn first, behind).
	cloakTone := shade
	for dy := 0; dy < cloakH; dy++ {
		w := cloakW/2 + dy/3
		c.HLine(footX-w, footX+w, hipY-2+dy, cloakTone)
	}
	c.HLine(footX-cloakW/2, footX+cloakW/2, hipY-2, dark) // hem shadow at the top

	// Legs hang from a fixed hip; walking lifts a foot clear of the ground and
	// sets it back down, alternating each step.
	lx, rx := footX-4, footX+4
	lFoot, rFoot := footY, footY
	if moving {
		if frame%2 == 0 {
			lFoot = footY - 5
		} else {
			rFoot = footY - 5
		}
	}
	c.FillRect(lx-legW/2, hipY, legW, lFoot-hipY, body)
	c.FillRect(rx-legW/2, hipY, legW, rFoot-hipY, body)
	c.VLine(lx-legW/2, hipY, lFoot-1, shade) // volume on the shadow side

	// Arms swing opposite the legs: the lower end (hand) lifts and drops.
	armTop := shoulderY + 2
	lHand, rHand := armTop+armH, armTop+armH
	if moving {
		if frame%2 == 0 {
			lHand -= 3
			rHand += 1
		} else {
			lHand += 1
			rHand -= 3
		}
	}
	c.FillRect(tx-armW+1, armTop, armW, lHand-armTop, shade)  // far/shadow arm
	c.FillRect(tx+torsoW-1, armTop, armW, rHand-armTop, body) // near/lit arm
	c.VLine(tx-armW+1, armTop, lHand-1, dark)

	// Torso: a rounded block, lit on the right, shaded on the left, with a
	// dark belt for definition.
	c.FillRect(tx, shoulderY, torsoW, torsoH, body)
	c.FillRect(tx, shoulderY, 4, torsoH, shade)
	c.FillRect(tx, hipY-3, torsoW, 2, dark)

	// Head with a hood: a darker cap over the top of the skull.
	c.FillCircle(footX, headCY, headR, body)
	c.FillCircle(footX-2, headCY, 2, shade) // shaded cheek
	for dy := -headR; dy <= -1; dy++ {      // hood: top half in a darker tone
		w := int(math.Sqrt(float64(headR*headR - dy*dy)))
		c.HLine(footX-w, footX+w, headCY+dy, shade)
	}

	// Facing cues: eyes on the front-facing side, none on the back.
	switch f {
	case FaceDown:
		c.FillCircle(footX-3, headCY+1, 1, dark)
		c.FillCircle(footX+3, headCY+1, 1, dark)
	case FaceUp:
		c.FillCircle(footX, headCY-1, 3, shade) // darker crown, no eyes
	case FaceLeft:
		c.FillCircle(footX-3, headCY+1, 1, dark)
		c.VLine(footX+headR-1, headCY-2, headCY+3, shade)
	case FaceRight:
		c.FillCircle(footX+3, headCY+1, 1, dark)
		c.VLine(footX-headR+1, headCY-2, headCY+3, shade)
	}
}
