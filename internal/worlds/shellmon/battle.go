package shellmon

import (
	"github.com/shellbound/shellbound/internal/render/canvas"
	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// battleSub is the active sub-screen within a battle.
type battleSub int

const (
	subMenu   battleSub = iota // top-level action menu
	subMove                    // choosing a move
	subSwitch                  // choosing a creature to send out
	subOver                    // battle resolved; press to continue
)

// menu actions, in 2×2 grid order.
var menuItems = []string{"Fight", "Catch", "Switch", "Run"}

// battleUI is the per-battle interaction state on top of the engine.
type battleUI struct {
	b      *mon.Battle
	wild   bool
	sub    battleSub
	menu   int // 0..3
	move   int
	swap   int
	forced bool // a faint forced the switch (can't back out)
	log    []string
	result string // outcome shown on the subOver screen
	won    bool

	// trainer battles (empty for wild encounters)
	trainerID   string
	trainerName string
	reward      string // cosmetic key granted on first defeat
	rewardN     string // its display name
	badge       string // gym badge id granted on first defeat ("" = none)
	wonBadge    bool   // set when this battle just earned the badge
}

// beginBattle opens a battle of the player's roster against an opponent team
// (a single creature for wild encounters, a full team for trainers).
func (m *model) beginBattle(team []*mon.Creature, wild bool, opp string) {
	bt := &battleUI{
		b:    mon.NewBattle(m.roster, team, m.rng),
		wild: wild,
		sub:  subMenu,
	}
	if wild {
		bt.log = []string{"A wild " + team[0].Name() + " appeared!"}
	} else {
		bt.log = []string{opp + " sent out " + team[0].Name() + "!"}
	}
	m.bt = bt
	m.youFX, m.foeFX, m.anim, m.animSeq = hpFX{}, hpFX{}, battleAnim{}, 0
	m.trans.begin(0.5) // encounter wipe
	m.state = stateBattle
}

// resolveTurn runs the player's action against the foe's AI, capturing both
// move types so the renderer can play the cast/impact effects.
func (m *model) resolveTurn(playerAct mon.Action, youCast bool, youType mon.Type) {
	bt := m.bt
	foeAct := bt.b.ChooseAI(false)
	foeType, foeCast := moveType(bt.b.Active(false), foeAct.Index)
	bt.log = appendLog(bt.log, bt.b.ResolveTurn(playerAct, foeAct)...)
	m.animSeq++
	m.anim.trigger(m.animSeq, youType, youCast, foeType, foeCast)
	bt.sub = subMenu
	m.afterTurn()
}

func (m *model) keyBattle(key string) {
	bt := m.bt
	if bt == nil {
		return
	}
	switch bt.sub {
	case subOver:
		m.finishBattle()
	case subMenu:
		m.keyBattleMenu(key)
	case subMove:
		m.keyBattleMove(key)
	case subSwitch:
		m.keyBattleSwitch(key)
	}
}

func (m *model) keyBattleMenu(key string) {
	bt := m.bt
	switch key {
	case "left", "a":
		if bt.menu%2 == 1 {
			bt.menu--
		}
	case "right", "d":
		if bt.menu%2 == 0 {
			bt.menu++
		}
	case "up", "w":
		if bt.menu >= 2 {
			bt.menu -= 2
		}
	case "down", "s":
		if bt.menu < 2 {
			bt.menu += 2
		}
	case "enter", " ":
		m.chooseMenu(bt.menu)
	case "f":
		m.chooseMenu(0)
	case "c":
		m.chooseMenu(1)
	case "x":
		m.chooseMenu(2)
	case "r":
		m.chooseMenu(3)
	}
}

func (m *model) chooseMenu(item int) {
	bt := m.bt
	switch item {
	case 0: // Fight
		bt.move = 0
		bt.sub = subMove
	case 1: // Catch
		m.attemptCatch()
	case 2: // Switch
		bt.forced = false
		bt.swap = m.firstSwitchable()
		bt.sub = subSwitch
	case 3: // Run
		if bt.wild {
			bt.log = appendLog(bt.log, "You fled the battle.")
			bt.result = "You got away safely."
			bt.won = false
			bt.sub = subOver
		} else {
			bt.log = appendLog(bt.log, "There's no running from this!")
		}
	}
}

func (m *model) keyBattleMove(key string) {
	bt := m.bt
	active := bt.b.Active(true)
	switch key {
	case "up", "w":
		if bt.move > 0 {
			bt.move--
		}
	case "down", "s":
		if bt.move < len(active.Moves)-1 {
			bt.move++
		}
	case "esc", "q":
		bt.sub = subMenu
	case "enter", " ":
		m.playerAttack(bt.move)
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			if i := int(key[0] - '1'); i < len(active.Moves) {
				m.playerAttack(i)
			}
		}
	}
}

