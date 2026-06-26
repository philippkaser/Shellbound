// Package shellmon is Shellbound's creature battler: an original roster of
// monochrome "Shellmon", a turn-based 6v6 battle engine with a three-type
// triangle, and the leveling/catching rules. It is pure game logic and data —
// no rendering, storage or networking — so it can be unit-tested on its own and
// reused by both the single-player wild route and (later) PvP.
package shellmon

// Type is a Shellmon's elemental class. Plain is the neutral class used by
// basic moves: never strong, never weak. The other three form a triangle.
type Type int

// Types. The triangle is Spark ▸ Bramble ▸ Tide ▸ Spark.
const (
	Plain Type = iota
	Spark
	Bramble
	Tide
)

// String is the display name of a type.
func (t Type) String() string {
	switch t {
	case Spark:
		return "Spark"
	case Bramble:
		return "Bramble"
	case Tide:
		return "Tide"
	default:
		return "Plain"
	}
}

// Effectiveness multipliers for an attacking type against a defending type.
const (
	superMult   = 1.5
	resistMult  = 0.67
	neutralMult = 1.0
)

// Effectiveness returns the damage multiplier for an atk-type move landing on a
// def-type target. Plain on either side is always neutral.
func Effectiveness(atk, def Type) float64 {
	if atk == Plain || def == Plain {
		return neutralMult
	}
	if beats(atk, def) {
		return superMult
	}
	if beats(def, atk) {
		return resistMult
	}
	return neutralMult
}

// beats reports whether type a is super-effective against type b.
func beats(a, b Type) bool {
	switch a {
	case Spark:
		return b == Bramble
	case Bramble:
		return b == Tide
	case Tide:
		return b == Spark
	}
	return false
}
