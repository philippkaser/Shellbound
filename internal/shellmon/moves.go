package shellmon

// Effect is an optional secondary a move applies (besides damage). Status moves
// have Power 0 and exist only for their effect.
type Effect int

// Effects.
const (
	EffNone     Effect = iota
	EffRaiseAtk        // raise the user's attack one stage
	EffHeal            // heal the user for a quarter of its max HP
	EffLowerDef        // lower the target's defense one stage
)

// Move is one action a Shellmon can take. Power 0 marks a status move.
type Move struct {
	Key    string
	Name   string
	Type   Type
	Power  int
	Acc    int // hit chance, 0..100
	Effect Effect
}

// moves is the move registry, keyed by Move.Key.
var moves = func() map[string]Move {
	list := []Move{
		// Plain basics — never strong, never weak.
		{Key: "tackle", Name: "Tackle", Type: Plain, Power: 40, Acc: 100},
		{Key: "bite", Name: "Bite", Type: Plain, Power: 60, Acc: 95},

		// Spark.
		{Key: "ember", Name: "Ember", Type: Spark, Power: 45, Acc: 100},
		{Key: "flare", Name: "Flare", Type: Spark, Power: 75, Acc: 90},
		{Key: "kindle", Name: "Kindle", Type: Spark, Power: 0, Acc: 100, Effect: EffRaiseAtk},

		// Bramble.
		{Key: "vine", Name: "Vine Lash", Type: Bramble, Power: 45, Acc: 100},
		{Key: "thorn", Name: "Thorn Volley", Type: Bramble, Power: 75, Acc: 90},
		{Key: "root", Name: "Rootbind", Type: Bramble, Power: 0, Acc: 100, Effect: EffHeal},

		// Tide.
		{Key: "splash", Name: "Splash", Type: Tide, Power: 45, Acc: 100},
		{Key: "wave", Name: "Wave Crash", Type: Tide, Power: 75, Acc: 90},
		{Key: "mist", Name: "Mist Veil", Type: Tide, Power: 0, Acc: 100, Effect: EffLowerDef},
	}
	m := make(map[string]Move, len(list))
	for _, mv := range list {
		m[mv.Key] = mv
	}
	return m
}()

// MoveByKey returns a move and whether it exists.
func MoveByKey(key string) (Move, bool) {
	mv, ok := moves[key]
	return mv, ok
}