func (m *model) keyBattleSwitch(key string) {
	bt := m.bt
	switch key {
	case "up", "w":
		if bt.swap > 0 {
			bt.swap--
		}
	case "down", "s":
		if bt.swap < len(m.roster)-1 {
			bt.swap++
		}
	case "esc", "q":
		if !bt.forced {
			bt.sub = subMenu
		}
	case "enter", " ":
		m.commitSwitch(bt.swap)
	}
}

// playerAttack resolves a turn with the player attacking and the foe's AI.
func (m *model) playerAttack(move int) {
	yt, yc := moveType(m.bt.b.Active(true), move)
	m.resolveTurn(mon.Action{Kind: mon.Attack, Index: move}, yc, yt)
}

// commitSwitch swaps the active creature. A forced switch (after a faint) is
// free; a voluntary one costs the turn and lets the foe act.
func (m *model) commitSwitch(idx int) {
	bt := m.bt
	if idx < 0 || idx >= len(m.roster) || m.roster[idx].Fainted() || idx == bt.b.ActiveIndex(true) {
		return
	}
	if bt.forced {
		ev := bt.b.ApplyForcedSwitch(true, idx)
		bt.log = appendLog(bt.log, ev...)
		bt.forced = false
		bt.sub = subMenu
		return
	}
	m.resolveTurn(mon.Action{Kind: mon.Switch, Index: idx}, false, mon.Plain)
}

// attemptCatch tries to capture a wild foe; a miss costs the turn.
func (m *model) attemptCatch() {
	bt := m.bt
	if !bt.wild {
		bt.log = appendLog(bt.log, "You can't catch another trainer's Shellmon!")
		return
	}
	if len(m.roster) >= maxParty {
		bt.log = appendLog(bt.log, "Your team is full!")
		return
	}
	foe := bt.b.Active(false)
	bt.log = appendLog(bt.log, "You toss a shell at "+foe.Name()+"…")
	if m.rng.Float64() < mon.CatchChance(foe) {
		m.roster = append(m.roster, foe)
		m.saveRoster()
		bt.log = appendLog(bt.log, "Gotcha! "+foe.Name()+" was caught!")
		bt.result = foe.Name() + " joined your team!"
		bt.won = true
		bt.sub = subOver
		return
	}
	bt.log = appendLog(bt.log, foe.Name()+" broke free!")
	// A failed catch costs the turn: skip (no-op self-switch) and let the foe act.
	m.resolveTurn(mon.Action{Kind: mon.Switch, Index: bt.b.ActiveIndex(true)}, false, mon.Plain)
}

// afterTurn checks for end-of-battle, awards XP, or prompts a forced switch.
func (m *model) afterTurn() {
	bt := m.bt
	if bt.b.Done() {
		if bt.b.WinnerA() {
			bt.won = true
			bt.result = "You won the battle!"
			m.awardXP()
			if !bt.wild && bt.trainerID != "" && !m.defeated[bt.trainerID] {
				m.defeated[bt.trainerID] = true
				bt.result = bt.trainerName + " was defeated!"
				if bt.reward != "" && m.ctx.Inventory != nil {
					_ = m.ctx.Inventory.Grant("cosmetic."+bt.reward, bt.rewardN, 1)
					bt.log = appendLog(bt.log, "Received "+bt.rewardN+"! (wear it in the plaza)")
				}
				if bt.badge != "" && !m.badges[bt.badge] {
					m.badges[bt.badge] = true
					bt.wonBadge = true
					if b, ok := badgeByID(bt.badge); ok {
						bt.log = appendLog(bt.log, "You earned the "+b.name+"!")
					}
				}
			}
		} else {
			bt.won = false
			bt.result = "Your team was overwhelmed…"
		}
		m.saveRoster()
		bt.sub = subOver
		return
	}
	if bt.b.NeedsSwitch(true) {
		bt.forced = true
		bt.swap = m.firstSwitchable()
		bt.sub = subSwitch
		bt.log = appendLog(bt.log, "Choose your next Shellmon!")
	}
}

