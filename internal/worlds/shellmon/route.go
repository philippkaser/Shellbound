package shellmon

import (
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/sprites"
	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// Route tuning.
const (
	routeTile     = 40   // tile size in pixels
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

func (m *model) drawRoute(pw, ph int, t float64) {
	r := m.route
	fieldW, fieldH := r.w*routeTile, r.h*routeTile
	ox := (pw - fieldW) / 2
	oy := (ph-fieldH)/2 - 6

	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			tx, ty := ox+x*routeTile, oy+y*routeTile
			switch r.tile(x, y) {
			case '#':
				drawTree(m.scr, tx, ty)
			case ',':
				drawGrass(m.scr, tx, ty, x, y)
			default:
				drawGround(m.scr, tx, ty, x, y)
			}
		}
	}

	// The player, standing front-and-center on their tile.
	footX := ox + r.px*routeTile + routeTile/2
	footY := oy + r.py*routeTile + routeTile - 6
	drawContactShadow(m.scr, footX, footY)
	sprites.Draw(m.scr, footX, footY, r.facing, 0, false)

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

// --- route tile art (top-down, monochrome) ---

func drawGround(c *canvas.Canvas, tx, ty, gx, gy int) {
	tone := canvas.Color(0x161616)
	if (gx+gy)&1 == 0 {
		tone = canvas.Color(0x1B1B1B)
	}
	c.FillRect(tx, ty, routeTile, routeTile, tone)
}

func drawGrass(c *canvas.Canvas, tx, ty, gx, gy int) {
	drawGround(c, tx, ty, gx, gy)
	// A few blades per tile, arranged from a deterministic per-tile pattern.
	seed := gx*7 + gy*13
	for i := 0; i < 5; i++ {
		bx := tx + 6 + ((seed + i*11) % (routeTile - 12))
		by := ty + routeTile - 6 - ((seed + i*7) % 10)
		tone := canvas.Color(0x3A3A3A)
		if i%2 == 0 {
			tone = canvas.Color(0x2A2A2A)
		}
		c.VLine(bx, by-7, by, tone)
		c.Set(bx-1, by-3, tone)
		c.Set(bx+1, by-5, tone)
	}
}

func drawTree(c *canvas.Canvas, tx, ty int) {
	cx := tx + routeTile/2
	baseY := ty + routeTile - 6
	c.FillRect(cx-2, baseY-10, 4, 12, canvas.Color(0x3A2F2A)) // trunk
	// Canopy: a top-lit dark-grey blob.
	cyc := ty + routeTile/2 - 2
	for dy := -14; dy <= 12; dy++ {
		w := int(float64(15) * sqrtClamp(1-float64(dy*dy)/float64(14*14)))
		v := float64(dy+14) / 28
		tone := canvas.Color(0x5A5A5A)
		if v < 0.3 {
			tone = canvas.Color(0x767676)
		} else if v > 0.7 {
			tone = canvas.Color(0x3E3E3E)
		}
		for dx := -w; dx <= w; dx++ {
			col := tone
			if dx < -w/3 {
				col = canvas.Color(0x3E3E3E)
			}
			c.Set(cx+dx, cyc+dy, col)
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
