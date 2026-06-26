package shellmon

import (
	"encoding/json"

	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// savedRoster is the JSON shape persisted in the per-world SaveStore.
type savedRoster struct {
	Party []*mon.Creature `json:"party"`
}

// loadRoster reads the party from the SaveStore (empty on first visit).
func (m *model) loadRoster() {
	data, err := m.ctx.Save.Load()
	if err != nil || len(data) == 0 {
		return
	}
	var sr savedRoster
	if json.Unmarshal(data, &sr) == nil {
		for _, c := range sr.Party {
			if c != nil && mon.ValidSpecies(c.Species) {
				m.roster = append(m.roster, c)
			}
		}
	}
}

// saveRoster writes the party back to the SaveStore.
func (m *model) saveRoster() {
	if m.ctx.Save == nil {
		return
	}
	data, err := json.Marshal(savedRoster{Party: m.roster})
	if err == nil {
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
