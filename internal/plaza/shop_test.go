package plaza

import "testing"

func TestShopPresentAndBlocks(t *testing.T) {
	m := Load()
	if len(m.Shops) != 1 {
		t.Fatalf("expected 1 shop stall, got %d", len(m.Shops))
	}
	s := m.Shops[0]
	if !m.Blocked(s.X, s.Y) {
		t.Errorf("shop stall at (%d,%d) should block movement", s.X, s.Y)
	}
}

func TestNearShop(t *testing.T) {
	m := Load()
	s := m.Shops[0]

	// Every cell in the 3×3 around the stall counts as "near".
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if !m.NearShop(s.X+dx, s.Y+dy) {
				t.Errorf("(%d,%d) next to the stall should be near", s.X+dx, s.Y+dy)
			}
		}
	}
	// Two cells away is not near.
	if m.NearShop(s.X+2, s.Y) || m.NearShop(s.X, s.Y-2) {
		t.Error("cells two tiles from the stall should not be near")
	}
	// At least one adjacent cell must be walkable so the player can reach it.
	reachable := false
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if (dx != 0 || dy != 0) && !m.Blocked(s.X+dx, s.Y+dy) {
				reachable = true
			}
		}
	}
	if !reachable {
		t.Error("shop stall is walled in — no walkable adjacent tile")
	}
}