// awardXP grants experience to the victorious active creature.
func (m *model) awardXP() {
	bt := m.bt
	winner := bt.b.Active(true)
	foeLevel := bt.b.Active(false).Level
	gain := 6 + foeLevel*5
	if msgs := winner.GainXP(gain); len(msgs) > 0 {
		bt.log = appendLog(bt.log, msgs...)
	}
}

// finishBattle closes the battle and returns to the route, healing on a loss.
// A battle that just earned a gym badge detours through the award animation.
func (m *model) finishBattle() {
	wonBadge, badge := false, ""
	if m.bt != nil {
		if !m.bt.won && m.bt.result != "You got away safely." && !m.partyAlive() {
			m.healParty()
		}
		wonBadge, badge = m.bt.wonBadge, m.bt.badge
	}
	m.bt = nil
	m.saveRoster()
	if wonBadge {
		m.presentBadge(badge)
		return
	}
	m.trans.begin(0.45) // wipe back out to the route
	m.state = stateRoute
}

// firstSwitchable returns the first alive, non-active roster slot (else 0).
func (m *model) firstSwitchable() int {
	active := m.bt.b.ActiveIndex(true)
	for i, c := range m.roster {
		if i != active && !c.Fainted() {
			return i
		}
	}
	return 0
}

// appendLog keeps the battle log to a recent tail.
func appendLog(log []string, lines ...string) []string {
	log = append(log, lines...)
	if len(log) > 6 {
		log = log[len(log)-6:]
	}
	return log
}

// === rendering ===

func (m *model) drawBattle(pw, ph int, t float64) {
	bt := m.bt
	bob := int(2 * sinf(t*2.2))

	// Backdrop: a graded sky, parallax ridges, clouds and a lit ground.
	drawArena(m.scr, pw, ph, t)

	// Advance the HP-bar easing and hit/faint reactions for both sides.
	foe := bt.b.Active(false)
	you := bt.b.Active(true)
	m.foeFX.sync(foe.Name(), foe.CurHP, foe.MaxHP())
	m.youFX.sync(you.Name(), you.CurHP, you.MaxHP())

	// A winner does a little celebratory bob on the results screen.
	vbob := 0
	if bt.won && bt.sub == subOver {
		vbob = -int(3 * (0.5 + 0.5*sinf(t*5)))
	}

	// Foe (upper-right) on a platform, with its info card upper-left.
	fx, fy := pw*70/100, ph*36/100
	drawPlatform(m.scr, fx, fy+34, 70)
	if !m.foeFX.gone() {
		mon.DrawCreature(m.scr, fx+m.foeFX.shakeX(), fy+bob+m.foeFX.sinkY(), 3, foe.Species)
	}
	if p := m.foeFX.appearP(); p < 1 {
		drawSendFlash(m.scr, fx, fy+10, p, typeColor(foe.Type()))
	}
	if p := m.foeFX.koP(); p >= 0 {
		drawKOBurst(m.scr, fx, fy+30, p, typeColor(foe.Type()))
	}
	infoCard(m.scr, 30, 40, 250, foe.Name(), foe.Level, m.foeFX.shownHP(), foe.MaxHP(), false, typeColor(foe.Type()))

	// Player active (lower-left), bigger, info card lower-right.
	yx, yy := pw*30/100, ph*72/100
	drawPlatform(m.scr, yx, yy+20, 92)
	if !m.youFX.gone() {
		mon.DrawCreature(m.scr, yx+m.youFX.shakeX(), yy-10+bob+vbob+m.youFX.sinkY(), 4, you.Species)
	}
	if p := m.youFX.appearP(); p < 1 {
		drawSendFlash(m.scr, yx, yy-6, p, typeColor(you.Type()))
	}
	if p := m.youFX.koP(); p >= 0 {
		drawKOBurst(m.scr, yx, yy+6, p, typeColor(you.Type()))
	}
	infoCard(m.scr, pw-290, ph*52/100, 260, you.Name(), you.Level, m.youFX.shownHP(), you.MaxHP(), true, typeColor(you.Type()))

	// Move casts and impact bursts for the current turn.
	m.anim.draw(m.scr, yx, yy-10, fx, fy)

	// Bottom command bar: log on the left, contextual UI on the right.
	barY := ph - 104
	panel(m.scr, 16, barY, pw-32, 92)
	m.drawLog(28, barY+10, pw*55/100)
	rx := pw * 58 / 100
	switch bt.sub {
	case subMenu:
		m.drawMenu(rx, barY+12)
	case subMove:
		m.drawMoves(rx, barY+12, you)
	case subSwitch:
		m.drawSwitch(rx, barY+10)
	case subOver:
		m.scr.DrawText(rx, barY+30, bt.result, uiText)
		m.scr.DrawText(rx, barY+30+canvas.LineH+4, "press any key…", uiDim)
	}
}

