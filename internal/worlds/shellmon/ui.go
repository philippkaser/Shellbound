package shellmon

import (
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// sinf is a thin wrapper so screens can bob sprites without importing math.
func sinf(x float64) float64 { return math.Sin(x) }

// sqrtClamp is sqrt with a floor at 0 for the (1 - x²) ellipse terms.
func sqrtClamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	return math.Sqrt(v)
}

// Shared monochrome UI tones for boxes and bars.
const (
	uiFill   = canvas.Color(0x0E0E0E)
	uiBorder = canvas.Color(0xD0D0D0)
	uiText   = canvas.Color(0xE8E8E8)
	uiDim    = canvas.Color(0x8A8A8A)
	uiTrack  = canvas.Color(0x303030)
	uiBar    = canvas.Color(0xD8D8D8)
	uiBarLow = canvas.Color(0x808080)
	uiSelBG  = canvas.Color(0x2A2A2A)
)

// panel draws a filled, bordered box.
func panel(c *canvas.Canvas, x, y, w, h int) {
	c.FillRect(x, y, w, h, uiFill)
	c.Rect(x, y, w, h, uiBorder)
}

// itoa is a tiny int formatter (the package stays import-light like the engine).
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

// hpBar draws a health bar of width w at (x, y) for cur/max HP.
func hpBar(c *canvas.Canvas, x, y, w, cur, max int) {
	if max < 1 {
		max = 1
	}
	if cur < 0 {
		cur = 0
	}
	const h = 6
	c.FillRect(x, y, w, h, uiTrack)
	c.Rect(x-1, y-1, w+2, h+2, uiDim)
	fill := (w - 2) * cur / max
	col := uiBar
	if cur*4 <= max {
		col = uiBarLow // low health reads dimmer
	}
	if fill > 0 {
		c.FillRect(x+1, y+1, fill, h-2, col)
	}
}

// infoCard draws a name, level and HP bar in a small box anchored at (x, y).
// withHP shows the numeric HP under the bar (used for the player side).
func infoCard(c *canvas.Canvas, x, y, w int, name string, level, cur, max int, withHP bool) {
	h := 30
	if withHP {
		h = 40
	}
	panel(c, x, y, w, h)
	c.DrawText(x+8, y+6, name, uiText)
	lv := "Lv" + itoa(level)
	c.DrawText(x+w-canvas.TextWidth(lv)-8, y+6, lv, uiDim)
	hpBar(c, x+8, y+18, w-16, cur, max)
	if withHP {
		hp := itoa(cur) + "/" + itoa(max)
		c.DrawText(x+w-canvas.TextWidth(hp)-8, y+27, hp, uiDim)
	}
}

// typeBadge prints a creature's elemental type as a short label.
func typeBadge(t mon.Type) string { return "[" + t.String() + "]" }
