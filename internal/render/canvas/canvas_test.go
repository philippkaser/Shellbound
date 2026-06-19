package canvas

import (
	"strings"
	"testing"
)

func TestPaletteGrayRamp(t *testing.T) {
	p := NewPalette(64, nil)
	if got := p.Index(Black); got != 0 {
		t.Errorf("black -> index %d, want 0", got)
	}
	if got := p.Index(RGB(255, 255, 255)); got != 63 {
		t.Errorf("white -> index %d, want 63 (top of 64-step ramp)", got)
	}
	// A mid grey lands on a mid-ramp register, monotonically.
	mid := p.Index(RGB(128, 128, 128))
	if mid <= 0 || mid >= 63 {
		t.Errorf("mid grey -> index %d, want strictly interior", mid)
	}
}

func TestPaletteNearestColor(t *testing.T) {
	pink := Hex("#FF5FAF")
	p := NewPalette(64, []Color{pink})
	idx := p.Index(pink)
	rgb := p.RGB()[idx]
	if rgb.R != pink.R() || rgb.G != pink.G() || rgb.B != pink.B() {
		t.Errorf("injected color did not round-trip: got %+v, want %v", rgb, pink)
	}
	// The grey ramp must not capture a saturated color.
	if int(idx) < 64 {
		t.Errorf("saturated color landed in grey ramp at index %d", idx)
	}
}

func TestDrawTextSetsPixels(t *testing.T) {
	c := New(40, 12)
	before := countLit(c)
	c.DrawText(1, 1, "Hi", RGB(255, 255, 255))
	if countLit(c) <= before {
		t.Fatal("DrawText lit no pixels")
	}
	// Width helper agrees with two glyphs.
	if w := TextWidth("Hi"); w != 2*AdvanceX-1 {
		t.Errorf("TextWidth=%d, want %d", w, 2*AdvanceX-1)
	}
}

func TestEncodeSixelNonEmpty(t *testing.T) {
	c := New(8, 8)
	c.Clear(Black)
	c.FillRect(2, 2, 4, 4, RGB(255, 255, 255))
	p := NewPalette(64, nil)
	var sb strings.Builder
	c.EncodeSixel(&sb, p)
	out := sb.String()
	if !strings.HasPrefix(out, "\x1bP") || !strings.HasSuffix(out, "\x1b\\") {
		t.Fatalf("output is not a DCS sixel string: %q", out[:min(16, len(out))])
	}
}

func countLit(c *Canvas) int {
	n := 0
	for _, px := range c.Pixels() {
		if px != Black {
			n++
		}
	}
	return n
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
