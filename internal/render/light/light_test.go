package light

import (
	"testing"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

func TestApplyDimsGreyButRevealsUnderLight(t *testing.T) {
	c := canvas.New(20, 20)
	c.Clear(canvas.RGB(0x80, 0x80, 0x80))
	f := NewField()

	// One bright light at the center.
	f.Apply(c, []Light{{X: 10, Y: 10, Radius: 6, Power: 1.0}}, 0.5)

	center := c.At(10, 10)
	corner := c.At(0, 0)
	if center.Luma() <= corner.Luma() {
		t.Errorf("lit center (%d) should be brighter than dim corner (%d)", center.Luma(), corner.Luma())
	}
	// The corner is outside the light radius, so it sits at the ambient floor.
	if corner.Luma() >= 0x80 {
		t.Errorf("corner should be dimmed below the source tone, got %d", corner.Luma())
	}
}

func TestApplyLeavesColorPopsVivid(t *testing.T) {
	c := canvas.New(8, 8)
	pop := canvas.Hex("#FF5FAF")
	c.Clear(pop)
	NewField().Apply(c, nil, 0.3)
	if c.At(4, 4) != pop {
		t.Error("saturated color pop must not be dimmed by lighting")
	}
}

func TestGlowOnlyBrightens(t *testing.T) {
	c := canvas.New(20, 20)
	c.Clear(canvas.RGB(0x20, 0x20, 0x20))
	Glow(c, 10, 10, 8, canvas.RGB(255, 255, 255))
	if c.At(10, 10).Luma() <= 0x20 {
		t.Error("glow should brighten the core pixel")
	}
}
