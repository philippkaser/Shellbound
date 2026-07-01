package shellmon

import (
	"testing"

	"github.com/shellbound/shellbound/internal/cosmetic"
	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// allAreas is every area key the overworld can build.
var allAreas = []string{areaOakhaven, areaRoute1, areaTidewell}

// TestAreasBuild checks each area constructs with a uniform-width grid and a
// walkable spawn cell.
func TestAreasBuild(t *testing.T) {
	for _, key := range allAreas {
		a := buildArea(key)
		if a == nil {
			t.Fatalf("%s: buildArea returned nil", key)
		}
		if a.w <= 0 || a.h <= 0 || len(a.tiles) != a.w*a.h {
			t.Fatalf("%s: bad grid %dx%d with %d tiles", key, a.w, a.h, len(a.tiles))
		}
		if a.blocks(a.spawnX, a.spawnY) {
			t.Errorf("%s: spawn (%d,%d) is blocked", key, a.spawnX, a.spawnY)
		}
	}
}

// TestAreaEntitiesWalkable checks that every placed entity, warp and item sits
// on a tile the player can actually stand on (stamp clears NPC/warp/sign/trainer
// tiles, so only items and the tiles beneath them need independent checking).
func TestAreaEntitiesWalkable(t *testing.T) {
	blocking := func(b byte) bool {
		switch b {
		case '#', 'o', '~', 'B', 'W', 'L', 'e', 'j':
			return true
		}
		return false
	}
	for _, key := range allAreas {
		a := buildArea(key)
		for _, w := range a.warps {
			if blocking(a.tile(w.x, w.y)) {
				t.Errorf("%s: warp to %s at (%d,%d) sits on a blocking tile %q", key, w.dest, w.x, w.y, a.tile(w.x, w.y))
			}
		}
		for _, it := range a.items {
			if blocking(a.tile(it.x, it.y)) {
				t.Errorf("%s: item %s at (%d,%d) sits on a blocking tile %q", key, it.id, it.x, it.y, a.tile(it.x, it.y))
			}
		}
		for _, n := range a.npcs {
			if blocking(a.tile(n.x, n.y)) {
				t.Errorf("%s: npc %s at (%d,%d) sits on a blocking tile %q", key, n.name, n.x, n.y, a.tile(n.x, n.y))
			}
		}
		for _, tr := range a.trainers {
			if blocking(a.tile(tr.x, tr.y)) {
				t.Errorf("%s: trainer %s at (%d,%d) sits on a blocking tile %q", key, tr.id, tr.x, tr.y, a.tile(tr.x, tr.y))
			}
		}
	}
}

// TestTownsHaveNoWildGrass checks that non-encounter areas (the towns) contain
// no tall-grass ',' tiles, so nothing can spawn there.
func TestTownsHaveNoWildGrass(t *testing.T) {
	for _, key := range allAreas {
		a := buildArea(key)
		if a.encounters {
			continue
		}
		for i, b := range a.tiles {
			if b == ',' {
				t.Errorf("%s: town has tall grass at (%d,%d)", key, i%a.w, i/a.w)
			}
		}
	}
}

// TestLedgesAreHoppable checks that every ledge has a walkable tile directly
// below it, so the southward hop always has somewhere to land.
func TestLedgesAreHoppable(t *testing.T) {
	for _, key := range allAreas {
		a := buildArea(key)
		for y := 0; y < a.h; y++ {
			for x := 0; x < a.w; x++ {
				if a.tile(x, y) == 'j' && a.blocks(x, y+1) {
					t.Errorf("%s: ledge at (%d,%d) has no landing below", key, x, y)
				}
			}
		}
	}
}

// reachable flood-fills the walkable tiles reachable from the spawn (ledges are
// treated as walls, so it is a conservative lower bound on what a player can get
// to). The result is indexed y*w+x.
func reachable(a *routeState) []bool {
	seen := make([]bool, a.w*a.h)
	start := a.spawnY*a.w + a.spawnX
	if a.blocks(a.spawnX, a.spawnY) {
		return seen
	}
	queue := []int{start}
	seen[start] = true
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		x, y := i%a.w, i/a.w
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := x+d[0], y+d[1]
			if nx < 0 || ny < 0 || nx >= a.w || ny >= a.h {
				continue
			}
			j := ny*a.w + nx
			if seen[j] || a.blocks(nx, ny) {
				continue
			}
			seen[j] = true
			queue = append(queue, j)
		}
	}
	return seen
}

