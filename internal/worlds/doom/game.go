// Package doom is Shellbound's Doom portal: a first-person raycast shooter
// rendered into the Sixel canvas. You stalk a pillared hall, gun down the imps
// (their eyes glow in the portal's hue) and clear them all to earn a trophy. It
// uses the same render environment as the plaza — palette, session writer and
// cell size off world.Context.Render — and the shared screen viewport.
package doom

import "math"

// Map dimensions.
const (
	dw = 24
	dh = 24
)

// Tuning.
const (
	moveStep   = 0.22 // forward/back per step
	strafeStep = 0.18
	turnStep   = 0.13 // radians per turn
	fov        = 0.66 // camera-plane half-width (~66° field of view)

	enemySpeed    = 0.055
	separation    = 0.6 // imps repel inside this range so they fan out
	contactRange  = 0.7
	lungeRange    = 1.8 // an imp rears up to strike inside this range
	contactDmg    = 7
	hurtCoolTicks = 10
	muzzleTicks   = 5
	hitMarkTicks  = 4 // crosshair flash frames after a connecting shot
	deathTicks    = 9 // frames an imp spends collapsing after a kill
	startHealth   = 100
	shotDamage    = 50
	aimDot        = 0.985 // cos of the aim cone half-angle (~10°)
)

type runState int

const (
	playing runState = iota
	won
	lost
)

type enemy struct {
	x, y  float64
	hp    int
	alive bool
	hurt  int     // brief flash after being hit
	dying int     // collapse-and-fade countdown after a kill (0 = gone)
	phase float64 // per-imp offset so idle bobs aren't in lockstep
}

// game is the pure FPS state; the renderer reads it but never mutates it.
type game struct {
	walls   [dh][dw]bool
	torches [dh][dw]bool // wall cells bearing a lit torch
	enemies []enemy

	posX, posY     float64
	dirX, dirY     float64
	planeX, planeY float64

	health       int
	kills, total int
	hurtCool     int
	muzzle       int
	hitMark      int // crosshair hit-marker countdown
	steps        int // move count, drives the weapon/view bob
	state        runState
}

func newGame() *game {
	g := &game{
		health: startHealth,
		dirX:   1, dirY: 0,
		planeX: 0, planeY: fov,
		state: playing,
	}
	g.build()
	return g
}

func (g *game) build() {
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			if x == 0 || y == 0 || x == dw-1 || y == dh-1 {
				g.walls[y][x] = true
			}
		}
	}
	// Pillars and a low central divider for cover (x, y, w, h).
	blocks := [][4]int{
		{5, 5, 2, 2}, {17, 5, 2, 2}, {5, 17, 2, 2}, {17, 17, 2, 2},
		{11, 3, 2, 2}, {11, 19, 2, 2}, {3, 11, 2, 2}, {19, 11, 2, 2},
		{10, 10, 4, 1}, {10, 13, 4, 1},
	}
	for _, b := range blocks {
		for y := b[1]; y < b[1]+b[3]; y++ {
			for x := b[0]; x < b[0]+b[2]; x++ {
				if inb(x, y) {
					g.walls[y][x] = true
				}
			}
		}
	}
	for i, s := range [][2]float64{
		{12.5, 7.5}, {7.5, 12.5}, {16.5, 12.5}, {12.5, 16.5}, {3.5, 21.5}, {20.5, 21.5},
	} {
		g.enemies = append(g.enemies, enemy{x: s[0], y: s[1], hp: 100, alive: true, phase: float64(i) * 1.7})
	}
	g.total = len(g.enemies)
	g.posX, g.posY = 2.5, 2.5

	// Torches bracketed along the walls — they light the hall in the portal hue.
	for _, tc := range [][2]int{
		{0, 6}, {0, 12}, {0, 18}, {23, 6}, {23, 12}, {23, 18},
		{6, 0}, {12, 0}, {18, 0}, {6, 23}, {12, 23}, {18, 23},
	} {
		if inb(tc[0], tc[1]) {
			g.torches[tc[1]][tc[0]] = true
		}
	}
}

func inb(x, y int) bool { return x >= 0 && y >= 0 && x < dw && y < dh }

func (g *game) wall(x, y float64) bool {
	ix, iy := int(x), int(y)
	if !inb(ix, iy) {
		return true
	}
	return g.walls[iy][ix]
}