func (m *model) drawLog(x, y, w int) {
	bt := m.bt
	start := 0
	if len(bt.log) > 4 {
		start = len(bt.log) - 4
	}
	for i, line := range bt.log[start:] {
		m.scr.DrawText(x, y+i*(canvas.LineH+2), line, uiText)
	}
}

func (m *model) drawMenu(x, y int) {
	bt := m.bt
	colW, rowH := 110, 28
	for i, item := range menuItems {
		ix, iy := x+(i%2)*colW, y+(i/2)*rowH
		if i == bt.menu {
			m.scr.FillRect(ix-4, iy-4, colW-6, rowH-6, uiSelBG)
			m.scr.DrawText(ix, iy, "▸ "+item, uiText)
		} else {
			m.scr.DrawText(ix+10, iy, item, uiDim)
		}
	}
}

func (m *model) drawMoves(x, y int, c *mon.Creature) {
	bt := m.bt
	for i, mk := range c.Moves {
		mv, ok := mon.MoveByKey(mk)
		if !ok {
			continue
		}
		label := mv.Name + "  " + typeBadge(mv.Type)
		if mv.Power > 0 {
			label += " " + itoa(mv.Power)
		}
		iy := y + i*(canvas.LineH+4)
		if i == bt.move {
			m.scr.FillRect(x-4, iy-3, 240, canvas.LineH+4, uiSelBG)
			m.scr.DrawText(x, iy, "▸ "+label, uiText)
		} else {
			m.scr.DrawText(x+10, iy, label, uiDim)
		}
	}
	m.scr.DrawText(x, y+4*(canvas.LineH+4)+2, "Esc back", uiDim)
}

func (m *model) drawSwitch(x, y int) {
	bt := m.bt
	for i, c := range m.roster {
		iy := y + i*(canvas.LineH+2)
		tag := c.Name() + " Lv" + itoa(c.Level)
		if c.Fainted() {
			tag += " (fainted)"
		} else if i == bt.b.ActiveIndex(true) {
			tag += " (out)"
		}
		col := uiDim
		if i == bt.swap {
			m.scr.FillRect(x-4, iy-2, 260, canvas.LineH+2, uiSelBG)
			col = uiText
			tag = "▸ " + tag
		} else {
			tag = "  " + tag
		}
		m.scr.DrawText(x, iy, tag, col)
	}
	if !bt.forced {
		m.scr.DrawText(x, y+6*(canvas.LineH+2)+2, "Esc back", uiDim)
	}
}

// drawPlatform draws a flat elliptical pad under a combatant.
func drawPlatform(c *canvas.Canvas, cx, cy, rx int) {
	ry := rx / 3
	for dy := -ry; dy <= ry; dy++ {
		w := int(float64(rx) * sqrtClamp(1-float64(dy*dy)/float64(ry*ry)))
		tone := canvas.Color(0x242424)
		if dy < 0 {
			tone = canvas.Color(0x2E2E2E)
		}
		for dx := -w; dx <= w; dx++ {
			c.Set(cx+dx, cy+dy, tone)
		}
	}
}
