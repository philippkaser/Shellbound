package shellmon

import "github.com/shellbound/shellbound/internal/render/sprites"

// The Shellmon overworld is a little chain of hand-built areas — a starting
// town, a wild route, and a town at the far end — linked by edge warps. Areas
// are stateless templates rebuilt on entry; per-player progress (which trainers
// are beaten, which secrets are found) lives in the model's defeated/found sets,
// persisted in the save.

const (
	areaOakhaven    = "oakhaven"     // start town
	areaRoute1      = "route1"       // the wild route
	areaTidewell    = "tidewell"     // end town
	areaTidewellGym = "tidewell_gym" // the gym interior
	startArea       = areaOakhaven
)

// warp moves the player to another area when stepped on.
type warp struct {
	x, y   int
	dest   string
	dx, dy int // spawn cell in the destination
}

// sign shows a line of text when bumped (it blocks like a post).
type sign struct {
	x, y int
	text string
}

// teamMon is one creature on a trainer's team (built fresh at battle time).
type teamMon struct {
	key string
	lvl int
}

// trainer challenges the player on sight or contact, then stays beaten.
type trainer struct {
	id      string
	name    string
	intro   string
	defeat  string
	x, y    int
	facing  sprites.Facing
	sight   int // how many tiles ahead they notice you
	team    []teamMon
	reward  string // optional cosmetic key granted on first defeat ("" = none)
	rewardN string // its display name
}

// hiddenItem is a pickup: visible ones show an item ball, hidden ones are found
// by walking over them. It grants a creature and/or a cosmetic, once.
type hiddenItem struct {
	id                     string
	x, y                   int
	visible                bool
	msg                    string
	cosmetic, cosmeticName string
	creature               string
	level                  int
}

// buildArea constructs an area template by key.
func buildArea(key string) *routeState {
	switch key {
	case areaRoute1:
		return areaRoute1Build()
	case areaTidewell:
		return areaTidewellBuild()
	case areaTidewellGym:
		return areaTidewellGymBuild()
	default:
		return areaOakhavenBuild()
	}
}

// parseArea turns a layout into a routeState. Every row must be the same width;
// a ragged layout is an authoring bug, so it panics (caught by the area tests)
// rather than silently shifting columns.
func parseArea(key, name string, rows []string) *routeState {
	h := len(rows)
	w := len(rows[0])
	for y, r := range rows {
		if len(r) != w {
			panic("shellmon: area " + key + " row " + itoa(y) + " is not " + itoa(w) + " wide")
		}
	}
	a := &routeState{key: key, name: name, w: w, h: h, tiles: make([]byte, w*h), facing: sprites.FaceDown}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a.tiles[y*w+x] = rows[y][x]
		}
	}
	return a
}

func (a *routeState) set(x, y int, t byte) {
	if x >= 0 && y >= 0 && x < a.w && y < a.h {
		a.tiles[y*a.w+x] = t
	}
}

// hrun/vrun/rect paint runs and blocks of a tile — used to lay roads, plazas,
// grass patches and water on top of a plain base field.
func (a *routeState) hrun(t byte, x0, x1, y int) {
	for x := x0; x <= x1; x++ {
		a.set(x, y, t)
	}
}
func (a *routeState) vrun(t byte, x, y0, y1 int) {
	for y := y0; y <= y1; y++ {
		a.set(x, y, t)
	}
}
func (a *routeState) rect(t byte, x0, y0, x1, y1 int) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			a.set(x, y, t)
		}
	}
}

// roughen eats an irregular treeline into the field's borders so the walkable
// area isn't a strict rectangle. The inset waves with a couple of sines (phased
// per area by seed) for an organic edge. It only thickens over plain short-grass
// ('g') that holds no item or entity, so paths, grass patches, water, buildings
// and every gate approach are left intact.
func (a *routeState) roughen(seed int) {
	ph := float64(seed)
	inset := func(i int) int {
		d := 1.3 + 1.2*sinf(float64(i)*0.6+ph) + 0.5*sinf(float64(i)*1.7+ph*1.7)
		n := int(d)
		if n < 0 {
			n = 0
		}
		if n > 3 {
			n = 3
		}
		return n
	}
	grow := func(x, y int) {
		if a.tile(x, y) != 'g' {
			return
		}
		if a.itemAt(x, y) != nil || a.npcAt(x, y) != nil || a.trainerAt(x, y) != nil || a.signAt(x, y) != nil {
			return
		}
		a.set(x, y, '#')
	}
	for x := 1; x < a.w-1; x++ {
		for d := 1; d <= inset(x); d++ {
			grow(x, d)
		}
		for d := 1; d <= inset(x+7); d++ {
			grow(x, a.h-1-d)
		}
	}
	for y := 1; y < a.h-1; y++ {
		for d := 1; d <= inset(y+3); d++ {
			grow(d, y)
		}
		for d := 1; d <= inset(y+11); d++ {
			grow(a.w-1-d, y)
		}
	}
}

