package iso

import (
	"math"
	"testing"
)

func TestProjectUnprojectRoundTrip(t *testing.T) {
	for _, p := range [][2]float64{{0, 0}, {10, 4}, {99, 49}, {3.5, 7.25}} {
		sx, sy := Project(p[0], p[1])
		gx, gy := Unproject(sx, sy)
		if math.Abs(gx-p[0]) > 1e-9 || math.Abs(gy-p[1]) > 1e-9 {
			t.Errorf("round trip (%v,%v) -> (%v,%v)", p[0], p[1], gx, gy)
		}
	}
}

func TestProjectIsometry(t *testing.T) {
	// Moving +1 in gx and +1 in gy both move the same number of rows down,
	// and in opposite horizontal directions — the defining 2:1 property.
	sx0, sy0 := Project(5, 5)
	sxX, syX := Project(6, 5)
	sxY, syY := Project(5, 6)
	if syX-sy0 != HH || syY-sy0 != HH {
		t.Errorf("both axes should descend by HH: dxRow=%v dyRow=%v", syX-sy0, syY-sy0)
	}
	if sxX-sx0 != HW || sxY-sx0 != -HW {
		t.Errorf("axes should split horizontally: +x=%v +y=%v", sxX-sx0, sxY-sx0)
	}
}

func TestDepthOrder(t *testing.T) {
	if !(Depth(0, 0) < Depth(1, 0) && Depth(1, 0) == Depth(0, 1) && Depth(1, 1) > Depth(1, 0)) {
		t.Error("depth key should grow with gx+gy")
	}
}

func TestVisibleCellRangeContainsCenter(t *testing.T) {
	// Origin chosen so the world cell (50,25) is centered in a 320x160 view.
	csx, csy := Project(50, 25)
	originSx, originSy := csx-160, csy-80
	gx0, gy0, gx1, gy1 := VisibleCellRange(originSx, originSy, 320, 160, 2)
	if !(gx0 <= 50 && 50 <= gx1 && gy0 <= 25 && 25 <= gy1) {
		t.Errorf("center cell not inside visible range [%d,%d]-[%d,%d]", gx0, gy0, gx1, gy1)
	}
}
