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
	"math"
	"strings"

	"github.com/shellbound/shellbound/internal/render/sixel"
)

// Canvas is a W×H RGB pixel buffer, row-major. The zero color is black.
type Canvas struct {
	W, H int
	px   []Color
	idx  []byte        // reused index scratch for Sixel encoding
	enc  sixel.Encoder // reused band scratch for Sixel encoding
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

// hspan fills the inclusive horizontal run [x0, x1] on row y after clipping,
// touching the backing slice directly. It is the workhorse under every filled
// primitive: clip once per row instead of bounds-checking every pixel.
func (c *Canvas) hspan(x0, x1, y int, col Color) {
	if y < 0 || y >= c.H {
		return
	}
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if x0 < 0 {
		x0 = 0
	}
	if x1 >= c.W {
		x1 = c.W - 1
	}
	row := y * c.W
	for x := x0; x <= x1; x++ {
		c.px[row+x] = col
	}
}

// HSpan fills the inclusive horizontal run [x0, x1] on row y, clipped.
func (c *Canvas) HSpan(x0, x1, y int, col Color) { c.hspan(x0, x1, y, col) }

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
		c.hspan(x, x+w-1, yy, col)
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
		half := isqrt(r2 - dy*dy)
		c.hspan(cx-half, cx+half, cy+dy, col)
	}
}

// FillEllipse fills an axis-aligned ellipse with half-extents rx×ry centered
// at (cx, cy). The 2:1 case is the footprint of everything round in the
// isometric ground plane (portal pools, contact shadows, pond banks).
func (c *Canvas) FillEllipse(cx, cy, rx, ry int, col Color) {
	if rx < 0 || ry < 0 {
		return
	}
	if ry == 0 {
		c.hspan(cx-rx, cx+rx, cy, col)
		return
	}
	ry2 := ry * ry
	for dy := -ry; dy <= ry; dy++ {
		// half = rx * sqrt(1 - dy²/ry²), in integer math.
		half := isqrt((ry2 - dy*dy) * rx * rx / ry2)
		c.hspan(cx-half, cx+half, cy+dy, col)
	}
}

// Line draws a 1px Bresenham line from (x0, y0) to (x1, y1).
func (c *Canvas) Line(x0, y0, x1, y1 int, col Color) {
	dx, dy := x1-x0, y1-y0
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy
	for {
		c.Set(x0, y0, col)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

// Blend alpha-blends col over pixel (x, y) with opacity a in [0,1].
func (c *Canvas) Blend(x, y int, col Color, a float64) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	i := y*c.W + x
	c.px[i] = c.px[i].Lerp(col, a)
}

// LightenPx additively brightens pixel (x, y) toward col (per-channel max).
func (c *Canvas) LightenPx(x, y int, col Color) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	i := y*c.W + x
	c.px[i] = c.px[i].Lighten(col)
}

// bayer4 is the classic 4×4 ordered-dither matrix, thresholds 0..15.
var bayer4 = [4][4]uint8{
	{0, 8, 2, 10},
	{12, 4, 14, 6},
	{3, 11, 1, 9},
	{15, 7, 13, 5},
}

// DitherAt reports whether pixel (x, y) is "on" for coverage t in [0,1] under
// a 4×4 ordered dither — the house pattern for soft edges and translucency in
// a hard-pixel world.
func DitherAt(x, y int, t float64) bool {
	if t <= 0 {
		return false
	}
	if t >= 1 {
		return true
	}
	return t*16 > float64(bayer4[y&3][x&3])+0.5
}

// isqrt is the integer square root (floor).
func isqrt(n int) int {
	if n <= 0 {
		return 0
	}
	x := int(math.Sqrt(float64(n)))
	for x*x > n {
		x--
	}
	for (x+1)*(x+1) <= n {
		x++
	}
	return x
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
