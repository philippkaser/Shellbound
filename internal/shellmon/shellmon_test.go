package shellmon

import (
	"math/rand"
	"testing"
)

func TestTypeTriangle(t *testing.T) {
	cases := []struct {
		atk, def Type
		want     float64
	}{
		{Spark, Bramble, superMult},
		{Bramble, Tide, superMult},
		{Tide, Spark, superMult},
		{Bramble, Spark, resistMult},
		{Tide, Bramble, resistMult},
		{Spark, Tide, resistMult},
		{Spark, Spark, neutralMult},
		{Plain, Tide, neutralMult},
		{Spark, Plain, neutralMult},
	}
	for _, c := range cases {
		if got := Effectiveness(c.atk, c.def); got != c.want {
			t.Errorf("Effectiveness(%v,%v)=%v want %v", c.atk, c.def, got, c.want)
		}
	}
}

func TestCatalogIntegrity(t *testing.T) {
	if len(All()) != 12 {
		t.Fatalf("expected 12 species, got %d", len(All()))
	}
	if len(Starters()) != 3 {
		t.Fatalf("expected 3 starters, got %d", len(Starters()))
	}
	seenType := map[Type]int{}
	for _, sp := range All() {
		if sp.sprite == nil {
			t.Errorf("%s has no sprite", sp.Key)
		}
		for _, e := range sp.Learnset {
			if _, ok := moves[e.Move]; !ok {
				t.Errorf("%s learns unknown move %q", sp.Key, e.Move)
			}
		}
		seenType[sp.Type]++
	}
	// Each starter should be a distinct type.
	st := map[Type]bool{}
	for _, sp := range Starters() {
		if st[sp.Type] {
			t.Errorf("two starters share type %v", sp.Type)
		}
		st[sp.Type] = true
	}
}

func TestNewCreatureScaling(t *testing.T) {
	lo := NewCreature("cindle", 5)
	hi := NewCreature("cindle", 40)
	if lo == nil || hi == nil {
		t.Fatal("NewCreature returned nil for a real species")
	}
	if hi.MaxHP() <= lo.MaxHP() || hi.Atk() <= lo.Atk() {
		t.Errorf("stats should grow with level: L5 HP=%d Atk=%d, L40 HP=%d Atk=%d",
			lo.MaxHP(), lo.Atk(), hi.MaxHP(), hi.Atk())
	}
	if lo.CurHP != lo.MaxHP() {
		t.Errorf("new creature should start at full HP")
	}
	if NewCreature("nope", 5) != nil {
		t.Error("unknown species should return nil")
	}
}

func TestLearnsetMovesByLevel(t *testing.T) {
	young := NewCreature("sprigling", 1)
	if len(young.Moves) != 2 { // tackle + vine at level 1
		t.Errorf("level-1 sprigling should know 2 moves, has %v", young.Moves)
	}
	grown := NewCreature("sprigling", 20)
	if len(grown.Moves) != 4 {
		t.Errorf("level-20 sprigling should cap at 4 moves, has %v", grown.Moves)
	}
}

func TestGainXPLevelsUp(t *testing.T) {
	c := NewCreature("dripling", 4)
	startMax := c.MaxHP()
	log := c.GainXP(10_000) // plenty to climb several levels
	if c.Level <= 4 {
		t.Fatalf("expected level up, still at %d", c.Level)
	}
	if c.MaxHP() <= startMax {
		t.Errorf("max HP should grow with levels")
	}
	if len(log) == 0 {
		t.Error("leveling should produce log lines")
	}
	if c.Level > MaxLevel {
		t.Errorf("level exceeded cap: %d", c.Level)
	}
}

func TestCatchChanceRisesAsHurt(t *testing.T) {
	c := NewCreature("gulper", 10)
	full := CatchChance(c)
	c.CurHP = 1
	hurt := CatchChance(c)
	if hurt <= full {
		t.Errorf("catch chance should rise as HP falls: full=%.2f hurt=%.2f", full, hurt)
	}
}

// team builds a fresh team of one species at a level.
func team(species string, n, level int) []*Creature {
	out := make([]*Creature, n)
	for i := range out {
		out[i] = NewCreature(species, level)
	}
	return out
}

func TestBattleSuperEffectiveEndsInWin(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	// Spark attacker vs a lone Bramble defender: Spark is super-effective.
	a := team("cindle", 1, 30)
	d := team("sprigling", 1, 5)
	b := NewBattle(a, d, rng)

	guard := 0
	for !b.Done() {
		guard++
		if guard > 100 {
			t.Fatal("battle did not terminate")
		}
		if b.NeedsSwitch(true) {
			b.ApplyForcedSwitch(true, b.BestSwitch(true))
			continue
		}
		if b.NeedsSwitch(false) {
			b.ApplyForcedSwitch(false, b.BestSwitch(false))
			continue
		}
		b.ResolveTurn(b.ChooseAI(true), b.ChooseAI(false))
	}
	if !b.WinnerA() {
		t.Errorf("a level-30 Spark team should beat a level-5 Bramble")
	}
}

func TestForcedSwitchAfterFaint(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	a := team("brineback", 2, 20)
	d := team("cindle", 6, 35) // strong foes to KO our lead
	b := NewBattle(a, d, rng)

	// Drive until our side must switch or the battle ends.
	for i := 0; i < 200 && !b.Done(); i++ {
		if b.NeedsSwitch(true) {
			before := b.Active(true)
			b.ApplyForcedSwitch(true, b.BestSwitch(true))
			if b.Active(true) == before {
				t.Fatal("forced switch did not change the active creature")
			}
			continue
		}
		if b.NeedsSwitch(false) {
			b.ApplyForcedSwitch(false, b.BestSwitch(false))
			continue
		}
		b.ResolveTurn(b.ChooseAI(true), b.ChooseAI(false))
	}
	// Either side could win; the point is it resolved without panicking and
	// the loser's whole team fainted.
	if !b.Done() {
		t.Fatal("battle stalled")
	}
	loser := !b.WinnerA()
	if b.AliveCount(loser) != 0 {
		t.Errorf("loser should have 0 alive, has %d", b.AliveCount(loser))
	}
}

func TestSwitchResolvesBeforeAttack(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	a := team("tidecoil", 2, 25)
	d := team("flickit", 1, 25)
	b := NewBattle(a, d, rng)
	first := b.Active(true)
	// We switch; the foe attacks. After the turn our active must be the new one.
	b.ResolveTurn(Action{Kind: Switch, Index: 1}, b.ChooseAI(false))
	if b.Active(true) == first {
		t.Error("switch should have changed our active before/at the turn")
	}
}
