package shellmon

import (
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/screen"
	mon "github.com/shellbound/shellbound/internal/shellmon"
	"github.com/shellbound/shellbound/internal/world"
)

// pvpMenu items.
var pvpMenuItems = []string{"Fight", "Switch", "Forfeit"}

// pvpModel is a live 6v6 duel between two players over a shared mon.Match. Both
// sides run an instance of this; each polls the match at the render tick and
// submits its action when it's their turn. The app swaps this in when a
// challenge is accepted (see hub.EvBattleStart).
type pvpModel struct {
	match    *mon.Match
	sideA    bool
	opponent string
	exit     func()

	scr          *canvas.Canvas
	sb           *strings.Builder
	pal          *canvas.Palette
	out          io.Writer
	cellW, cellH int
	termW, termH int
	start        time.Time

	sub     battleSub // menu / move / switch (meaningful only while it's our turn)
	menu    int
	move    int
	swap    int
	exiting bool

	youFX hpFX
	foeFX hpFX
	anim  battleAnim
	trans transition
}

// NewPvP builds the PvP battle model for one side of a shared match. exit is
// called to return the player to the plaza when the duel ends.
func NewPvP(render world.Render, match *mon.Match, sideA bool, opponent string, exit func()) tea.Model {
	m := &pvpModel{
		match:    match,
		sideA:    sideA,
		opponent: opponent,
		exit:     exit,
		scr:      canvas.New(1, 1),
		sb:       &strings.Builder{},
		pal:      render.Palette,
		out:      render.Out,
		cellW:    render.CellW,
		cellH:    render.CellH,
		sub:      subMenu,
	}
	if m.cellW <= 0 {
		m.cellW = 8
	}
	if m.cellH <= 0 {
		m.cellH = 16
	}
	m.trans.begin(0.55) // wipe into the duel
	return m
}

// Init implements tea.Model.
func (m *pvpModel) Init() tea.Cmd { return pvpTick() }

type pvpTickMsg time.Time

func pvpTick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return pvpTickMsg(t) })
}

// Stop lets the app halt the model's ticker on teardown (matches the world's
// interface{ Stop() } hook).
func (m *pvpModel) Stop() { m.exiting = true }

// Update implements tea.Model.
func (m *pvpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termW, m.termH = msg.Width, msg.Height
		m.render()
		return m, nil
	case pvpTickMsg:
		m.render()
		if m.exiting {
			return m, nil
		}
		return m, pvpTick()
	case tea.KeyMsg:
		m.handleKey(msg.String())
		m.render()
		return m, nil
	}
	return m, nil
}

// View implements tea.Model: a constant sentinel; frames ship as Sixel.
func (m *pvpModel) View() string { return " " }

func (m *pvpModel) handleKey(key string) {
	if key == "ctrl+c" {
		m.leave()
		return
	}
	v := m.match.Snapshot(m.sideA)
	if v.Done {
		m.leave()
		return
	}
	if v.MustSwitchYou {
		m.navSwitch(key, v, true)
		return
	}
	if !v.AwaitingYou {
		return // waiting on the opponent
	}
	switch m.sub {
	case subMove:
		m.navMove(key, v)
	case subSwitch:
		m.navSwitch(key, v, false)
	default:
		m.navMenu(key, v)
	}
}

func (m *pvpModel) navMenu(key string, v mon.View) {
	switch key {
	case "up", "w":
		if m.menu > 0 {
			m.menu--
		}
	case "down", "s":
		if m.menu < len(pvpMenuItems)-1 {
			m.menu++
		}
	case "enter", " ":
		switch m.menu {
		case 0:
			m.move, m.sub = 0, subMove
		case 1:
			m.swap, m.sub = m.firstSwitchable(v), subSwitch
		case 2:
			m.match.Forfeit(m.sideA)
		}
	case "f":
		m.move, m.sub = 0, subMove
	case "x":
		m.swap, m.sub = m.firstSwitchable(v), subSwitch
	}
}

