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
// place name tags Height+gap above the foot point. Sized to read clearly at
// the isometric tile scale.
const (
	Width  = 24
	Height = 44
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
		legH    = 14
		torsoH  = 16
		headR   = 7
		legOff  = 5
		legW    = 4
		torsoW  = 15
		legStep = 3 // shortened stride leg
	)
	hipY := footY - legH
	shoulderY := hipY - torsoH
	headCY := shoulderY - headR

	// Legs. While walking, alternate which leg leads so the stride reads
	// against the moving ground.
	llen, rlen := legH, legH
	if moving {
		if frame%2 == 0 {
			rlen = legH - legStep
		} else {
			llen = legH - legStep
		}
	}
	lx, rx := footX-legOff, footX+legOff
	c.FillRect(lx-legW/2, footY-llen, legW, llen, body)
	c.FillRect(rx-legW/2, footY-rlen, legW, rlen, body)
	c.VLine(lx-legW/2, footY-llen, footY-1, shade) // left-edge shade

	// Torso: a rounded block, lit on the right, shaded on the left.
	tx := footX - torsoW/2
	c.FillRect(tx, shoulderY, torsoW, torsoH, body)
	c.FillRect(tx, shoulderY, 4, torsoH, shade)

	// Head.
	c.FillCircle(footX, headCY, headR, body)
	c.FillCircle(footX-headR/2, headCY, 2, shade) // shaded cheek

	// Facing cues: eyes on the front-facing side, none on the back.
	switch f {
	case FaceDown:
		c.FillCircle(footX-3, headCY, 1, dark)
		c.FillCircle(footX+3, headCY, 1, dark)
	case FaceUp:
		c.FillCircle(footX, headCY-2, 3, shade) // back of the head
	case FaceLeft:
		c.FillCircle(footX-3, headCY, 1, dark)
		c.VLine(footX-headR, headCY-3, headCY+3, shade)
	case FaceRight:
		c.FillCircle(footX+3, headCY, 1, dark)
		c.VLine(footX+headR, headCY-3, headCY+3, shade)
	}
}
