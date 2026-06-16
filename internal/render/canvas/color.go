package canvas

import (
	"math"
	"strconv"
)

// Color is a 24-bit 0xRRGGBB color. The zero value is pure black, which is
// also the canvas's idea of "empty".
type Color uint32

// Black is the zero Color.
const Black Color = 0

// RGB builds a Color from 8-bit components.
func RGB(r, g, b uint8) Color {
	return Color(uint32(r)<<16 | uint32(g)<<8 | uint32(b))
}

// R, G, B return the color's components.
func (c Color) R() uint8 { return uint8(c >> 16) }
func (c Color) G() uint8 { return uint8(c >> 8) }
func (c Color) B() uint8 { return uint8(c) }

// IsGray reports whether the three channels are equal (the world is drawn in
// pure greys, so this is the fast path for palette quantization).
func (c Color) IsGray() bool {
	return c.R() == c.G() && c.G() == c.B()
}

// Luma returns the perceptual brightness 0..255 (Rec. 601 weights).
func (c Color) Luma() uint8 {
	y := (299*int(c.R()) + 587*int(c.G()) + 114*int(c.B()) + 500) / 1000
	if y > 255 {
		y = 255
	}
	return uint8(y)
}

// Scale multiplies every channel by k (clamped to [0,1]), darkening the
// color. On a grey it stays grey, which keeps the lighting pass on the fast
// quantization path.
func (c Color) Scale(k float64) Color {
	if k <= 0 {
		return Black
	}
	if k >= 1 {
		return c
	}
	return RGB(
		uint8(float64(c.R())*k),
		uint8(float64(c.G())*k),
		uint8(float64(c.B())*k),
	)
}

// Lighten returns the per-channel maximum of c and o — an additive-style
// blend that can only brighten, used for glow halos.
func (c Color) Lighten(o Color) Color {
	maxc := func(a, b uint8) uint8 {
		if a > b {
			return a
		}
		return b
	}
	return RGB(maxc(c.R(), o.R()), maxc(c.G(), o.G()), maxc(c.B(), o.B()))
}

// Lerp blends from c toward o by t in [0,1].
func (c Color) Lerp(o Color, t float64) Color {
	if t <= 0 {
		return c
	}
	if t >= 1 {
		return o
	}
	mix := func(a, b uint8) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }
	return RGB(mix(c.R(), o.R()), mix(c.G(), o.G()), mix(c.B(), o.B()))
}

// Hex parses "#RRGGBB" into a Color. Malformed input yields Black; the
// renderer must never fail mid-frame.
func Hex(s string) Color {
	if len(s) != 7 || s[0] != '#' {
		return Black
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return Black
	}
	return Color(v)
}

// HSL converts hue (degrees, any value), saturation and lightness (0..1) to a
// Color. Used for the portal shimmer ring and its palette.
func HSL(h, s, l float64) Color {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	s = clamp01(s)
	l = clamp01(l)

	cc := (1 - math.Abs(2*l-1)) * s
	x := cc * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - cc/2

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = cc, x, 0
	case h < 120:
		r, g, b = x, cc, 0
	case h < 180:
		r, g, b = 0, cc, x
	case h < 240:
		r, g, b = 0, x, cc
	case h < 300:
		r, g, b = x, 0, cc
	default:
		r, g, b = cc, 0, x
	}
	return RGB(to255(r+m), to255(g+m), to255(b+m))
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