// TestAreasConnected checks that after roughening the borders, every warp, heal
// pad and item is reachable from the spawn, and every NPC/trainer/sign has a
// reachable tile beside it to interact from — nothing gets walled off.
func TestAreasConnected(t *testing.T) {
	for _, key := range allAreas {
		a := buildArea(key)
		seen := reachable(a)
		at := func(x, y int) bool { return x >= 0 && y >= 0 && x < a.w && y < a.h && seen[y*a.w+x] }
		adj := func(x, y int) bool { return at(x+1, y) || at(x-1, y) || at(x, y+1) || at(x, y-1) }

		for _, w := range a.warps {
			if !at(w.x, w.y) {
				t.Errorf("%s: warp at (%d,%d) is unreachable from spawn", key, w.x, w.y)
			}
		}
		for _, it := range a.items {
			if !at(it.x, it.y) {
				t.Errorf("%s: item %s at (%d,%d) is unreachable", key, it.id, it.x, it.y)
			}
		}
		for y := 0; y < a.h; y++ {
			for x := 0; x < a.w; x++ {
				if a.tile(x, y) == 'H' && !at(x, y) {
					t.Errorf("%s: heal pad (%d,%d) is unreachable", key, x, y)
				}
			}
		}
		for _, n := range a.npcs {
			if !adj(n.x, n.y) {
				t.Errorf("%s: npc %s at (%d,%d) has no reachable tile beside it", key, n.name, n.x, n.y)
			}
		}
		for _, tr := range a.trainers {
			if !adj(tr.x, tr.y) {
				t.Errorf("%s: trainer %s at (%d,%d) has no reachable tile beside it", key, tr.id, tr.x, tr.y)
			}
		}
		for _, s := range a.signs {
			if !adj(s.x, s.y) {
				t.Errorf("%s: sign at (%d,%d) has no reachable tile beside it", key, s.x, s.y)
			}
		}
	}
}

// TestWarpsConnect checks that every warp targets a real area and lands the
// player on a walkable cell there.
func TestWarpsConnect(t *testing.T) {
	known := map[string]bool{}
	for _, k := range allAreas {
		known[k] = true
	}
	for _, key := range allAreas {
		a := buildArea(key)
		for _, w := range a.warps {
			if !known[w.dest] {
				t.Errorf("%s: warp targets unknown area %q", key, w.dest)
				continue
			}
			dest := buildArea(w.dest)
			if dest.blocks(w.dx, w.dy) {
				t.Errorf("%s: warp to %s lands on blocked cell (%d,%d)", key, w.dest, w.dx, w.dy)
			}
		}
	}
}

// TestAreaContentValid checks that every species and cosmetic referenced by an
// area actually exists, so a trainer or pickup can never reference a dead key.
func TestAreaContentValid(t *testing.T) {
	for _, key := range allAreas {
		a := buildArea(key)
		for _, sp := range a.wildPool {
			if mon.NewCreature(sp, 5) == nil {
				t.Errorf("%s: wild pool species %q does not exist", key, sp)
			}
		}
		if a.rareSpecies != "" && mon.NewCreature(a.rareSpecies, a.rareLevel) == nil {
			t.Errorf("%s: rare species %q does not exist", key, a.rareSpecies)
		}
		for _, tr := range a.trainers {
			if len(tr.team) == 0 {
				t.Errorf("%s: trainer %s has an empty team", key, tr.id)
			}
			for _, tm := range tr.team {
				if mon.NewCreature(tm.key, tm.lvl) == nil {
					t.Errorf("%s: trainer %s references missing species %q", key, tr.id, tm.key)
				}
			}
			if tr.reward != "" && !cosmetic.Valid(tr.reward) {
				t.Errorf("%s: trainer %s rewards unknown cosmetic %q", key, tr.id, tr.reward)
			}
		}
		for _, it := range a.items {
			if it.creature != "" && mon.NewCreature(it.creature, it.level) == nil {
				t.Errorf("%s: item %s grants missing species %q", key, it.id, it.creature)
			}
			if it.cosmetic != "" && !cosmetic.Valid(it.cosmetic) {
				t.Errorf("%s: item %s grants unknown cosmetic %q", key, it.id, it.cosmetic)
			}
		}
	}
}
