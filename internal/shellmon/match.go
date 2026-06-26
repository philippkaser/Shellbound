package shellmon

import (
	"math/rand"
	"sync"
)

// Match coordinates a turn-based 6v6 duel between two live players who share one
// process. Both sides hold the same *Match and poll it (it is mutex-guarded);
// each submits its action for the turn, and the turn resolves once both are in.
// Snapshot returns an immutable view so rendering never races the engine.
type Match struct {
	mu       sync.Mutex
	b        *Battle
	pendingA *Action
	pendingB *Action
	log      []string
	done     bool
	winnerA  bool

	// last-resolved-turn effects, for the renderers to animate.
	turnSeq              int
	lastTypeA, lastTypeB Type
	lastCastA, lastCastB bool
}

// NewMatch builds a duel between two teams, healing both to full so the fight is
// fair regardless of the rosters' saved condition. seed makes resolution
// deterministic for tests.
func NewMatch(teamA, teamB []*Creature, seed int64) *Match {
	a, b := CloneTeam(teamA), CloneTeam(teamB)
	for _, c := range a {
		c.Heal()
	}
	for _, c := range b {
		c.Heal()
	}
	m := &Match{b: NewBattle(a, b, rand.New(rand.NewSource(seed)))}
	m.log = []string{"Battle start!"}
	return m
}

// Submit records a side's action; when both sides have acted (or the acting
// side resolves a forced switch) the turn advances.
func (m *Match) Submit(sideA bool, act Action) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done {
		return
	}
	// Forced-switch phase: only the fainted side may act, and only by switching.
	if m.b.NeedsSwitch(true) || m.b.NeedsSwitch(false) {
		if m.b.NeedsSwitch(sideA) && act.Kind == Switch {
			m.log = appendTail(m.log, m.b.ApplyForcedSwitch(sideA, act.Index)...)
			m.refresh()
		}
		return
	}
	// Choose phase: stash this side's action, resolve when both are present.
	if sideA {
		if m.pendingA == nil {
			a := act
			m.pendingA = &a
		}
	} else {
		if m.pendingB == nil {
			b := act
			m.pendingB = &b
		}
	}
	if m.pendingA != nil && m.pendingB != nil {
		m.lastTypeA, m.lastCastA = actionMoveType(m.b.Active(true), *m.pendingA)
		m.lastTypeB, m.lastCastB = actionMoveType(m.b.Active(false), *m.pendingB)
		m.turnSeq++
		m.log = appendTail(m.log, m.b.ResolveTurn(*m.pendingA, *m.pendingB)...)
		m.pendingA, m.pendingB = nil, nil
		m.refresh()
	}
}

// Forfeit ends the match immediately, awarding the win to the other side.
func (m *Match) Forfeit(sideA bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done {
		return
	}
	m.done, m.winnerA = true, !sideA
	m.log = appendTail(m.log, "A challenger forfeited.")
}

// refresh syncs the match's done/winner from the engine after a resolution.
func (m *Match) refresh() {
	if m.b.Done() {
		m.done, m.winnerA = true, m.b.WinnerA()
	}
}

func (m *Match) submitted(sideA bool) bool {
	if sideA {
		return m.pendingA != nil
	}
	return m.pendingB != nil
}

// --- snapshot for race-free rendering ---

// MoveView is a move as the UI shows it.
type MoveView struct {
	Name  string
	Type  Type
	Power int
}

// Combatant is the on-field creature as the UI shows it.
type Combatant struct {
	Name    string
	Species string
	Level   int
	HP      int
	MaxHP   int
	Type    Type
	Moves   []MoveView
}

// PartyMember is a roster entry as the switch menu shows it.
type PartyMember struct {
	Name    string
	Level   int
	HP      int
	MaxHP   int
	Type    Type
	Fainted bool
	Active  bool
}

// View is an immutable snapshot of the match from one side's perspective.
type View struct {
	You, Foe        Combatant
	Party           []PartyMember
	YouAlive        int
	FoeAlive        int
	FoeTotal        int
	Log             []string
	Done            bool
	YouWon          bool
	AwaitingYou     bool // you must choose an action this turn
	MustSwitchYou   bool // your active fainted; choose a replacement
	WaitingOpponent bool // your move is in; waiting on the other player

	// Last-resolved turn, for cast/impact effects.
	TurnSeq     int
	YouLastType Type
	FoeLastType Type
	YouCast     bool
	FoeCast     bool
}

// Snapshot captures everything the given side needs to render, under the lock.
func (m *Match) Snapshot(sideA bool) View {
	m.mu.Lock()
	defer m.mu.Unlock()

	v := View{
		You:      combatant(m.b.Active(sideA)),
		Foe:      combatant(m.b.Active(!sideA)),
		YouAlive: m.b.AliveCount(sideA),
		FoeAlive: m.b.AliveCount(!sideA),
		FoeTotal: len(m.b.Team(!sideA)),
		Log:      append([]string(nil), m.log...),
		Done:     m.done,
		YouWon:   m.winnerA == sideA,
		TurnSeq:  m.turnSeq,
	}
	if sideA {
		v.YouLastType, v.YouCast = m.lastTypeA, m.lastCastA
		v.FoeLastType, v.FoeCast = m.lastTypeB, m.lastCastB
	} else {
		v.YouLastType, v.YouCast = m.lastTypeB, m.lastCastB
		v.FoeLastType, v.FoeCast = m.lastTypeA, m.lastCastA
	}
	for i, c := range m.b.Team(sideA) {
		v.Party = append(v.Party, PartyMember{
			Name: c.Name(), Level: c.Level, HP: c.CurHP, MaxHP: c.MaxHP(),
			Type: c.Type(), Fainted: c.Fainted(), Active: i == m.b.ActiveIndex(sideA),
		})
	}
	switch {
	case m.done:
	case m.b.NeedsSwitch(sideA):
		v.MustSwitchYou = true
	case m.b.NeedsSwitch(!sideA):
		v.WaitingOpponent = true
	case m.submitted(sideA):
		v.WaitingOpponent = true
	default:
		v.AwaitingYou = true
	}
	return v
}

func combatant(c *Creature) Combatant {
	cb := Combatant{Name: c.Name(), Species: c.Species, Level: c.Level, HP: c.CurHP, MaxHP: c.MaxHP(), Type: c.Type()}
	for _, mk := range c.Moves {
		if mv, ok := moves[mk]; ok {
			cb.Moves = append(cb.Moves, MoveView{Name: mv.Name, Type: mv.Type, Power: mv.Power})
		}
	}
	return cb
}

// ActiveIndexFor exposes the on-field slot for a side (for the switch cursor).
func (m *Match) ActiveIndexFor(sideA bool) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.b.ActiveIndex(sideA)
}

// actionMoveType returns the elemental type of an attack action and whether it
// is a damaging move worth animating a cast for.
func actionMoveType(c *Creature, act Action) (Type, bool) {
	if act.Kind != Attack || act.Index < 0 || act.Index >= len(c.Moves) {
		return Plain, false
	}
	mv, ok := moves[c.Moves[act.Index]]
	if !ok {
		return Plain, false
	}
	return mv.Type, mv.Power > 0
}

// appendTail keeps a log to a recent window.
func appendTail(log []string, lines ...string) []string {
	log = append(log, lines...)
	if len(log) > 8 {
		log = log[len(log)-8:]
	}
	return log
}
