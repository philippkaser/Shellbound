package halfblock

import (
	"strings"
	"testing"
)

func TestHex(t *testing.T) {
	cases := map[string]Color{
		"#FFFFFF": 0xFFFFFF,
		"#000000": 0x000000,
		"#FF5FAF": 0xFF5FAF,
		"":        0,
		"FFFFFF":  0,
		"#GGGGGG": 0,
		"#FFF":    0,
	}
	for in, want := range cases {
		if got := Hex(in); got != want {
			t.Errorf("Hex(%q) = %06X, want %06X", in, uint32(got), uint32(want))
		}
	}
}

func TestRGB(t *testing.T) {
	if RGB(0x12, 0x34, 0x56) != 0x123456 {
		t.Error("RGB packing wrong")
	}
}

func TestPixelBounds(t *testing.T) {
	c := New(4, 2) // pixel space 4x4
	c.SetPx(-1, 0, 1)
	c.SetPx(0, -1, 1)
	c.SetPx(4, 0, 1)
	c.SetPx(0, 4, 1)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if c.PxAt(x, y) != Black {
				t.Fatalf("out-of-bounds write leaked to (%d,%d)", x, y)
			}
		}
	}
	c.SetPx(3, 3, 0xABCDEF)
	if c.PxAt(3, 3) != 0xABCDEF {
		t.Error("in-bounds pixel write lost")
	}
}

func TestRenderRowEmitsRunes(t *testing.T) {
	c := New(3, 1)
	c.SetPx(0, 0, 0xFFFFFF) // top pixel of cell 0
	c.SetGlyph(2, 0, 'X', 0xFF00FF)
	var sb strings.Builder
	c.RenderRow(0, &sb)
	out := sb.String()
	if !strings.ContainsRune(out, '▀') {
		t.Errorf("expected half block in output: %q", out)
	}
	if !strings.ContainsRune(out, 'X') {
		t.Errorf("expected glyph in output: %q", out)
	}
	if !strings.HasSuffix(out, "\x1b[0m") {
		t.Errorf("row must end with SGR reset: %q", out)
	}
}

func TestBlitCopiesBothLayers(t *testing.T) {
	src := New(2, 2)
	src.SetPx(0, 0, 0x111111)
	src.SetPx(0, 1, 0x222222)
	src.SetGlyph(1, 1, 'G', 0x333333)

	dst := New(4, 4)
	dst.Blit(src, 0, 0, 2, 2, 1, 1)

	if dst.PxAt(1, 2) != 0x111111 || dst.PxAt(1, 3) != 0x222222 {
		t.Error("pixel layer not blitted correctly")
	}
	if g := dst.GlyphAt(2, 2); g.Ch != 'G' || g.FG != 0x333333 {
		t.Errorf("glyph layer not blitted correctly: %+v", g)
	}
}

func TestBlitClips(t *testing.T) {
	src := New(2, 2)
	src.SetGlyph(0, 0, 'A', 1)
	dst := New(2, 2)
	// Should not panic and should clip silently.
	dst.Blit(src, -5, -5, 10, 10, -1, -1)
	dst.Blit(src, 0, 0, 2, 2, 5, 5)
}

func TestWriteTextClips(t *testing.T) {
	c := New(3, 1)
	c.WriteText(1, 0, "abcdef", 0xFFFFFF)
	if g := c.GlyphAt(1, 0); g.Ch != 'a' {
		t.Errorf("expected 'a' at (1,0), got %q", g.Ch)
	}
	if g := c.GlyphAt(2, 0); g.Ch != 'b' {
		t.Errorf("expected 'b' at (2,0), got %q", g.Ch)
	}
}
