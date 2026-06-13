// Package halfblock implements Shellbound's main framebuffer: a grid of
// terminal cells where each cell carries two vertically stacked "pixels"
// rendered with the upper-half block ▀ (foreground = top pixel, background
// = bottom pixel). A glyph layer sits on top for text, walls and other
// character art; a glyph cell is opaque and renders on pure black.
//
// Rendering emits raw 24-bit SGR sequences instead of going through
// lipgloss styles: the canvas is the hot path (rendered every frame for
// every session) and run-length minimized escape output is dramatically
// cheaper. The SSH layer forces a TrueColor profile so this is safe.
package halfblock

import (
	"strconv"
	"strings"
)

// Color is a 24-bit 0xRRGGBB color. The zero value is pure black, which is
// also the canvas's idea of "empty".
type Color uint32

// Black is the zero Color.
const Black Color = 0

// RGB builds a Color from components.
func RGB(r, g, b uint8) Color {
	return Color(uint32(r)<<16 | uint32(g)<<8 | uint32(b))
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

// Glyph is one cell of the text layer. A zero Ch means "no glyph here"
// (the pixel layer shows through). A ' ' glyph is opaque black.
type Glyph struct {
	Ch rune
	FG Color
}

// Canvas is a W×H cell framebuffer with a W×2H pixel layer underneath a
// W×H glyph layer.
type Canvas struct {
	W, H   int
	px     []Color // len W*2H, row-major in pixel rows
	glyphs []Glyph // len W*H, row-major in cell rows
}

// New creates an empty canvas of w×h cells. Dimensions are clamped to at
// least 1×1.
func New(w, h int) *Canvas {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &Canvas{
		W:      w,
		H:      h,
		px:     make([]Color, w*h*2),
		glyphs: make([]Glyph, w*h),
	}
}

// Clear resets every pixel and glyph.
func (c *Canvas) Clear() {
	for i := range c.px {
		c.px[i] = Black
	}
	for i := range c.glyphs {
		c.glyphs[i] = Glyph{}
	}
}

// SetPx sets pixel (x, y); x is in cells/columns, y in pixel rows
// (0 .. 2H-1). Out-of-bounds writes are ignored.
func (c *Canvas) SetPx(x, y int, col Color) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H*2 {
		return
	}
	c.px[y*c.W+x] = col
}

// PxAt returns pixel (x, y), or Black when out of bounds.
func (c *Canvas) PxAt(x, y int) Color {
	if x < 0 || y < 0 || x >= c.W || y >= c.H*2 {
		return Black
	}
	return c.px[y*c.W+x]
}

// FillPx fills the pixel-space rectangle (x, y)-(x+w, y+h) with col.
func (c *Canvas) FillPx(x, y, w, h int, col Color) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			c.SetPx(xx, yy, col)
		}
	}
}

// SetGlyph places ch at cell (x, y) with foreground fg. Out-of-bounds
// writes are ignored.
func (c *Canvas) SetGlyph(x, y int, ch rune, fg Color) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	c.glyphs[y*c.W+x] = Glyph{Ch: ch, FG: fg}
}

// ClearGlyph removes any glyph at cell (x, y), letting pixels show again.
func (c *Canvas) ClearGlyph(x, y int) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	c.glyphs[y*c.W+x] = Glyph{}
}

// GlyphAt returns the glyph at cell (x, y); zero Glyph when out of bounds.
func (c *Canvas) GlyphAt(x, y int) Glyph {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return Glyph{}
	}
	return c.glyphs[y*c.W+x]
}

// WriteText writes s as glyphs starting at cell (x, y), clipping at the
// canvas edge. Spaces are written too (opaque black), which gives text a
// readable black backing over busy pixels.
func (c *Canvas) WriteText(x, y int, s string, fg Color) {
	for _, r := range s {
		if x >= c.W {
			return
		}
		c.SetGlyph(x, y, r, fg)
		x++
	}
}

// Blit copies a w×h cell rectangle from src at (srcX, srcY) to this canvas
// at (dstX, dstY). Both layers are copied; regions are clipped to both
// canvases.
func (c *Canvas) Blit(src *Canvas, srcX, srcY, w, h, dstX, dstY int) {
	if src == nil {
		return
	}
	for row := 0; row < h; row++ {
		sy, dy := srcY+row, dstY+row
		if sy < 0 || dy < 0 || sy >= src.H || dy >= c.H {
			continue
		}
		for col := 0; col < w; col++ {
			sx, dx := srcX+col, dstX+col
			if sx < 0 || dx < 0 || sx >= src.W || dx >= c.W {
				continue
			}
			c.glyphs[dy*c.W+dx] = src.glyphs[sy*src.W+sx]
			c.px[(dy*2)*c.W+dx] = src.px[(sy*2)*src.W+sx]
			c.px[(dy*2+1)*c.W+dx] = src.px[(sy*2+1)*src.W+sx]
		}
	}
}

// Clone returns a deep copy of the canvas (used to snapshot the static
// plaza base once at startup).
func (c *Canvas) Clone() *Canvas {
	n := New(c.W, c.H)
	copy(n.px, c.px)
	copy(n.glyphs, c.glyphs)
	return n
}

// sgr writing helpers ------------------------------------------------------

func writeColor(sb *strings.Builder, prefix string, col Color) {
	sb.WriteString(prefix) // "\x1b[38;2;" or "\x1b[48;2;"
	sb.WriteString(strconv.Itoa(int(col >> 16 & 0xFF)))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(int(col >> 8 & 0xFF)))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(int(col & 0xFF)))
	sb.WriteByte('m')
}

// RenderRow appends one terminal line for cell row y to sb, including a
// trailing SGR reset. Escape sequences are emitted only when the color
// state changes between cells.
func (c *Canvas) RenderRow(y int, sb *strings.Builder) {
	if y < 0 || y >= c.H {
		return
	}
	const noColor = Color(0xFFFFFFFF) // sentinel: nothing emitted yet
	curFG, curBG := noColor, noColor

	top := (y * 2) * c.W
	bot := (y*2 + 1) * c.W
	row := y * c.W

	for x := 0; x < c.W; x++ {
		g := c.glyphs[row+x]
		var r rune
		var fg, bg Color
		fgMatters := true

		if g.Ch != 0 {
			r, fg, bg = g.Ch, g.FG, Black
			if r == ' ' {
				fgMatters = false
			}
		} else {
			t, b := c.px[top+x], c.px[bot+x]
			if t == Black && b == Black {
				r, bg, fgMatters = ' ', Black, false
			} else {
				r, fg, bg = '▀', t, b
			}
		}

		if fgMatters && fg != curFG {
			writeColor(sb, "\x1b[38;2;", fg)
			curFG = fg
		}
		if bg != curBG {
			writeColor(sb, "\x1b[48;2;", bg)
			curBG = bg
		}
		sb.WriteRune(r)
	}
	sb.WriteString("\x1b[0m")
}

// Render appends the whole canvas to sb, rows separated by newlines, with
// no trailing newline.
func (c *Canvas) Render(sb *strings.Builder) {
	for y := 0; y < c.H; y++ {
		if y > 0 {
			sb.WriteByte('\n')
		}
		c.RenderRow(y, sb)
	}
}
