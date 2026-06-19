// Package frame holds rendering geometry shared by every Sixel screen (the
// plaza and the portal worlds): how big the pixel canvas should be and where
// to position it so the image is centered in the terminal.
package frame

import "strconv"

// Geometry returns the frame's pixel size (a whole number of terminal cells,
// bounded by maxW×maxH) and the cursor-positioning prefix that centers the
// image. Bounding in pixels means every player sees the same slice of the
// world regardless of window or cell size; bigger windows just get letterbox.
func Geometry(termW, termH, cellW, cellH, maxW, maxH int) (pw, ph int, prefix string) {
	if cellW <= 0 {
		cellW = 8
	}
	if cellH <= 0 {
		cellH = 16
	}
	pw, ph = termW*cellW, termH*cellH
	if pw > maxW {
		pw = maxW
	}
	if ph > maxH {
		ph = maxH
	}
	// Snap to whole cells so the centering offset is exact.
	pw = (pw / cellW) * cellW
	ph = (ph / cellH) * cellH
	if pw < cellW {
		pw = cellW
	}
	if ph < cellH {
		ph = cellH
	}

	imgCols, imgRows := pw/cellW, ph/cellH
	leftCols := (termW - imgCols) / 2
	topRows := (termH - imgRows) / 2
	if leftCols < 0 {
		leftCols = 0
	}
	if topRows < 0 {
		topRows = 0
	}
	// Hide cursor, then position at the centered cell (CUP is 1-based).
	prefix = "\x1b[?25l\x1b[" + strconv.Itoa(topRows+1) + ";" + strconv.Itoa(leftCols+1) + "H"
	return pw, ph, prefix
}
