package shellmon

import "math/rand"

// ActionKind is what a side does on its turn.
type ActionKind int

// Action kinds.
const (
	Attack ActionKind = iota
	Switch
)

// Action is one side's choice for a turn. For Attack, Index is the move slot;
// for Switch, Index is the team slot to bring in.
type Action struct {
	Kind  ActionKind
	Index int
}

// battleMon wraps a Creature with battle-only stat stages (reset each battle).
type battleMon struct {
	c        *Creature
	atkStage int // -3..3
	defStage int
}

// Battle is a 6v6 turn-based duel between two teams. Side A is, by convention,
// the local player; side B the opponent (AI for the wild route, a remote player
// for PvP). The same engine drives both.
type Battle struct {
	teamA, teamB     []*battleMon
	activeA, activeB int
	rng              *rand.Rand
	done             bool
	winnerA          bool
}

// NewBattle wraps two teams (each 1..6 creatures) and leads with each side's
// first unfainted member. rng must be non-nil (inject a seeded source in tests).
func NewBattle(teamA, teamB []*Creature, rng *rand.Rand) *Battle {
	b := &Battle{rng: rng}
	b.teamA = wrap(teamA)
	b.teamB = wrap(teamB)
	b.activeA = firstAlive(b.teamA)
	b.activeB = firstAlive(b.teamB)
	return b
}

func wrap(team []*Creature) []*battleMon {
	out := make([]*battleMon, len(team))
	for i, c := range team {
		out[i] = &battleMon{c: c}
	}
	return out
}

func firstAlive(team []*battleMon) int {
	for i, m := range team {
		if !m.c.Fainted() {
			return i
		}
	}
	return 0
}

// side picks the team and active index for a side.
func (b *Battle) side(a bool) ([]*battleMon, *int) {
	if a {
		return b.teamA, &b.activeA
	}
	return b.teamB, &b.activeB
}

// Active returns the on-field creature for a side.
func (b *Battle) Active(a bool) *Creature {
	team, idx := b.side(a)
	return team[*idx].c
}

// ActiveIndex returns the team slot of a side's on-field creature. It's useful
// for a "skip" turn (switching to the already-active slot is a no-op that still
// lets the opponent act, e.g. while attempting a catch).
func (b *Battle) ActiveIndex(a bool) int {
	_, idx := b.side(a)
	return *idx
}

// Team returns the creatures of a side, in slot order.
func (b *Battle) Team(a bool) []*Creature {
	team, _ := b.side(a)
	out := make([]*Creature, len(team))
	for i, m := range team {
		out[i] = m.c
	}
	return out
}

// AliveCount reports how many of a side's creatures can still fight.
func (b *Battle) AliveCount(a bool) int {
	team, _ := b.side(a)
	n := 0
	for _, m := range team {
		if !m.c.Fainted() {
			n++
		}
	}
	return n
}

// NeedsSwitch reports whether a side's active has fainted but it still has
// creatures to send out — the controller must call ApplyForcedSwitch.
func (b *Battle) NeedsSwitch(a bool) bool {
	if b.done {
		return false
	}
	team, idx := b.side(a)
	return team[*idx].c.Fainted() && b.AliveCount(a) > 0
}

// LegalSwitches returns the slots a side may switch to (alive and not active).
func (b *Battle) LegalSwitches(a bool) []int {
	team, idx := b.side(a)
	var out []int
	for i, m := range team {
		if i != *idx && !m.c.Fainted() {
			out = append(out, i)
		}
	}
	return out
}

// Done reports whether the battle is over; WinnerA says which side won.
func (b *Battle) Done() bool    { return b.done }
func (b *Battle) WinnerA() bool { return b.winnerA }

// ApplyForcedSwitch brings in a fainted side's replacement after a KO.
func (b *Battle) ApplyForcedSwitch(a bool, slot int) []string {
	team, idx := b.side(a)
	if slot < 0 || slot >= len(team) || team[slot].c.Fainted() {
		return nil
	}
	*idx = slot
	return []string{nameOf(a) + " sent out " + team[slot].c.Name() + "!"}
}

// ResolveTurn plays one full turn from both sides' actions: switches first, then
// attacks in speed order (a KO cancels the victim's queued attack). It returns
// the turn's event log and updates the battle's done/winner state.
func (b *Battle) ResolveTurn(actA, actB Action) []string {
	if b.done {
		return nil
	}
	var log []string

	// Switches resolve before any attack.
	if actA.Kind == Switch {
		log = append(log, b.doSwitch(true, actA.Index)...)
	}
	if actB.Kind == Switch {
		log = append(log, b.doSwitch(false, actB.Index)...)
	}

	// Order the attackers by speed (ties broken randomly).
	type turn struct{ a bool }
	var order []turn
	if actA.Kind == Attack {
		order = append(order, turn{true})
	}
	if actB.Kind == Attack {
		order = append(order, turn{false})
	}
	if len(order) == 2 {
		sa, sb := b.Active(true).Spd(), b.Active(false).Spd()
		bFirst := sb > sa || (sb == sa && b.rng.Intn(2) == 0)
		if bFirst {
			order[0], order[1] = order[1], order[0]
		}
	}

	for _, tn := range order {
		if b.done {
			break
		}
		team, idx := b.side(tn.a)
		if team[*idx].c.Fainted() {
			continue // KO'd by the faster attacker this turn
		}
		log = append(log, b.doAttack(tn.a, actA, actB)...)
	}
	return log
}

