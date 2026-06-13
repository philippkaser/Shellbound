package plaza

import (
	"github.com/shellbound/shellbound/internal/anim"
	"github.com/shellbound/shellbound/internal/render/halfblock"
	"github.com/shellbound/shellbound/internal/render/pixelbuf"
)

// Monochrome tones used by the plaza renderers (the only colors the map
// itself is allowed to use).
const (
	toneWhite halfblock.Color = 0xFFFFFF
	toneLight halfblock.Color = 0xD4D4D4
	toneMid   halfblock.Color = 0xA1A1A1
	toneDim   halfblock.Color = 0x737373
	toneDark  halfblock.Color = 0x404040
)

// RenderBase paints every static tile into a world-sized canvas. It runs
// once at startup; frames blit from the result.
func (m *Map) RenderBase(c *halfblock.Canvas) {
	for cy := 0; cy < m.H; cy++ {
		for cx := 0; cx < m.W; cx++ {
			switch m.Tile(cx, cy) {
			case ' ':
				// Plain floor: pure black, nothing to draw.
			case '.':
				// Light speck: single top pixel.
				c.SetPx(cx, cy*2, toneDark)
			case ',':
				// Dark speck: single bottom pixel, dimmer.
				c.SetPx(cx, cy*2+1, 0x2E2E2E)
			case '#':
				m.renderWall(c, cx, cy)
			case 'P':
				m.renderPillar(c, cx, cy)
			case 'B':
				m.renderBench(c, cx, cy)
			case 'F':
				m.renderStatue(c, cx, cy)
			case 'L':
				// Post only; the glowing head is dynamic.
				c.SetGlyph(cx, cy, '┃', toneDim)
			case '~':
				// Water gets a still base so the dynamic pass only needs to
				// touch visible cells.
				c.SetGlyph(cx, cy, '░', toneDim)
			}
		}
	}
	// Second pass: soft shadows cast on the floor to the lower-right of
	// pillars and the statue base.
	for cy := 0; cy < m.H; cy++ {
		for cx := 0; cx < m.W; cx++ {
			t := m.Tile(cx, cy)
			if (t == 'P' || t == 'F') && m.Tile(cx, cy+1) != t {
				if m.Tile(cx+1, cy) == ' ' || m.Tile(cx+1, cy) == '.' || m.Tile(cx+1, cy) == ',' {
					c.SetGlyph(cx+1, cy, '░', 0x262626)
				}
			}
		}
	}
}

// renderWall draws one wall cell. The face that borders the plaza floor is
// brighter so walls read as lit volumes, not flat fills.
func (m *Map) renderWall(c *halfblock.Canvas, cx, cy int) {
	facesFloor := m.Tile(cx, cy+1) != '#' || m.Tile(cx, cy-1) != '#' ||
		m.Tile(cx+1, cy) != '#' || m.Tile(cx-1, cy) != '#'
	if !facesFloor {
		// Deep wall: nearly invisible, lets the border fade into black.
		c.SetGlyph(cx, cy, '█', 0x2E2E2E)
		return
	}
	// Edge wall: a lit cap over a darker body, drawn in pixels for a
	// beveled look.
	c.SetPx(cx, cy*2, toneMid)
	c.SetPx(cx, cy*2+1, toneDark)
}

// renderPillar draws half of a pillar pair: the capital on top, the shaft
// at the bottom.
func (m *Map) renderPillar(c *halfblock.Canvas, cx, cy int) {
	if m.Tile(cx, cy+1) == 'P' {
		c.SetGlyph(cx, cy, '▆', toneLight) // capital
	} else {
		c.SetGlyph(cx, cy, '█', toneMid) // shaft/base
	}
}

// renderBench draws one bench segment, with tapered ends.
func (m *Map) renderBench(c *halfblock.Canvas, cx, cy int) {
	left := m.Tile(cx-1, cy) == 'B'
	right := m.Tile(cx+1, cy) == 'B'
	switch {
	case !left && right:
		c.SetGlyph(cx, cy, '▗', toneLight)
	case left && !right:
		c.SetGlyph(cx, cy, '▖', toneLight)
	default:
		c.SetGlyph(cx, cy, '▄', toneLight)
	}
}

