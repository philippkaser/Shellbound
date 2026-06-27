package overworld

import (
	"math"
	"time"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// The plaza runs a shared day/night cycle and weather, both derived purely from
// the wall clock so every session — all in one server process — sees the same
// sky with no extra networking. Strictly greyscale (brightness, not hue), so the
// world's three colour splashes stay reserved for names, chat and portals.

const (
	dayPeriod     = 360.0 // seconds for a full day→night→day cycle
	weatherWindow = 150.0 // seconds each weather spell lasts
)

// weatherKind is the current precipitation.
type weatherKind int

const (
	weatherClear weatherKind = iota
	weatherRain
	weatherSnow
)

// skyState is the time-derived sky everyone shares.
type skyState struct {
	phase   float64 // 0..1 through the day (0/1 = dawn, .25 = noon, .75 = midnight)
	ambient float64 // base light level for the lighting pass
	night   float64 // 0 by day … 1 at deep midnight
	weather weatherKind
	label   string // "dawn" / "day" / "dusk" / "night" (+ weather)
}

// skyAt computes the shared sky for a moment in time.
func skyAt(now time.Time) skyState {
	secs := float64(now.Unix()) + float64(now.Nanosecond())/1e9
	phase := math.Mod(secs, dayPeriod) / dayPeriod
	s := math.Sin(2 * math.Pi * phase)
	amb := 0.50 + 0.27*s // brightest at noon, darkest at midnight
	night := math.Max(0, -s)

	w := weatherFor(now)
	if w == weatherRain {
		amb -= 0.07 // overcast
	} else if w == weatherSnow {
		amb -= 0.03
	}

	label := "night"
	switch {
	case phase < 0.06 || phase >= 0.94:
		label = "dawn"
	case phase < 0.44:
		label = "day"
	case phase < 0.56:
		label = "dusk"
	}
	switch w {
	case weatherRain:
		label += " · rain"
	case weatherSnow:
		label += " · snow"
	}
	return skyState{phase: phase, ambient: amb, night: night, weather: w, label: label}
}

// weatherFor picks the weather for the current window (mostly clear).
func weatherFor(now time.Time) weatherKind {
	bucket := now.Unix() / int64(weatherWindow)
	switch hashU(bucket) % 100 {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24:
		return weatherRain // ~25%
	case 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38:
		return weatherSnow // ~14%
	default:
		return weatherClear
	}
}

func hashU(i int64) uint32 {
	x := uint32(i)*2654435761 + 1013904223
	x ^= x >> 15
	x *= 2246822519
	x ^= x >> 13
	return x
}

// drawSky paints the celestial body and night stars onto the freshly cleared
// canvas, before the world — so the plaza's ground and skyline draw over them
// and only the true background sky shows through.
func drawSky(c *canvas.Canvas, pw, ph int, t float64, sky skyState) {
	if sky.night < 0.5 {
		drawSun(c, pw, ph, sky.phase/0.5)
	} else {
		drawMoon(c, pw, ph, (sky.phase-0.5)/0.5)
	}
	if sky.night <= 0.04 {
		return
	}
	for i := 0; i < 70; i++ {
		h := hashU(int64(i * 131))
		sx := int(h % uint32(pw))
		sy := int((h / 7) % uint32(ph*45/100))
		tw := sky.night * (0.4 + 0.5*math.Sin(t*2+float64(i)))
		if tw > 0.55 {
			g := uint8(200 * math.Min(1, tw))
			c.Set(sx, sy, canvas.RGB(g, g, g+5))
		}
	}
}

// celestial arc position: rises and sets across the upper sky over prog 0..1.
func arcPos(pw, ph int, prog float64) (int, int) {
	x := int(float64(pw) * prog)
	y := int(float64(ph)*0.30 - math.Sin(prog*math.Pi)*float64(ph)*0.22)
	return x, y
}

func drawSun(c *canvas.Canvas, pw, ph int, prog float64) {
	cx, cy := arcPos(pw, ph, prog)
	for r := 0; r < 12; r++ { // soft corona
		for a := 0; a < 24; a++ {
			ang := float64(a) / 24 * 2 * math.Pi
			x := cx + int(math.Cos(ang)*float64(11+r))
			y := cy + int(math.Sin(ang)*float64(11+r))
			c.Set(x, y, c.At(x, y).Lighten(canvas.RGB(40, 40, 40).Scale(1-float64(r)/12)))
		}
	}
	c.FillCircle(cx, cy, 9, canvas.RGB(235, 235, 235))
	// rays
	for a := 0; a < 8; a++ {
		ang := float64(a) / 8 * 2 * math.Pi
		x := cx + int(math.Cos(ang)*16)
		y := cy + int(math.Sin(ang)*16)
		c.Set(x, y, canvas.RGB(210, 210, 210))
	}
}

func drawMoon(c *canvas.Canvas, pw, ph int, prog float64) {
	cx, cy := arcPos(pw, ph, prog)
	c.FillCircle(cx, cy, 8, canvas.RGB(200, 200, 205))
	c.FillCircle(cx+4, cy-2, 7, canvas.Black) // crescent shadow
	// a couple of craters on the lit edge
	c.Set(cx-3, cy+1, canvas.RGB(150, 150, 150))
	c.Set(cx-1, cy+3, canvas.RGB(150, 150, 150))
}

// drawWeather overlays precipitation across the whole frame (after the scene,
// before the HUD), scrolling with t so it falls continuously.
func drawWeather(c *canvas.Canvas, pw, ph int, t float64, w weatherKind) {
	switch w {
	case weatherRain:
		for i := 0; i < 240; i++ {
			h := hashU(int64(i))
			x := (int(h%uint32(pw)) + int(t*620)) % (pw + 40)
			y := (int((h/3)%uint32(ph)) + int(t*900)) % (ph + 20)
			drawRainStreak(c, x-20, y-10)
		}
	case weatherSnow:
		for i := 0; i < 150; i++ {
			h := hashU(int64(i * 7))
			drift := int(math.Sin(t*1.3+float64(i)) * 6)
			x := (int(h%uint32(pw)) + drift) % pw
			y := (int((h/5)%uint32(ph)) + int(t*70)) % ph
			c.Set(x, y, canvas.RGB(225, 225, 230))
			if i%3 == 0 {
				c.Set(x+1, y, canvas.RGB(170, 170, 175))
			}
		}
	}
}

func drawRainStreak(c *canvas.Canvas, x, y int) {
	for k := 0; k < 8; k++ {
		c.Set(x+k/2, y+k, canvas.RGB(150, 150, 160))
	}
}

// drawSkyLabel prints the time-of-day (and weather) in the top centre.
func drawSkyLabel(c *canvas.Canvas, pw int, sky skyState) {
	w := canvas.TextWidth(sky.label)
	c.DrawTextShadow(pw/2-w/2, 4, sky.label, 0x9A9A9A, 0x000000)
}
