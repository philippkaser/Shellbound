package plaza

import (
	"math"
	"sort"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
)

// sin01 is math.Sin, here so the twinkle reads clearly at the call site.
func sin01(x float64) float64 { return math.Sin(x) }

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

// towerCell is one skyscraper just outside the plaza, on the back (north/west)
// edges where it sits behind the plaza in the isometric view.
type towerCell struct {
	gx, gy, h int
}

// buildSkyline computes the ring of background towers once (deterministic
// heights, pre-sorted back-to-front) so the per-frame render only culls.
func (m *Map) buildSkyline() {
	add := func(gx, gy int) {
		seed := gx*73856093 ^ gy*19349663
		if seed < 0 {
			seed = -seed
		}
		m.skyline = append(m.skyline, towerCell{gx, gy, 44 + seed%78})
	}
	for d := 2; d <= skylineDepth+1; d++ {
		for gx := -8; gx < m.W+8; gx += skylineStep {
			add(gx, -d) // north band
		}
		for gy := -8; gy < m.H+8; gy += skylineStep {
			add(-d, gy) // west band
		}
	}
	sort.Slice(m.skyline, func(i, j int) bool {
		return m.skyline[i].gx+m.skyline[i].gy < m.skyline[j].gx+m.skyline[j].gy
	})
}

// renderSkyline draws the precomputed city behind the plaza's two back edges,
// so the plaza feels like a small clearing in a big, lived-in place. Drawn
// before the ground (everything paints over them) and only beyond the
// north/west edges, so they never occlude the player. t drives the slow
// twinkle of the city's windows.
func (m *Map) renderSkyline(c *canvas.Canvas, originSx, originSy, t float64) {
	for _, tc := range m.skyline {
		px, py := project(tc.gx, tc.gy, originSx, originSy)
		if px < -iso.HW-4 || px > c.W+iso.HW+4 || py-tc.h > c.H || py < -tc.h {
			continue // off-screen
		}
		drawTower(c, px, py, tc.h, tc.gx, tc.gy, t)
	}
}

// haze fades a tower tone toward the near-black sky with distance, so far
// ranks of the city recede into atmosphere instead of stacking at one value.
func haze(col canvas.Color, depth float64) canvas.Color {
	const sky = canvas.Color(0x050505)
	k := depth * 0.13
	if k > 0.75 {
		k = 0.75
	}
	return col.Lerp(sky, k)
}

// drawTower draws one extruded skyscraper with a scatter of windows on its
// right (lit) face, hazed by distance, with a seed-picked roofline (flat cap,
// stepped crown or antenna) so the silhouette varies.
func drawTower(c *canvas.Canvas, px, py, h, gx, gy int, t float64) {
	seed := gx*73856093 ^ gy*19349663
	if seed < 0 {
		seed = -seed
	}
	// Distance behind the plaza edge: towers sit at negative gx/gy, so the
	// more negative, the further back.
	depth := 0.0
	if gy < 0 {
		depth = float64(-gy - 1)
	}
	if gx < 0 && float64(-gx-1) > depth {
		depth = float64(-gx - 1)
	}
	top, left, right := haze(towerTop, depth), haze(towerL, depth), haze(towerR, depth)
	iso.DrawCube(c, px, py, h, top, left, right)

	topY := py + iso.HH - h
	// Roofline variety.
	switch seed % 4 {
	case 0: // slim antenna rising from the roof center, beacon blinking slowly
		ah := 8 + seed%9
		c.VLine(px, topY-ah, topY, right)
		if (int(t*0.8)+seed)%2 == 0 {
			c.Set(px, topY-ah-1, winLit)
		}
	case 1: // rooftop penthouse/water-tank block breaking the flat roofline
		c.FillRect(px-6, topY-7, 12, 7, right)
		c.FillRect(px-6, topY-9, 12, 2, top)
	}

	// Windows on the right face: a regular grid. Most are dark; a scatter is
	// lit, and a few of those twinkle slowly so the city reads alive.
	for wy := topY + 4; wy < py+iso.HH-3; wy += 6 {
		for wx := 3; wx < iso.HW-1; wx += 4 {
			// Follow the face's downward slope so windows sit flat on the wall.
			yy := wy + wx/2
			k := gx*7 + gy*13 + wx + wy
			col := haze(winDark, depth)
			if k%5 == 0 {
				col = haze(winLit, depth)
				// Roughly a third of the lit windows fade in and out on their
				// own slow clocks.
				if k%3 == 0 {
					phase := 0.5 + 0.5*sin01(t*0.35+float64(k))
					col = haze(winDark, depth).Lerp(haze(winLit, depth), phase)
				}
			}
			c.Set(px+wx, yy, col)
			c.Set(px+wx+1, yy, col)
		}
	}
}
