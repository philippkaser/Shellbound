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

import "github.com/shellbound/shellbound/internal/render/canvas"

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
// the walk pose; facing orients the head. A plain little figure — head, torso,
// swinging arms and legs — with soft shading.
func Draw(c *canvas.Canvas, footX, footY int, f Facing, frame int, moving bool) {
	const (
		legH   = 14
		legW   = 5
		torsoH = 16
		torsoW = 14
		headR  = 6
		armW   = 4
		armH   = 12
	)
	hipY := footY - legH
	shoulderY := hipY - torsoH
	headCY := shoulderY - headR
	tx := footX - torsoW/2

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

	// Arms swing opposite the legs: the hand lifts and drops. Both are lit; the
	// shadow lives on the figure's outline, and a thin seam separates the right
	// arm from the torso.
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
	laX, raX := tx-armW, tx+torsoW
	c.FillRect(laX, armTop, armW, lHand-armTop, body)
	c.FillRect(raX, armTop, armW, rHand-armTop, body)
	c.VLine(laX, armTop, lHand-1, shade) // shadow contour down the left arm
	c.VLine(raX, armTop, rHand-1, shade) // seam between the right arm and torso

	// Torso: a rounded block with a thin shadow down its left edge and a dark
	// belt for definition.
	c.FillRect(tx, shoulderY, torsoW, torsoH, body)
	c.FillRect(tx, shoulderY, 2, torsoH, shade)
	c.FillRect(tx, hipY-3, torsoW, 2, dark)

	// Head: a thin shadow on the left rim only — clear of the eyes.
	c.FillCircle(footX, headCY, headR, body)
	c.VLine(footX-headR+1, headCY-2, headCY+2, shade)

	// Facing cues: eyes on the front-facing side, none on the back.
	switch f {
	case FaceDown:
		c.FillCircle(footX-3, headCY, 1, dark)
		c.FillCircle(footX+3, headCY, 1, dark)
	case FaceUp:
		c.FillCircle(footX, headCY-1, 3, shade) // darker crown, no eyes
	case FaceLeft:
		c.FillCircle(footX-2, headCY, 1, dark)
	case FaceRight:
		c.FillCircle(footX+2, headCY, 1, dark)
	}
}
