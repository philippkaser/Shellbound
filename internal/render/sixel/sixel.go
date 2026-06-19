// Package sixel encodes an indexed framebuffer into a Sixel data string
// (the DEC graphics protocol most modern terminals understand). It is the
// lowest layer of Shellbound's pixel renderer: the canvas quantizes its RGB
// buffer to palette indices and hands them here.
//
// The encoder mirrors the project's run-length-minimized output philosophy:
// each 6-pixel-tall band emits, per palette color present in it, the column
// run of sixel characters with `!count` compression. Pure flat regions
// (most of the black background) collapse to a few bytes per band.
package sixel

import (
	"strconv"
	"strings"
)

// RGB is a 24-bit color split into 8-bit components.
type RGB struct{ R, G, B uint8 }

// to100 scales an 8-bit channel to Sixel's 0..100 percentage range (color
// register space 2 is defined in percent, not 0..255).
func to100(v uint8) int { return (int(v)*100 + 127) / 255 }

// Encode appends a complete Sixel image of the w×h indexed framebuffer pix
// (row-major palette indices, len w*h) to sb, using palette as the index→RGB
// map. It is a no-op when the inputs are inconsistent so the renderer never
// fails mid-frame.
//
// The output is self-contained: DCS introducer, color register definitions,
// banded pixel data and the ST terminator. Every pixel — including index 0 —
// is painted, so the frame is fully opaque regardless of the terminal's
// background-fill interpretation.
func Encode(sb *strings.Builder, pix []byte, w, h int, palette []RGB) {
	if w <= 0 || h <= 0 || len(pix) < w*h || len(palette) == 0 {
		return
	}

	// DCS introducer. P1=0 aspect (overridden by raster attrs), P2=1 (leave
	// unset pixels transparent — irrelevant here since we paint all), P3=0.
	sb.WriteString("\x1bP0;1;0q")
	// Raster attributes: 1:1 pixel aspect, picture w×h. Lets terminals size
	// the image deterministically instead of guessing from the data.
	sb.WriteString("\"1;1;")
	sb.WriteString(strconv.Itoa(w))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(h))

	// Color register definitions, in percent (register space 2 = RGB).
	for i, c := range palette {
		sb.WriteByte('#')
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(";2;")
		sb.WriteString(strconv.Itoa(to100(c.R)))
		sb.WriteByte(';')
		sb.WriteString(strconv.Itoa(to100(c.G)))
		sb.WriteByte(';')
		sb.WriteString(strconv.Itoa(to100(c.B)))
	}

	nColors := len(palette)
	present := make([]bool, nColors)
	band := make([]byte, w) // 6-bit sixel value per column, for one color

	for top := 0; top < h; top += 6 {
		rows := 6
		if top+rows > h {
			rows = h - top
		}

		// Which colors appear anywhere in this band.
		for i := range present {
			present[i] = false
		}
		for r := 0; r < rows; r++ {
			rowOff := (top + r) * w
			for x := 0; x < w; x++ {
				if idx := pix[rowOff+x]; int(idx) < nColors {
					present[idx] = true
				}
			}
		}

		first := true
		for ci := 0; ci < nColors; ci++ {
			if !present[ci] {
				continue
			}
			// 6-bit column mask for this color across the band.
			for x := 0; x < w; x++ {
				var v byte
				for r := 0; r < rows; r++ {
					if int(pix[(top+r)*w+x]) == ci {
						v |= 1 << uint(r)
					}
				}
				band[x] = v
			}
			if !first {
				sb.WriteByte('$') // graphics CR: overlay next color on this band
			}
			first = false
			sb.WriteByte('#')
			sb.WriteString(strconv.Itoa(ci))
			writeRLE(sb, band)
		}
		sb.WriteByte('-') // graphics NL: advance to the next band
	}

	sb.WriteString("\x1b\\") // ST terminator
}

// writeRLE emits one band's sixel characters with `!count` run-length
// compression. A sixel data byte is '?' (0x3F) plus the 6-bit value.
func writeRLE(sb *strings.Builder, band []byte) {
	n := len(band)
	for i := 0; i < n; {
		v := band[i]
		j := i + 1
		for j < n && band[j] == v {
			j++
		}
		run := j - i
		ch := byte('?') + v
		// `!Pn` + char costs >=3 bytes, so only worth it past a run of 3.
		if run >= 4 {
			sb.WriteByte('!')
			sb.WriteString(strconv.Itoa(run))
			sb.WriteByte(ch)
		} else {
			for k := 0; k < run; k++ {
				sb.WriteByte(ch)
			}
		}
		i = j
	}
}