// tryMove slides along walls (axis-separated) like the plaza.
func (g *game) tryMove(nx, ny float64) {
	if !g.wall(nx, g.posY) {
		g.posX = nx
	}
	if !g.wall(g.posX, ny) {
		g.posY = ny
	}
}

func (g *game) forward(d float64) { g.steps++; g.tryMove(g.posX+g.dirX*d, g.posY+g.dirY*d) }
func (g *game) strafe(d float64)  { g.steps++; g.tryMove(g.posX+g.dirY*d, g.posY-g.dirX*d) }

func (g *game) turn(a float64) {
	cs, sn := math.Cos(a), math.Sin(a)
	g.dirX, g.dirY = g.dirX*cs-g.dirY*sn, g.dirX*sn+g.dirY*cs
	g.planeX, g.planeY = g.planeX*cs-g.planeY*sn, g.planeX*sn+g.planeY*cs
}

// fire is a hitscan down the crosshair: the nearest alive imp inside the aim
// cone with a clear line of sight takes a hit.
func (g *game) fire() {
	if g.state != playing {
		return
	}
	g.muzzle = muzzleTicks
	best, bestDist := -1, math.MaxFloat64
	for i := range g.enemies {
		e := &g.enemies[i]
		if !e.alive {
			continue
		}
		dx, dy := e.x-g.posX, e.y-g.posY
		dist := math.Hypot(dx, dy)
		if dist < 1e-3 {
			continue
		}
		if (dx*g.dirX+dy*g.dirY)/dist < aimDot {
			continue
		}
		if !g.los(g.posX, g.posY, e.x, e.y) {
			continue
		}
		if dist < bestDist {
			best, bestDist = i, dist
		}
	}
	if best >= 0 {
		g.hitMark = hitMarkTicks
		g.enemies[best].hp -= shotDamage
		g.enemies[best].hurt = 3
		if g.enemies[best].hp <= 0 {
			g.enemies[best].alive = false
			g.enemies[best].dying = deathTicks
			g.kills++
		}
	}
}

// los reports a clear straight path between two points (no wall in between).
func (g *game) los(ax, ay, bx, by float64) bool {
	dx, dy := bx-ax, by-ay
	steps := int(math.Hypot(dx, dy) / 0.05)
	for i := 1; i < steps; i++ {
		t := float64(i) / float64(steps)
		if g.wall(ax+dx*t, ay+dy*t) {
			return false
		}
	}
	return true
}

// tick advances imps and resolves contact damage and win/lose.
func (g *game) tick() {
	if g.state != playing {
		return
	}
	if g.hurtCool > 0 {
		g.hurtCool--
	}
	if g.muzzle > 0 {
		g.muzzle--
	}
	if g.hitMark > 0 {
		g.hitMark--
	}
	for i := range g.enemies {
		e := &g.enemies[i]
		if !e.alive {
			if e.dying > 0 {
				e.dying-- // finish the collapse animation, then it's gone
			}
			continue
		}
		if e.hurt > 0 {
			e.hurt--
		}
		dx, dy := g.posX-e.x, g.posY-e.y
		dist := math.Hypot(dx, dy)
		if dist < contactRange {
			if g.hurtCool == 0 {
				g.health -= contactDmg
				g.hurtCool = hurtCoolTicks
				if g.health <= 0 {
					g.health = 0
					g.state = lost
				}
			}
			continue
		}
		mx, my := 0.0, 0.0
		if dist > 0 && g.los(e.x, e.y, g.posX, g.posY) {
			mx, my = dx/dist*enemySpeed, dy/dist*enemySpeed
		}
		// Separation: imps repel each other so a pack fans out into a front
		// instead of stacking on one point.
		for j := range g.enemies {
			if j == i || !g.enemies[j].alive {
				continue
			}
			sx, sy := e.x-g.enemies[j].x, e.y-g.enemies[j].y
			d := math.Hypot(sx, sy)
			if d > 1e-4 && d < separation {
				push := (separation - d) / separation * enemySpeed
				mx += sx / d * push
				my += sy / d * push
			}
		}
		if mx != 0 || my != 0 {
			nx, ny := e.x+mx, e.y+my
			if !g.wall(nx, e.y) {
				e.x = nx
			}
			if !g.wall(e.x, ny) {
				e.y = ny
			}
		}
	}
	if g.state == playing && g.kills >= g.total {
		g.state = won
	}
}

func (g *game) restart() { *g = *newGame() }

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
