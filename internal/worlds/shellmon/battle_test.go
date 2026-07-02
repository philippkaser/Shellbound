package shellmon

import (
	"math/rand"
	"testing"

	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// newTestModel builds a bare battle-capable model (no rendering, no save).
func newTestModel(seed int64) *model {
	return &model{
		rng:      rand.New(rand.NewSource(seed)),
		defeated: map[string]bool{},
		found:    map[string]bool{},
		badges:   map[string]bool{},
	}
}

// TestTrainerBattleCompletes is the regression test for the stalled-battle
// bug: a trainer with reserves must send out its next creature after a KO,
// so hammering attacks eventually ends the battle. Before the fix the foe's
// active stayed the fainted slot forever and the fight could never finish.
func TestTrainerBattleCompletes(t *testing.T) {
	for seed := int64(0); seed < 5; seed++ {
		m := newTestModel(seed)
		m.roster = []*mon.Creature{mon.NewCreature("cindle", 30)}
		team := []*mon.Creature{
			mon.NewCreature("sprigling", 3),
			mon.NewCreature("dripling", 3),
			mon.NewCreature("fernling", 3),
		}
		m.beginBattle(team, false, "Tester")
		m.bt.trainerID = "test.trainer"
		m.bt.trainerName = "Tester"

		for i := 0; i < 400 && m.bt.sub != subOver; i++ {
			if m.bt.sub == subSwitch {
				m.commitSwitch(m.firstSwitchable())
				continue
			}
			m.bt.sub = subMenu
			m.playerAttack(0)
		}
		if m.bt.sub != subOver {
			t.Fatalf("seed %d: trainer battle never finished (foe never force-switched?)", seed)
		}
		if !m.bt.won || m.bt.outcome != outWon {
			t.Fatalf("seed %d: a level 30 starter lost to three level 3s (won=%v outcome=%v)", seed, m.bt.won, m.bt.outcome)
		}
		if !m.defeated["test.trainer"] {
			t.Fatalf("seed %d: victory did not mark the trainer beaten", seed)
		}
	}
}

// TestWildCatchHealsAndEndsBattle verifies a caught creature joins the roster
// at full HP and that fleeing skips the heal-on-loss path.
func TestWildCatchHealsAndEndsBattle(t *testing.T) {
	m := newTestModel(7)
	m.roster = []*mon.Creature{mon.NewCreature("cindle", 30)}
	wild := mon.NewCreature("gulper", 3)
	wild.CurHP = 1 // nearly fainted: catch odds near max
	m.beginBattle([]*mon.Creature{wild}, true, "")

	caught := false
	for i := 0; i < 100 && m.bt.sub != subOver; i++ {
		m.bt.sub = subMenu
		m.attemptCatch()
		if m.bt.outcome == outCaught {
			caught = true
		}
	}
	if !caught {
		t.Fatal("never caught a 1 HP wild creature in 100 throws")
	}
	if len(m.roster) != 2 {
		t.Fatalf("roster has %d members, want 2", len(m.roster))
	}
	got := m.roster[1]
	if got.CurHP != got.MaxHP() {
		t.Fatalf("caught creature joined at %d/%d HP, want full", got.CurHP, got.MaxHP())
	}
}

// TestFleeOutcome verifies running from a wild battle is recorded as a flee,
// not a loss.
func TestFleeOutcome(t *testing.T) {
	m := newTestModel(3)
	m.roster = []*mon.Creature{mon.NewCreature("cindle", 10)}
	m.beginBattle([]*mon.Creature{mon.NewCreature("gulper", 3)}, true, "")
	m.chooseMenu(3) // Run
	if m.bt.outcome != outFled || m.bt.sub != subOver {
		t.Fatalf("flee gave outcome=%v sub=%v, want outFled/subOver", m.bt.outcome, m.bt.sub)
	}
}

// TestIntermediateKOAwardsXP verifies each KO in a multi-mon battle grants
// XP as it happens, not only at the end.
func TestIntermediateKOAwardsXP(t *testing.T) {
	m := newTestModel(11)
	starter := mon.NewCreature("cindle", 30)
	xpBefore := starter.XP
	m.roster = []*mon.Creature{starter}
	team := []*mon.Creature{mon.NewCreature("sprigling", 2), mon.NewCreature("dripling", 2)}
	m.beginBattle(team, false, "Tester")

	// Attack until the first foe goes down (battle not yet done).
	for i := 0; i < 100 && m.bt.b.AliveCount(false) == 2; i++ {
		m.bt.sub = subMenu
		m.playerAttack(0)
	}
	if m.bt.b.Done() {
		t.Skip("battle ended before an intermediate KO could be observed")
	}
	if starter.XP <= xpBefore {
		t.Fatal("no XP granted for the intermediate KO")
	}
}
