package shellmon

import (
	"sort"
	"time"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
	"github.com/shellbound/shellbound/internal/render/sprites"
	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// encounterRate is the chance per step into tall grass of a wild battle.
const encounterRate = 0.16

// npc is a non-player character standing on the route, with a flavor line shown
// when the player bumps into them.
type npc struct {
	x, y   int
	name   string
	line   string
	facing sprites.Facing
}

// routeState is one walkable area (a town or route): its tiles, the player's
// position, and the entities/links/secrets placed on it.
type routeState struct {
	key, name string
	w, h      int
	tiles     []byte
	px, py    int
	facing    sprites.Facing
	lastStep  time.Time // for the walk animation
	hopAt     time.Time // start of a ledge-hop arc

	npcs     []npc
	trainers []trainer
	signs    []sign
	warps    []warp
	items    []hiddenItem

	encounters     bool
	lvlMin, lvlMax int
	wildPool       []string
	rareSpecies    string
	rareLevel      int
	rareChance     float64
	spawnX, spawnY int
}

// enterArea builds an area and places the player at (x, y), or the area's
// default spawn when x < 0. It plays the diamond wipe to cover the swap.
func (m *model) enterArea(key string, x, y int) {
	a := buildArea(key)
	if x < 0 {
		x, y = a.spawnX, a.spawnY
	}
	a.px, a.py = x, y
	m.route = a
	m.state = stateRoute
	m.trans.begin(0.4)
}

func (r *routeState) tile(x, y int) byte {
	if x < 0 || y < 0 || x >= r.w || y >= r.h {
		return '#'
	}
	return r.tiles[y*r.w+x]
}

// landAdjacent reports whether a cell borders walkable land (sand, grass, path,
// flowers) — used to draw surf only where water meets the shore.
func (r *routeState) landAdjacent(x, y int) bool {
	for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		switch r.tile(x+d[0], y+d[1]) {
		case 's', 'g', '.', 'f', ',':
			return true
		}
	}
	return false
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

// trainerAt returns the trainer standing on a cell, if any.
func (r *routeState) trainerAt(x, y int) *trainer {
	for i := range r.trainers {
		if r.trainers[i].x == x && r.trainers[i].y == y {
			return &r.trainers[i]
		}
	}
	return nil
}

// signAt returns the sign at a cell, if any.
func (r *routeState) signAt(x, y int) *sign {
	for i := range r.signs {
		if r.signs[i].x == x && r.signs[i].y == y {
			return &r.signs[i]
		}
	}
	return nil
}

// warpAt returns the warp on a cell, if any.
func (r *routeState) warpAt(x, y int) *warp {
	for i := range r.warps {
		if r.warps[i].x == x && r.warps[i].y == y {
			return &r.warps[i]
		}
	}
	return nil
}

// blocks reports whether a cell stops movement (trees, rocks, water, buildings,
// landmarks, ledges and standing NPCs/trainers/signs). Ledges block ordinary
// steps; keyRoute handles the special downward hop before this check.
func (r *routeState) blocks(x, y int) bool {
	if solidTile(r.tile(x, y)) {
		return true
	}
	return r.npcAt(x, y) != nil || r.trainerAt(x, y) != nil || r.signAt(x, y) != nil
}

// keyRoute handles overworld input; returns true to leave the world.
func (m *model) keyRoute(key string) bool {
	r := m.route
	switch key {
	case "p":
		m.partyCur = 0
		m.state = stateParty
		return false
	case "esc", "q":
		return true
	}
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
	default:
		return false
	}
	if dx == 0 && dy == 0 {
		return false
	}
	nx, ny := r.px+dx, r.py+dy

	// Facing into a person or sign talks to them (no step).
	if n := r.npcAt(nx, ny); n != nil {
		m.say(n.name + ": " + n.line)
		return false
	}
	if tr := r.trainerAt(nx, ny); tr != nil {
		if m.defeated[tr.id] {
			m.say(tr.name + ": " + tr.defeat)
		} else {
			m.beginTrainer(tr)
		}
		return false
	}
	if s := r.signAt(nx, ny); s != nil {
		m.say(s.text)
		return false
	}

	// Move: step() applies ledge hops and current sliding and returns the resting
	// cell. If it didn't move, the way was blocked.
	ledgeHop := r.tile(nx, ny) == 'j' && dy == 1
	tx, ty := r.step(r.px, r.py, dx, dy)
	if tx == r.px && ty == r.py {
		return false
	}
	r.px, r.py = tx, ty
	r.lastStep = time.Now()
	if ledgeHop {
		r.hopAt = time.Now()
	}
	m.resolveLanding()
	return false
}

