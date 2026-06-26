package shellmon

import (
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// drawArena paints the battle backdrop: a graded sky over distant parallax
// ridges, drifting clouds, a glowing horizon and a textured ground — so the
// fight reads as happening in a place rather than on a flat split. Strictly
// monochrome, lit from above like the rest of Shellbound. t is seconds.
func drawArena(c *canvas.Canvas, pw, ph int, t float64) {
	horizon := ph * 52 / 100

	// Sky: dark at the top, brightening toward the horizon (per-row fill).
	for y := 0; y < horizon; y++ {
		f := float64(y) / float64(horizon)
		g := uint8(6 + 22*f)
		c.FillRect(0, y, pw, 1, canvas.RGB(g, g, g+2))
	}
	// Ground: brightest at the horizon, falling off toward the camera.
	for y := horizon; y < ph; y++ {
		f := float64(y-horizon) / float64(ph-horizon)
		g := uint8(26 - 14*f)
		c.FillRect(0, y, pw, 1, canvas.RGB(g, g, g))
	}

	// Faint stars, twinkling.
	for i := 0; i < 40; i++ {
		sx := (i*97 + 13) % pw
		sy := (i*53 + 7) % (horizon - 20)
		tw := 0.35 + 0.45*math.Sin(t*2+float64(i)*1.7)
		if tw > 0.45 {
			lighten(c, sx, sy, canvas.RGB(200, 200, 210).Scale(tw))
		}
	}

	// A couple of slow drifting clouds.
	for i, cl := range []struct{ y, w, h, speed, off int }{
		{horizon - 70, 60, 12, 7, 0}, {horizon - 100, 44, 9, 4, 400}, {horizon - 48, 70, 14, 10, 800},
	} {
		cx := (int(t*float64(cl.speed))+cl.off)%(pw+260) - 130
		_ = i
		drawCloud(c, cx, cl.y, cl.w, cl.h)
	}

	// Two parallax ridgelines rising to the horizon.
	ridge(c, pw, horizon, 46, 0.012, 0.0, canvas.Color(0x1D1D1F))
	ridge(c, pw, horizon, 30, 0.020, 2.1, canvas.Color(0x141416))

	// Horizon glow — a soft bright band where land meets sky.
	for dy := -6; dy <= 6; dy++ {
		k := 1 - math.Abs(float64(dy))/6
		col := canvas.RGB(120, 120, 130).Scale(0.5 * k)
		for x := 0; x < pw; x++ {
			lighten(c, x, horizon+dy, col)
		}
	}

	// A broad, faint arena pool of light on the ground where combatants stand.
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

// drawCloud lightens a soft elliptical puff (additive, so it floats over sky).
func drawCloud(c *canvas.Canvas, cx, cy, rx, ry int) {
	for dy := -ry; dy <= ry; dy++ {
		w := int(float64(rx) * sqrtClamp(1-float64(dy*dy)/float64(ry*ry)))
		for dx := -w; dx <= w; dx++ {
			edge := 1 - float64(dx*dx)/float64(w*w+1)
			lighten(c, cx+dx, cy+dy, canvas.RGB(60, 60, 66).Scale(0.5*edge))
		}
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
