package shellmon

// Level bounds for a Shellmon.
const (
	MinLevel = 1
	MaxLevel = 50
)

// Creature is one owned Shellmon instance. Stats are derived from the species
// and Level; CurHP persists between battles (you heal up at the plaza), and
// Moves is the known set (at most four), newest-learned last.
type Creature struct {
	Species string
	Nick    string // optional nickname; "" falls back to the species name
	Level   int
	XP      int // progress toward the next level
	CurHP   int
	Moves   []string
}

// NewCreature builds a fresh instance of a species at the given level, knowing
// the (up to four) most recent moves from its learnset and at full health.
func NewCreature(speciesKey string, level int) *Creature {
	sp, ok := bySpecies[speciesKey]
	if !ok {
		return nil
	}
	level = clampLevel(level)
	c := &Creature{Species: speciesKey, Level: level, Moves: movesAtLevel(sp, level)}
	c.CurHP = c.MaxHP()
	return c
}

// movesAtLevel returns the most recent (up to four) learnset moves a species
// knows at the given level.
func movesAtLevel(sp Species, level int) []string {
	var known []string
	for _, e := range sp.Learnset {
		if e.Level <= level {
			known = append(known, e.Move)
		}
	}
	if len(known) > 4 {
		known = known[len(known)-4:]
	}
	return known
}

// species returns the creature's species (falls back to the first catalog entry
// for an unknown key, which only happens on corrupt data).
func (c *Creature) species() Species {
	if sp, ok := bySpecies[c.Species]; ok {
		return sp
	}
	return catalog[0]
}

// Name is the nickname if set, else the species name.
func (c *Creature) Name() string {
	if c.Nick != "" {
		return c.Nick
	}
	return c.species().Name
}

// Type is the creature's elemental class.
func (c *Creature) Type() Type { return c.species().Type }

// scale grows a base stat with level: roughly +7% of base per level.
func scale(base, level int) int { return base + base*(level-1)/15 }

// MaxHP is the creature's full health at its level (HP scales a touch faster and
// carries a flat floor so low levels aren't one-shot).
func (c *Creature) MaxHP() int {
	sp := c.species()
	return sp.BaseHP + sp.BaseHP*(c.Level-1)/8 + c.Level + 8
}

// Atk, Def and Spd are the level-scaled combat stats.
func (c *Creature) Atk() int { return scale(c.species().BaseAtk, c.Level) }
func (c *Creature) Def() int { return scale(c.species().BaseDef, c.Level) }
func (c *Creature) Spd() int { return scale(c.species().BaseSpd, c.Level) }

// Fainted reports whether the creature is out of HP.
func (c *Creature) Fainted() bool { return c.CurHP <= 0 }

// Heal restores the creature to full health (used at the plaza between trips).
func (c *Creature) Heal() { c.CurHP = c.MaxHP() }

// XPToNext is the experience needed to reach the next level from the current.
func XPToNext(level int) int { return 25 + (level-1)*15 }

// GainXP awards experience, leveling up as thresholds are crossed (raising
// stats, topping up HP by the gain, and learning any new moves). It returns a
// log of what happened and stops at MaxLevel.
func (c *Creature) GainXP(amount int) []string {
	var log []string
	if amount <= 0 || c.Level >= MaxLevel {
		return log
	}
	c.XP += amount
	for c.Level < MaxLevel && c.XP >= XPToNext(c.Level) {
		c.XP -= XPToNext(c.Level)
		before := c.MaxHP()
		c.Level++
		c.CurHP += c.MaxHP() - before // level-up tops up by the HP gained
		log = append(log, c.Name()+" grew to level "+itoa(c.Level)+"!")
		if mv := c.learnAt(c.Level); mv != "" {
			log = append(log, c.Name()+" learned "+mv+"!")
		}
	}
	if c.Level >= MaxLevel {
		c.XP = 0
	}
	return log
}

// learnAt teaches any move the species gains exactly at this level, dropping the
// oldest when already at four. It returns the learned move's display name (or
// "" if none).
func (c *Creature) learnAt(level int) string {
	sp := c.species()
	var learned string
	for _, e := range sp.Learnset {
		if e.Level != level {
			continue
		}
		if has(c.Moves, e.Move) {
			continue
		}
		c.Moves = append(c.Moves, e.Move)
		if len(c.Moves) > 4 {
			c.Moves = c.Moves[1:]
		}
		if mv, ok := moves[e.Move]; ok {
			learned = mv.Name
		}
	}
	return learned
}

// CatchChance is the probability (0..1) of catching a wild creature given how
// hurt it is — full health is hard, near-fainting is easy.
func CatchChance(c *Creature) float64 {
	if c.MaxHP() <= 0 {
		return 0.9
	}
	frac := float64(c.CurHP) / float64(c.MaxHP())
	chance := 0.2 + 0.6*(1-frac)
	if chance > 0.9 {
		chance = 0.9
	}
	if chance < 0.05 {
		chance = 0.05
	}
	return chance
}

func clampLevel(l int) int {
	if l < MinLevel {
		return MinLevel
	}
	if l > MaxLevel {
		return MaxLevel
	}
	return l
}

func has(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// itoa is a tiny strconv.Itoa to keep this package import-light.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
