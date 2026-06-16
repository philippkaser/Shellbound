// Package shimmer animates the rainbow gradient inside portals: a hue band
// sweeping through the oval over time, the only saturated color on the map.
//
// The saturation/lightness it emits are kept in sync with the canvas palette's
// shimmer ring so every shimmer pixel quantizes exactly onto a register.
package shimmer

import (
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// Field computes shimmer colors for portal interiors. It caches HSL
// conversions per quantized hue so a frame of portal pixels costs a map
// lookup per pixel instead of trig per pixel.
type Field struct {
	cache map[int]canvas.Color
}

// NewField creates an empty shimmer field.
func NewField() *Field {
	return &Field{cache: make(map[int]canvas.Color, 360)}
}

// At returns the shimmer color for portal-local pixel (x, y) at time t
// (seconds since the server started). The sweep direction follows (x + y/2)
// so the band moves diagonally through the oval.
func (f *Field) At(x, y int, t float64) canvas.Color {
	hue := math.Mod(float64(x)*8+float64(y)*4+t*60, 360)
	if hue < 0 {
		hue += 360
	}
	// Quantize to whole degrees: invisible visually, tiny cache.
	key := int(hue)
	if c, ok := f.cache[key]; ok {
		return c
	}
	c := canvas.HSL(float64(key), 0.9, 0.6)
	f.cache[key] = c
	return c
}
