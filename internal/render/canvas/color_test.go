package canvas

import "testing"

func TestHSLPrimaries(t *testing.T) {
	cases := []struct {
		h, s, l float64
		want    Color
	}{
		{0, 1, 0.5, 0xFF0000},   // red
		{120, 1, 0.5, 0x00FF00}, // green
		{240, 1, 0.5, 0x0000FF}, // blue
		{60, 1, 0.5, 0xFFFF00},  // yellow
		{180, 1, 0.5, 0x00FFFF}, // cyan
		{300, 1, 0.5, 0xFF00FF}, // magenta
		{0, 0, 1, 0xFFFFFF},     // white
		{0, 0, 0, 0x000000},     // black
		{123, 0, 0.5, 0x808080}, // grey regardless of hue
	}
	for _, tc := range cases {
		if got := HSL(tc.h, tc.s, tc.l); got != tc.want {
			t.Errorf("HSL(%v,%v,%v) = %06X, want %06X", tc.h, tc.s, tc.l, uint32(got), uint32(tc.want))
		}
	}
}

func TestHSLWrapsHue(t *testing.T) {
	if HSL(360, 1, 0.5) != HSL(0, 1, 0.5) {
		t.Error("hue 360 should equal hue 0")
	}
	if HSL(-120, 1, 0.5) != HSL(240, 1, 0.5) {
		t.Error("hue -120 should equal hue 240")
	}
	if HSL(720+45, 1, 0.5) != HSL(45, 1, 0.5) {
		t.Error("hue should wrap modulo 360")
	}
}

func TestHSLClampsSatLight(t *testing.T) {
	if HSL(0, 2, 0.5) != HSL(0, 1, 0.5) {
		t.Error("saturation should clamp to 1")
	}
	if HSL(0, 1, -1) != HSL(0, 1, 0) {
		t.Error("lightness should clamp to 0")
	}
}

func TestColorGrayAndScale(t *testing.T) {
	if !RGB(0x40, 0x40, 0x40).IsGray() {
		t.Error("equal channels should be grey")
	}
	if Hex("#FF5FAF").IsGray() {
		t.Error("saturated color should not be grey")
	}
	// Scaling a grey stays grey (keeps lighting on the quantizer fast path).
	if g := RGB(200, 200, 200).Scale(0.5); !g.IsGray() {
		t.Errorf("scaled grey is no longer grey: %06X", uint32(g))
	}
}
