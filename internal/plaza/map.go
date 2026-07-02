// Package plaza holds the hand-designed plaza: the tile layout, its
// renderers (static base + animated decorations), collision data and the
// portal placements.
//
// The layout below is human-editable. Tile legend:
//
//	'#'  wall (blocks movement)
//	' '  plain stone floor
//	'.'  floor speck, light  (purely cosmetic)
//	','  floor speck, dark   (purely cosmetic)
//	'P'  pillar; written as vertical pairs, top = capital, bottom = base
//	'B'  bench segment (blocks)
//	'~'  fountain water (blocks, animated)
//	'F'  fountain statue; vertical pair like pillars (blocks)
//	'L'  lamp post (blocks; its glowing head is drawn one cell above)
//	'H'  cosmetics shop stall (blocks; walk up to an adjacent tile to trade)
//	's'  spawn point (floor; marker is stripped at load)
//
// Rows shorter than Width are padded with floor; extra rows/columns are
// clipped. Portals are not tiles — see portals.go.
package plaza

// Plaza dimensions in cells. The hand-drawn layout below is wider/taller than
// this; Load clips it to these bounds and forces the border, so the plaza can
// be resized here without re-drawing the art.
const (
	Width  = 76
	Height = 40
)

const layout = `############################################################################
############################################################################
##        .           ,                                 ,                 ##
##   .                          .                      .  ,               ##
## ,   ,                    .                                             ##
## ,                        . ,                                           ##
##                  , .        ,                                         .##
##,                                           .    .  .              ,    ##
##           .        ,                          .             .          ##
##         .       ,             .                ,    ,                  ##
##                                .    .               ,               .  ##
##        .      .                                      ,    .         .  ##
##   .  . L               .                       ,              L        ##
##                   .                      .                            ,##
##      ,                            BBB         , ,           ,          ##
##   ,                                   .                           ,    ##
##  .        .                   ,  ~~~~~ ,    ,          ,  ,       .    ##
##          .                      ~~~~~~~ .                 ,           .##
##        .          .  .          ~~~~~~~        .                       ##
##                             .   ~~~F~~~                                ##
## ,                        .      ~~~F~~~                       ,        ##
##         .                       ~~~~~~~       ,          ,       ,     ##
##                      ,  ..       ~~~~~     ,               .    .      ##
##   .      ,                .                                           .##
##    ,                                           ,        .            . ##
##                       .    ,                    , ,                    ##
##                                 , BBB                               ,  ##
##       ,            P    ,     .  ,      .          P                   ##
##            .       P                               P             .     ##
##                                ,                                   .   ##
##        L                                         .   .        L        ##
##      ,                  ,                                    ,         ##
##                            H       s                 ,              .  ##
##  .         ,  , .                           ,                          ##
##                        ,          ,       ,                   ,        ##
##        .  ,                                       .                    ##
##                              ,                     ,            .      ##
##      .    .              .     , . .         ,                         ##
############################################################################
############################################################################`

// Point is a cell coordinate.
type Point struct {
	X, Y int
}

// Map is the parsed plaza: tiles, collision grid and decoration indexes.
type Map struct {
	W, H    int
	tiles   []byte
	collide []bool
	// SpawnX/SpawnY is the spawn cell (where 's' was in the layout).
	SpawnX, SpawnY int
	// Lamps, Water and StatueTops index the animated cells so the
	// per-frame pass never scans the whole grid. Shops indexes the cosmetics
	// stall cells (where the player can open the shop from an adjacent tile).
	Lamps      []Point
	Water      []Point
	StatueTops []Point
	Shops      []Point

	// structures is every solid cube cell (walls, pillars, benches, lamps)
	// pre-sorted back-to-front, and skyline is the city of background towers.
	// Both are built once and read-only, so the per-frame render only iterates
	// and culls — no allocation or sorting per frame, and safe to share across
	// the session render goroutines.
	structures []structCell
	skyline    []towerCell
	// ao counts a floor cell's solid tall neighbors (0..2, clamped), baked at
	// load; the render darkens floor by it so walls sit on the ground.
	ao []uint8
}

// structCell is one pre-sorted solid cell in the plaza.
type structCell struct {
	X, Y int
	tile byte
}

