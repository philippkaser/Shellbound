// Package sprites bakes Shellbound's pixel sprites as Go string literals.
// The player sprite is 3 pixels wide and 8 pixels tall in half-block pixel
// space (3 cells wide, 4 cells tall on screen), with a 2-frame walk cycle
// per facing direction.
package sprites

import (
	"strings"

	"github.com/shellbound/shellbound/internal/render/halfblock"
)

// Facing is a 4-way sprite direction.
type Facing int

// Facing values, ordered to match hub.Dir.
const (
	FaceDown Facing = iota
	FaceUp
	FaceLeft
	FaceRight
)

// Player sprite dimensions in half-block pixels.
const (
	PlayerW = 3
	PlayerH = 8
)

// Frame is a parsed sprite frame. Px is row-major, len W*H; zero entries
// are transparent.
type Frame struct {
	W, H int
	Px   []halfblock.Color
}

// Draw paints the frame onto c with its top-left at column x (cells) and
// pixel row y. Transparent pixels are skipped.
func (f Frame) Draw(c *halfblock.Canvas, x, y int) {
	for dy := 0; dy < f.H; dy++ {
		for dx := 0; dx < f.W; dx++ {
			if col := f.Px[dy*f.W+dx]; col != 0 {
				c.SetPx(x+dx, y+dy, col)
			}
		}
	}
}

// Mask legend: '.' transparent, '#' white, '+' light grey shading.
var maskColors = map[byte]halfblock.Color{
	'#': 0xFFFFFF,
	'+': 0xD4D4D4,
}

// parse converts a mask (rows separated by newlines, all the same width)
// into a Frame. It is called only on package data at init; malformed rows
// are tolerated by clipping.
func parse(mask string) Frame {
	rows := strings.Split(strings.Trim(mask, "\n"), "\n")
	f := Frame{W: PlayerW, H: PlayerH, Px: make([]halfblock.Color, PlayerW*PlayerH)}
	for y, row := range rows {
		if y >= f.H {
			break
		}
		for x := 0; x < len(row) && x < f.W; x++ {
			if col, ok := maskColors[row[x]]; ok {
				f.Px[y*f.W+x] = col
			}
		}
	}
	return f
}

// Walk-cycle masks. Frame 0 = legs apart, frame 1 = legs together; the
// alternation against a moving background reads as a stride.
var playerMasks = map[Facing][2]string{
	FaceDown: {`
.#.
###
###
###
.#.
.#.
#.#
#.#`, `
.#.
###
###
###
.#.
.#.
.#.
.#.`},
	FaceUp: {`
.+.
+++
###
###
.#.
.#.
#.#
#.#`, `
.+.
+++
###
###
.#.
.#.
.#.
.#.`},
	FaceLeft: {`
.#.
##+
##.
##.
.#.
.#.
#.#
#..`, `
.#.
##+
##.
##.
.#.
.#.
.#.
.#.`},
	FaceRight: {`
.#.
+##
.##
.##
.#.
.#.
#.#
..#`, `
.#.
+##
.##
.##
.#.
.#.
.#.
.#.`},
}

var playerFrames map[Facing][2]Frame

func init() {
	playerFrames = make(map[Facing][2]Frame, len(playerMasks))
	for face, masks := range playerMasks {
		playerFrames[face] = [2]Frame{parse(masks[0]), parse(masks[1])}
	}
}

// Player returns the sprite frame for a facing direction and walk-cycle
// counter (any integer; even = frame 0, odd = frame 1). Unknown facings
// fall back to FaceDown.
func Player(f Facing, frame int) Frame {
	frames, ok := playerFrames[f]
	if !ok {
		frames = playerFrames[FaceDown]
	}
	if frame%2 == 0 {
		return frames[0]
	}
	return frames[1]
}
