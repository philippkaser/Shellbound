package shellmon

import (
	"sort"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
	"github.com/shellbound/shellbound/internal/render/sprites"
	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// Route tuning.
const (
	encounterRate = 0.16 // chance per step into tall grass
	routeLevelMin = 3
	routeLevelMax = 8
)

// The wild route layout. '#' trees (block), ',' tall grass (encounters),
// '.' trodden path, 's' the entrance. The border is forced to trees.
const routeLayout = `###################
#,,,..,,,,..,,,..,#
#,,,,,#,,,,#,,,,,,#
#,..,,,,..,,,,..,,#
#,,,,,,,#,,,#,,,,,#
#..,,,..,,,,,..,,.#
#,,,#,,,,..,,,,#,,#
#,,,,,..,,,#,,,,,,#
#,..,,,,,,,,,,..,,#
#,,,,...,s,...,,,,#
###################`

// routeState is the walkable field and the player's position on it.
type routeState struct {
	w, h   int
	tiles  []byte
	px, py int
	facing sprites.Facing
}

// enterRoute (re)builds the route and places the player at the entrance.
func (m *model) enterRoute() {
	lines := splitLines(routeLayout)
	h := len(lines)
	w := 0
	for _, l := range lines {
		if len(l) > w {
			w = len(l)
		}
	}
	r := &routeState{w: w, h: h, tiles: make([]byte, w*h), facing: sprites.FaceDown}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := byte(',')
			if x < len(lines[y]) {
				t = lines[y][x]
			}
			if x == 0 || y == 0 || x == w-1 || y == h-1 {
				t = '#'
			}
			if t == 's' {
				r.px, r.py = x, y
				t = '.'
			}
			r.tiles[y*w+x] = t
		}
	}
	m.route = r
	m.state = stateRoute
}

func (r *routeState) tile(x, y int) byte {
	if x < 0 || y < 0 || x >= r.w || y >= r.h {
		return '#'
	}
	return r.tiles[y*r.w+x]
}

// keyRoute handles route input; returns true to leave the world.
func (m *model) keyRoute(key string) bool {
	r := m.route
	dx, dy := 0, 0
	switch key {
	case "up", "w":
		dy, r.facing = -1, sprites.FaceUp
	case "down", "s":
		dy, r.facing = 1, sprites.FaceDown
	case "left", "a":
		dx, r.facing = -1, sprites.FaceLeft
	case "right", "d":
		dx, r.facing = 1, sprites.FaceRight
	case "p":
		m.partyCur = 0
		m.state = stateParty
		return false
	case "esc", "q":
		return true
	default:
		return false
	}
	if dx == 0 && dy == 0 {
		return false
	}
	nx, ny := r.px+dx, r.py+dy
	if r.tile(nx, ny) == '#' {
		return false // blocked by trees
	}
	r.px, r.py = nx, ny
	if r.tile(nx, ny) == ',' && m.rng.Float64() < encounterRate {
		m.startWildBattle()
	}
	return false
}

// startWildBattle rolls a random wild Shellmon and opens the battle.
func (m *model) startWildBattle() {
	all := mon.All()
	sp := all[m.rng.Intn(len(all))]
	level := routeLevelMin + m.rng.Intn(routeLevelMax-routeLevelMin+1)
	wild := mon.NewCreature(sp.Key, level)
	m.beginBattle(wild, true)
}

// Route ground tones (monochrome, lit like the plaza floor).
const (
	routePath  = canvas.Color(0x202020)
	routePathB = canvas.Color(0x262626)
	routeEdge  = canvas.Color(0x141414)
	routeGrass = canvas.Color(0x161616)
	routeGrasB = canvas.Color(0x1B1B1B)
)