func (m *pvpModel) navMove(key string, v mon.View) {
	switch key {
	case "up", "w":
		if m.move > 0 {
			m.move--
		}
	case "down", "s":
		if m.move < len(v.You.Moves)-1 {
			m.move++
		}
	case "esc", "q":
		m.sub = subMenu
	case "enter", " ":
		m.match.Submit(m.sideA, mon.Action{Kind: mon.Attack, Index: m.move})
		m.sub = subMenu
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			if i := int(key[0] - '1'); i < len(v.You.Moves) {
				m.match.Submit(m.sideA, mon.Action{Kind: mon.Attack, Index: i})
				m.sub = subMenu
			}
		}
	}
}

func (m *pvpModel) navSwitch(key string, v mon.View, forced bool) {
	switch key {
	case "up", "w":
		if m.swap > 0 {
			m.swap--
		}
	case "down", "s":
		if m.swap < len(v.Party)-1 {
			m.swap++
		}
	case "esc", "q":
		if !forced {
			m.sub = subMenu
		}
	case "enter", " ":
		if m.swap >= 0 && m.swap < len(v.Party) {
			p := v.Party[m.swap]
			if !p.Fainted && !p.Active {
				m.match.Submit(m.sideA, mon.Action{Kind: mon.Switch, Index: m.swap})
				m.sub = subMenu
			}
		}
	}
}

func (m *pvpModel) firstSwitchable(v mon.View) int {
	for i, p := range v.Party {
		if !p.Fainted && !p.Active {
			return i
		}
	}
	return 0
}

func (m *pvpModel) leave() {
	if !m.exiting {
		m.exiting = true
		if m.exit != nil {
			m.exit()
		}
	}
}

func (m *pvpModel) render() {
	if m.exiting || m.out == nil {
		return
	}
	if m.start.IsZero() {
		m.start = time.Now()
	}
	t := time.Since(m.start).Seconds()
	pw, ph, left, top := screen.Dims(m.termW, m.termH, m.cellW, m.cellH)
	if pw <= 0 || ph <= 0 {
		return
	}
	m.scr.Resize(pw, ph)
	m.scr.Clear(canvas.Black)
	if m.termW < minTermW || m.termH < minTermH {
		msg := "please resize your terminal to at least 60x20"
		m.scr.DrawText(pw/2-canvas.TextWidth(msg)/2, ph/2, msg, 0xA1A1A1)
	} else {
		m.draw(pw, ph, t)
	}
	m.trans.overlay(m.scr, pw, ph)
	m.sb.Reset()
	screen.Place(m.sb, m.scr, m.pal, left, top)
	if s := m.sb.String(); s != "" {
		_, _ = io.WriteString(m.out, s)
	}
}

func (m *pvpModel) draw(pw, ph int, t float64) {
	v := m.match.Snapshot(m.sideA)
	bob := int(2 * sinf(t*2.2))

	drawArena(m.scr, pw, ph, t)

	// Opponent banner.
	banner := "vs " + m.opponent
	m.scr.DrawText(pw/2-canvas.TextWidth(banner)/2, 14, banner, uiDim)

	// Advance HP easing + hit/faint reactions and the per-turn effects.
	m.foeFX.sync(v.Foe.Name, v.Foe.HP, v.Foe.MaxHP)
	m.youFX.sync(v.You.Name, v.You.HP, v.You.MaxHP)
	m.anim.trigger(v.TurnSeq, v.YouLastType, v.YouCast, v.FoeLastType, v.FoeCast)

	// Foe (upper-right).
	fx, fy := pw*70/100, ph*36/100
	drawPlatform(m.scr, fx, fy+34, 70)
	if !m.foeFX.gone() {
		mon.DrawCreature(m.scr, fx+m.foeFX.shakeX(), fy+bob+m.foeFX.sinkY(), 3, v.Foe.Species)
	}
	infoCard(m.scr, 30, 40, 250, v.Foe.Name, v.Foe.Level, m.foeFX.shownHP(), v.Foe.MaxHP, false)
	teamPips(m.scr, 30, 74, v.FoeTotal, v.FoeAlive)

	// You (lower-left).
	yx, yy := pw*30/100, ph*72/100
	drawPlatform(m.scr, yx, yy+20, 92)
	if !m.youFX.gone() {
		mon.DrawCreature(m.scr, yx+m.youFX.shakeX(), yy-10+bob+m.youFX.sinkY(), 4, v.You.Species)
	}
	infoCard(m.scr, pw-290, ph*52/100, 260, v.You.Name, v.You.Level, m.youFX.shownHP(), v.You.MaxHP, true)
	teamPips(m.scr, pw-290, ph*52/100+44, len(v.Party), v.YouAlive)

	// Move casts and impact bursts for the current turn.
	m.anim.draw(m.scr, yx, yy-10, fx, fy)

	// Bottom bar: log + contextual UI.
	barY := ph - 104
	panel(m.scr, 16, barY, pw-32, 92)
	m.drawLog(v, 28, barY+10)
	rx := pw * 58 / 100
	switch {
	case v.Done:
		res := "You won the duel!"
		if !v.YouWon {
			res = "You lost the duel."
		}
		m.scr.DrawText(rx, barY+30, res, uiText)
		m.scr.DrawText(rx, barY+30+canvas.LineH+4, "press any key…", uiDim)
	case v.MustSwitchYou:
		m.scr.DrawText(rx, barY+8, "Your Shellmon fainted — switch!", uiText)
		m.drawSwitchList(v, rx, barY+22, true)
	case v.AwaitingYou && m.sub == subMove:
		m.drawMoveList(v, rx, barY+12)
	case v.AwaitingYou && m.sub == subSwitch:
		m.drawSwitchList(v, rx, barY+12, false)
	case v.AwaitingYou:
		m.drawPvpMenu(rx, barY+14)
	default:
		m.scr.DrawText(rx, barY+34, "Waiting for "+m.opponent+"…", uiDim)
	}
}

