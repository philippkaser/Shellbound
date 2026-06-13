// Package pixelbuf implements a 2x2-subpixel buffer over terminal cells
// using Unicode quadrant blocks. Each character cell covers a 2-wide,
// 2-tall pixel patch; flushing chooses the quadrant rune whose filled
// corners match the lit pixels.
//
// A terminal cell has only one foreground color, so when a cell contains
// pixels of more than one color the dominant color wins and the others are
// still rendered as lit quadrants in that color (lossy but stable). For the
// monochrome decorations Shellbound uses it for, this never matters.
package pixelbuf

// Color is a 24-bit 0xRRGGBB color. The zero value means "unlit".
type Color uint32

// quadRunes maps a 4-bit corner mask to the quadrant rune. Bit 1 is the
// upper-left pixel, bit 2 upper-right, bit 4 lower-left, bit 8 lower-right.
var quadRunes = [16]rune{
	' ', '▘', '▝', '▀',
	'▖', '▌', '▞', '▛',
	'▗', '▚', '▐', '▜',
	'▄', '▙', '▟', '█',
}

// Cell is one flushed terminal cell: a quadrant rune plus its foreground
// color. A space rune means the cell is empty.
type Cell struct {
	Rune rune
	FG   Color
}

// Buffer is a fixed-size pixel buffer. Pixel (0,0) is the top-left.
type Buffer struct {
	w, h int // in pixels
	px   []Color
}

// New creates a buffer that is wPx pixels wide and hPx pixels tall.
// Dimensions are rounded up to even numbers so they map to whole cells.
func New(wPx, hPx int) *Buffer {
	if wPx < 2 {
		wPx = 2
	}
	if hPx < 2 {
		hPx = 2
	}
	if wPx%2 != 0 {
		wPx++
	}
	if hPx%2 != 0 {
		hPx++
	}
	return &Buffer{w: wPx, h: hPx, px: make([]Color, wPx*hPx)}
}

// Width returns the buffer width in pixels.
func (b *Buffer) Width() int { return b.w }

// Height returns the buffer height in pixels.
func (b *Buffer) Height() int { return b.h }

// CellWidth returns the flushed width in terminal cells.
func (b *Buffer) CellWidth() int { return b.w / 2 }

// CellHeight returns the flushed height in terminal cells.
func (b *Buffer) CellHeight() int { return b.h / 2 }

// Set lights pixel (x, y) with color c. Out-of-bounds writes are ignored.
func (b *Buffer) Set(x, y int, c Color) {
	if x < 0 || y < 0 || x >= b.w || y >= b.h {
		return
	}
	b.px[y*b.w+x] = c
}

// At returns the color of pixel (x, y); out-of-bounds reads return 0.
func (b *Buffer) At(x, y int) Color {
	if x < 0 || y < 0 || x >= b.w || y >= b.h {
		return 0
	}
	return b.px[y*b.w+x]
}

// Clear unlights every pixel.
func (b *Buffer) Clear() {
	for i := range b.px {
		b.px[i] = 0
	}
}

// Cells flushes the buffer to terminal cells, row-major, CellWidth() wide
// and CellHeight() tall.
func (b *Buffer) Cells() []Cell {
	cw, ch := b.CellWidth(), b.CellHeight()
	out := make([]Cell, cw*ch)
	for cy := 0; cy < ch; cy++ {
		for cx := 0; cx < cw; cx++ {
			ul := b.At(cx*2, cy*2)
			ur := b.At(cx*2+1, cy*2)
			ll := b.At(cx*2, cy*2+1)
			lr := b.At(cx*2+1, cy*2+1)

			fg := dominant(ul, ur, ll, lr)
			mask := 0
			if ul != 0 {
				mask |= 1
			}
			if ur != 0 {
				mask |= 2
			}
			if ll != 0 {
				mask |= 4
			}
			if lr != 0 {
				mask |= 8
			}
			out[cy*cw+cx] = Cell{Rune: quadRunes[mask], FG: fg}
		}
	}
	return out
}

// dominant returns the most frequent non-zero color among the four corner
// pixels; ties break toward the earliest argument (upper-left first).
func dominant(cs ...Color) Color {
	var best Color
	bestN := 0
	for _, c := range cs {
		if c == 0 {
			continue
		}
		n := 0
		for _, o := range cs {
			if o == c {
				n++
			}
		}
		// Strict > keeps the earliest color on ties because later equal
		// counts do not displace it.
		if n > bestN {
			best, bestN = c, n
		}
	}
	return best
}