// renderStatue draws half of the fountain statue pair.
func (m *Map) renderStatue(c *halfblock.Canvas, cx, cy int) {
	if m.Tile(cx, cy+1) == 'F' {
		c.SetGlyph(cx, cy, '▆', toneLight)
	} else {
		c.SetGlyph(cx, cy, '█', toneMid)
	}
}

// waterRunes is the ripple cycle for fountain water.
var waterRunes = [4]rune{'░', '▒', '▓', '▒'}

// RenderDynamic draws the animated decorations (water, lamp glow, statue
// spray, clouds) into a screen canvas. (ox, oy) is the world cell at the
// canvas's top-left; t is seconds since server start. Out-of-view cells
// are clipped by the canvas itself.
func (m *Map) RenderDynamic(c *halfblock.Canvas, t float64, ox, oy int) {
	// Fountain ripples: phase varies per-cell so the surface scintillates
	// outward rather than blinking in unison.
	for _, p := range m.Water {
		ph := anim.Phase(t, 3, len(waterRunes), p.X+p.Y*3)
		c.SetGlyph(p.X-ox, p.Y-oy, waterRunes[ph], toneDim)
	}
	// Statue spray: a flickering crest above the capital.
	for _, p := range m.StatueTops {
		ph := anim.Phase(t, 4, 3, p.X)
		sprayRunes := [3]rune{'░', '▒', '░'}
		c.SetGlyph(p.X-ox, p.Y-1-oy, sprayRunes[ph], toneMid)
	}
	// Lamp heads: a glowing block whose brightness flickers organically.
	for _, p := range m.Lamps {
		k := anim.Flicker(t, p.X*31+p.Y*7)
		col := halfblock.Color(anim.Scale(uint32(toneWhite), k))
		c.SetGlyph(p.X-ox, p.Y-1-oy, '█', col)
	}
	m.renderClouds(c, t, ox, oy)
}

// Clouds: monochrome wisps drifting along the top of the plaza, rendered
// once into quadrant cells via pixelbuf and replayed with a time offset.
type cloud struct {
	cells  []pixelbuf.Cell
	cw, ch int     // cell dimensions
	y      int     // cell row it drifts along
	speed  float64 // cells per second
	phase  float64 // initial offset in cells
}

var clouds = buildClouds()

// buildClouds pre-renders three cloud shapes from pixel ellipses.
func buildClouds() []cloud {
	shape := func(wPx, hPx int, col pixelbuf.Color) ([]pixelbuf.Cell, int, int) {
		b := pixelbuf.New(wPx, hPx)
		cxf, cyf := float64(wPx-1)/2, float64(hPx-1)/2
		for y := 0; y < hPx; y++ {
			for x := 0; x < wPx; x++ {
				dx := (float64(x) - cxf) / (cxf + 0.5)
				dy := (float64(y) - cyf) / (cyf + 0.5)
				if dx*dx+dy*dy <= 1 {
					b.Set(x, y, col)
				}
			}
		}
		return b.Cells(), b.CellWidth(), b.CellHeight()
	}
	var out []cloud
	specs := []struct {
		wPx, hPx int
		y        int
		speed    float64
		phase    float64
	}{
		{18, 4, 3, 1.1, 5},
		{12, 4, 5, 0.7, 40},
		{22, 4, 4, 0.9, 70},
	}
	for _, s := range specs {
		cells, cw, ch := shape(s.wPx, s.hPx, 0x404040)
		out = append(out, cloud{cells: cells, cw: cw, ch: ch, y: s.y, speed: s.speed, phase: s.phase})
	}
	return out
}

// renderClouds draws the drifting clouds, wrapping around the map width.
func (m *Map) renderClouds(c *halfblock.Canvas, t float64, ox, oy int) {
	for _, cl := range clouds {
		// World cell x of the cloud's left edge, wrapped to map width.
		x0 := int(cl.phase+t*cl.speed) % (m.W + cl.cw)
		if x0 < 0 {
			x0 += m.W + cl.cw
		}
		x0 -= cl.cw // enter from the left edge
		for cy := 0; cy < cl.ch; cy++ {
			for cx := 0; cx < cl.cw; cx++ {
				cell := cl.cells[cy*cl.cw+cx]
				if cell.Rune == ' ' {
					continue
				}
				c.SetGlyph(x0+cx-ox, cl.y+cy-oy, cell.Rune, halfblock.Color(cell.FG))
			}
		}
	}
}