// baseField builds a w×h field enclosed by trees with a short-grass ('g')
// interior. Towns and routes both start from this and stamp features on top.
func baseField(w, h int) []string {
	rows := make([]string, h)
	for y := 0; y < h; y++ {
		b := make([]byte, w)
		for x := 0; x < w; x++ {
			if x == 0 || y == 0 || x == w-1 || y == h-1 {
				b[x] = '#'
			} else {
				b[x] = 'g'
			}
		}
		rows[y] = string(b)
	}
	return rows
}

// roomField builds a w×h indoor room: stone walls ('X') around a paved floor
// ('.'). Used for interiors like the gym.
func roomField(w, h int) []string {
	rows := make([]string, h)
	for y := 0; y < h; y++ {
		b := make([]byte, w)
		for x := 0; x < w; x++ {
			if x == 0 || y == 0 || x == w-1 || y == h-1 {
				b[x] = 'X'
			} else {
				b[x] = '.'
			}
		}
		rows[y] = string(b)
	}
	return rows
}

// stamp clears the tiles under entities/warps to walkable ground so they sit on
// path, and records the spawn.
func (a *routeState) stamp() {
	for _, w := range a.warps {
		a.set(w.x, w.y, '.')
	}
	for _, n := range a.npcs {
		a.set(n.x, n.y, '.')
	}
	for _, tr := range a.trainers {
		a.set(tr.x, tr.y, '.')
	}
	for _, s := range a.signs {
		a.set(s.x, s.y, '.')
	}
}

// === the areas ===

func areaOakhavenBuild() *routeState {
	// Oakhaven — a cozy inland starting village: a stone wishing-well at the
	// heart of a paved square, cottages around a tidy lawn, flower beds and a
	// fenced paddock. Paved and lawn ground only, so nothing wild spawns here.
	// The east gate (col 22, row 7) opens onto Route 1.
	a := parseArea(areaOakhaven, "Oakhaven", baseField(23, 14))
	a.spawnX, a.spawnY = 3, 7

	// Roads and the central paved square.
	a.hrun('.', 1, 21, 7)    // main road to the east gate
	a.vrun('.', 11, 1, 12)   // north–south road
	a.rect('.', 9, 5, 13, 9) // town square
	a.set(11, 6, 'W')        // the wishing-well (walk around it)

	// Cottages around the green.
	for _, p := range [][2]int{{3, 2}, {8, 2}, {15, 2}, {19, 2}, {4, 11}, {18, 11}} {
		a.set(p[0], p[1], 'B')
	}
	a.set(19, 4, 'H') // rest pad by the eastern cottage

	// Flower beds and a little fenced paddock in the southwest.
	for _, p := range [][2]int{{5, 4}, {16, 4}, {6, 9}, {15, 3}} {
		a.set(p[0], p[1], 'f')
	}
	a.hrun('e', 3, 6, 12)
	a.set(3, 11, 'e')
	a.set(6, 11, 'e')

	a.warps = []warp{{x: 22, y: 7, dest: areaRoute1, dx: 1, dy: 6}}
	a.npcs = []npc{
		{x: 5, y: 7, name: "Mom", line: "Rest your team on the pad by the eastern cottage before you set out, dear.", facing: sprites.FaceDown},
		{x: 14, y: 7, name: "Old Conch", line: "East lies Route 1. Mind the trainers lurking in the tall grass.", facing: sprites.FaceRight},
	}
	a.signs = []sign{
		{x: 9, y: 3, text: "Oakhaven — a quiet shell of a town. Make a wish at the well!"},
		{x: 20, y: 6, text: "Route 1 ahead. Tidewell lies beyond the grass."},
	}
	a.roughen(1)
	a.stamp()
	return a
}

