// Package bomber is Shellbound's first real portal world: "The Vault", a
// single-player bomb arena reached through the Bomberman portal. You plant
// charges to shatter crates and the wandering wisps; clear them all and a Spark
// Core is granted to your inventory. It renders with the same Sixel/isometric/
// lighting stack as the plaza, and paints its accents in the portal's own hue
// so the world and its gateway look like one place.
package bomber

import "math/rand"

// Arena dimensions in cells (odd, so the indestructible lattice frames it).
const (
	cols = 15
	rows = 13
)

// Cell kinds.
const (
	floor = iota
	wall  // indestructible
	crate // destructible
)

// Powerup kinds dropped (sometimes) when a crate shatters.
const (
	powNone  = iota
	powBomb  // +1 simultaneous bomb
	powReach // +1 blast reach
)

// Facings, matching sprites.Facing order (down, up, left, right).
const (
	faceDown = iota
	faceUp
	faceLeft
	faceRight
)

// Timing, in logic ticks (the model ticks at tickEvery).
const (
	fuseTicks  = 40 // ~2.0s before a bomb blows
	blastTicks = 9  // how long a blast cell stays lethal
	enemyCool  = 12 // ticks between enemy steps
	respawnInv = 24 // post-respawn safety ticks
	shakeTicks = 5  // camera-jolt frames after a detonation
	enemyCount = 4
)

type runState int

const (
	playing runState = iota
	won
	lost
)

type bomb struct {
	x, y, fuse, reach int
	gone              bool
}

type enemy struct {
	x, y   int
	fx, fy float64 // render-interpolated position
	cool   int
	pace   int // ticks between steps — varied per wisp so the pack staggers
}

// game is the pure arena state and rules; rendering reads it but never mutates.
type game struct {
	grid  [rows][cols]int
	power [rows][cols]int
	blast [rows][cols]int // ticks of lethal blast remaining on a cell

	bombs   []bomb
	enemies []enemy

	px, py   int     // player grid cell
	pfx, pfy float64 // render-interpolated player position
	face     int

	bombsMax int
	reach    int
	lives    int
	invuln   int
	shake    int // camera-jolt ticks after a detonation or a hit
	state    runState

	rnd *rand.Rand
}

func newGame(seed int64) *game {
	g := &game{
		bombsMax: 1,
		reach:    2,
		lives:    3,
		px:       1, py: 1,
		pfx: 1, pfy: 1,
		face:  faceDown,
		state: playing,
		rnd:   rand.New(rand.NewSource(seed)),
	}
	g.build()
	return g
}

// build lays out walls, scatters crates (keeping the spawn corner open) and
// places the wisps on far floor cells.
func (g *game) build() {
	safe := map[[2]int]bool{{1, 1}: true, {2, 1}: true, {1, 2}: true}
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			border := x == 0 || y == 0 || x == cols-1 || y == rows-1
			lattice := x%2 == 0 && y%2 == 0
			switch {
			case border || lattice:
				g.grid[y][x] = wall
			case !safe[[2]int{x, y}] && g.rnd.Float64() < 0.45:
				g.grid[y][x] = crate
			default:
				g.grid[y][x] = floor
			}
		}
	}
	for placed := 0; placed < enemyCount; {
		x := 1 + g.rnd.Intn(cols-2)
		y := 1 + g.rnd.Intn(rows-2)
		if g.grid[y][x] != floor || abs(x-1)+abs(y-1) < 5 || g.enemyAt(x, y) != nil {
			continue
		}
		pace := enemyCool - 2 + g.rnd.Intn(6) // staggered speeds per wisp
		g.enemies = append(g.enemies, enemy{x: x, y: y, fx: float64(x), fy: float64(y), cool: pace, pace: pace})
		placed++
	}
}

func (g *game) inBounds(x, y int) bool { return x >= 0 && y >= 0 && x < cols && y < rows }

// solid reports an indestructible wall (stops blasts and movement).
func (g *game) solid(x, y int) bool { return !g.inBounds(x, y) || g.grid[y][x] == wall }

// blockedForMove reports whether a walker can enter (x, y).
func (g *game) blockedForMove(x, y int) bool {
	if !g.inBounds(x, y) || g.grid[y][x] != floor {
		return true
	}
	return g.bombAt(x, y) != nil
}

func (g *game) bombAt(x, y int) *bomb {
	for i := range g.bombs {
		if !g.bombs[i].gone && g.bombs[i].x == x && g.bombs[i].y == y {
			return &g.bombs[i]
		}
	}
	return nil
}

func (g *game) enemyAt(x, y int) *enemy {
	for i := range g.enemies {
		if g.enemies[i].x == x && g.enemies[i].y == y {
			return &g.enemies[i]
		}
	}
	return nil
}

