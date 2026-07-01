package shellmon

import (
	"math"
	"time"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// A gym badge is earned by beating a gym leader. Badges are tracked in the save
// (m.badges), presented with a short award animation, and shown on the party
// screen. Like the battle FX, they are monochrome with a single type-hued pop.
type badgeInfo struct {
	id     string
	name   string
	blurb  string
	accent canvas.Color
}

var badgeCatalog = []badgeInfo{
	{id: "coral", name: "Coral Badge", blurb: "Tidewell Gym · Leader Pearl", accent: canvas.Color(0x6FA8C7)},
}

func badgeByID(id string) (badgeInfo, bool) {
	for _, b := range badgeCatalog {
		if b.id == id {
			return b, true
		}
	}
	return badgeInfo{}, false
}

// presentBadge switches to the award animation for a freshly earned badge.
func (m *model) presentBadge(id string) {
	m.awardBadge = id
	m.awardAt = time.Now()
	m.state = stateBadge
	m.trans.begin(0.5)
}

// keyBadge dismisses the award once it has had a moment to read.
func (m *model) keyBadge(string) {
	if time.Since(m.awardAt) < 1200*time.Millisecond {
		return
	}
	m.awardBadge = ""
	m.trans.begin(0.4)
	m.state = stateRoute
}

// drawBadge paints a shell-medallion badge centered at (cx, cy) with the given
// radius: a scalloped metal rim, concentric shell ridges and an accent gem.
func drawBadge(c *canvas.Canvas, cx, cy, rad int, accent canvas.Color, t float64) {
	// Scalloped shell points around the rim.
	const pts = 9
	for i := 0; i < pts; i++ {
		a := float64(i) / pts * 2 * math.Pi
		px := cx + int(float64(rad+1)*math.Cos(a))
		py := cy + int(float64(rad+1)*math.Sin(a))
		c.FillCircle(px, py, rad/4+1, canvas.Color(0xB4B4B4))
	}
	// Metal rim, then concentric ridge bands (outer→inner overwrite).
	c.FillCircle(cx, cy, rad, canvas.Color(0xD0D0D0))
	for r := rad - 1; r >= 3; r-- {
		tone := canvas.Color(0x3C3C3C)
		if (r/2)%2 == 0 {
			tone = canvas.Color(0x565656)
		}
		c.FillCircle(cx, cy, r, tone)
	}
	// A faint accent wash on the lower half, and the gem at the center.
	for dy := 0; dy <= rad-3; dy++ {
		w := int(float64(rad-3) * sqrtClamp(1-float64(dy*dy)/float64((rad-3)*(rad-3))))
		for dx := -w; dx <= w; dx++ {
			c.Set(cx+dx, cy+dy, c.At(cx+dx, cy+dy).Lerp(accent, 0.25))
		}
	}
	c.FillCircle(cx, cy, 3, accent)
	c.Set(cx-1, cy-1, canvas.Color(0xF4F4F4)) // glint
}

// drawBadgeAward animates a just-earned badge over a dark field: the medallion
// rises and settles with a ring of sparkles, then invites a keypress.
func (m *model) drawBadgeAward(pw, ph int, t float64) {
	b, ok := badgeByID(m.awardBadge)
	if !ok {
		return
	}
	el := time.Since(m.awardAt).Seconds()

	title := "GYM BADGE EARNED!"
	m.scr.DrawText(pw/2-canvas.TextWidth(title)/2, ph/2-90, title, uiText)

	// Scale-in with a gentle settle bob.
	pop := el / 0.5
	if pop > 1 {
		pop = 1
	}
	rad := int(float64(46) * pop)
	cy := ph/2 - 10 + int(2*sinf(t*2.5))

	// Radiating sparkles once the badge has popped in.
	if el > 0.5 {
		n := 12
		for i := 0; i < n; i++ {
			a := float64(i)/float64(n)*2*math.Pi + t*0.6
			rr := 54.0 + 10*sinf(t*3+float64(i))
			sx := pw/2 + int(rr*math.Cos(a))
			sy := cy + int(rr*math.Sin(a)*0.6)
			m.scr.Set(sx, sy, b.accent)
			m.scr.Set(sx+1, sy, canvas.Color(0xEDEDED))
		}
	}
	if rad > 4 {
		drawBadge(m.scr, pw/2, cy, rad, b.accent, t)
	}

	m.scr.DrawText(pw/2-canvas.TextWidth(b.name)/2, cy+62, b.name, uiText)
	m.scr.DrawText(pw/2-canvas.TextWidth(b.blurb)/2, cy+62+canvas.LineH+2, b.blurb, uiDim)
	if el > 1.2 {
		hint := "press any key…"
		m.scr.DrawText(pw/2-canvas.TextWidth(hint)/2, ph-40, hint, uiDim)
	}
}

// drawBadgeCase draws the full badge catalog at (x, y): earned badges in accent,
// unearned ones as dim empty slots. Used on the party screen.
func (m *model) drawBadgeCase(x, y int) {
	m.scr.DrawText(x, y, "Badges", uiText)
	slot := 44
	for i, b := range badgeCatalog {
		bx := x + 20 + i*slot
		by := y + 30
		if m.badges[b.id] {
			drawBadge(m.scr, bx, by, 15, b.accent, 0)
			m.scr.DrawText(bx-canvas.TextWidth(b.name)/2, by+22, b.name, uiDim)
		} else {
			m.scr.FillCircle(bx, by, 15, canvas.Color(0x1E1E1E))
			m.scr.FillCircle(bx, by, 13, canvas.Color(0x101010))
			m.scr.DrawText(bx-canvas.TextWidth("— locked —")/2, by+22, "— locked —", uiDim)
		}
	}
}
