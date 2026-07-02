package shellmon

import (
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// drawArena paints the battle backdrop: a stark black-and-white night stage —
// a near-black starfield sky, dark ridge silhouettes, one crisp bright horizon
// line and an almost-black ground with a faint pool of light where the
// combatants stand. Kept high-contrast and colourless so the only colour is the
// sparse type-hued pops (names, attack effects). t is seconds.
func drawArena(c *canvas.Canvas, pw, ph int, t float64) {
	horizon := ph * 52 / 100

	// Sky: black, with only a whisper of lift toward the horizon.
	for y := 0; y < horizon; y++ {
		g := uint8(3 + 9*float64(y)/float64(horizon))
		c.FillRect(0, y, pw, 1, canvas.RGB(g, g, g))
	}
	// Ground: near-black, darkening toward the camera.
	for y := horizon; y < ph; y++ {
		g := uint8(16 - 12*float64(y-horizon)/float64(ph-horizon))
		c.FillRect(0, y, pw, 1, canvas.RGB(g, g, g))
	}

	// Crisp white stars, twinkling.
	for i := 0; i < 48; i++ {
		sx := (i*97 + 13) % pw
		sy := (i*53 + 7) % (horizon - 16)
		tw := 0.4 + 0.5*math.Sin(t*2+float64(i)*1.7)
		if tw > 0.55 {
			g := uint8(255 * math.Min(1, tw))
			c.Set(sx, sy, canvas.RGB(g, g, g))
		}
	}

	// Two ridge silhouettes — dark shapes against the sky, no grey haze.
	ridge(c, pw, horizon, 44, 0.012, 0.0, canvas.Color(0x141414))
	ridge(c, pw, horizon, 28, 0.020, 2.1, canvas.Color(0x080808))

	// A low fog bank drifting slowly along the far ridge line: a soft sine
	// band a hair brighter than the ridges, so the backdrop breathes without
	// stealing attention (or colour).
	for x := 0; x < pw; x++ {
		fx := float64(x)
		h := 6 + 4*math.Sin(fx*0.017+t*0.35) + 2.5*math.Sin(fx*0.041-t*0.22)
		top := horizon - int(h)
		for y := top; y < horizon; y++ {
			fade := float64(y-top) / math.Max(1, float64(horizon-top))
			lighten(c, x, y, canvas.RGB(16, 16, 17).Scale(0.4+0.6*fade))
		}
	}

	// One crisp, bright horizon line (the brightest thing on the stage).
	for x := 0; x < pw; x++ {
		c.Set(x, horizon, canvas.RGB(190, 190, 195))
		c.Set(x, horizon+1, canvas.RGB(70, 70, 72))
	}

	// A faint pool of light on the ground where the combatants stand.
	softGround(c, pw/2, horizon+(ph-horizon)*55/100, pw*44/100, (ph-horizon)*60/100)
}

// ridge fills a rolling silhouette from its sine crest down to the horizon.
func ridge(c *canvas.Canvas, pw, horizon int, amp, freq, phase float64, tone canvas.Color) {
	for x := 0; x < pw; x++ {
		fx := float64(x)
		h := amp * (0.5 + 0.5*math.Sin(fx*freq+phase))
		h += 0.4 * amp * (0.5 + 0.5*math.Sin(fx*freq*2.7+phase*1.5)) // a little jaggedness
		ry := horizon - int(h)
		c.VLine(x, ry, horizon, tone)
	}
}

// softGround adds a gentle elliptical wash of light on the ground plane.
func softGround(c *canvas.Canvas, cx, cy, rx, ry int) {
	for dy := -ry; dy <= ry; dy++ {
		ny := float64(dy) / float64(ry)
		for dx := -rx; dx <= rx; dx++ {
			nx := float64(dx) / float64(rx)
			d := nx*nx + ny*ny
			if d >= 1 {
				continue
			}
			s := 1 - math.Sqrt(d)
			lighten(c, cx+dx, cy+dy, canvas.RGB(70, 70, 74).Scale(0.35*s*s))
		}
	}
}

func lighten(c *canvas.Canvas, x, y int, col canvas.Color) {
	c.Set(x, y, c.At(x, y).Lighten(col))
}
