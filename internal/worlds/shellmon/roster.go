package shellmon

import (
	"encoding/json"

	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// savedRoster is the JSON shape persisted in the per-world SaveStore. The
// "party" key is kept stable so the plaza's mon.UnmarshalParty (used to load a
// team for PvP) still reads it; the progress fields are extra and ignored there.
type savedRoster struct {
	Party    []*mon.Creature `json:"party"`
	Defeated []string        `json:"defeated"` // beaten trainer ids
	Found    []string        `json:"found"`    // collected secret/item ids
}

// loadRoster reads the party and overworld progress (empty on first visit).
func (m *model) loadRoster() {
	m.defeated = map[string]bool{}
	m.found = map[string]bool{}
	data, err := m.ctx.Save.Load()
	if err != nil || len(data) == 0 {
		return
	}
	var sr savedRoster
	if json.Unmarshal(data, &sr) != nil {
		return
	}
	for _, c := range sr.Party {
		if c != nil && mon.ValidSpecies(c.Species) {
			m.roster = append(m.roster, c)
		}
	}
	for _, id := range sr.Defeated {
		m.defeated[id] = true
	}
	for _, id := range sr.Found {
		m.found[id] = true
	}
}

// saveRoster writes the party and progress back to the SaveStore.
func (m *model) saveRoster() {
	if m.ctx.Save == nil {
		return
	}
	sr := savedRoster{Party: m.roster}
	for id := range m.defeated {
		sr.Defeated = append(sr.Defeated, id)
	}
	for id := range m.found {
		sr.Found = append(sr.Found, id)
	}
	if data, err := json.Marshal(sr); err == nil {
		_ = m.ctx.Save.Save(data)
	}
}

// healParty restores every party member to full health.
func (m *model) healParty() {
	for _, c := range m.roster {
		c.Heal()
	}
}

// partyAlive reports whether any party member can still fight.
func (m *model) partyAlive() bool {
	for _, c := range m.roster {
		if !c.Fainted() {
			return true
		}
	}
	return false
}
