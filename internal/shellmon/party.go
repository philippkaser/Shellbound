package shellmon

import "encoding/json"

// partyJSON is the on-disk/wire shape of a saved team.
type partyJSON struct {
	Party []*Creature `json:"party"`
}

// MarshalParty encodes a team to JSON (used by the world's SaveStore).
func MarshalParty(team []*Creature) ([]byte, error) {
	return json.Marshal(partyJSON{Party: team})
}

// UnmarshalParty decodes a team, dropping entries with an unknown species so a
// stale save can't crash the game.
func UnmarshalParty(data []byte) []*Creature {
	if len(data) == 0 {
		return nil
	}
	var p partyJSON
	if json.Unmarshal(data, &p) != nil {
		return nil
	}
	out := make([]*Creature, 0, len(p.Party))
	for _, c := range p.Party {
		if c != nil && ValidSpecies(c.Species) {
			out = append(out, c)
		}
	}
	return out
}

// CloneTeam returns deep copies of a team (fresh Creature values), so a battle
// can mutate HP without touching the owner's saved roster.
func CloneTeam(team []*Creature) []*Creature {
	out := make([]*Creature, len(team))
	for i, c := range team {
		cp := *c
		cp.Moves = append([]string(nil), c.Moves...)
		out[i] = &cp
	}
	return out
}