func (m *pvpModel) drawLog(v mon.View, x, y int) {
	start := 0
	if len(v.Log) > 4 {
		start = len(v.Log) - 4
	}
	for i, line := range v.Log[start:] {
		m.scr.DrawText(x, y+i*(canvas.LineH+2), line, uiText)
	}
}

func (m *pvpModel) drawPvpMenu(x, y int) {
	for i, item := range pvpMenuItems {
		iy := y + i*(canvas.LineH+6)
		if i == m.menu {
			m.scr.FillRect(x-4, iy-3, 150, canvas.LineH+4, uiSelBG)
			m.scr.DrawText(x, iy, "▸ "+item, uiText)
		} else {
			m.scr.DrawText(x+10, iy, item, uiDim)
		}
	}
}

func (m *pvpModel) drawMoveList(v mon.View, x, y int) {
	for i, mv := range v.You.Moves {
		label := mv.Name + "  " + typeBadge(mv.Type)
		if mv.Power > 0 {
			label += " " + itoa(mv.Power)
		}
		iy := y + i*(canvas.LineH+4)
		if i == m.move {
			m.scr.FillRect(x-4, iy-3, 240, canvas.LineH+4, uiSelBG)
			m.scr.DrawText(x, iy, "▸ "+label, uiText)
		} else {
			m.scr.DrawText(x+10, iy, label, uiDim)
		}
	}
	m.scr.DrawText(x, y+4*(canvas.LineH+4)+2, "Esc back", uiDim)
}

func (m *pvpModel) drawSwitchList(v mon.View, x, y int, forced bool) {
	for i, p := range v.Party {
		iy := y + i*(canvas.LineH+2)
		tag := p.Name + " Lv" + itoa(p.Level)
		if p.Fainted {
			tag += " (fainted)"
		} else if p.Active {
			tag += " (out)"
		}
		col := uiDim
		if i == m.swap {
			m.scr.FillRect(x-4, iy-2, 260, canvas.LineH+2, uiSelBG)
			col = uiText
			tag = "▸ " + tag
		} else {
			tag = "  " + tag
		}
		m.scr.DrawText(x, iy, tag, col)
	}
	if !forced {
		m.scr.DrawText(x, y+6*(canvas.LineH+2)+2, "Esc back", uiDim)
	}
}

// teamPips draws `total` small squares with `alive` filled, as a party tracker.
func teamPips(c *canvas.Canvas, x, y, total, alive int) {
	for i := 0; i < total; i++ {
		px := x + i*12
		if i < alive {
			c.FillRect(px, y, 8, 8, uiBar)
		} else {
			c.Rect(px, y, 8, 8, uiDim)
		}
	}
}
