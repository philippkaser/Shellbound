package plaza

import "testing"

func TestLoadDimensions(t *testing.T) {
	m := Load()
	if m.W != Width || m.H != Height {
		t.Fatalf("expected %dx%d, got %dx%d", Width, Height, m.W, m.H)
	}
}

func TestBorderWallsBlock(t *testing.T) {
	m := Load()
	for x := 0; x < m.W; x++ {
		if !m.Blocked(x, 0) || !m.Blocked(x, 1) {
			t.Fatalf("top wall open at x=%d", x)
		}
		if !m.Blocked(x, m.H-1) || !m.Blocked(x, m.H-2) {
			t.Fatalf("bottom wall open at x=%d", x)
		}
	}
	for y := 0; y < m.H; y++ {
		if !m.Blocked(0, y) || !m.Blocked(1, y) {
			t.Fatalf("left wall open at y=%d", y)
		}
		if !m.Blocked(m.W-1, y) || !m.Blocked(m.W-2, y) {
			t.Fatalf("right wall open at y=%d", y)
		}
	}
}

func TestOutOfBoundsBlocked(t *testing.T) {
	m := Load()
	cases := [][2]int{{-1, 5}, {5, -1}, {m.W, 5}, {5, m.H}, {-100, -100}}
	for _, c := range cases {
		if !m.Blocked(c[0], c[1]) {
			t.Errorf("out-of-bounds (%d,%d) should block", c[0], c[1])
		}
	}
}

func TestSpawnIsWalkable(t *testing.T) {
	m := Load()
	if m.Blocked(m.SpawnX, m.SpawnY) {
		t.Fatalf("spawn (%d,%d) is blocked", m.SpawnX, m.SpawnY)
	}
	if m.Tile(m.SpawnX, m.SpawnY) == 's' {
		t.Error("spawn marker should be stripped from tiles")
	}
}

func TestDecorationsBlock(t *testing.T) {
	m := Load()
	if len(m.Water) == 0 {
		t.Fatal("no fountain water cells found")
	}
	for _, p := range m.Water {
		if !m.Blocked(p.X, p.Y) {
			t.Errorf("water at (%d,%d) should block", p.X, p.Y)
		}
	}
	if len(m.Lamps) != 4 {
		t.Errorf("expected 4 lamps, got %d", len(m.Lamps))
	}
	for _, p := range m.Lamps {
		if !m.Blocked(p.X, p.Y) {
			t.Errorf("lamp at (%d,%d) should block", p.X, p.Y)
		}
	}
	if len(m.StatueTops) != 1 {
		t.Errorf("expected 1 statue top, got %d", len(m.StatueTops))
	}
}

func TestFloorIsOpen(t *testing.T) {
	m := Load()
	open := 0
	for y := 2; y < m.H-2; y++ {
		for x := 2; x < m.W-2; x++ {
			if !m.Blocked(x, y) {
				open++
			}
		}
	}
	if open < 3000 {
		t.Errorf("plaza floor suspiciously small: %d open cells", open)
	}
}

func TestPortalTriggers(t *testing.T) {
	if len(Portals) != 3 {
		t.Fatalf("expected 3 portals, got %d", len(Portals))
	}
	m := Load()
	seen := map[string]bool{}
	for _, p := range Portals {
		if seen[p.Key] {
			t.Errorf("duplicate portal key %q", p.Key)
		}
		seen[p.Key] = true

		// The mouth must contain its own center and be walkable.
		ccx, ccy := p.X+PortalW/2, p.Y+PortalH-1
		if !p.TriggerContains(ccx, ccy) {
			t.Errorf("portal %s: center (%d,%d) not in trigger", p.Key, ccx, ccy)
		}
		if m.Blocked(ccx, ccy) {
			t.Errorf("portal %s: trigger center (%d,%d) is blocked", p.Key, ccx, ccy)
		}
		// Just outside must not trigger.
		if p.TriggerContains(p.X-1, p.Y) || p.TriggerContains(p.X+PortalW, p.Y+PortalH) {
			t.Errorf("portal %s: trigger leaks outside the arch", p.Key)
		}

		got, ok := PortalAt(ccx, ccy)
		if !ok || got.Key != p.Key {
			t.Errorf("PortalAt(%d,%d) = %v,%v; want %s", ccx, ccy, got.Key, ok, p.Key)
		}
	}
	if _, ok := PortalAt(50, 32); ok {
		t.Error("spawn area should not be inside any portal trigger")
	}
}
