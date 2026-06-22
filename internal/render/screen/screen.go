// Package screen centralizes how Shellbound's Sixel frames are sized and placed
// in the terminal. Both the plaza and the portal worlds render into a pixel
// canvas and ship it as one Sixel image; this package decides the fixed,
// letterboxed viewport (so every player sees the same slice of world regardless
// of terminal size) and writes the cursor-positioning + image bytes.
package screen

import (
	"strconv"
	"strings"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// ViewW/ViewH is the fixed play-area viewport in pixels. The frame is
// letterboxed to this size and centered: a larger terminal only gets wider
// margins, never more world. Sized to suit a roomy terminal; smaller terminals
// clamp to what fits.
const (
	ViewW = 864
	ViewH = 480
)

// Dims returns the frame's pixel size and the top-left cell offset that centers
// it, for a terminal of cols×rows character cells whose cell is cellW×cellH
// pixels. The image is the fixed viewport snapped down to a whole number of
// cells (so it lands on cell boundaries, which makes centering exact) and
// clamped to what the terminal can show, holding one row free as cheap
// insurance against a Sixel scroll.
func Dims(cols, rows, cellW, cellH int) (pw, ph, left, top int) {
	if cellW < 1 {
		cellW = 8
	}
	if cellH < 1 {
		cellH = 16
	}
	imgW := ViewW
	if m := cols * cellW; imgW > m {
		imgW = m
	}
	imgH := ViewH
	if m := (rows - 1) * cellH; imgH > m {
		imgH = m
	}
	imgCols, imgRows := imgW/cellW, imgH/cellH
	if imgCols < 1 {
		imgCols = 1
	}
	if imgRows < 1 {
		imgRows = 1
	}
	pw, ph = imgCols*cellW, imgRows*cellH
	if left = (cols - imgCols) / 2; left < 0 {
		left = 0
	}
	if top = (rows - imgRows) / 2; top < 0 {
		top = 0
	}
	return
}

// Place writes the cursor positioning (so the image is centered at the given
// cell offset) followed by the canvas's Sixel encoding into sb.
func Place(sb *strings.Builder, scr *canvas.Canvas, pal *canvas.Palette, left, top int) {
	sb.WriteString("\x1b[?25l\x1b[")
	sb.WriteString(strconv.Itoa(top + 1))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(left + 1))
	sb.WriteByte('H')
	scr.EncodeSixel(sb, pal)
}