// isCurrent reports whether t is a directional water-current tile.
func isCurrent(t byte) bool { return t == '<' || t == '>' || t == '^' || t == 'v' }

// currentDir returns the flow vector of a current tile.
func currentDir(t byte) (int, int) {
	switch t {
	case '<':
		return -1, 0
	case '>':
		return 1, 0
	case '^':
		return 0, -1
	default: // 'v'
		return 0, 1
	}
}

// step returns the cell the player rests in after trying to move by (dx, dy)
// from (x, y): it applies a southward ledge hop, then rides any water currents
// to their end (a floor tile or a wall). It returns (x, y) unchanged when the
// way is blocked. Pure geometry — no side effects — so tests can reuse it.
func (r *routeState) step(x, y, dx, dy int) (int, int) {
	tx, ty := x+dx, y+dy
	switch {
	case r.tile(tx, ty) == 'j': // ledge: hop down only, landing one tile beyond
		if dy == 1 && !r.blocks(tx, ty+1) {
			ty++
		} else {
			return x, y
		}
	case r.blocks(tx, ty):
		return x, y
	}
	// Ride currents: keep going in the tile's flow until a non-current stop (a
	// floor tile) or a wall. The cap guards against a mis-authored current loop.
	for i := 0; i < 64 && isCurrent(r.tile(tx, ty)); i++ {
		cdx, cdy := currentDir(r.tile(tx, ty))
		if r.blocks(tx+cdx, ty+cdy) {
			break
		}
		tx, ty = tx+cdx, ty+cdy
		if !isCurrent(r.tile(tx, ty)) {
			break
		}
	}
	return tx, ty
}

// resolveLanding reacts to the tile the player just stepped (or hopped) onto: a
// warp, a heal pad, a pickup, a wild encounter, or a trainer's line of sight.
func (m *model) resolveLanding() {
	r := m.route
	nx, ny := r.px, r.py
	if w := r.warpAt(nx, ny); w != nil {
		m.enterArea(w.dest, w.dx, w.dy)
		return
	}
	if r.tile(nx, ny) == 'H' {
		m.healParty()
		m.say("Your team was fully healed!")
		return
	}
	if it := r.itemAt(nx, ny); it != nil && !m.found[it.id] {
		m.collect(it)
		return
	}
	if r.encounters && r.tile(nx, ny) == ',' && m.rng.Float64() < encounterRate {
		m.startWildBattle()
		return
	}
	if tr := m.spotter(); tr != nil {
		m.say(tr.name + " spotted you!")
		m.beginTrainer(tr)
	}
}

// say shows a transient line in the overworld speech panel.
func (m *model) say(text string) { m.routeMsg, m.routeMsgAt = text, time.Now() }

// itemAt returns the pickup on a cell, if any.
func (r *routeState) itemAt(x, y int) *hiddenItem {
	for i := range r.items {
		if r.items[i].x == x && r.items[i].y == y {
			return &r.items[i]
		}
	}
	return nil
}

// spotter returns the first undefeated trainer whose line of sight reaches the
// player (clear of obstacles), or nil.
func (m *model) spotter() *trainer {
	r := m.route
	for i := range r.trainers {
		tr := &r.trainers[i]
		if m.defeated[tr.id] {
			continue
		}
		vx, vy := facingVec(tr.facing)
		for d := 1; d <= tr.sight; d++ {
			cx, cy := tr.x+vx*d, tr.y+vy*d
			if cx == r.px && cy == r.py {
				return tr
			}
			if r.blocks(cx, cy) {
				break
			}
		}
	}
	return nil
}