func (g *game) activeBombs() int {
	n := 0
	for i := range g.bombs {
		if !g.bombs[i].gone {
			n++
		}
	}
	return n
}

// move turns to face (dx, dy) and steps there if it's open. Returns whether it
// actually moved.
func (g *game) move(dx, dy int) bool {
	switch {
	case dx < 0:
		g.face = faceLeft
	case dx > 0:
		g.face = faceRight
	case dy < 0:
		g.face = faceUp
	case dy > 0:
		g.face = faceDown
	}
	nx, ny := g.px+dx, g.py+dy
	if g.blockedForMove(nx, ny) {
		return false
	}
	g.px, g.py = nx, ny
	return true
}

// plant drops a bomb on the player's cell if one isn't there and the bomb
// budget allows.
func (g *game) plant() bool {
	if g.state != playing || g.activeBombs() >= g.bombsMax || g.bombAt(g.px, g.py) != nil {
		return false
	}
	g.bombs = append(g.bombs, bomb{x: g.px, y: g.py, fuse: fuseTicks, reach: g.reach})
	return true
}

// tick advances the simulation one step.
func (g *game) tick() {
	if g.state != playing {
		return
	}
	if g.invuln > 0 {
		g.invuln--
	}
	if g.shake > 0 {
		g.shake--
	}
	// Age existing blasts.
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			if g.blast[y][x] > 0 {
				g.blast[y][x]--
			}
		}
	}
	// Fuses; detonate any that reached zero (chains handled inside).
	var initial []int
	for i := range g.bombs {
		if g.bombs[i].gone {
			continue
		}
		g.bombs[i].fuse--
		if g.bombs[i].fuse <= 0 {
			initial = append(initial, i)
		}
	}
	if len(initial) > 0 {
		g.detonate(initial)
		g.shake = shakeTicks
	}
	g.compactBombs()
	g.moveEnemies()
	g.damage()
	g.pickup()
	if g.state == playing && len(g.enemies) == 0 {
		g.state = won
	}
}