func areaRoute1Build() *routeState {
	// Route 1 in the Pokémon idiom: a dirt path threads a field of short grass,
	// past defined patches of tall grass (the only tiles that hide wild Shellmon)
	// and a ledge you can hop down but not climb back up. Trainers watch the path
	// from the grass. West gate (col 0, row 6) → Oakhaven; east (col 28) →
	// Tidewell.
	a := parseArea(areaRoute1, "Route 1", baseField(29, 13))
	a.spawnX, a.spawnY = 1, 6

	// The path: a spine across the route with a northeast spur into the ace's
	// corner.
	a.hrun('.', 1, 27, 6)
	a.vrun('.', 21, 3, 6)
	a.hrun('.', 21, 25, 3)

	// Tall-grass patches (wild encounters happen only on these ',' tiles).
	a.rect(',', 4, 2, 9, 4)    // north patch, over the path — Cole's ground
	a.rect(',', 10, 8, 16, 10) // south patch below a ledge
	a.rect(',', 19, 2, 25, 4)  // northeast patch — the ace trains here

	// A ledge along the south edge of the path: hop down into the south grass.
	a.hrun('j', 10, 16, 7)

	// Scenery.
	a.set(17, 4, 'o')
	a.set(8, 10, 'o')
	a.set(4, 10, 'f')
	a.set(26, 3, 'f')
	a.rect('#', 12, 1, 14, 1) // a little copse on the north treeline

	a.encounters = true
	a.lvlMin, a.lvlMax = 3, 7
	a.wildPool = []string{"sprigling", "fernling", "dripling", "cindle", "flickit", "thornpod"}
	a.rareSpecies, a.rareLevel, a.rareChance = "voltun", 10, 0.06 // a fast, uncommon spark
	a.warps = []warp{
		{x: 0, y: 6, dest: areaOakhaven, dx: 21, dy: 7},
		{x: 28, y: 6, dest: areaTidewell, dx: 1, dy: 7},
	}
	a.trainers = []trainer{
		{id: "r1-cole", name: "Youngster Cole", intro: "You can't sneak past me!", defeat: "Aw, my Sprigling…",
			x: 7, y: 4, facing: sprites.FaceDown, sight: 3, team: []teamMon{{"sprigling", 4}}},
		{id: "r1-wren", name: "Lass Wren", intro: "Let's have a quick battle!", defeat: "You're strong!",
			x: 13, y: 9, facing: sprites.FaceLeft, sight: 3, team: []teamMon{{"dripling", 4}, {"fernling", 5}}},
		{id: "r1-ace", name: "Ace Mossa", intro: "So the rumors were true — a challenger!", defeat: "Take this, you've earned it.",
			x: 23, y: 2, facing: sprites.FaceDown, sight: 2, team: []teamMon{{"flickit", 7}, {"thornpod", 7}, {"tidecoil", 9}},
			reward: "halo", rewardN: "Wanderer's Halo"},
	}
	a.signs = []sign{
		{x: 2, y: 5, text: "ROUTE 1 — tall grass hides wild Shellmon. Press into it to find them."},
		{x: 20, y: 5, text: "Locals whisper of an ace who trains in the northeast grass…"},
	}
	a.items = []hiddenItem{
		{id: "r1-frost", x: 3, y: 7, visible: true, msg: "A lonely Frostnip tags along!", creature: "frostnip", level: 5},
		{id: "r1-buried", x: 25, y: 2, visible: false, msg: "You dig up a buried Wizard Hat!", cosmetic: "wizard", cosmeticName: "Wizard Hat"},
	}
	a.roughen(2)
	a.stamp()
	return a
}

