package server

import (
	"bytes"
	"testing"
)

func TestFindCellResponse(t *testing.T) {
	// Reply is ESC [ 6 ; height ; width t.
	w, h, lo, hi, ok := findCellResponse([]byte("\x1b[6;30;12t"))
	if !ok || w != 12 || h != 30 || lo != 0 || hi != 10 {
		t.Fatalf("got w=%d h=%d lo=%d hi=%d ok=%v", w, h, lo, hi, ok)
	}

	// Embedded in other bytes.
	w, h, lo, hi, ok = findCellResponse([]byte("ab\x1b[6;20;10tZ"))
	if !ok || w != 10 || h != 20 || lo != 2 || hi != 12 {
		t.Fatalf("embedded: w=%d h=%d lo=%d hi=%d ok=%v", w, h, lo, hi, ok)
	}

	for _, s := range []string{"hello", "\x1b[6;20;", "\x1b[6;20;10", "\x1b[4;100;200t"} {
		if _, _, _, _, ok := findCellResponse([]byte(s)); ok {
			t.Errorf("expected no match for %q", s)
		}
	}
}

func TestCellSizeReaderStripsAndPassesThrough(t *testing.T) {
	src := bytes.NewReader([]byte("\x1b[6;30;12tWASD"))
	var gotW, gotH int
	r := &cellSizeReader{src: src, onCell: func(w, h int) { gotW, gotH = w, h }}

	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	if string(buf[:n]) != "WASD" {
		t.Fatalf("after strip got %q, want %q", buf[:n], "WASD")
	}
	if gotW != 12 || gotH != 30 {
		t.Fatalf("reported cell size w=%d h=%d, want 12x30", gotW, gotH)
	}
}

func TestCellSizeReaderNoResponseForwards(t *testing.T) {
	src := bytes.NewReader([]byte("wasd"))
	called := false
	r := &cellSizeReader{src: src, onCell: func(int, int) { called = true }}
	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	if string(buf[:n]) != "wasd" || called {
		t.Fatalf("got %q called=%v, want unchanged input and no callback", buf[:n], called)
	}
}
