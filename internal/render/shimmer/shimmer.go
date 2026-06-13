// Package shimmer animates the rainbow gradient inside portals: a hue band
// sweeping through the oval over time, the only saturated color on the map.
package shimmer

import (
	"math"

	"github.com/shellbound/shellbound/internal/render/halfblock"
)

// HSL converts hue (degrees, any value), saturation and lightness (0..1)
// to a packed 0xRRGGBB color.
func HSL(h, s, l float64) halfblock.Color {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	s = clamp01(s)
	l = clamp01(l)

	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return halfblock.RGB(to255(r+m), to255(g+m), to255(b+m))
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func to255(v float64) uint8 {
	n := int(math.Round(v * 255))
	if n < 0 {
		n = 0
	}
	if n > 255 {
		n = 255
	}
	return uint8(n)
}

// Field computes shimmer colors for portal interiors. It caches HSL
// conversions per quantized hue so a frame of portal pixels costs a map
// lookup per pixel instead of trig per pixel.
type Field struct {
	cache map[int]halfblock.Color
}

// NewField creates an empty shimmer field.
func NewField() *Field {
	return &Field{cache: make(map[int]halfblock.Color, 360)}
}

// At returns the shimmer color for portal-local pixel (x, y) at time t
// (seconds since the server started). The sweep direction follows
// (x + y/2) so the band moves diagonally through the oval.
func (f *Field) At(x, y int, t float64) halfblock.Color {
	hue := math.Mod(float64(x)*8+float64(y)*4+t*60, 360)
	if hue < 0 {
		hue += 360
	}
	// Quantize to whole degrees: invisible visually, tiny cache.
	key := int(hue)
	if c, ok := f.cache[key]; ok {
		return c
	}
	c := HSL(float64(key), 0.9, 0.6)
	f.cache[key] = c
	return c
}