func areaTidewellBuild() *routeState {
	// Tidewell — a breezy seaside port that opens straight onto the ocean to the
	// south: no treeline there, the sea itself is the edge. A sandy shore fronts
	// the water, crossed by a wooden fishing pier, with a striped lighthouse on
	// the eastern point and the town's gym up on the green. West gate (col 0, row
	// 7) → Route 1.
	a := parseArea(areaTidewell, "Tidewell", baseField(23, 14))
	a.spawnX, a.spawnY = 2, 7

	// Landward town first, then rough the inland treeline.
	a.hrun('.', 1, 19, 7) // seafront promenade / west gate
	a.vrun('.', 8, 1, 6)  // lane up to the cottages
	for _, p := range [][2]int{{3, 2}, {9, 2}} {
		a.set(p[0], p[1], 'B')
	}
	a.set(16, 3, 'G') // the gym hall
	a.set(12, 4, 'H') // rest pad
	a.set(5, 4, 'f')
	a.set(20, 3, 'f')
	a.roughen(3)

	// Open ocean across the south (painted after roughen so it replaces the
	// would-be south treeline — the sea is the map edge here).
	a.rect('s', 1, 8, 21, 10)  // broad sandy shore
	a.rect('~', 0, 11, 22, 13) // open sea to the edge
	a.rect('~', 4, 9, 18, 12)  // the bay
	a.rect('~', 7, 8, 15, 8)   // an inlet lapping the promenade

	// The fishing pier out over the water, and the lighthouse on the east point.
	a.vrun('P', 11, 8, 12)
	a.set(10, 12, 'P')
	a.set(12, 12, 'P')
	a.set(19, 9, 'L')

	a.warps = []warp{
		{x: 0, y: 7, dest: areaRoute1, dx: 27, dy: 6},
		{x: 16, y: 4, dest: areaTidewellGym, dx: 7, dy: 11}, // the gym door
	}
	a.npcs = []npc{
		{x: 13, y: 6, name: "Sailor Finn", line: "The sea breeze carries odd whispers from the old well on the beach…", facing: sprites.FaceUp},
		{x: 14, y: 5, name: "Gym Guide", line: "The Tidewell Gym! Leader Pearl commands the tides — beat her for the Coral Badge.", facing: sprites.FaceRight},
	}
	a.signs = []sign{
		{x: 5, y: 5, text: "Tidewell — where Route 1 meets the sea. Mind the pier!"},
		{x: 14, y: 4, text: "TIDEWELL GYM — Leader Pearl. Step to the doors to enter."},
	}
	// Hidden on the west beach — search the sand beside the old well.
	a.items = []hiddenItem{
		{id: "tw-well", x: 3, y: 9, visible: false, msg: "Something glints in the old well — a Flower Crown!", cosmetic: "flower", cosmeticName: "Flower Crown"},
	}
	a.stamp()
	return a
}

func areaTidewellGymBuild() *routeState {
	// The Tidewell Gym interior: a stone hall with two junior trainers guarding
	// the aisle and Leader Pearl at the head of the room. The door (bottom
	// centre) warps back out to the town. No wild encounters indoors.
	a := parseArea(areaTidewellGym, "Tidewell Gym", roomField(15, 13))
	a.spawnX, a.spawnY = 7, 11

	// Stone pillars flanking the aisle.
	for _, p := range [][2]int{{3, 3}, {11, 3}, {3, 8}, {11, 8}} {
		a.set(p[0], p[1], 'o')
	}

	a.warps = []warp{{x: 7, y: 12, dest: areaTidewell, dx: 16, dy: 5}}
	a.trainers = []trainer{
		{id: "gym-swimmer", name: "Swimmer Dana", intro: "The Leader's waters run deep — get past me first!", defeat: "Nice moves!",
			x: 4, y: 6, facing: sprites.FaceRight, sight: 3, team: []teamMon{{"dripling", 6}, {"frostnip", 6}}},
		{id: "gym-angler", name: "Angler Reef", intro: "Hooked yet? Let's battle!", defeat: "You're a catch.",
			x: 10, y: 6, facing: sprites.FaceLeft, sight: 3, team: []teamMon{{"gulper", 7}}},
		{id: "gym-pearl", name: "Leader Pearl", intro: "Welcome to my gym. Show me the tide can be turned!",
			defeat: "Magnificent — the Coral Badge is yours.",
			x:      7, y: 2, facing: sprites.FaceDown, sight: 6,
			team:   []teamMon{{"dripling", 9}, {"brineback", 10}, {"tidecoil", 12}},
			reward: "captain", rewardN: "Captain's Cap"},
	}
	a.signs = []sign{
		{x: 5, y: 11, text: "TIDEWELL GYM — Leader Pearl. Reward: the Coral Badge."},
	}
	a.stamp()
	return a
}