func facingVec(f sprites.Facing) (int, int) {
	switch f {
	case sprites.FaceUp:
		return 0, -1
	case sprites.FaceLeft:
		return -1, 0
	case sprites.FaceRight:
		return 1, 0
	default:
		return 0, 1
	}
}

// beginTrainer opens a battle against a trainer's freshly built team.
func (m *model) beginTrainer(tr *trainer) {
	var team []*mon.Creature
	for _, tm := range tr.team {
		if c := mon.NewCreature(tm.key, tm.lvl); c != nil {
			team = append(team, c)
		}
	}
	if len(team) == 0 {
		return
	}
	m.beginBattle(team, false, tr.name)
	m.bt.trainerID, m.bt.trainerName = tr.id, tr.name
	m.bt.reward, m.bt.rewardN = tr.reward, tr.rewardN
	m.bt.badge = tr.badge
	m.bt.log = []string{tr.name + ": " + tr.intro, m.bt.log[0]}
}

// collect grants a pickup's contents once and marks it found.
func (m *model) collect(it *hiddenItem) {
	m.found[it.id] = true
	if it.creature != "" {
		if len(m.roster) < maxParty {
			m.roster = append(m.roster, mon.NewCreature(it.creature, it.level))
			m.say(it.msg)
		} else {
			m.say("You found a Shellmon, but your team is full!")
		}
	}
	if it.cosmetic != "" && m.ctx.Inventory != nil {
		_ = m.ctx.Inventory.Grant("cosmetic."+it.cosmetic, it.cosmeticName, 1)
		m.say(it.msg + " (wear it with c in the plaza)")
	}
	m.saveRoster()
}

// startWildBattle rolls a wild Shellmon from the area's pool (with a small
// chance of its rare species) and opens the battle.
func (m *model) startWildBattle() {
	r := m.route
	pool := r.wildPool
	if len(pool) == 0 {
		return
	}
	sp := pool[m.rng.Intn(len(pool))]
	span := r.lvlMax - r.lvlMin + 1
	if span < 1 {
		span = 1
	}
	lvl := r.lvlMin + m.rng.Intn(span)
	if r.rareSpecies != "" && m.rng.Float64() < r.rareChance {
		sp, lvl = r.rareSpecies, r.rareLevel
	}
	m.beginBattle([]*mon.Creature{mon.NewCreature(sp, lvl)}, true, "")
}

// Route ground tones (monochrome, lit like the plaza floor).
const (
	routePath  = canvas.Color(0x202020)
	routePathB = canvas.Color(0x262626)
	routeEdge  = canvas.Color(0x141414)
	routeGrass = canvas.Color(0x161616)
	routeGrasB = canvas.Color(0x1B1B1B)
)

// tall objects sorted back-to-front each frame.
const (
	kindPlayer = iota
	kindTree
	kindRock
	kindHouse
	kindSign
	kindNPC
	kindTrainer
	kindItem
	kindWell
	kindLighthouse
	kindFence
	kindLedge
	kindWall
	kindGym
)

// tileProp captures a map tile's behavior in one place: whether it stops
// movement, and which tall object (if any) is drawn standing on it. Adding a
// tile means adding one row here — blocks(), the tall-object pass and the area
// tests all read from this table, so they can't drift apart. The ground-plane
// look still lives in drawRoute's ground switch (drawers vary too much to table).
type tileProp struct {
	solid bool
	tall  int // tall-object kind, or 0 (kindPlayer) meaning ground-only
}

var tileProps = map[byte]tileProp{
	// Solid tall objects.
	'#': {solid: true, tall: kindTree},
	'o': {solid: true, tall: kindRock},
	'B': {solid: true, tall: kindHouse},
	'W': {solid: true, tall: kindWell},
	'L': {solid: true, tall: kindLighthouse},
	'e': {solid: true, tall: kindFence},
	'j': {solid: true, tall: kindLedge}, // ledge: solid except the down-hop in step()
	'X': {solid: true, tall: kindWall},
	'G': {solid: true, tall: kindGym},
	'~': {solid: true}, // deep water: solid, drawn on the ground plane
	// Walkable ground.
	'.': {}, ',': {}, 'f': {}, 'g': {}, 's': {}, 'P': {}, 'H': {},
	'<': {}, '>': {}, '^': {}, 'v': {},
}