// detonate explodes the given bombs and any caught in their blasts (a BFS chain
// reaction), stamping lethal cells and shattering crates.
func (g *game) detonate(initial []int) {
	queue := make([]int, 0, len(initial))
	for _, i := range initial {
		if !g.bombs[i].gone {
			g.bombs[i].gone = true
			queue = append(queue, i)
		}
	}
	dirs := [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for len(queue) > 0 {
		b := g.bombs[queue[0]]
		queue = queue[1:]
		g.stampBlast(b.x, b.y)
		for _, d := range dirs {
			for i := 1; i <= b.reach; i++ {
				x, y := b.x+d[0]*i, b.y+d[1]*i
				if g.solid(x, y) {
					break
				}
				if g.grid[y][x] == crate {
					g.shatter(x, y)
					g.stampBlast(x, y)
					break
				}
				g.stampBlast(x, y)
				if nb := g.bombAt(x, y); nb != nil {
					for j := range g.bombs {
						if &g.bombs[j] == nb && !g.bombs[j].gone {
							g.bombs[j].gone = true
							queue = append(queue, j)
						}
					}
				}
			}
		}
	}
}

func (g *game) stampBlast(x, y int) {
	if g.inBounds(x, y) {
		g.blast[y][x] = blastTicks
	}
}

// shatter turns a crate to floor and sometimes reveals a powerup.
func (g *game) shatter(x, y int) {
	g.grid[y][x] = floor
	if r := g.rnd.Float64(); r < 0.16 {
		g.power[y][x] = powBomb
	} else if r < 0.30 {
		g.power[y][x] = powReach
	}
}

func (g *game) compactBombs() {
	kept := g.bombs[:0]
	for _, b := range g.bombs {
		if !b.gone {
			kept = append(kept, b)
		}
	}
	g.bombs = kept
}

// pursuitRange is the Manhattan distance inside which a wisp hunts the player.
const pursuitRange = 7

// dangerous reports whether a cell is (or is about to be) lethal: an active
// blast, or in the cross of a bomb whose fuse is running out.
func (g *game) dangerous(x, y int) bool {
	if !g.inBounds(x, y) {
		return true
	}
	if g.blast[y][x] > 0 {
		return true
	}
	for i := range g.bombs {
		b := &g.bombs[i]
		if b.gone || b.fuse > fuseTicks/2 {
			continue
		}
		if b.x == x && b.y == y {
			return true
		}
		// Same row or column within reach, with no wall between.
		if b.y == y && abs(b.x-x) <= b.reach {
			clear := true
			step := sign(x - b.x)
			for cx := b.x + step; cx != x; cx += step {
				if g.solid(cx, y) || g.grid[y][cx] == crate {
					clear = false
					break
				}
			}
			if clear {
				return true
			}
		}
		if b.x == x && abs(b.y-y) <= b.reach {
			clear := true
			step := sign(y - b.y)
			for cy := b.y + step; cy != y; cy += step {
				if g.solid(x, cy) || g.grid[cy][x] == crate {
					clear = false
					break
				}
			}
			if clear {
				return true
			}
		}
	}
	return false
}

// moveEnemies steps each wisp on its own cadence. A wisp flees a cell that is
// about to blow, hunts the player when they're near, and otherwise wanders —
// so the arena has pressure without the wisps reading as psychic.
func (g *game) moveEnemies() {
	dirs := [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for i := range g.enemies {
		e := &g.enemies[i]
		if e.cool > 0 {
			e.cool--
			continue
		}
		e.cool = e.pace
		type cand struct {
			x, y, dist int
			danger     bool
		}
		var cands []cand
		for _, k := range g.rnd.Perm(4) {
			nx, ny := e.x+dirs[k][0], e.y+dirs[k][1]
			if g.blockedForMove(nx, ny) || g.enemyAt(nx, ny) != nil {
				continue
			}
			cands = append(cands, cand{nx, ny, abs(nx-g.px) + abs(ny-g.py), g.dangerous(nx, ny)})
		}
		if len(cands) == 0 {
			continue
		}
		// If safe moves exist, never step into danger.
		safe := cands[:0]
		for _, c := range cands {
			if !c.danger {
				safe = append(safe, c)
			}
		}
		pool := cands
		if len(safe) > 0 {
			pool = safe
		}
		// Standing on a fuse: sprint to any safe cell (nearest-to-player bias
		// off, survival first). Otherwise hunt when the player is close.
		best := pool[0]
		if !g.dangerous(e.x, e.y) && abs(e.x-g.px)+abs(e.y-g.py) <= pursuitRange {
			for _, c := range pool[1:] {
				if c.dist < best.dist {
					best = c
				}
			}
		}
		e.x, e.y = best.x, best.y
	}
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// damage applies blasts and contact to the player and wisps.
func (g *game) damage() {
	// Wisps caught in a blast die.
	kept := g.enemies[:0]
	for _, e := range g.enemies {
		if g.blast[e.y][e.x] > 0 {
			continue
		}
		kept = append(kept, e)
	}
	g.enemies = kept

	if g.invuln > 0 {
		return
	}
	if g.blast[g.py][g.px] > 0 || g.enemyAt(g.px, g.py) != nil {
		g.hit()
	}
}

func (g *game) hit() {
	g.lives--
	g.shake = shakeTicks // the hit itself also jolts the camera
	if g.lives <= 0 {
		g.lives = 0
		g.state = lost
		return
	}
	// Respawn at the safest cell near the spawn corner rather than blindly
	// at (1,1), which could drop the player straight onto a wisp or a blast.
	g.px, g.py = g.safeSpawn()
	g.pfx, g.pfy = float64(g.px), float64(g.py)
	g.invuln = respawnInv
}

// safeSpawn breadth-first-searches from the spawn corner for the nearest
// floor cell with no live blast, no imminent bomb cross, and no wisp within
// two cells. Falls back to (1,1) if the whole arena is on fire.
func (g *game) safeSpawn() (int, int) {
	type pt struct{ x, y int }
	visited := map[pt]bool{{1, 1}: true}
	queue := []pt{{1, 1}}
	dirs := [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if g.grid[p.y][p.x] == floor && !g.dangerous(p.x, p.y) && g.bombAt(p.x, p.y) == nil {
			clear := true
			for i := range g.enemies {
				if abs(g.enemies[i].x-p.x)+abs(g.enemies[i].y-p.y) < 3 {
					clear = false
					break
				}
			}
			if clear {
				return p.x, p.y
			}
		}
		for _, d := range dirs {
			n := pt{p.x + d[0], p.y + d[1]}
			if g.inBounds(n.x, n.y) && !visited[n] && g.grid[n.y][n.x] != wall {
				visited[n] = true
				queue = append(queue, n)
			}
		}
	}
	return 1, 1
}

func (g *game) pickup() {
	switch g.power[g.py][g.px] {
	case powBomb:
		g.bombsMax++
	case powReach:
		g.reach++
	default:
		return
	}
	g.power[g.py][g.px] = powNone
}

// restart rebuilds a fresh arena, keeping the same RNG stream for variety.
func (g *game) restart() {
	seed := g.rnd.Int63()
	*g = *newGame(seed)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
