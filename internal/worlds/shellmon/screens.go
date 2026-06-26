package shellmon

import (
	"github.com/shellbound/shellbound/internal/render/canvas"
	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// --- starter selection ---

// keyStarter handles input on the starter screen; it returns true to leave the
// world (the player declined to pick).
func (m *model) keyStarter(key string) bool {
	switch key {
	case "left", "a":
		if m.starterCursor > 0 {
			m.starterCursor--
		}
	case "right", "d":
		if m.starterCursor < len(m.starters)-1 {
			m.starterCursor++
		}
	case "enter", " ":
		sp := m.starters[m.starterCursor]
		m.roster = append(m.roster, mon.NewCreature(sp.Key, 5))
		m.saveRoster()
		m.state = stateRoute
		m.enterRoute()
	case "esc", "q":
		return true
	}
	return false
}

func (m *model) drawStarter(pw, ph int, t float64) {
	title := "Choose your first Shellmon"
	m.scr.DrawText(pw/2-canvas.TextWidth(title)/2, 40, title, uiText)

	n := len(m.starters)
	cardW, cardH := 200, 200
	gap := 30
	totalW := n*cardW + (n-1)*gap
	x0 := pw/2 - totalW/2
	cy := ph/2 - 10
	for i, sp := range m.starters {
		x := x0 + i*(cardW+gap)
		y := cy - cardH/2
		panel(m.scr, x, y, cardW, cardH)
		if i == m.starterCursor {
			m.scr.Rect(x-2, y-2, cardW+4, cardH+4, uiBorder)
			m.scr.Rect(x-3, y-3, cardW+6, cardH+6, uiDim)
		}
		bob := 0
		if i == m.starterCursor {
			bob = int(2 * sinf(t*3))
		}
		mon.DrawCreature(m.scr, x+cardW/2, y+cardH/2-10+bob, 4, sp.Key)
		name := sp.Name
		m.scr.DrawText(x+cardW/2-canvas.TextWidth(name)/2, y+cardH-44, name, uiText)
		badge := typeBadge(sp.Type)
		m.scr.DrawText(x+cardW/2-canvas.TextWidth(badge)/2, y+cardH-30, badge, uiDim)
		stats := "HP " + itoa(sp.BaseHP) + "  ATK " + itoa(sp.BaseAtk)
		m.scr.DrawText(x+cardW/2-canvas.TextWidth(stats)/2, y+cardH-16, stats, uiDim)
	}
	hint := "A / D choose   Enter confirm   Esc leave"
	m.scr.DrawText(pw/2-canvas.TextWidth(hint)/2, ph-30, hint, uiDim)
}

// --- party view ---

func (m *model) keyParty(key string) {
	switch key {
	case "up", "w":
		if m.partyCur > 0 {
			m.partyCur--
		}
	case "down", "s":
		if m.partyCur < len(m.roster)-1 {
			m.partyCur++
		}
	case "enter", " ":
		// Move the selected creature to the front (set the battle lead).
		if m.partyCur > 0 && m.partyCur < len(m.roster) {
			c := m.roster[m.partyCur]
			m.roster = append(m.roster[:m.partyCur], m.roster[m.partyCur+1:]...)
			m.roster = append([]*mon.Creature{c}, m.roster...)
			m.partyCur = 0
			m.saveRoster()
		}
	case "esc", "p", "q":
		m.state = stateRoute
	}
}

func (m *model) drawParty(pw, ph int, t float64) {
	title := "Your Team (" + itoa(len(m.roster)) + "/" + itoa(maxParty) + ")"
	m.scr.DrawText(40, 30, title, uiText)

	// List on the left.
	lx, ly := 40, 60
	rowH := 46
	for i, c := range m.roster {
		y := ly + i*rowH
		if i == m.partyCur {
			m.scr.FillRect(lx-6, y-6, 320, rowH-4, uiSelBG)
		}
		m.scr.DrawText(lx, y, c.Name(), uiText)
		lv := "Lv" + itoa(c.Level) + " " + typeBadge(c.Type())
		m.scr.DrawText(lx+180, y, lv, uiDim)
		hpBar(m.scr, lx, y+14, 300, c.CurHP, c.MaxHP())
		if i == 0 {
			m.scr.DrawText(lx+180+canvas.TextWidth(lv)+10, y, "lead", uiDim)
		}
	}

	// Selected creature portrait + detail on the right.
	if m.partyCur < len(m.roster) {
		c := m.roster[m.partyCur]
		px := pw*3/4 - 20
		mon.DrawCreature(m.scr, px, ph/2-30, 5, c.Species)
		m.scr.DrawText(px-canvas.TextWidth(c.Name())/2, ph/2+50, c.Name(), uiText)
		stat := "HP " + itoa(c.CurHP) + "/" + itoa(c.MaxHP()) + "   ATK " + itoa(c.Atk()) + "   DEF " + itoa(c.Def()) + "   SPD " + itoa(c.Spd())
		m.scr.DrawText(px-canvas.TextWidth(stat)/2, ph/2+66, stat, uiDim)
		// Move list.
		my := ph/2 + 86
		for _, mk := range c.Moves {
			if mv, ok := mon.MoveByKey(mk); ok {
				line := "• " + mv.Name + " " + typeBadge(mv.Type)
				m.scr.DrawText(px-90, my, line, uiDim)
				my += canvas.LineH + 2
			}
		}
	}

	hint := "W / S select   Enter set lead   Esc back"
	m.scr.DrawText(pw/2-canvas.TextWidth(hint)/2, ph-30, hint, uiDim)
}