// blockingTiles is derived from the legend above.
func blocking(t byte) bool {
	switch t {
	case '#', 'P', 'B', '~', 'F', 'L', 'H':
		return true
	}
	return false
}

// Load parses the layout into a Map. It is defensive: short rows are
// padded with floor, overlong input is clipped, and a missing spawn marker
// falls back to the map center.
func Load() *Map {
	m := &Map{
		W:       Width,
		H:       Height,
		tiles:   make([]byte, Width*Height),
		collide: make([]bool, Width*Height),
		SpawnX:  Width / 2,
		SpawnY:  Height / 2,
	}
	for i := range m.tiles {
		m.tiles[i] = ' '
	}

	y := 0
	row := 0
	for row < len(layout) && y < Height {
		end := row
		for end < len(layout) && layout[end] != '\n' {
			end++
		}
		line := layout[row:end]
		for x := 0; x < len(line) && x < Width; x++ {
			t := line[x]
			if t == 's' {
				m.SpawnX, m.SpawnY = x, y
				t = ' '
			}
			m.tiles[y*Width+x] = t
		}
		y++
		row = end + 1
	}

	// Force a 2-cell wall border regardless of the (possibly clipped) art, so
	// shrinking the map can never leave an open edge.
	for cy := 0; cy < Height; cy++ {
		for cx := 0; cx < Width; cx++ {
			if cx < 2 || cx >= Width-2 || cy < 2 || cy >= Height-2 {
				m.tiles[cy*Width+cx] = '#'
			}
		}
	}

	for cy := 0; cy < Height; cy++ {
		for cx := 0; cx < Width; cx++ {
			t := m.tiles[cy*Width+cx]
			m.collide[cy*Width+cx] = blocking(t)
			switch t {
			case 'L':
				m.Lamps = append(m.Lamps, Point{cx, cy})
			case '~':
				m.Water = append(m.Water, Point{cx, cy})
			case 'F':
				if cy+1 < Height && m.tiles[(cy+1)*Width+cx] == 'F' {
					m.StatueTops = append(m.StatueTops, Point{cx, cy})
				}
			case 'H':
				m.Shops = append(m.Shops, Point{cx, cy})
			}
		}
	}
	m.buildStructures()
	m.buildSkyline()
	m.buildAO()
	return m
}

// buildAO bakes each floor cell's count of adjacent tall solids (walls and
// pillars — the volumes with real height), clamped to 2. The renderer darkens
// floor tiles by it, a cheap contact-occlusion pass.
func (m *Map) buildAO() {
	m.ao = make([]uint8, m.W*m.H)
	tall := func(x, y int) bool {
		switch m.Tile(x, y) {
		case '#', 'P':
			return true
		}
		return false
	}
	for cy := 0; cy < m.H; cy++ {
		for cx := 0; cx < m.W; cx++ {
			if m.collide[cy*m.W+cx] {
				continue
			}
			n := 0
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				if tall(cx+d[0], cy+d[1]) {
					n++
				}
			}
			if n > 2 {
				n = 2
			}
			m.ao[cy*m.W+cx] = uint8(n)
		}
	}
}

// floorAO returns the baked contact-occlusion level for a floor cell.
func (m *Map) floorAO(x, y int) uint8 {
	if x < 0 || y < 0 || x >= m.W || y >= m.H {
		return 0
	}
	return m.ao[y*m.W+x]
}

// Tile returns the tile byte at cell (x, y); out of bounds reads as '#'.
func (m *Map) Tile(x, y int) byte {
	if x < 0 || y < 0 || x >= m.W || y >= m.H {
		return '#'
	}
	return m.tiles[y*m.W+x]
}

// Blocked reports whether cell (x, y) blocks movement. Everything outside
// the map blocks.
func (m *Map) Blocked(x, y int) bool {
	if x < 0 || y < 0 || x >= m.W || y >= m.H {
		return true
	}
	return m.collide[y*m.W+x]
}

// NearShop reports whether feet at cell (cx, cy) are standing next to a shop
// stall — within one tile (the 3×3 around any stall cell), so a player walks up
// to the counter and trades from the adjacent floor rather than on top of it.
func (m *Map) NearShop(cx, cy int) bool {
	for _, s := range m.Shops {
		if abs(cx-s.X) <= 1 && abs(cy-s.Y) <= 1 {
			return true
		}
	}
	return false
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
