package canvas

import "github.com/shellbound/shellbound/internal/render/sixel"

// Palette is a fixed index→RGB table for Sixel encoding. It is built once
// (shared, read-only across sessions) and maps any drawn Color to its nearest
// register: a dense grey ramp for the monochrome world and lighting, plus a
// ring of shimmer hues and the curated player-name colors as the only
// saturated entries — matching the game's strict colour discipline.
//
// Quantization is O(1) for greys (the overwhelming majority of pixels) and a
// short linear search for the sparse saturated pixels (portals, names).
type Palette struct {
	entries  []sixel.RGB
	grays    int   // number of grey ramp entries, at indices [0, grays)
	colorIdx []int // entry indices of the non-grey (saturated) registers
}

// Shimmer saturation/lightness — kept in sync with the portal shimmer field
// so quantization lands exactly on a ring entry.
const (
	shimmerS = 0.9
	shimmerL = 0.6
)

// NewPalette builds a palette with `grays` grey levels and `hues` shimmer-ring
// entries, plus every color in extra (typically the player-name palette). The
// total must stay ≤ 256 (Sixel's register limit); callers pass small counts.
func NewPalette(grays, hues int, extra []Color) *Palette {
	if grays < 2 {
		grays = 2
	}
	p := &Palette{grays: grays}

	// Grey ramp, black → white.
	for i := 0; i < grays; i++ {
		v := uint8(i * 255 / (grays - 1))
		p.entries = append(p.entries, sixel.RGB{R: v, G: v, B: v})
	}
	// Shimmer hue ring.
	for i := 0; i < hues; i++ {
		c := HSL(float64(i)*360/float64(hues), shimmerS, shimmerL)
		p.colorIdx = append(p.colorIdx, len(p.entries))
		p.entries = append(p.entries, sixel.RGB{R: c.R(), G: c.G(), B: c.B()})
	}
	// Injected saturated colors (deduplicated against existing entries).
	for _, c := range extra {
		p.colorIdx = append(p.colorIdx, len(p.entries))
		p.entries = append(p.entries, sixel.RGB{R: c.R(), G: c.G(), B: c.B()})
	}
	return p
}

// DefaultPalette is the standard plaza palette: a smooth 64-step grey ramp for
// the world and its lighting, a 32-hue shimmer ring, and the supplied player
// colors. 64+32+len(extra) stays well under 256.
func DefaultPalette(playerColors []Color) *Palette {
	return NewPalette(64, 32, playerColors)
}

// RGB returns the register table for the encoder.
func (p *Palette) RGB() []sixel.RGB { return p.entries }

// Index maps a Color to its nearest register. Greys hit the ramp directly;
// saturated colors fall back to a nearest-neighbour search over the colored
// registers.
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
