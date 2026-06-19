package plaza

import (
	"sort"

	"github.com/shellbound/shellbound/internal/anim"
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
)

// Monochrome tones for the plaza (the world is strictly black/white/grey;
// the only color comes from portals and player names).
const (
	toneFloor     = canvas.Color(0x0C0C0C)
	toneFloorEdge = canvas.Color(0x1A1A1A)
	toneSpeck     = canvas.Color(0x383838)
	toneSpeckDim  = canvas.Color(0x242424)
	toneWhite     = canvas.Color(0xF2F2F2)
	toneLight     = canvas.Color(0xD4D4D4)
	toneMid       = canvas.Color(0xA1A1A1)
	toneDim       = canvas.Color(0x6E6E6E)
	toneDark      = canvas.Color(0x404040)
	toneShadow    = canvas.Color(0x2A2A2A)
)

// Structure heights in pixels (scaled to the isometric tile size).
const (
	wallH    = 24
	pillarH  = 38
	benchH   = 9
	statueH  = 34
	lampPost = 34
)

// project converts a cell to its ground-diamond top vertex in canvas pixels,
// given the screen-space origin at the canvas's top-left.
func project(gx, gy int, originSx, originSy float64) (int, int) {
	sx, sy := iso.Project(float64(gx), float64(gy))
	return int(sx - originSx), int(sy - originSy)
}

// RenderIso paints the plaza into the screen canvas: a tiled ground plane and
// the depth-sorted structures rising from it. (originSx, originSy) is the
// screen-space point at the canvas's top-left; t is seconds since server
// start (drives water ripple and statue spray).
func (m *Map) RenderIso(c *canvas.Canvas, originSx, originSy, t float64) {
	gx0, gy0, gx1, gy1 := iso.VisibleCellRange(originSx, originSy, c.W, c.H, 4)
	gx0, gy0 = clampi(gx0, 0, m.W-1), clampi(gy0, 0, m.H-1)
	gx1, gy1 = clampi(gx1, 0, m.W-1), clampi(gy1, 0, m.H-1)

	// Ground plane (flat, so draw order is irrelevant).
	for gy := gy0; gy <= gy1; gy++ {
		for gx := gx0; gx <= gx1; gx++ {
			px, py := project(gx, gy, originSx, originSy)
			switch tile := m.Tile(gx, gy); tile {
			case '~':
				m.drawWater(c, px, py, gx, gy, t)
			default:
				iso.DrawDiamond(c, px, py, toneFloor, toneFloorEdge)
				if tile == '.' {
					c.Set(px, py+iso.HH, toneSpeck)
				} else if tile == ',' {
					c.Set(px, py+iso.HH, toneSpeckDim)
				}
			}
		}
	}

	// Structures, painter-sorted back-to-front so near cubes overlap far ones.
	type cell struct{ gx, gy int }
	var structs []cell
	for gy := gy0; gy <= gy1; gy++ {
		for gx := gx0; gx <= gx1; gx++ {
			switch m.Tile(gx, gy) {
			case '#', 'P', 'B', 'F', 'L':
				structs = append(structs, cell{gx, gy})
			}
		}
	}
	sort.Slice(structs, func(i, j int) bool {
		return iso.Depth(structs[i].gx, structs[i].gy) < iso.Depth(structs[j].gx, structs[j].gy)
	})
	for _, s := range structs {
		px, py := project(s.gx, s.gy, originSx, originSy)
		switch m.Tile(s.gx, s.gy) {
		case '#':
			iso.DrawCube(c, px, py, wallH, toneMid, toneDark, toneDim)
		case 'P':
			iso.DrawCube(c, px, py, pillarH, toneLight, toneDim, toneMid)
		case 'B':
			iso.DrawCube(c, px, py, benchH, toneLight, toneDim, toneMid)
		case 'F':
			iso.DrawCube(c, px, py, statueH, toneWhite, toneMid, toneLight)
		case 'L':
			m.drawLampPost(c, px, py)
		}
	}

	// Statue spray crest, flickering above each fountain statue.
	for _, p := range m.StatueTops {
		if p.X < gx0 || p.X > gx1 || p.Y < gy0 || p.Y > gy1 {
			continue
		}
		px, py := project(p.X, p.Y, originSx, originSy)
		ph := anim.Phase(t, 4, 3, p.X)
		crest := []canvas.Color{toneMid, toneLight, toneMid}[ph]
		topY := py - statueH
		c.FillCircle(px, topY-4, 2, crest)
		c.Set(px, topY-7, toneLight)
	}
}

// drawWater renders an animated water tile at ground level: the diamond
// scintillates between grey levels per-cell so the surface shimmers outward
// rather than blinking in unison.
func (m *Map) drawWater(c *canvas.Canvas, px, py, gx, gy int, t float64) {
	levels := []canvas.Color{0x2A2A2A, 0x3A3A3A, 0x505050, 0x3A3A3A}
	ph := anim.Phase(t, 3, len(levels), gx+gy*3)
	iso.DrawDiamond(c, px, py, levels[ph], toneShadow)
}

// drawLampPost draws the unlit lamp: a slim post with a white head. The glow
// and its block-char halo are added by the lighting pass.
func (m *Map) drawLampPost(c *canvas.Canvas, px, py int) {
	cx := px
	baseY := py + iso.HH
	c.FillRect(cx-1, baseY-lampPost, 3, lampPost, toneDim)
	c.FillCircle(cx, baseY-lampPost, 3, toneWhite)
}

// LampHead returns the canvas pixel of a lamp's glowing head for a cell,
// given the screen-space origin. Used by the lighting pass.
func LampHead(gx, gy int, originSx, originSy float64) (int, int) {
	px, py := project(gx, gy, originSx, originSy)
	return px, py + iso.HH - lampPost
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
