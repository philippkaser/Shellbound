package server

import "io"

// cellSizeReader wraps the session input to capture the terminal's reply to a
// "report cell size" query (CSI 16 t -> ESC [ 6 ; height ; width t). Many
// terminals don't report pixel dimensions over SSH, so this direct query is
// the most reliable way to learn the real cell size and center the Sixel image.
//
// It only ever inspects the very first read and strips a complete response
// from it; every read after that is a pure passthrough, so it can never
// corrupt steady-state keyboard input.
type cellSizeReader struct {
	src    io.Reader
	onCell func(w, h int)
	done   bool
}

func (r *cellSizeReader) Read(p []byte) (int, error) {
	if r.done {
		return r.src.Read(p)
	}
	r.done = true // inspect only the first read
	n, err := r.src.Read(p)
	if n <= 0 {
		return n, err
	}
	w, h, lo, hi, ok := findCellResponse(p[:n])
	if !ok {
		return n, err
	}
	if r.onCell != nil && w > 0 && h > 0 {
		r.onCell(w, h)
	}
	// Drop the matched [lo,hi) bytes, compacting whatever else was read.
	copy(p[lo:], p[hi:n])
	m := n - (hi - lo)
	if m == 0 && err == nil {
		// The read held nothing but the response; block for real input.
		return r.src.Read(p)
	}
	return m, err
}

// findCellResponse scans b for ESC [ 6 ; <height> ; <width> t and returns the
// width, height and the byte range it occupies. The reply lists height first.
func findCellResponse(b []byte) (w, h, lo, hi int, ok bool) {
	for i := 0; i+3 < len(b); i++ {
		if b[i] != 0x1b || b[i+1] != '[' || b[i+2] != '6' || b[i+3] != ';' {
			continue
		}
		height, j, okH := parseNum(b, i+4)
		if !okH || j >= len(b) || b[j] != ';' {
			continue
		}
		width, k, okW := parseNum(b, j+1)
		if !okW || k >= len(b) || b[k] != 't' {
			continue
		}
		return width, height, i, k + 1, true
	}
	return 0, 0, 0, 0, false
}

// parseNum reads a base-10 number starting at i, returning its value and the
// index just past it. ok is false if no digit was present.
func parseNum(b []byte, i int) (val, next int, ok bool) {
	start := i
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		val = val*10 + int(b[i]-'0')
		i++
	}
	return val, i, i > start
}
