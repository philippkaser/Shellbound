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

// Encoder holds the per-band scratch buffers so a session encoding a frame
// every tick never re-allocates. The zero value is ready to use; an Encoder
// must not be shared between goroutines.
type Encoder struct {
	masks []byte
	used  []bool
	order []int
}

// Encode appends a complete Sixel image of the w×h indexed framebuffer pix
// (row-major palette indices, len w*h) to sb, using palette as the index→RGB
// map. It is a no-op when the inputs are inconsistent so the renderer never
// fails mid-frame. Convenience wrapper over Encoder for one-shot callers.
func Encode(sb *strings.Builder, pix []byte, w, h int, palette []RGB) {
	var e Encoder
	e.Encode(sb, pix, w, h, palette)
}

// Encode appends a complete Sixel image of the w×h indexed framebuffer pix
// (row-major palette indices, len w*h) to sb, using palette as the index→RGB
// map. It is a no-op when the inputs are inconsistent so the renderer never
// fails mid-frame.
//
// The output is self-contained: DCS introducer, color register definitions,
// banded pixel data and the ST terminator. Every pixel — including index 0 —
// is painted, so the frame is fully opaque regardless of the terminal's
// background-fill interpretation.
func (e *Encoder) Encode(sb *strings.Builder, pix []byte, w, h int, palette []RGB) {
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
	// Per-color 6-bit column masks for one band, built in a single pass over
	// the band's pixels (rather than re-scanning the band once per color).
	// Scratch is reused across frames; masks are kept zeroed between bands.
	if cap(e.masks) < nColors*w {
		e.masks = make([]byte, nColors*w)
	}
	if cap(e.used) < nColors {
		e.used = make([]bool, nColors)
	}
	if cap(e.order) < nColors {
		e.order = make([]int, 0, nColors)
	}
	masks := e.masks[:nColors*w]
	used := e.used[:nColors]
	order := e.order[:0] // colors in first-appearance order

	for top := 0; top < h; top += 6 {
		rows := 6
		if top+rows > h {
			rows = h - top
		}

		order = order[:0]
		for r := 0; r < rows; r++ {
			rowOff := (top + r) * w
			bit := byte(1) << uint(r)
			for x := 0; x < w; x++ {
				idx := pix[rowOff+x]
				if int(idx) >= nColors {
					continue
				}
				if !used[idx] {
					used[idx] = true
					order = append(order, int(idx))
				}
				masks[int(idx)*w+x] |= bit
			}
		}

		for i, ci := range order {
			band := masks[ci*w : ci*w+w]
			if i > 0 {
				sb.WriteByte('$') // graphics CR: overlay next color on this band
			}
			sb.WriteByte('#')
			sb.WriteString(strconv.Itoa(ci))
			writeRLE(sb, band)
			// Reset this color's mask for the next band.
			for x := range band {
				band[x] = 0
			}
			used[ci] = false
		}
		sb.WriteByte('-') // graphics NL: advance to the next band
	}

	sb.WriteString("\x1b\\") // ST terminator
}

// writeRLE emits one band's sixel characters with `!count` run-length
// compression. A sixel data byte is '?' (0x3F) plus the 6-bit value.
// Trailing empty columns are trimmed: a zero sixel paints nothing, so the
// bytes are pure overhead (the graphics CR/NL reset the column anyway).
func writeRLE(sb *strings.Builder, band []byte) {
	n := len(band)
	for n > 0 && band[n-1] == 0 {
		n--
	}
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
