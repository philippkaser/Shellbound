package shimmer

import "testing"

func TestFieldDeterministicAndAnimated(t *testing.T) {
	f := NewField()
	a := f.At(3, 2, 1.0)
	b := f.At(3, 2, 1.0)
	if a != b {
		t.Error("Field.At not deterministic for same inputs")
	}
	// Over a half rotation the hue must change the color.
	c := f.At(3, 2, 4.0)
	if a == c {
		t.Error("Field.At should animate over time")
	}
}

func TestFieldCacheConsistent(t *testing.T) {
	f := NewField()
	// Same effective hue from different coordinates must give same color.
	a := f.At(0, 0, 0)  // hue 0
	b := f.At(45, 0, 0) // 45*8 = 360 -> hue 0
	if a != b {
		t.Errorf("equivalent hues differ: %06X vs %06X", uint32(a), uint32(b))
	}
}
