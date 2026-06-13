package overworld

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/shellbound/shellbound/internal/hub"
	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/render/halfblock"
	"github.com/shellbound/shellbound/internal/render/sprites"
)

// View renders one frame. It is defined on *Model so the screen canvas and
// frame builder can be reused across frames; the session app always holds
// the model in an addressable field.
func (m *Model) View() string {
	if m.termW <= 0 || m.termH <= 0 {
		return ""
	}
	if m.termW < minTermW || m.termH < minTermH {
		return lipgloss.Place(m.termW, m.termH, lipgloss.Center, lipgloss.Center,
			m.theme.Dim.Render("please resize your terminal to at least 60×20"))
	}

	viewW := min(m.termW, m.world.W)
	viewH := min(m.termH, m.world.H)
	if m.screen == nil || m.screen.W != viewW || m.screen.H != viewH {
		m.screen = halfblock.New(viewW, viewH)
	}

	// Camera: spring position is in (cell, cell-row) world space; clamp the
	// window to the map so edges never show the void.
	ox := clamp(int(math.Round(m.cam.X))-viewW/2, 0, m.world.W-viewW)
	oy := clamp(int(math.Round(m.cam.Y))-viewH/2, 0, m.world.H-viewH)

	t := time.Since(m.start).Seconds()
	now := time.Now()

	// 1. Static base (covers every cell, so no Clear needed).
	m.screen.Blit(m.base, ox, oy, viewW, viewH, 0, 0)
	// 2. Animated decorations.
	m.world.RenderDynamic(m.screen, t, ox, oy)
	// 3. Portals (the only color on the map).
	for _, p := range plaza.Portals {
		p.Render(m.screen, m.field, t, ox, oy)
	}
	// 4. Players, painter-sorted by feet row.
	m.drawPlayers(ox, oy, t)
	// 5. Chat history, bottom-left.
	m.chat.RenderHistory(m.screen, now, 1, viewH-2, viewW*2/3)
	// 6. HUD.
	m.drawHUD(viewW, viewH)

	// Materialize rows so overlays can replace whole lines.
	rows := make([]string, viewH)
	for y := 0; y < viewH; y++ {
		m.sb.Reset()
		m.screen.RenderRow(y, m.sb)
		rows[y] = m.sb.String()
	}

	// Overlays: toast banner, panels, chat input bar.
	if m.toasts.Active() {
		overlayRows(rows, []string{m.toasts.View()}, 1, viewW)
	}
	switch {
	case m.friends.IsOpen():
		lines := strings.Split(m.friends.View(), "\n")
		overlayRows(rows, lines, max(0, (viewH-len(lines))/2), viewW)
	case m.inv.IsOpen():
		lines := strings.Split(m.inv.View(), "\n")
		overlayRows(rows, lines, max(0, (viewH-len(lines))/2), viewW)
	}
	if m.chat.IsOpen() {
		lines := strings.Split(m.chat.ViewInput(viewW), "\n")
		overlayRows(rows, lines, max(0, viewH-len(lines)-1), viewW)
	}

	// Letterbox into the full terminal.
	leftPad := (m.termW - viewW) / 2
	topPad := (m.termH - viewH) / 2
	bottomPad := m.termH - viewH - topPad

	m.sb.Reset()
	for i := 0; i < topPad; i++ {
		m.sb.WriteByte('\n')
	}
	pad := strings.Repeat(" ", max(0, leftPad))
	for y, row := range rows {
		if y > 0 {
			m.sb.WriteByte('\n')
		}
		m.sb.WriteString(pad)
		m.sb.WriteString(row)
	}
	for i := 0; i < bottomPad; i++ {
		m.sb.WriteByte('\n')
	}
	return m.sb.String()
}

// drawPlayers renders every avatar (remote and local) with name tags.
func (m *Model) drawPlayers(ox, oy int, t float64) {
	states := make([]hub.PlayerState, 0, len(m.remotes)+1)
	for _, st := range m.remotes {
		states = append(states, st)
	}
	states = append(states, hub.PlayerState{
		Info:   hub.PlayerInfo{ID: m.player.ID, Name: m.player.Username, Color: m.player.Color},
		Pos:    hub.Pos{X: m.px, Y: m.py},
		Dir:    m.dir,
		Moving: m.moving,
	})
	sort.Slice(states, func(i, j int) bool { return states[i].Pos.Y < states[j].Pos.Y })

	for _, st := range states {
		frame := 0
		if st.Moving {
			if st.Info.ID == m.player.ID {
				frame = m.walkCount / 2
			} else {
				frame = int(t * 6)
			}
		}
		spr := sprites.Player(sprites.Facing(st.Dir), frame)
		// Feet at (Pos.X, Pos.Y) in (cell, pixel-row) space; the sprite is
		// 3 cells wide and 8 pixel rows tall.
		spr.Draw(m.screen, st.Pos.X-1-ox, st.Pos.Y-(sprites.PlayerH-1)-oy*2)

		// Name tag one cell above the head, centered, in the player color.
		headCell := (st.Pos.Y - (sprites.PlayerH - 1)) / 2
		nameW := utf8.RuneCountInString(st.Info.Name)
		m.screen.WriteText(st.Pos.X-nameW/2-ox, headCell-1-oy, st.Info.Name, halfblock.Hex(st.Info.Color))
	}
}

// drawHUD writes the hint line and the unread-DM indicator.
func (m *Model) drawHUD(viewW, viewH int) {
	hint := "Enter chat · i inventory · f friends · q quit"
	hw := utf8.RuneCountInString(hint)
	m.screen.WriteText(viewW-hw-1, viewH-1, hint, 0x404040)

	if len(m.unread) > 0 && !m.friends.IsOpen() {
		// Show one name; summarize the rest.
		var name string
		for _, n := range m.unread {
			name = n
			break
		}
		ind := "✉ " + name
		if extra := len(m.unread) - 1; extra > 0 {
			ind += " +" + itoa(extra)
		}
		iw := utf8.RuneCountInString(ind)
		m.screen.WriteText(viewW-iw-1, 0, ind, 0xFFFFFF)
	}
}

// overlayRows replaces full canvas rows with centered overlay lines.
func overlayRows(rows []string, lines []string, startRow, viewW int) {
	for i, line := range lines {
		r := startRow + i
		if r < 0 || r >= len(rows) {
			continue
		}
		w := lipgloss.Width(line)
		left := max(0, (viewW-w)/2)
		right := max(0, viewW-w-left)
		rows[r] = strings.Repeat(" ", left) + line + strings.Repeat(" ", right)
	}
}

// clamp bounds v to [lo, hi]; if hi < lo it returns lo.
func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// itoa is a tiny positive-int formatter to keep fmt out of the frame path.
func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 && i > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
