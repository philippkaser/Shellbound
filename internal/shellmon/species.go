package shellmon

// LearnEntry is a move a species gains at a given level.
type LearnEntry struct {
	Level int
	Move  string
}

// Species is one kind of Shellmon: its identity, elemental type, base stats and
// what it learns. Instances (Creature) scale these by level.
type Species struct {
	Key      string
	Name     string
	Type     Type
	Starter  bool // offered as a first partner
	BaseHP   int
	BaseAtk  int
	BaseDef  int
	BaseSpd  int
	Learnset []LearnEntry // ascending by level
	sprite   func(s *spriteCtx)
}

// learnset builds the standard four-move arc for a typed line: a Plain basic, a
// weak elemental hit, a status trick, then a strong elemental hit, with a
// stronger Plain move later for coverage.
func learnset(weak, status, strong string) []LearnEntry {
	return []LearnEntry{
		{1, "tackle"},
		{1, weak},
		{6, status},
		{12, strong},
		{18, "bite"},
	}
}

// catalog is the full roster, grouped by type. Stat spreads give each line a
// role (fast/frail, bulky, balanced) so 6-Shellmon teams have texture.
var catalog = []Species{
	// --- Spark ---
	{Key: "cindle", Name: "Cindle", Type: Spark, Starter: true,
		BaseHP: 42, BaseAtk: 56, BaseDef: 40, BaseSpd: 58,
		Learnset: learnset("ember", "kindle", "flare"), sprite: spriteCindle},
	{Key: "flickit", Name: "Flickit", Type: Spark,
		BaseHP: 46, BaseAtk: 60, BaseDef: 44, BaseSpd: 56,
		Learnset: learnset("ember", "kindle", "flare"), sprite: spriteFlickit},
	{Key: "cindershell", Name: "Cindershell", Type: Spark,
		BaseHP: 60, BaseAtk: 52, BaseDef: 64, BaseSpd: 34,
		Learnset: learnset("ember", "kindle", "flare"), sprite: spriteCindershell},
	{Key: "ashfin", Name: "Ashfin", Type: Spark,
		BaseHP: 50, BaseAtk: 66, BaseDef: 44, BaseSpd: 50,
		Learnset: learnset("ember", "kindle", "flare"), sprite: spriteAshfin},

	// --- Bramble ---
	{Key: "sprigling", Name: "Sprigling", Type: Bramble, Starter: true,
		BaseHP: 50, BaseAtk: 50, BaseDef: 54, BaseSpd: 46,
		Learnset: learnset("vine", "root", "thorn"), sprite: spriteSprigling},
	{Key: "thornpod", Name: "Thornpod", Type: Bramble,
		BaseHP: 54, BaseAtk: 56, BaseDef: 60, BaseSpd: 34,
		Learnset: learnset("vine", "root", "thorn"), sprite: spriteThornpod},
	{Key: "mossmaw", Name: "Mossmaw", Type: Bramble,
		BaseHP: 66, BaseAtk: 60, BaseDef: 50, BaseSpd: 38,
		Learnset: learnset("vine", "root", "thorn"), sprite: spriteMossmaw},
	{Key: "fernling", Name: "Fernling", Type: Bramble,
		BaseHP: 46, BaseAtk: 50, BaseDef: 50, BaseSpd: 60,
		Learnset: learnset("vine", "root", "thorn"), sprite: spriteFernling},

	// --- Tide ---
	{Key: "dripling", Name: "Dripling", Type: Tide, Starter: true,
		BaseHP: 50, BaseAtk: 46, BaseDef: 52, BaseSpd: 54,
		Learnset: learnset("splash", "mist", "wave"), sprite: spriteDripling},
	{Key: "brineback", Name: "Brineback", Type: Tide,
		BaseHP: 62, BaseAtk: 50, BaseDef: 66, BaseSpd: 34,
		Learnset: learnset("splash", "mist", "wave"), sprite: spriteBrineback},
	{Key: "tidecoil", Name: "Tidecoil", Type: Tide,
		BaseHP: 50, BaseAtk: 62, BaseDef: 44, BaseSpd: 60,
		Learnset: learnset("splash", "mist", "wave"), sprite: spriteTidecoil},
	{Key: "gulper", Name: "Gulper", Type: Tide,
		BaseHP: 66, BaseAtk: 55, BaseDef: 50, BaseSpd: 40,
		Learnset: learnset("splash", "mist", "wave"), sprite: spriteGulper},

	// --- second wave ---

	{Key: "voltun", Name: "Voltun", Type: Spark,
		BaseHP: 48, BaseAtk: 58, BaseDef: 46, BaseSpd: 66,
		Learnset: learnset("ember", "kindle", "flare"), sprite: spriteVoltun},
	{Key: "magmaw", Name: "Magmaw", Type: Spark,
		BaseHP: 64, BaseAtk: 64, BaseDef: 54, BaseSpd: 30,
		Learnset: learnset("ember", "kindle", "flare"), sprite: spriteMagmaw},
	{Key: "frostnip", Name: "Frostnip", Type: Tide,
		BaseHP: 46, BaseAtk: 52, BaseDef: 48, BaseSpd: 60,
		Learnset: learnset("splash", "mist", "wave"), sprite: spriteFrostnip},
	{Key: "anchora", Name: "Anchora", Type: Tide,
		BaseHP: 68, BaseAtk: 56, BaseDef: 66, BaseSpd: 28,
		Learnset: learnset("splash", "mist", "wave"), sprite: spriteAnchora},
	{Key: "pricklepup", Name: "Pricklepup", Type: Bramble,
		BaseHP: 50, BaseAtk: 58, BaseDef: 48, BaseSpd: 56,
		Learnset: learnset("vine", "root", "thorn"), sprite: spritePricklepup},
	{Key: "bloomback", Name: "Bloomback", Type: Bramble,
		BaseHP: 66, BaseAtk: 52, BaseDef: 64, BaseSpd: 30,
		Learnset: learnset("vine", "root", "thorn"), sprite: spriteBloomback},
}

var bySpecies = func() map[string]Species {
	m := make(map[string]Species, len(catalog))
	for _, s := range catalog {
		m[s.Key] = s
	}
	return m
}()

// All returns the full species roster in catalog order.
func All() []Species { return catalog }

// Starters returns the species offered as a first partner, one per type.
func Starters() []Species {
	var out []Species
	for _, s := range catalog {
		if s.Starter {
			out = append(out, s)
		}
	}
	return out
}

// SpeciesByKey returns a species and whether it exists.
func SpeciesByKey(key string) (Species, bool) {
	s, ok := bySpecies[key]
	return s, ok
}

// ValidSpecies reports whether key names a real species.
func ValidSpecies(key string) bool { _, ok := bySpecies[key]; return ok }