// doSwitch swaps a side's active to slot, resetting the outgoing one's stages.
func (b *Battle) doSwitch(a bool, slot int) []string {
	team, idx := b.side(a)
	if slot < 0 || slot >= len(team) || slot == *idx || team[slot].c.Fainted() {
		return nil
	}
	out := team[*idx]
	out.atkStage, out.defStage = 0, 0
	*idx = slot
	return []string{nameOf(a) + " switched to " + team[slot].c.Name() + "!"}
}

// doAttack performs the attacking side's move against the current defender.
func (b *Battle) doAttack(a bool, actA, actB Action) []string {
	act := actA
	if !a {
		act = actB
	}
	team, idx := b.side(a)
	attacker := team[*idx]
	defTeam, defIdx := b.side(!a)
	defender := defTeam[*defIdx]

	mv := attacker.moveAt(act.Index)
	log := []string{attacker.c.Name() + " used " + mv.Name + "."}

	// Accuracy.
	if b.rng.Intn(100) >= mv.Acc {
		return append(log, "It missed!")
	}

	if mv.Power > 0 {
		dmg, eff := b.damage(attacker, defender, mv)
		defender.c.CurHP -= dmg
		if defender.c.CurHP < 0 {
			defender.c.CurHP = 0
		}
		if eff >= superMult {
			log = append(log, "It's super effective!")
		} else if eff <= resistMult {
			log = append(log, "It's not very effective…")
		}
	}
	log = append(log, b.applyEffect(attacker, defender, mv)...)

	if defender.c.Fainted() {
		log = append(log, defender.c.Name()+" fainted!")
		b.checkEnd()
	}
	return log
}

// applyEffect runs a move's secondary (status moves are pure effect).
func (b *Battle) applyEffect(attacker, defender *battleMon, mv Move) []string {
	switch mv.Effect {
	case EffRaiseAtk:
		if attacker.atkStage < 3 {
			attacker.atkStage++
			return []string{attacker.c.Name() + "'s attack rose!"}
		}
	case EffLowerDef:
		if defender.defStage > -3 {
			defender.defStage--
			return []string{defender.c.Name() + "'s defense fell!"}
		}
	case EffHeal:
		heal := attacker.c.MaxHP() / 4
		attacker.c.CurHP += heal
		if attacker.c.CurHP > attacker.c.MaxHP() {
			attacker.c.CurHP = attacker.c.MaxHP()
		}
		return []string{attacker.c.Name() + " drew in health."}
	}
	return nil
}

// damage computes a move's damage and the type multiplier it landed at.
func (b *Battle) damage(attacker, defender *battleMon, mv Move) (int, float64) {
	atk := float64(attacker.c.Atk()) * stageMul(attacker.atkStage)
	def := float64(defender.c.Def()) * stageMul(defender.defStage)
	if def < 1 {
		def = 1
	}
	l := float64(attacker.c.Level)
	base := (2*l/5+2)*float64(mv.Power)*atk/def/50 + 2
	eff := Effectiveness(mv.Type, defender.c.Type())
	roll := 0.85 + 0.15*b.rng.Float64()
	dmg := int(base * eff * roll)
	if dmg < 1 {
		dmg = 1
	}
	return dmg, eff
}

// checkEnd marks the battle over when one side has no fighters left.
func (b *Battle) checkEnd() {
	switch {
	case b.AliveCount(false) == 0:
		b.done, b.winnerA = true, true
	case b.AliveCount(true) == 0:
		b.done, b.winnerA = true, false
	}
}

// ChooseAI picks an action for a side: the known move with the best expected
// damage against the current foe (falling back to any status move).
func (b *Battle) ChooseAI(a bool) Action {
	attacker := b.Active(a)
	foe := b.Active(!a)
	best, bestScore := 0, -1.0
	for i, key := range attacker.Moves {
		mv := moves[key]
		score := float64(mv.Power) * Effectiveness(mv.Type, foe.Type())
		if mv.Power == 0 {
			score = 1 // a status move is a weak fallback
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	return Action{Kind: Attack, Index: best}
}

// BestSwitch picks the strongest replacement for a side after a KO: the alive
// reserve with the best type matchup against the current foe.
func (b *Battle) BestSwitch(a bool) int {
	foe := b.Active(!a)
	team, _ := b.side(a)
	best, bestEff := -1, -1.0
	for i, m := range team {
		if m.c.Fainted() {
			continue
		}
		if eff := Effectiveness(m.c.Type(), foe.Type()); eff > bestEff {
			best, bestEff = i, eff
		}
	}
	if best < 0 {
		best = 0
	}
	return best
}

// moveAt returns the creature's move at slot i (clamped to its first move).
func (m *battleMon) moveAt(i int) Move {
	if i < 0 || i >= len(m.c.Moves) {
		i = 0
	}
	if len(m.c.Moves) == 0 {
		return moves["tackle"]
	}
	return moves[m.c.Moves[i]]
}

// stageMul converts a stat stage (-3..3) into a multiplier (±25% per stage).
func stageMul(stage int) float64 {
	if stage < -3 {
		stage = -3
	}
	if stage > 3 {
		stage = 3
	}
	return 1 + 0.25*float64(stage)
}

func nameOf(a bool) string {
	if a {
		return "You"
	}
	return "The foe"
}
