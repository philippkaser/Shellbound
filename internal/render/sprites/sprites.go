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
// the walk pose (even = mid-stride, odd = passing); facing orients the head.
func Draw(c *canvas.Canvas, footX, footY int, f Facing, frame int, moving bool) {
	const (
		legH   = 14
		legW   = 5
		torsoH = 16
		torsoW = 15
		headR  = 6
	)
	hipY := footY - legH
	shoulderY := hipY - torsoH
	headCY := shoulderY - headR

	// Legs hang from a fixed hip; walking lifts a foot (the lower end) clear
	// of the ground and sets it back down, alternating each step — so it's the
	// feet that move, not the hips.
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
	// Shade the left leg's left edge for volume.
	c.VLine(lx-legW/2, hipY, lFoot-1, shade)

	// Torso: a rounded block, lit on the right, shaded on the left, with a
	// dark belt for a little definition.
	tx := footX - torsoW/2
	c.FillRect(tx, shoulderY, torsoW, torsoH, body)
	c.FillRect(tx, shoulderY, 5, torsoH, shade)
	c.FillRect(tx, hipY-3, torsoW, 2, dark)

	// Head.
	c.FillCircle(footX, headCY, headR, body)
	c.FillCircle(footX-2, headCY, 2, shade) // shaded cheek

	// Facing cues: eyes on the front-facing side, none on the back.
	switch f {
	case FaceDown:
		c.FillCircle(footX-3, headCY, 1, dark)
		c.FillCircle(footX+3, headCY, 1, dark)
	case FaceUp:
		c.FillCircle(footX, headCY-1, 3, shade) // darker crown, no eyes
	case FaceLeft:
		c.FillCircle(footX-3, headCY, 1, dark)
		c.VLine(footX+headR-1, headCY-3, headCY+3, shade)
	case FaceRight:
		c.FillCircle(footX+3, headCY, 1, dark)
		c.VLine(footX-headR+1, headCY-3, headCY+3, shade)
	}
}
