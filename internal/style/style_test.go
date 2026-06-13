package style

import (
	"strings"
	"testing"
)

func TestPlayerColorDeterministic(t *testing.T) {
	fp := "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	a := PlayerColor(fp)
	b := PlayerColor(fp)
	if a != b {
		t.Fatalf("PlayerColor not deterministic: %q vs %q", a, b)
	}
}

func TestPlayerColorInPalette(t *testing.T) {
	fps := []string{
		"SHA256:one",
		"SHA256:two",
		"SHA256:three",
		"", // degenerate input must not panic and must still map into the palette
		"SHA256:Q0aXl+0Zz/q0Yl1Yl1Yl1Yl1Yl1Yl1Yl1Yl1Yl1Yl1Y",
	}
	for _, fp := range fps {
		c := PlayerColor(fp)
		found := false
		for _, p := range Palette {
			if p == c {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("PlayerColor(%q) = %q, not in Palette", fp, c)
		}
	}
}

func TestPaletteWellFormed(t *testing.T) {
	if len(Palette) < 20 {
		t.Fatalf("palette too small: %d", len(Palette))
	}
	seen := map[string]bool{}
	for _, p := range Palette {
		if !strings.HasPrefix(p, "#") || len(p) != 7 {
			t.Errorf("palette entry %q is not a #RRGGBB hex color", p)
		}
		if seen[p] {
			t.Errorf("duplicate palette entry %q", p)
		}
		seen[p] = true
	}
}

func TestDifferentFingerprintsCanDiffer(t *testing.T) {
	// Not guaranteed for any two inputs, but across many inputs we must see
	// more than one distinct color, otherwise the hash is broken.
	colors := map[string]bool{}
	inputs := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	for _, in := range inputs {
		colors[PlayerColor("SHA256:"+in)] = true
	}
	if len(colors) < 2 {
		t.Fatalf("expected variety across inputs, got %d distinct color(s)", len(colors))
	}
}
