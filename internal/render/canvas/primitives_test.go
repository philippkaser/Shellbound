package canvas

import "testing"

func TestFillEllipseBoundsAndSymmetry(t *testing.T) {
	c := New(41, 21)
	white := RGB(255, 255, 255)
	c.FillEllipse(20, 10, 16, 8, white)

	// Extremes are painted.
	for _, p := range [][2]int{{4, 10}, {36, 10}, {20, 2}, {20, 18}} {
		if c.At(p[0], p[1]) != white {
			t.Fatalf("expected extreme (%d,%d) painted", p[0], p[1])
		}
	}
	// Corners stay empty.
	for _, p := range [][2]int{{4, 2}, {36, 2}, {4, 18}, {36, 18}} {
		if c.At(p[0], p[1]) != Black {
			t.Fatalf("expected corner (%d,%d) empty", p[0], p[1])
		}
	}
	// Horizontal and vertical symmetry.
	for dy := -8; dy <= 8; dy++ {
		for dx := -16; dx <= 16; dx++ {
			if c.At(20+dx, 10+dy) != c.At(20-dx, 10+dy) || c.At(20+dx, 10+dy) != c.At(20+dx, 10-dy) {
				t.Fatalf("asymmetry at offset (%d,%d)", dx, dy)
			}
		}
	}
}

func TestLineEndpointsAndClipping(t *testing.T) {
	c := New(10, 10)
	white := RGB(255, 255, 255)
	c.Line(1, 1, 8, 5, white)
	if c.At(1, 1) != white || c.At(8, 5) != white {
		t.Fatal("line endpoints not painted")
	}
	// A line running off-canvas must not panic and must paint its in-bounds part.
	c.Line(-5, 3, 15, 3, white)
	if c.At(0, 3) != white || c.At(9, 3) != white {
		t.Fatal("clipped horizontal line missing in-bounds pixels")
	}
}

func TestDitherAtCoverage(t *testing.T) {
	if DitherAt(3, 7, 0) {
		t.Fatal("t=0 must always be off")
	}
	if !DitherAt(3, 7, 1) {
		t.Fatal("t=1 must always be on")
	}
	// Coverage is monotonic in t over one 4×4 cell.
	count := func(tv float64) int {
		n := 0
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				if DitherAt(x, y, tv) {
					n++
				}
			}
		}
		return n
	}
	prev := 0
	for _, tv := range []float64{0.1, 0.25, 0.5, 0.75, 0.9} {
		n := count(tv)
		if n < prev {
			t.Fatalf("dither coverage not monotonic at t=%v: %d < %d", tv, n, prev)
		}
		prev = n
	}
}

func TestDrawTextScaledMatchesUnscaled(t *testing.T) {
	small := New(40, 12)
	big := New(80, 24)
	white := RGB(255, 255, 255)
	small.DrawText(0, 0, "A1", white)
	big.DrawTextScaled(0, 0, "A1", white, 2)

	for y := 0; y < 12; y++ {
		for x := 0; x < 40; x++ {
			on := small.At(x, y) == white
			for _, p := range [][2]int{{2 * x, 2 * y}, {2*x + 1, 2 * y}, {2 * x, 2*y + 1}, {2*x + 1, 2*y + 1}} {
				if (big.At(p[0], p[1]) == white) != on {
					t.Fatalf("scale-2 mismatch at (%d,%d)", x, y)
				}
			}
		}
	}
	if got, want := TextWidthScaled("A1", 2), TextWidth("A1")*2; got != want {
		t.Fatalf("TextWidthScaled = %d, want %d", got, want)
	}
}
