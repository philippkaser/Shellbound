package shellmon

import (
	"sort"
	"time"

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

// npc is a non-player character standing on the route, with a flavor line shown
// when the player bumps into them.
type npc struct {
	x, y   int
	name   string
	line   string
	facing sprites.Facing
}

// routeState is the walkable field and the player's position on it.
type routeState struct {
	w, h     int
	tiles    []byte
	px, py   int
	facing   sprites.Facing
	npcs     []npc
	lastStep time.Time // for the walk animation
}

// routeNPCs are the wanderers dotted around the route (placed on open grass).
var routeNPCs = []npc{
	{x: 4, y: 3, name: "Hiker Bram", line: "The tall grass is thick with wild Shellmon — wade in!", facing: sprites.FaceRight},
	{x: 14, y: 4, name: "Ranger Mossa", line: "Spark singes Bramble, Bramble drinks Tide, Tide douses Spark.", facing: sprites.FaceLeft},
	{x: 9, y: 7, name: "Kid Pip", line: "A wild Gulper ate my sandwich once. Worth it.", facing: sprites.FaceUp},
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
	// Sprinkle scenery onto open grass: occasional rocks (block) and flower
	// clusters (decorative), placed deterministically and kept off the entrance.
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			if r.tiles[y*w+x] != ',' {
				continue
			}
			if iabs(x-r.px) <= 1 && iabs(y-r.py) <= 1 {
				continue
			}
			switch {
			case (x*7+y*5)%23 == 0:
				r.tiles[y*w+x] = 'o' // rock
			case (x*5+y*9)%19 == 0:
				r.tiles[y*w+x] = 'f' // flowers
			}
		}
	}
	r.npcs = append([]npc(nil), routeNPCs...)
	for _, n := range r.npcs {
		if n.x > 0 && n.y > 0 && n.x < w-1 && n.y < h-1 {
			r.tiles[n.y*w+n.x] = '.' // stand on clear ground
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

// npcAt returns the NPC standing on a cell, if any.
func (r *routeState) npcAt(x, y int) *npc {
	for i := range r.npcs {
		if r.npcs[i].x == x && r.npcs[i].y == y {
			return &r.npcs[i]
		}
	}
	return nil
}

// blocks reports whether a cell stops movement (trees, rocks, NPCs).
func (r *routeState) blocks(x, y int) bool {
	switch r.tile(x, y) {
	case '#', 'o':
		return true
	}
	return r.npcAt(x, y) != nil
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
	if n := r.npcAt(nx, ny); n != nil {
		m.routeMsg, m.routeMsgAt = n.name+": "+n.line, time.Now()
		return false // bump into the NPC: chat, don't move
	}
	if r.blocks(nx, ny) {
		return false // trees and rocks block
	}
	r.px, r.py = nx, ny
	r.lastStep = time.Now()
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
			tile := r.tile(x, y)
			grass := tile == ',' || tile == 'f'
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
			switch tile {
			case ',':
				drawTallGrass(m.scr, px, py+iso.HH, x, y, t)
			case 'f':
				drawFlowers(m.scr, px, py+iso.HH, x, y, t)
			}
		}
	}

	// Tall things (trees, rocks, NPCs and the player) drawn back-to-front.
	const (
		kindPlayer = iota
		kindTree
		kindRock
		kindNPC
	)
	type tall struct {
		depth  int
		kind   int
		px, py int
		n      *npc
	}
	var items []tall
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			k := -1
			switch r.tile(x, y) {
			case '#':
				k = kindTree
			case 'o':
				k = kindRock
			}
			if k < 0 {
				continue
			}
			px, py := project(x, y)
			if px < -60 || px > pw+60 || py < -80 || py > ph+80 {
				continue
			}
			items = append(items, tall{depth: iso.Depth(x, y), kind: k, px: px, py: py})
		}
	}
	for i := range r.npcs {
		n := &r.npcs[i]
		px, py := project(n.x, n.y)
		items = append(items, tall{depth: iso.Depth(n.x, n.y), kind: kindNPC, px: px, py: py, n: n})
	}
	ppx, ppy := project(r.px, r.py)
	items = append(items, tall{depth: iso.Depth(r.px, r.py), kind: kindPlayer, px: ppx, py: ppy})
	sort.Slice(items, func(i, j int) bool { return items[i].depth < items[j].depth })
	for _, it := range items {
		footX, footY := it.px, it.py+iso.HH
		switch it.kind {
		case kindTree:
			drawIsoTree(m.scr, footX, footY, it.px*3+it.py)
		case kindRock:
			drawRock(m.scr, footX, footY)
		case kindNPC:
			_, nbob := sprites.Pose(t, false, float64(it.n.x+it.n.y)) // gentle idle breathing
			drawContactShadow(m.scr, footX, footY)
			sprites.Draw(m.scr, footX, footY+nbob, it.n.facing, 0, false)
			nameW := canvas.TextWidth(it.n.name)
			m.scr.DrawTextShadow(footX-nameW/2, footY+nbob-sprites.Height-canvas.LineH, it.n.name, 0xB8B8B8, 0x000000)
		default:
			// The player animates exactly like the plaza avatar (shared
			// sprites.Pose): a walk just after a step, an idle bob otherwise.
			moving := time.Since(r.lastStep) < 280*time.Millisecond
			frame, bob := sprites.Pose(t, moving, 0)
			drawContactShadow(m.scr, footX, footY)
			sprites.Draw(m.scr, footX, footY+bob, r.facing, frame, moving)
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

	// A recently bumped NPC's line, in a speech panel near the bottom.
	if m.routeMsg != "" && time.Since(m.routeMsgAt) < 4*time.Second {
		w := canvas.TextWidth(m.routeMsg) + 24
		x := pw/2 - w/2
		panel(m.scr, x, ph-64, w, 26)
		m.scr.DrawText(x+12, ph-56, m.routeMsg, uiText)
	}
}

// --- isometric route art (monochrome, top-lit like the plaza) ---

// drawTallGrass paints a lush, swaying clump of blades on a grass tile — taller
// and denser than mere texture, so "wild grass" reads as somewhere to search.
func drawTallGrass(c *canvas.Canvas, cx, cy, gx, gy int, t float64) {
	seed := gx*7 + gy*13
	for i := 0; i < 9; i++ {
		bx := cx - 16 + ((seed + i*9) % 32)
		base := cy - 2 + ((seed + i*5) % 7)
		hgt := 7 + (seed+i*3)%5
		sway := sinf(t*1.7 + float64(seed+i)*0.6)
		tone := canvas.Color(0x444444)
		if i%3 == 0 {
			tone = canvas.Color(0x303030)
		}
		// a curved blade: each segment leans a touch further with the wind
		for s := 0; s <= hgt; s++ {
			x := bx + int(sway*float64(s)*0.35)
			c.Set(x, base-s, tone)
		}
		c.Set(bx+int(sway*float64(hgt)*0.4), base-hgt-1, canvas.Color(0x5A5A5A)) // tip catch-light
	}
}

// drawFlowers paints a small cluster of blossoms on a decorative tile.
func drawFlowers(c *canvas.Canvas, cx, cy, gx, gy int, t float64) {
	seed := gx*5 + gy*11
	for i := 0; i < 3; i++ {
		fx := cx - 10 + ((seed + i*13) % 20)
		fy := cy - 2 + ((seed + i*7) % 6)
		c.VLine(fx, fy, fy+4, canvas.Color(0x3A3A3A)) // stem
		// four petals around a bright center
		c.Set(fx, fy-2, canvas.Color(0xCFCFCF))
		c.Set(fx-1, fy-1, canvas.Color(0x9A9A9A))
		c.Set(fx+1, fy-1, canvas.Color(0x9A9A9A))
		c.Set(fx, fy, canvas.Color(0xF2F2F2))
		c.Set(fx, fy-1, canvas.Color(0xEDEDED))
	}
}

// drawRock paints a small top-lit boulder sitting on the ground point.
func drawRock(c *canvas.Canvas, baseX, baseY int) {
	for dy := -10; dy <= 0; dy++ {
		w := int(float64(12) * sqrtClamp(1-float64(dy*dy)/float64(11*11)))
		v := float64(dy+10) / 12
		tone := canvas.Color(0x8A8A8A)
		switch {
		case v < 0.3:
			tone = canvas.Color(0xAEAEAE)
		case v > 0.7:
			tone = canvas.Color(0x5E5E5E)
		}
		for dx := -w; dx <= w; dx++ {
			col := tone
			if dx < -w/3 {
				col = canvas.Color(0x5E5E5E)
			}
			c.Set(baseX+dx, baseY-2+dy, col)
		}
	}
	// a couple of cracks
	c.Set(baseX-1, baseY-6, canvas.Color(0x4A4A4A))
	c.Set(baseX, baseY-5, canvas.Color(0x4A4A4A))
	c.Set(baseX+1, baseY-7, canvas.Color(0x4A4A4A))
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
