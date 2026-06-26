package shellmon

import (
	"testing"

	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// fakeSave is an in-memory SaveStore for tests.
type fakeSave struct{ data []byte }

func (f *fakeSave) Load() ([]byte, error) { return f.data, nil }
func (f *fakeSave) Save(d []byte) error   { f.data = append([]byte(nil), d...); return nil }

func TestRosterRoundTrip(t *testing.T) {
	save := &fakeSave{}
	m := &model{}
	m.ctx.Save = save

	// Build a party and persist it.
	c := mon.NewCreature("cindle", 5)
	c.GainXP(500) // level it up so we exercise non-default fields
	c.Nick = "Flicker"
	c.CurHP = 3
	m.roster = []*mon.Creature{c, mon.NewCreature("brineback", 8)}
	m.saveRoster()

	// Reload into a fresh model.
	m2 := &model{}
	m2.ctx.Save = save
	m2.loadRoster()

	if len(m2.roster) != 2 {
		t.Fatalf("expected 2 creatures after reload, got %d", len(m2.roster))
	}
	got := m2.roster[0]
	if got.Species != "cindle" || got.Nick != "Flicker" || got.Level != c.Level || got.CurHP != 3 {
		t.Errorf("creature did not round-trip: %+v vs %+v", got, c)
	}
	if len(got.Moves) == 0 {
		t.Error("moves should survive serialization")
	}
}

func TestHealParty(t *testing.T) {
	m := &model{}
	c := mon.NewCreature("gulper", 10)
	c.CurHP = 1
	m.roster = []*mon.Creature{c}
	if m.partyAlive() != true {
		t.Fatal("a 1-HP creature is still alive")
	}
	m.healParty()
	if c.CurHP != c.MaxHP() {
		t.Errorf("heal should restore full HP, got %d/%d", c.CurHP, c.MaxHP())
	}
}

func TestLoadEmptyRosterIsSafe(t *testing.T) {
	m := &model{}
	m.ctx.Save = &fakeSave{}
	m.loadRoster() // no data
	if len(m.roster) != 0 {
		t.Errorf("empty save should yield no creatures, got %d", len(m.roster))
	}
}
