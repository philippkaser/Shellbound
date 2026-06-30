package shellmon

import "github.com/shellbound/shellbound/internal/render/sprites"

// The Shellmon overworld is a little chain of hand-built areas — a starting
// town, a wild route, and a town at the far end — linked by edge warps. Areas
// are stateless templates rebuilt on entry; per-player progress (which trainers
// are beaten, which secrets are found) lives in the model's defeated/found sets,
// persisted in the save.

const (
	areaOakhaven = "oakhaven" // start town
	areaRoute1   = "route1"   // the wild route
	areaTidewell = "tidewell" // end town
	startArea    = areaOakhaven
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
	// A wide, open starting town. The east edge (col 22, row 7) is the gate out
	// to Route 1; a path leads to it. Houses are dotted around an open green.
	a := parseArea(areaOakhaven, "Oakhaven", []string{
		"#######################",
		"#,,,,,,,,,,,,,,,,,,,,,#",
		"#,,B,,,,,B,,,,,B,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,#",
		"#,,....................",
		"#,,,,,,,,~~~,,,,,,,,,,#",
		"#,,,,,,,,~~~,,,,,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,#",
		"#,,,,,,,,,,,,,,B,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,#",
		"#######################",
	})
	a.spawnX, a.spawnY = 10, 7
	a.warps = []warp{{x: 22, y: 7, dest: areaRoute1, dx: 1, dy: 6}}
	a.npcs = []npc{
		{x: 4, y: 4, name: "Mom", line: "Rest your team on the well-pad (the cross) before you set out, dear.", facing: sprites.FaceDown},
		{x: 18, y: 6, name: "Old Conch", line: "East lies Route 1. Mind the trainers lurking in the grass.", facing: sprites.FaceLeft},
	}
	a.signs = []sign{
		{x: 3, y: 11, text: "Oakhaven — a quiet shell of a town."},
		{x: 20, y: 6, text: "Route 1 ahead. Tidewell lies beyond the grass."},
	}
	a.set(8, 5, 'H')   // rest/heal well-pad
	a.set(3, 10, 'f')  // flower beds
	a.set(17, 10, 'f') // flower beds
	a.stamp()
	return a
}

func areaRoute1Build() *routeState {
	// A long, open route. Row 6 is a clear path running the full width (col 0 =
	// west warp back to Oakhaven, col 28 = east warp on to Tidewell). Above and
	// below the path is open tall grass framed by irregular treelines.
	a := parseArea(areaRoute1, "Route 1", []string{
		"#############################",
		"#,,,,,,,#####,,,,,,#####,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,,,,,,,#",
		".............................",
		"#,,,,,,,,,,,,,,,,,,,,,,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,,,,,,,#",
		"#,,,,,,,#####,,,,,,#####,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,,,,,,,#",
		"#############################",
	})
	a.spawnX, a.spawnY = 1, 6 // only entered via warp, but keep a sane default
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
			x: 16, y: 8, facing: sprites.FaceUp, sight: 3, team: []teamMon{{"dripling", 4}, {"fernling", 5}}},
		{id: "r1-ace", name: "Ace Mossa", intro: "So the rumors were true — a challenger!", defeat: "Take this, you've earned it.",
			x: 24, y: 2, facing: sprites.FaceDown, sight: 2, team: []teamMon{{"flickit", 7}, {"thornpod", 7}, {"tidecoil", 9}},
			reward: "halo", rewardN: "Wanderer's Halo"},
	}
	a.signs = []sign{
		{x: 2, y: 5, text: "ROUTE 1 — tall grass hides wild Shellmon. Press into it to find them."},
		{x: 22, y: 4, text: "Locals whisper of an ace who trains in the northeast grass…"},
	}
	a.items = []hiddenItem{
		{id: "r1-frost", x: 4, y: 9, visible: true, msg: "A lonely Frostnip tags along!", creature: "frostnip", level: 5},
		{id: "r1-buried", x: 26, y: 9, visible: false, msg: "You dig up a buried Wizard Hat!", cosmetic: "wizard", cosmeticName: "Wizard Hat"},
	}
	// Scattered scenery on otherwise-grassy tiles.
	a.set(13, 3, 'o')  // a boulder in the open
	a.set(20, 8, 'o')  // another to the south
	a.set(10, 9, 'f')  // a patch of flowers
	a.set(25, 11, 'f') // flowers near the south treeline
	a.stamp()
	return a
}

func areaTidewellBuild() *routeState {
	// The open seaside town at the far end of Route 1. A wide bay of water fills
	// the south; the west edge (col 0, row 7) is the gate back to the route.
	a := parseArea(areaTidewell, "Tidewell", []string{
		"#######################",
		"#,,,,,,,,,,,,,,,,,,,,,#",
		"#,,B,,,,,,,,,,,,,B,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,#",
		".....................,#",
		"#,,,,,~~~~~~~~,,,,,,,,#",
		"#,,,,~~~~~~~~~~,,,,,,,#",
		"#,,,,~~~~~~~~~~,,,f,,,#",
		"#,,,,,~~~~~~~~,,,,,,,,#",
		"#,,,,,,,,,,,,,,,,,,,,,#",
		"#######################",
	})
	a.spawnX, a.spawnY = 2, 7
	a.warps = []warp{{x: 0, y: 7, dest: areaRoute1, dx: 27, dy: 6}}
	a.npcs = []npc{
		{x: 17, y: 5, name: "Champion Pearl", line: "You crossed Route 1? Tidewell salutes you, traveler.", facing: sprites.FaceDown},
		{x: 13, y: 6, name: "Sailor Finn", line: "The sea breeze carries odd whispers from the old well…", facing: sprites.FaceLeft},
	}
	a.signs = []sign{
		{x: 18, y: 6, text: "Tidewell — where the route meets the sea."},
	}
	// Hidden on the bank at the bay's west edge — search beside the "old well".
	a.items = []hiddenItem{
		{id: "tw-well", x: 4, y: 9, visible: false, msg: "Something glints in the old well — a Flower Crown!", cosmetic: "flower", cosmeticName: "Flower Crown"},
	}
	a.set(8, 4, 'H') // rest/heal well-pad
	a.stamp()
	return a
}