// solidTile reports whether a tile char stops movement. Unknown tiles are solid
// so a stray character can never open an invisible gap in a wall.
func solidTile(t byte) bool {
	p, ok := tileProps[t]
	return !ok || p.solid
}

// tallKind returns the tall-object kind drawn on a tile (0 = none).
func tallKind(t byte) int { return tileProps[t].tall }

type tallObj struct {
	depth  int
	kind   int
	px, py int
	n      *npc
	tr     *trainer
	sg     *sign
	it     *hiddenItem
}

func (m *model) drawRoute(pw, ph int, t float64) {
	r := m.route
	// Camera centered on the player, exactly like the plaza.
	csx, csy := iso.Project(float64(r.px), float64(r.py))
	// Focus on the tile's centre so the player's cell sits dead-centre.
	originSx := csx - float64(pw)/2
	originSy := csy + float64(iso.HH) - float64(ph)/2
	project := func(gx, gy int) (int, int) {
		sx, sy := iso.Project(float64(gx), float64(gy))
		return int(sx - originSx), int(sy - originSy)
	}

	// Ground plane (flat): paved path, grass, water and heal pads.
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			px, py := project(x, y)
			if px < -iso.TileW || px > pw+iso.TileW || py < -iso.TileH || py > ph+iso.TileH {
				continue
			}
			switch r.tile(x, y) {
			case ',':
				groundGrass(m.scr, px, py, x, y)
				drawTallGrass(m.scr, px, py+iso.HH, x, y, t)
			case 'f':
				groundLawn(m.scr, px, py, x, y)
				drawFlowers(m.scr, px, py+iso.HH, x, y, t)
			case '~':
				drawWaterTile(m.scr, px, py, x, y, t)
				if r.landAdjacent(x, y) {
					drawShoreFoam(m.scr, px, py, x, y, t)
				}
			case 'H':
				drawHealPad(m.scr, px, py)
			case 's':
				groundSand(m.scr, px, py, x, y)
			case 'P':
				drawPierTile(m.scr, px, py, x, y)
			case '<', '>', '^', 'v':
				drawCurrentTile(m.scr, px, py, x, y, r.tile(x, y), t)
			case 'g', '#', 'o', 'j':
				// Short town/route grass; trees, rocks and ledges sit on it.
				groundLawn(m.scr, px, py, x, y)
			default: // '.', 'B', 'W', 'L', 'e' sit on paved/dirt ground
				fill := routePath
				if (x+y)&1 == 0 {
					fill = routePathB
				}
				iso.DrawDiamond(m.scr, px, py, fill, routeEdge)
			}
		}
	}

	// Collect every tall object, depth-sort, and draw back-to-front.
	var objs []tallObj
	add := func(o tallObj) {
		if o.px < -80 || o.px > pw+80 || o.py < -100 || o.py > ph+100 {
			return
		}
		objs = append(objs, o)
	}
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			k := tallKind(r.tile(x, y))
			if k == 0 {
				continue
			}
			px, py := project(x, y)
			add(tallObj{depth: iso.Depth(x, y), kind: k, px: px, py: py})
		}
	}
	for i := range r.signs {
		s := &r.signs[i]
		px, py := project(s.x, s.y)
		add(tallObj{depth: iso.Depth(s.x, s.y), kind: kindSign, px: px, py: py, sg: s})
	}
	for i := range r.npcs {
		n := &r.npcs[i]
		px, py := project(n.x, n.y)
		add(tallObj{depth: iso.Depth(n.x, n.y), kind: kindNPC, px: px, py: py, n: n})
	}
	for i := range r.trainers {
		tr := &r.trainers[i]
		px, py := project(tr.x, tr.y)
		add(tallObj{depth: iso.Depth(tr.x, tr.y), kind: kindTrainer, px: px, py: py, tr: tr})
	}
	for i := range r.items {
		it := &r.items[i]
		if !it.visible || m.found[it.id] {
			continue
		}
		px, py := project(it.x, it.y)
		add(tallObj{depth: iso.Depth(it.x, it.y), kind: kindItem, px: px, py: py, it: it})
	}
	ppx, ppy := project(r.px, r.py)
	add(tallObj{depth: iso.Depth(r.px, r.py), kind: kindPlayer, px: ppx, py: ppy})

	sort.Slice(objs, func(i, j int) bool { return objs[i].depth < objs[j].depth })
	for _, o := range objs {
		footX, footY := o.px, o.py+iso.HH
		switch o.kind {
		case kindTree:
			drawIsoTree(m.scr, footX, footY, o.px*3+o.py)
		case kindRock:
			drawRock(m.scr, footX, footY)
		case kindHouse:
			drawHouse(m.scr, o.px, o.py)
		case kindWell:
			drawWell(m.scr, footX, footY)
		case kindLighthouse:
			drawLighthouse(m.scr, footX, footY, t)
		case kindFence:
			drawFence(m.scr, footX, footY)
		case kindLedge:
			drawLedge(m.scr, o.px, o.py)
		case kindWall:
			drawWall(m.scr, o.px, o.py)
		case kindGym:
			drawGym(m.scr, o.px, o.py)
		case kindSign:
			drawSign(m.scr, footX, footY)
		case kindItem:
			drawItemBall(m.scr, footX, footY, t)
		case kindNPC:
			_, nbob := sprites.Pose(t, false, float64(o.n.x+o.n.y))
			drawContactShadow(m.scr, footX, footY)
			sprites.Draw(m.scr, footX, footY+nbob, o.n.facing, 0, false)
			drawNameTag(m.scr, footX, footY+nbob, o.n.name, 0xB8B8B8)
		case kindTrainer:
			_, nbob := sprites.Pose(t, false, float64(o.tr.x*2+o.tr.y))
			drawContactShadow(m.scr, footX, footY)
			sprites.Draw(m.scr, footX, footY+nbob, o.tr.facing, 0, false)
			name := o.tr.name
			col := canvas.Color(0xE0E0E0)
			if m.defeated[o.tr.id] {
				name, col = name+" (beaten)", 0x707070
			}
			drawNameTag(m.scr, footX, footY+nbob, name, col)
		default:
			// The player animates like the plaza avatar (shared sprites.Pose).
			moving := time.Since(r.lastStep) < 280*time.Millisecond
			frame, bob := sprites.Pose(t, moving, 0)
			// A ledge hop lifts the sprite in a short arc while the shadow stays put.
			hop := 0
			if !r.hopAt.IsZero() {
				if el := time.Since(r.hopAt).Seconds(); el < 0.28 {
					hop = -int(sinf(el/0.28*3.14159) * 12)
				}
			}
			drawContactShadow(m.scr, footX, footY)
			sprites.Draw(m.scr, footX, footY+bob+hop, r.facing, frame, moving)
		}
	}

	// HUD: area name + lead Shellmon + controls.
	panel(m.scr, 16, 14, 220, 24)
	m.scr.DrawText(26, 22, r.name, uiText)
	lead := ""
	if len(m.roster) > 0 {
		lead = m.roster[0].Name() + " Lv" + itoa(m.roster[0].Level)
	}
	m.scr.DrawText(pw-canvas.TextWidth(lead)-20, 22, lead, uiDim)
	hint := "WASD walk · talk by facing · p team · esc leave"
	m.scr.DrawText(pw/2-canvas.TextWidth(hint)/2, ph-26, hint, uiDim)

	// Speech / event line.
	if m.routeMsg != "" && time.Since(m.routeMsgAt) < 4*time.Second {
		w := canvas.TextWidth(m.routeMsg) + 24
		x := pw/2 - w/2
		panel(m.scr, x, ph-64, w, 26)
		m.scr.DrawText(x+12, ph-56, m.routeMsg, uiText)
	}
}
