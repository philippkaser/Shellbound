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
	Width  = 14
	Height = 26
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
		legH   = 8
		torsoH = 10
		headR  = 4
	)
	hipY := footY - legH
	shoulderY := hipY - torsoH
	headCY := shoulderY - headR

	// Legs. While walking, alternate which leg leads so the stride reads
	// against the moving ground.
	lead := 0
	if moving && frame%2 == 1 {
		lead = 1
	}
	lx, rx := footX-3, footX+3
	llen, rlen := legH, legH
	if moving {
		if lead == 0 {
			llen, rlen = legH, legH-2
		} else {
			llen, rlen = legH-2, legH
		}
	}
	c.FillRect(lx-1, footY-llen, 3, llen, body)
	c.FillRect(rx-1, footY-rlen, 3, rlen, body)
	// Shade the left leg's left edge for volume.
	c.VLine(lx-1, footY-llen, footY-1, shade)

	// Torso: a rounded block, lit on the right, shaded on the left.
	tw := 9
	tx := footX - tw/2
	c.FillRect(tx, shoulderY, tw, torsoH, body)
	c.FillRect(tx, shoulderY, 3, torsoH, shade)

	// Head.
	c.FillCircle(footX, headCY, headR, body)
	c.FillCircle(footX-headR/2, headCY, 1, shade) // shaded cheek

	// Facing cues: eyes on the front-facing side, none on the back.
	switch f {
	case FaceDown:
		c.Set(footX-2, headCY, dark)
		c.Set(footX+2, headCY, dark)
	case FaceUp:
		// Back of the head: a darker crown, no eyes.
		c.FillCircle(footX, headCY-1, 2, shade)
	case FaceLeft:
		c.Set(footX-2, headCY, dark)
		c.VLine(footX-headR, headCY-2, headCY+2, shade)
	case FaceRight:
		c.Set(footX+2, headCY, dark)
		c.VLine(footX+headR, headCY-2, headCY+2, shade)
	}
}