func (m *model) drawRoute(pw, ph int, t float64) {
	r := m.route
	// Camera centered on the player, exactly like the plaza.
	csx, csy := iso.Project(float64(r.px), float64(r.py))
	originSx := csx - float64(pw)/2
	originSy := csy - float64(ph)/2
	project := func(gx, gy int) (int, int) {
		sx, sy := iso.Project(float64(gx), float64(gy))
		return int(sx - originSx), int(sy - originSy)
	}

	// Ground plane: a paved diamond per cell (grass tiles a touch darker, the
	// trodden path paler), drawn flat so order doesn't matter.
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			px, py := project(x, y)
			if px < -iso.TileW || px > pw+iso.TileW || py < -iso.TileH || py > ph+iso.TileH {
				continue
			}
			grass := r.tile(x, y) == ','
			fill := routePath
			if grass {
				fill = routeGrass
				if (x+y)&1 == 0 {
					fill = routeGrasB
				}
			} else if (x+y)&1 == 0 {
				fill = routePathB
			}
			iso.DrawDiamond(m.scr, px, py, fill, routeEdge)
			if grass {
				drawGrassTuft(m.scr, px, py+iso.HH, x, y, t)
			}
		}
	}

	// Tall things (trees and the player) drawn back-to-front so they occlude.
	type tall struct {
		depth  int
		tree   bool
		px, py int
	}
	var items []tall
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			if r.tile(x, y) != '#' {
				continue
			}
			px, py := project(x, y)
			if px < -60 || px > pw+60 || py < -80 || py > ph+80 {
				continue
			}
			items = append(items, tall{depth: iso.Depth(x, y), tree: true, px: px, py: py})
		}
	}
	ppx, ppy := project(r.px, r.py)
	items = append(items, tall{depth: iso.Depth(r.px, r.py), px: ppx, py: ppy})
	sort.Slice(items, func(i, j int) bool { return items[i].depth < items[j].depth })
	for _, it := range items {
		if it.tree {
			drawIsoTree(m.scr, it.px, it.py+iso.HH, it.px*3+it.py)
		} else {
			footX, footY := it.px, it.py+iso.HH
			drawContactShadow(m.scr, footX, footY)
			sprites.Draw(m.scr, footX, footY, r.facing, 0, false)
		}
	}

	// HUD.
	panel(m.scr, 16, 14, 200, 24)
	m.scr.DrawText(26, 22, "Wild Route", uiText)
	lead := ""
	if len(m.roster) > 0 {
		lead = m.roster[0].Name() + " Lv" + itoa(m.roster[0].Level)
	}
	m.scr.DrawText(pw-canvas.TextWidth(lead)-20, 22, lead, uiDim)
	hint := "WASD walk · search the grass · p team · esc leave"
	m.scr.DrawText(pw/2-canvas.TextWidth(hint)/2, ph-26, hint, uiDim)
}

// --- isometric route art (monochrome, top-lit like the plaza) ---

// drawGrassTuft scatters a few swaying blades around a tile's ground center.
func drawGrassTuft(c *canvas.Canvas, cx, cy, gx, gy int, t float64) {
	seed := gx*7 + gy*13
	for i := 0; i < 4; i++ {
		bx := cx - 14 + ((seed + i*13) % 28)
		by := cy - 4 + ((seed + i*5) % 8)
		sway := int(sinf(t*1.6+float64(seed+i)) * 1.5)
		tone := canvas.Color(0x3A3A3A)
		if i%2 == 0 {
			tone = canvas.Color(0x2E2E2E)
		}
		c.Set(bx, by, tone)
		c.Set(bx+sway/2, by-3, tone)
		c.Set(bx+sway, by-6, tone)
	}
}

// drawIsoTree draws a tree rising from the ground point (baseX, baseY): a trunk
// and a top-lit canopy, with a soft ground shadow.
func drawIsoTree(c *canvas.Canvas, baseX, baseY, seed int) {
	// Ground shadow.
	for dy := -4; dy <= 4; dy++ {
		w := int(16 * sqrtClamp(1-float64(dy*dy)/16.0))
		yy := baseY + dy - 1
		for dx := -w; dx <= w; dx++ {
			c.Set(baseX+dx, yy, c.At(baseX+dx, yy).Scale(0.6))
		}
	}
	c.FillRect(baseX-2, baseY-18, 4, 18, canvas.Color(0x3A2F2A)) // trunk
	// Canopy: two overlapping top-lit blobs for a fuller silhouette.
	canopy(c, baseX-5, baseY-26, 12)
	canopy(c, baseX+5, baseY-24, 11)
	canopy(c, baseX, baseY-32, 13)
}

func canopy(c *canvas.Canvas, cx, cy, r int) {
	for dy := -r; dy <= r; dy++ {
		w := int(float64(r) * sqrtClamp(1-float64(dy*dy)/float64(r*r)))
		v := float64(dy+r) / float64(2*r)
		tone := canvas.Color(0x5E5E5E)
		switch {
		case v < 0.28:
			tone = canvas.Color(0x7A7A7A)
		case v > 0.72:
			tone = canvas.Color(0x3C3C3C)
		}
		for dx := -w; dx <= w; dx++ {
			col := tone
			if dx < -w/3 {
				col = canvas.Color(0x3C3C3C)
			}
			c.Set(cx+dx, cy+dy, col)
		}
	}
}

func drawContactShadow(c *canvas.Canvas, footX, footY int) {
	for dy := -3; dy <= 3; dy++ {
		w := int(9 * sqrtClamp(1-float64(dy*dy)/9.0))
		yy := footY + dy - 1
		for dx := -w; dx <= w; dx++ {
			c.Set(footX+dx, yy, c.At(footX+dx, yy).Scale(0.5))
		}
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
