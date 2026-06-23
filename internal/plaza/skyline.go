package plaza

import (
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
)

// Skyline tones: a distant city is darker than the plaza so it reads as
// background, with two window states for a "lived-in" texture.
const (
	towerTop = canvas.Color(0x4A4A4A)
	towerL   = canvas.Color(0x202020)
	towerR   = canvas.Color(0x343434)
	winLit   = canvas.Color(0xBFBFBF)
	winDark  = canvas.Color(0x2A2A2A)
)

// skylineDepth is how many rows of towers ring each back edge, and skylineStep
// the spacing between towers along an edge.
const (
	skylineDepth = 7
	skylineStep  = 2
)

// tower is one skyscraper just outside the plaza, on the back (north/west) edges
// where it sits behind the plaza in the isometric view.
type tower struct {
	gx, gy, h int
}

// renderSkyline draws a city of towers ringing the plaza's two back edges, so
// the plaza feels like a small clearing in a big, lived-in place. Towers are
// drawn before the ground (everything in the plaza paints over them) and only
// beyond the north/west edges, so they never occlude the player.
func (m *Map) renderSkyline(c *canvas.Canvas, originSx, originSy float64) {
	towers := make([]tower, 0, 256)
	add := func(gx, gy int) {
		// Deterministic per-position height so the skyline is stable.
		seed := gx*73856093 ^ gy*19349663
		if seed < 0 {
			seed = -seed
		}
		towers = append(towers, tower{gx, gy, 44 + seed%78})
	}
	for d := 2; d <= skylineDepth+1; d++ {
		for gx := -8; gx < m.W+8; gx += skylineStep {
			add(gx, -d) // north band
		}
		for gy := -8; gy < m.H+8; gy += skylineStep {
			add(-d, gy) // west band
		}
	}
	// Back-to-front so nearer towers overlap farther ones.
	for i := 1; i < len(towers); i++ {
		for j := i; j > 0 && towers[j].gx+towers[j].gy < towers[j-1].gx+towers[j-1].gy; j-- {
			towers[j], towers[j-1] = towers[j-1], towers[j]
		}
	}
	for _, t := range towers {
		px, py := project(t.gx, t.gy, originSx, originSy)
		if px < -iso.HW-4 || px > c.W+iso.HW+4 || py-t.h > c.H || py < -t.h {
			continue // off-screen
		}
		drawTower(c, px, py, t.h, t.gx, t.gy)
	}
}

// drawTower draws one extruded skyscraper with a scatter of lit/dark windows on
// its right (lit) face.
func drawTower(c *canvas.Canvas, px, py, h, gx, gy int) {
	iso.DrawCube(c, px, py, h, towerTop, towerL, towerR)

	// Windows on the right face: a regular grid, lit deterministically so the
	// city looks occupied. The right face spans dx in [1, HW], rising h tall.
	topY := py + iso.HH - h
	for wy := topY + 4; wy < py+iso.HH-3; wy += 6 {
		for wx := 3; wx < iso.HW-1; wx += 4 {
			// Follow the face's downward slope so windows sit flat on the wall.
			yy := wy + wx/2
			col := winDark
			if (gx*7+gy*13+wx+wy)%5 == 0 {
				col = winLit
			}
			c.Set(px+wx, yy, col)
			c.Set(px+wx+1, yy, col)
		}
	}
}
