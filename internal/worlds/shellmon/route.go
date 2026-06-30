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
// and standing NPCs/trainers/signs).
func (r *routeState) blocks(x, y int) bool {
	switch r.tile(x, y) {
	case '#', 'o', '~', 'B':
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
	if r.blocks(nx, ny) {
		return false
	}

	// Step, then resolve what we walked onto.
	r.px, r.py = nx, ny
	r.lastStep = time.Now()

	if w := r.warpAt(nx, ny); w != nil {
		m.enterArea(w.dest, w.dx, w.dy)
		return false
	}
	if r.tile(nx, ny) == 'H' {
		m.healParty()
		m.say("Your team was fully healed!")
		return false
	}
	if it := r.itemAt(nx, ny); it != nil && !m.found[it.id] {
		m.collect(it)
		return false
	}
	if r.encounters && r.tile(nx, ny) == ',' && m.rng.Float64() < encounterRate {
		m.startWildBattle()
		return false
	}
	// A trainer may notice you from down their line of sight.
	if tr := m.spotter(); tr != nil {
		m.say(tr.name + " spotted you!")
		m.beginTrainer(tr)
	}
	return false
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
)

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
	originSx := csx - float64(pw)/2
	originSy := csy - float64(ph)/2
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
				groundGrass(m.scr, px, py, x, y)
				drawFlowers(m.scr, px, py+iso.HH, x, y, t)
			case '~':
				drawWaterTile(m.scr, px, py, x, y, t)
			case 'H':
				drawHealPad(m.scr, px, py)
			default: // '.', '#', 'o', 'B' all sit on paved ground
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
			k := -1
			switch r.tile(x, y) {
			case '#':
				k = kindTree
			case 'o':
				k = kindRock
			case 'B':
				k = kindHouse
			}
			if k < 0 {
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
			drawHouse(m.scr, footX, footY)
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
			drawContactShadow(m.scr, footX, footY)
			sprites.Draw(m.scr, footX, footY+bob, r.facing, frame, moving)
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

func drawNameTag(c *canvas.Canvas, footX, footY int, name string, col canvas.Color) {
	w := canvas.TextWidth(name)
	c.DrawTextShadow(footX-w/2, footY-sprites.Height-canvas.LineH, name, col, 0x000000)
}

func groundGrass(c *canvas.Canvas, px, py, x, y int) {
	fill := routeGrass
	if (x+y)&1 == 0 {
		fill = routeGrasB
	}
	iso.DrawDiamond(c, px, py, fill, routeEdge)
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

// drawWaterTile paints a shimmering pond diamond.
func drawWaterTile(c *canvas.Canvas, px, py, x, y int, t float64) {
	shade := 0.5 + 0.4*sinf(float64(x*5+y*7)+t*2.2)
	g := uint8(26 + 32*shade)
	iso.DrawDiamond(c, px, py, canvas.RGB(g, g, g+10), canvas.Color(0x0E0E16))
	if shade > 0.78 { // a drifting glint
		c.Set(px, py+iso.HH-2, canvas.Color(0xC8C8E0))
	}
}

// drawHealPad paints the well/rest pad: a pale diamond with a bright cross.
func drawHealPad(c *canvas.Canvas, px, py int) {
	iso.DrawDiamond(c, px, py, canvas.Color(0x2E2E2E), canvas.Color(0x565656))
	cx, cy := px, py+iso.HH
	c.FillRect(cx-1, cy-4, 3, 9, canvas.Color(0xE6E6E6))
	c.FillRect(cx-4, cy-1, 9, 3, canvas.Color(0xE6E6E6))
}

// drawHouse paints a camera-facing cottage standing on the ground point
// (footX, footY): a gabled roof over a flat front wall, with a centered door
// and two windows. Like the trees and avatar it is a billboard, so the door and
// windows read straight-on instead of warping across an isometric corner.
func drawHouse(c *canvas.Canvas, footX, footY int) {
	const (
		wallHalf = 20 // wall half-width
		wallH    = 30 // wall height
		eaveHalf = 25 // roof overhang half-width
		roofH    = 16 // roof peak height above the wall top
	)
	wallTop := footY - wallH
	roofTop := wallTop - roofH

	// Soft ground shadow so it sits on the tile.
	for dy := -3; dy <= 3; dy++ {
		w := int(float64(wallHalf+2) * sqrtClamp(1-float64(dy*dy)/9.0))
		yy := footY + dy - 1
		for dx := -w; dx <= w; dx++ {
			c.Set(footX+dx, yy, c.At(footX+dx, yy).Scale(0.55))
		}
	}

	// Front wall: top-lit, a touch darker toward the left edge.
	for y := wallTop; y < footY; y++ {
		for dx := -wallHalf; dx <= wallHalf; dx++ {
			tone := canvas.Color(0x969696)
			if dx < -wallHalf/2 {
				tone = canvas.Color(0x787878) // shaded left return
			}
			if y >= footY-3 {
				tone = tone.Scale(0.7) // grounding shadow at the base
			}
			c.Set(footX+dx, y, tone)
		}
	}

	// Gabled roof: a filled triangle from the eaves up to the peak, brightest
	// on top where the light lands.
	for y := roofTop; y <= wallTop; y++ {
		f := float64(y-roofTop) / float64(roofH) // 0 at peak, 1 at eaves
		half := int(float64(eaveHalf) * f)
		tone := canvas.Color(0xC4C4C4)
		if y > roofTop+roofH/2 {
			tone = canvas.Color(0xA0A0A0) // lower roof in shade
		}
		for dx := -half; dx <= half; dx++ {
			c.Set(footX+dx, y, tone)
		}
	}
	// Eave line and ridge highlight.
	c.HLine(footX-eaveHalf, footX+eaveHalf, wallTop, canvas.Color(0x4E4E4E))
	c.Set(footX, roofTop, canvas.Color(0xE2E2E2))

	// Door: centered, set into the wall, with a knob.
	const doorHalf, doorH = 5, 15
	c.FillRect(footX-doorHalf, footY-doorH, doorHalf*2+1, doorH, canvas.Color(0x2A2421))
	c.Rect(footX-doorHalf, footY-doorH, doorHalf*2+1, doorH, canvas.Color(0x161210))
	c.Set(footX+doorHalf-2, footY-doorH/2, canvas.Color(0xD8D8D8)) // knob

	// Two lit windows flanking the door, with a cross frame.
	for _, wx := range []int{-12, 12} {
		wy := wallTop + 9
		c.FillRect(footX+wx-3, wy-3, 7, 7, canvas.Color(0xDADADA))
		c.Rect(footX+wx-3, wy-3, 7, 7, canvas.Color(0x3A3A3A))
		c.VLine(footX+wx, wy-3, wy+3, canvas.Color(0x3A3A3A))
		c.HLine(footX+wx-3, footX+wx+3, wy, canvas.Color(0x3A3A3A))
	}
}

// drawSign paints a small wooden signpost.
func drawSign(c *canvas.Canvas, footX, footY int) {
	c.FillRect(footX-1, footY-13, 2, 13, canvas.Color(0x554636))  // post
	c.FillRect(footX-7, footY-22, 15, 10, canvas.Color(0x8A7654)) // board
	c.Rect(footX-7, footY-22, 15, 10, canvas.Color(0x3C3026))
	c.HLine(footX-4, footX+4, footY-18, canvas.Color(0x3C3026)) // "text"
	c.HLine(footX-4, footX+2, footY-15, canvas.Color(0x3C3026))
}

// drawItemBall paints a small bobbing pickup sphere with a band and glint.
func drawItemBall(c *canvas.Canvas, footX, footY int, t float64) {
	cy := footY - 6 + int(sinf(t*3)*1.5)
	for dy := -5; dy <= 5; dy++ {
		w := int(5 * sqrtClamp(1-float64(dy*dy)/25.0))
		tone := canvas.Color(0xD2D2D2)
		if dy > 1 {
			tone = canvas.Color(0x808080)
		}
		for dx := -w; dx <= w; dx++ {
			c.Set(footX+dx, cy+dy, tone)
		}
	}
	c.HLine(footX-5, footX+5, cy, canvas.Color(0x2E2E2E)) // band
	c.Set(footX, cy, canvas.Color(0xF4F4F4))              // button
	c.Set(footX-2, cy-3, canvas.Color(0xFFFFFF))          // glint
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
