package pixelbuf

import "testing"

func TestDimensionsRoundUp(t *testing.T) {
	b := New(5, 3)
	if b.Width() != 6 || b.Height() != 4 {
		t.Fatalf("expected 6x4 px, got %dx%d", b.Width(), b.Height())
	}
	if b.CellWidth() != 3 || b.CellHeight() != 2 {
		t.Fatalf("expected 3x2 cells, got %dx%d", b.CellWidth(), b.CellHeight())
	}
}

func TestQuadrantRunes(t *testing.T) {
	white := Color(0xFFFFFF)
	cases := []struct {
		name string
		lit  [][2]int // pixels in the single top-left cell
		want rune
	}{
		{"empty", nil, ' '},
		{"upper-left", [][2]int{{0, 0}}, '▘'},
		{"upper-right", [][2]int{{1, 0}}, '▝'},
		{"lower-left", [][2]int{{0, 1}}, '▖'},
		{"lower-right", [][2]int{{1, 1}}, '▗'},
		{"top half", [][2]int{{0, 0}, {1, 0}}, '▀'},
		{"bottom half", [][2]int{{0, 1}, {1, 1}}, '▄'},
		{"left half", [][2]int{{0, 0}, {0, 1}}, '▌'},
		{"right half", [][2]int{{1, 0}, {1, 1}}, '▐'},
		{"diagonal", [][2]int{{0, 0}, {1, 1}}, '▚'},
		{"anti-diagonal", [][2]int{{1, 0}, {0, 1}}, '▞'},
		{"full", [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}}, '█'},
	}
	for _, tc := range cases {
		b := New(2, 2)
		for _, p := range tc.lit {
			b.Set(p[0], p[1], white)
		}
		cells := b.Cells()
		if len(cells) != 1 {
			t.Fatalf("%s: expected 1 cell, got %d", tc.name, len(cells))
		}
		if cells[0].Rune != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.name, tc.want, cells[0].Rune)
		}
	}
}

func TestDominantColorWins(t *testing.T) {
	a := Color(0xAAAAAA)
	z := Color(0x111111)
	b := New(2, 2)
	b.Set(0, 0, a)
	b.Set(1, 0, a)
	b.Set(0, 1, z)
	cells := b.Cells()
	if cells[0].FG != a {
		t.Errorf("expected dominant color %06X, got %06X", uint32(a), uint32(cells[0].FG))
	}
	if cells[0].Rune != '▛' {
		t.Errorf("expected ▛ (UL+UR+LL), got %q", cells[0].Rune)
	}
}

func TestOutOfBoundsSafe(t *testing.T) {
	b := New(4, 4)
	// Must not panic.
	b.Set(-1, 0, 1)
	b.Set(0, -1, 1)
	b.Set(4, 0, 1)
	b.Set(0, 4, 1)
	if got := b.At(-1, -1); got != 0 {
		t.Errorf("out-of-bounds At should be 0, got %v", got)
	}
	for _, c := range b.Cells() {
		if c.Rune != ' ' {
			t.Errorf("out-of-bounds writes leaked into the buffer: %q", c.Rune)
		}
	}
}

func TestClear(t *testing.T) {
	b := New(4, 4)
	b.Set(1, 1, 0xFFFFFF)
	b.Clear()
	if b.At(1, 1) != 0 {
		t.Error("Clear did not reset pixels")
	}
}

func TestCellGridLayout(t *testing.T) {
	// Light exactly the second cell (cells are 2x2 px): pixels (2,0)-(3,1).
	b := New(4, 2)
	b.Set(2, 0, 0xFFFFFF)
	b.Set(3, 1, 0xFFFFFF)
	cells := b.Cells()
	if len(cells) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(cells))
	}
	if cells[0].Rune != ' ' {
		t.Errorf("cell 0 should be empty, got %q", cells[0].Rune)
	}
	if cells[1].Rune != '▚' {
		t.Errorf("cell 1 should be ▚, got %q", cells[1].Rune)
	}
}
