package plaza

import (
	"math"
	"sort"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
)

// Monochrome tones for the plaza (the world is strictly black/white/grey;
// the only color comes from portals and player names).
const (
	toneFloor     = canvas.Color(0x0C0C0C)
	toneFloorB    = canvas.Color(0x111111) // alternate paving tone (checker)
	tonePaving    = canvas.Color(0x202020) // pale flagstone ring around the fountain
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

// Structure heights in pixels (scaled to the larger tiles).
const (
	wallH    = 22
	pillarH  = 34
	benchH   = 8
	lampPost = 32
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
	// The distant city behind the plaza, drawn first so everything paints over it.
	m.renderSkyline(c, originSx, originSy)

	gx0, gy0, gx1, gy1 := iso.VisibleCellRange(originSx, originSy, c.W, c.H, 3)
	gx0, gy0 = clampi(gx0, 0, m.W-1), clampi(gy0, 0, m.H-1)
	gx1, gy1 = clampi(gx1, 0, m.W-1), clampi(gy1, 0, m.H-1)

	// Fountain center (for water ripple rings); -1 if there's no fountain.
	fcx, fcy := -1, -1
	if len(m.StatueTops) > 0 {
		fcx, fcy = m.StatueTops[0].X, m.StatueTops[0].Y
	}

	// Ground plane (flat, so draw order is irrelevant). The floor alternates
	// between two near-black tones for a paved texture, and a ring of paler
	// flagstones rings the fountain.
	for gy := gy0; gy <= gy1; gy++ {
		for gx := gx0; gx <= gx1; gx++ {
			px, py := project(gx, gy, originSx, originSy)
			switch tile := m.Tile(gx, gy); tile {
			case '~', 'F': // the statue cells are pool too, so there's no gap under the fountain
				drawWater(c, px, py, gx, gy, fcx, fcy, t)
			default:
				fill := toneFloor
				if (gx+gy)&1 == 0 {
					fill = toneFloorB
				}
				if fcx >= 0 {
					if d := math.Hypot(float64(gx-fcx), float64(gy-fcy)); d >= 4 && d < 5.4 {
						fill = tonePaving // decorative ring around the pool
					}
				}
				iso.DrawDiamond(c, px, py, fill, toneFloorEdge)
				if tile == '.' {
					c.Set(px, py+iso.HH, toneSpeck)
				} else if tile == ',' {
					c.Set(px, py+iso.HH, toneSpeckDim)
				}
			}
		}
	}

	// Structures: iterate the pre-sorted (back-to-front) list, culling cells
	// outside the visible range — no per-frame collect or sort.
	for _, s := range m.structures {
		if s.X < gx0 || s.X > gx1 || s.Y < gy0 || s.Y > gy1 {
			continue
		}
		px, py := project(s.X, s.Y, originSx, originSy)
		switch s.tile {
		case '#':
			iso.DrawCube(c, px, py, wallH, toneMid, toneDark, toneDim)
		case 'P':
			iso.DrawCube(c, px, py, pillarH, toneLight, toneDim, toneMid)
		case 'B':
			iso.DrawCube(c, px, py, benchH, toneLight, toneDim, toneMid)
		case 'L':
			m.drawLampPost(c, px, py)
		}
	}

	// The fountain: a detailed tiered sculpture rising from the pool, with
	// spilling sheets and fine spray.
	for _, p := range m.StatueTops {
		if p.X < gx0 || p.X > gx1 || p.Y < gy0 || p.Y > gy1 {
			continue
		}
		px, py := project(p.X, p.Y, originSx, originSy)
		drawFountain(c, px, py+iso.HH, t)
	}
}

// buildStructures collects every solid cube cell ('#', 'P', 'B', 'L'; the
// fountain 'F' is drawn separately) and sorts it back-to-front once, so the
// per-frame render is a simple cull-and-draw.
func (m *Map) buildStructures() {
	for cy := 0; cy < m.H; cy++ {
		for cx := 0; cx < m.W; cx++ {
			switch t := m.tiles[cy*m.W+cx]; t {
			case '#', 'P', 'B', 'L':
				m.structures = append(m.structures, structCell{cx, cy, t})
			}
		}
	}
	sort.Slice(m.structures, func(i, j int) bool {
		return iso.Depth(m.structures[i].X, m.structures[i].Y) <
			iso.Depth(m.structures[j].X, m.structures[j].Y)
	})
}

// drawWater renders an animated pool tile: concentric ripple rings travel
// outward from the fountain center (fcx, fcy) and a fine per-cell sparkle rides
// on top, so the surface reads as moving water rather than a flat blink.
func drawWater(c *canvas.Canvas, px, py, gx, gy, fcx, fcy int, t float64) {
	dist := math.Hypot(float64(gx-fcx), float64(gy-fcy))
	ripple := math.Sin(dist*1.9 - t*3.4)
	sparkle := math.Sin(float64(gx*5+gy*7) + t*3.0)
	shade := clamp01f(0.5 + 0.42*ripple + 0.10*sparkle)
	g8 := uint8(34 + 52*shade)
	iso.DrawDiamond(c, px, py, canvas.RGB(g8, g8, g8), toneShadow)
}

// fillEllipse fills a 2:1-friendly ellipse (radii rx, ry) centered at (cx, cy).
func fillEllipse(c *canvas.Canvas, cx, cy, rx, ry int, col canvas.Color) {
	if rx <= 0 || ry <= 0 {
		return
	}
	for dy := -ry; dy <= ry; dy++ {
		w := float64(rx) * math.Sqrt(math.Max(0, 1-float64(dy*dy)/float64(ry*ry)))
		iw := int(w)
		c.HLine(cx-iw, cx+iw, cy+dy, col)
	}
}

// fountProfile is the fountain's silhouette radius at height h above its base:
// a wide basin bowl, a slim column, then a flared upper bowl.
func fountProfile(h int) int {
	switch {
	case h <= 8:
		return 18 - h // 18..10 — basin bowl tapering in
	case h <= 11:
		return 10 + 2*(h-8) // 10..16 — basin rim flare
	case h <= 22:
		return 5 // column
	case h <= 26:
		return 5 + 2*(h-22) // 5..13 — upper bowl flare
	case h <= 30:
		return 13 // upper bowl rim
	default:
		return 0
	}
}

// drawFountain paints the tiered stone fountain whose base sits at (ax, groundY):
// a revolved body shaded like a lit cylinder, two shimmering water surfaces,
// sheets spilling from the upper bowl, and a fine crest of spray.
func drawFountain(c *canvas.Canvas, ax, groundY int, t float64) {
	// Body: a vertical profile, each row shaded left→right for a 3D cylinder.
	for h := 0; h <= 30; h++ {
		r := fountProfile(h)
		if r <= 0 {
			continue
		}
		yy := groundY - h
		for dx := -r; dx <= r; dx++ {
			nx := float64(dx) / float64(r)
			col := toneLight
			switch {
			case nx < -0.45:
				col = toneDim
			case nx < -0.1:
				col = toneMid
			case nx > 0.55:
				col = toneWhite
			}
			c.Set(ax+dx, yy, col)
		}
	}
	// Rim highlights on the two bowls.
	fillEllipse(c, ax, groundY-11, 16, 4, toneMid)
	fillEllipse(c, ax, groundY-30, 13, 4, toneMid)

	// Shimmering water surfaces (basin + upper bowl).
	waterSurface(c, ax, groundY-12, 14, 5, t)
	waterSurface(c, ax, groundY-31, 11, 4, t)

	// Sheets of water spilling from the upper bowl down toward the basin, as
	// short bright dashes that travel downward.
	for i := -2; i <= 2; i++ {
		sx := ax + i*5
		phase := math.Mod(t*1.6+float64(i)*0.3, 1.0)
		for k := 0; k < 3; k++ {
			yy := groundY - 28 + int((phase+float64(k)*0.34)*16)%16
			c.Set(sx, yy, toneLight)
			c.Set(sx, yy+1, toneMid)
		}
	}

	// Fine spray from the spout: many small droplets arcing up and falling back.
	topY := groundY - 32
	for d := 0; d < 16; d++ {
		fd := float64(d)
		prog := math.Mod(t*1.5+fd*0.16, 1.0)
		ang := fd * 2.4
		dx := int(math.Cos(ang) * prog * 10)
		dy := int(-26*prog + 30*prog*prog)
		c.Set(ax+dx, topY+dy, toneWhite)
	}
}

// waterSurface fills a shimmering elliptical pool of water at (cx, cy).
func waterSurface(c *canvas.Canvas, cx, cy, rx, ry int, t float64) {
	for dy := -ry; dy <= ry; dy++ {
		w := float64(rx) * math.Sqrt(math.Max(0, 1-float64(dy*dy)/float64(ry*ry)))
		iw := int(w)
		for dx := -iw; dx <= iw; dx++ {
			rd := math.Hypot(float64(dx)/float64(rx), float64(dy)/float64(ry))
			shade := clamp01f(0.5 + 0.4*math.Sin(rd*5-t*3.2))
			g8 := uint8(40 + 46*shade)
			c.Set(cx+dx, cy+dy, canvas.RGB(g8, g8, g8))
		}
	}
}

func clamp01f(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// drawLampPost draws the unlit lamp: a slim post with a white head. The glow
// and its block-char halo are added by the lighting pass.
func (m *Map) drawLampPost(c *canvas.Canvas, px, py int) {
	cx := px
	baseY := py + iso.HH
	c.FillRect(cx-1, baseY-lampPost, 2, lampPost, toneDim)
	c.FillCircle(cx, baseY-lampPost, 2, toneWhite)
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
