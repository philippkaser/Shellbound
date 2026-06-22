package canvas

import "github.com/shellbound/shellbound/internal/render/sixel"

// Palette is a fixed index→RGB table for Sixel encoding. It is built once
// (shared, read-only across sessions): a dense grey ramp for the monochrome
// world and its lighting, plus a set of saturated "accent" registers — the
// portal shimmer ramps and the curated player-name colors, the only color the
// game emits.
//
// Quantization is O(1) for greys (the overwhelming majority of pixels) and a
// short linear search for the sparse saturated pixels (portals, names).
type Palette struct {
	entries  []sixel.RGB
	grays    int   // number of grey ramp entries, at indices [0, grays)
	colorIdx []int // entry indices of the non-grey (accent) registers
}

// NewPalette builds a palette with `grays` grey levels followed by every accent
// color. The total must stay ≤ 256 (Sixel's register limit); callers keep the
// accent set small (a few ramps).
func NewPalette(grays int, accents []Color) *Palette {
	if grays < 2 {
		grays = 2
	}
	p := &Palette{grays: grays}

	// Grey ramp, black → white.
	for i := 0; i < grays; i++ {
		v := uint8(i * 255 / (grays - 1))
		p.entries = append(p.entries, sixel.RGB{R: v, G: v, B: v})
	}
	// Saturated accents (portal shimmer ramps, player colors).
	for _, c := range accents {
		p.colorIdx = append(p.colorIdx, len(p.entries))
		p.entries = append(p.entries, sixel.RGB{R: c.R(), G: c.G(), B: c.B()})
	}
	return p
}

// DefaultPalette is the standard plaza palette: a smooth 64-step grey ramp for
// the world and its lighting, plus the supplied accent colors.
func DefaultPalette(accents []Color) *Palette {
	return NewPalette(64, accents)
}

// RGB returns the register table for the encoder.
func (p *Palette) RGB() []sixel.RGB { return p.entries }

// Index maps a Color to its nearest register. Greys hit the ramp directly;
// saturated colors fall back to a nearest-neighbour search over the accents.
func (p *Palette) Index(c Color) byte {
	if c.IsGray() {
		return byte(int(c.R()) * (p.grays - 1) / 255)
	}
	best, bestD := 0, 1<<31-1
	cr, cg, cb := int(c.R()), int(c.G()), int(c.B())
	for _, ei := range p.colorIdx {
		e := p.entries[ei]
		dr, dg, db := cr-int(e.R), cg-int(e.G), cb-int(e.B)
		d := dr*dr + dg*dg + db*db
		if d < bestD {
			best, bestD = ei, d
		}
	}
	return byte(best)
}
