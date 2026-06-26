package shellmon

import "testing"

func TestMatchHealsAndIsolates(t *testing.T) {
	orig := NewCreature("cindle", 20)
	orig.CurHP = 1
	teamA := []*Creature{orig}
	teamB := []*Creature{NewCreature("sprigling", 20)}

	m := NewMatch(teamA, teamB, 1)
	v := m.Snapshot(true)
	if v.You.HP != v.You.MaxHP {
		t.Errorf("match should heal teams to full, got %d/%d", v.You.HP, v.You.MaxHP)
	}
	// The owner's creature must be untouched (the match works on a clone).
	if orig.CurHP != 1 {
		t.Errorf("match mutated the owner's creature: HP=%d, want 1", orig.CurHP)
	}
}

func TestMatchTurnResolvesWhenBothSubmit(t *testing.T) {
	m := NewMatch([]*Creature{NewCreature("cindle", 20)}, []*Creature{NewCreature("sprigling", 20)}, 5)

	// Only A submits: still awaiting B, no resolution.
	m.Submit(true, Action{Kind: Attack, Index: 0})
	v := m.Snapshot(true)
	if !v.WaitingOpponent {
		t.Error("after A submits, A should be waiting on the opponent")
	}
	vb := m.Snapshot(false)
	if !vb.AwaitingYou {
		t.Error("B should still owe an action")
	}

	// B submits: the turn resolves (someone took damage).
	m.Submit(false, Action{Kind: Attack, Index: 0})
	v = m.Snapshot(true)
	if v.Foe.HP == v.Foe.MaxHP && v.You.HP == v.You.MaxHP {
		t.Error("a resolved turn should have dealt damage to someone")
	}
}

func TestMatchPlaysToCompletion(t *testing.T) {
	// A strong Spark team versus a weak lone Bramble: A should win and the
	// lockstep loop should terminate, exercising forced switches on B's side.
	teamA := []*Creature{NewCreature("cindle", 35), NewCreature("ashfin", 35)}
	teamB := []*Creature{NewCreature("sprigling", 5)}
	m := NewMatch(teamA, teamB, 9)

	for guard := 0; !m.snapshotDone() && guard < 500; guard++ {
		if m.Snapshot(true).MustSwitchYou {
			m.Submit(true, Action{Kind: Switch, Index: firstAliveSlot(m, true)})
			continue
		}
		if m.Snapshot(false).MustSwitchYou {
			m.Submit(false, Action{Kind: Switch, Index: firstAliveSlot(m, false)})
			continue
		}
		// Both attack with their first damaging move.
		m.Submit(true, Action{Kind: Attack, Index: firstDamaging(m, true)})
		m.Submit(false, Action{Kind: Attack, Index: firstDamaging(m, false)})
	}
	if !m.snapshotDone() {
		t.Fatal("match did not finish")
	}
	if !m.Snapshot(true).YouWon {
		t.Error("the much stronger team should win")
	}
}

// helpers for the test

func (m *Match) snapshotDone() bool { return m.Snapshot(true).Done }

func firstAliveSlot(m *Match, sideA bool) int {
	for i, p := range m.Snapshot(sideA).Party {
		if !p.Fainted && !p.Active {
			return i
		}
	}
	return 0
}

func firstDamaging(m *Match, sideA bool) int {
	for i, mv := range m.Snapshot(sideA).You.Moves {
		if mv.Power > 0 {
			return i
		}
	}
	return 0
}
