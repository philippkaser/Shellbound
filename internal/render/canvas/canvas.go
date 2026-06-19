// Package canvas is Shellbound's pixel framebuffer: a true W×H grid of RGB
// pixels that the world, sprites, lighting and a baked bitmap font all draw
// into, then flushed to the terminal as a single Sixel image.
//
// It replaces the old half-block/quadrant scheme: there are no more "two
// pixels per cell" tricks — one canvas pixel is one image pixel, and text is
// drawn as pixels too (see font.go) so the whole frame composites in one
// place and ships as one graphic.
package canvas

import (
	"strings"

	"github.com/shellbound/shellbound/internal/render/sixel"
)

// Canvas is a W×H RGB pixel buffer, row-major. The zero color is black.
type Canvas struct {
	W, H int
	px   []Color
	idx  []byte        // reused index scratch for Sixel encoding
	enc  sixel.Encoder // reused encoder scratch (no per-frame garbage)
}

// New allocates a w×h canvas, clamped to at least 1×1.
func New(w, h int) *Canvas {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &Canvas{W: w, H: h, px: make([]Color, w*h)}
}

// Resize reallocates the buffer if the dimensions changed; otherwise it is a
// no-op so frame-to-frame the same backing slice is reused.
func (c *Canvas) Resize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if c.W == w && c.H == h {
		return
	}
	c.W, c.H = w, h
	c.px = make([]Color, w*h)
}

// Clear resets every pixel to col.
func (c *Canvas) Clear(col Color) {
	for i := range c.px {
		c.px[i] = col
	}
}

// Set writes pixel (x, y). Out-of-bounds writes are ignored.
func (c *Canvas) Set(x, y int, col Color) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	c.px[y*c.W+x] = col
}

// At returns pixel (x, y), or Black when out of bounds.
func (c *Canvas) At(x, y int) Color {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return Black
	}
	return c.px[y*c.W+x]
}

// index returns the backing slice (read/write) for hot passes like lighting.
func (c *Canvas) Pixels() []Color { return c.px }

// FillRect fills the rectangle (x, y)-(x+w, y+h) with col, clipped to bounds.
func (c *Canvas) FillRect(x, y, w, h int, col Color) {
	for yy := y; yy < y+h; yy++ {
		if yy < 0 || yy >= c.H {
			continue
		}
		row := yy * c.W
		for xx := x; xx < x+w; xx++ {
			if xx < 0 || xx >= c.W {
				continue
			}
			c.px[row+xx] = col
		}
	}
}

// Rect strokes a one-pixel border around (x, y)-(x+w, y+h).
func (c *Canvas) Rect(x, y, w, h int, col Color) {
	if w <= 0 || h <= 0 {
		return
	}
	c.HLine(x, x+w-1, y, col)
	c.HLine(x, x+w-1, y+h-1, col)
	c.VLine(x, y, y+h-1, col)
	c.VLine(x+w-1, y, y+h-1, col)
}

// HLine draws a horizontal line from x0 to x1 (inclusive) at row y.
func (c *Canvas) HLine(x0, x1, y int, col Color) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	for x := x0; x <= x1; x++ {
		c.Set(x, y, col)
	}
}

// VLine draws a vertical line from y0 to y1 (inclusive) at column x.
func (c *Canvas) VLine(x, y0, y1 int, col Color) {
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	for y := y0; y <= y1; y++ {
		c.Set(x, y, col)
	}
}

// FillCircle fills a disc of radius r centered at (cx, cy) with col.
func (c *Canvas) FillCircle(cx, cy, r int, col Color) {
	if r < 0 {
		return
	}
	r2 := r * r
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r2 {
				c.Set(cx+dx, cy+dy, col)
			}
		}
	}
}

// FillEllipse fills an axis-aligned ellipse of radii (rx, ry) centered at
// (cx, cy). Used for avatar drop shadows and portal ground glows.
func (c *Canvas) FillEllipse(cx, cy, rx, ry int, col Color) {
	if rx <= 0 || ry <= 0 {
		return
	}
	rx2, ry2 := rx*rx, ry*ry
	for dy := -ry; dy <= ry; dy++ {
		for dx := -rx; dx <= rx; dx++ {
			if dx*dx*ry2+dy*dy*rx2 <= rx2*ry2 {
				c.Set(cx+dx, cy+dy, col)
			}
		}
	}
}

// Clone returns a deep copy (used to snapshot the static plaza base once).
func (c *Canvas) Clone() *Canvas {
	n := New(c.W, c.H)
	copy(n.px, c.px)
	return n
}

// Blit copies all of src onto this canvas with src's top-left at (dstX, dstY).
// Pixels equal to transparent are skipped; pass Black-impossible sentinel by
// using BlitOpaque when every pixel should copy.
func (c *Canvas) Blit(src *Canvas, dstX, dstY int, transparent Color) {
	if src == nil {
		return
	}
	for sy := 0; sy < src.H; sy++ {
		dy := dstY + sy
		if dy < 0 || dy >= c.H {
			continue
		}
		for sx := 0; sx < src.W; sx++ {
			col := src.px[sy*src.W+sx]
			if col == transparent {
				continue
			}
			dx := dstX + sx
			if dx < 0 || dx >= c.W {
				continue
			}
			c.px[dy*c.W+dx] = col
		}
	}
}

// EncodeSixel quantizes the canvas through pal and appends a complete Sixel
// image to sb. The index scratch is reused across frames.
func (c *Canvas) EncodeSixel(sb *strings.Builder, pal *Palette) {
	n := c.W * c.H
	if cap(c.idx) < n {
		c.idx = make([]byte, n)
	}
	c.idx = c.idx[:n]
	for i, col := range c.px {
		c.idx[i] = pal.Index(col)
	}
	c.enc.Encode(sb, c.idx, c.W, c.H, pal.RGB())
}
