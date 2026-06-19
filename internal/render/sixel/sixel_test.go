package sixel

import (
	"strings"
	"testing"
)

func encode(pix []byte, w, h int, pal []RGB) string {
	var sb strings.Builder
	Encode(&sb, pix, w, h, pal)
	return sb.String()
}

func TestEncodeTwoByTwo(t *testing.T) {
	// row0 = black,white ; row1 = white,black
	pix := []byte{0, 1, 1, 0}
	pal := []RGB{{0, 0, 0}, {255, 255, 255}}

	got := encode(pix, 2, 2, pal)

	// Hand-computed: introducer, raster 2x2, two color regs (in percent),
	// one band carrying both colors, ST.
	want := "\x1bP0;1;0q" +
		"\"1;1;2;2" +
		"#0;2;0;0;0" +
		"#1;2;100;100;100" +
		"#0@A" + // black plane: col0 row0 (bit0=1='@'), col1 row1 (bit1=2='A')
		"$#1A@" + // white plane overlays: col0 row1 (2='A'), col1 row0 (1='@')
		"-" +
		"\x1b\\"
	if got != want {
		t.Fatalf("encode mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestEncodeRLE(t *testing.T) {
	// A single 6-tall, 5-wide band fully one color exercises `!count`.
	w, h := 5, 1
	pix := make([]byte, w*h) // all index 1
	for i := range pix {
		pix[i] = 1
	}
	pal := []RGB{{0, 0, 0}, {255, 255, 255}}

	got := encode(pix, w, h, pal)

	// Band has only color 1; each column value = bit0 (1) => char '@'.
	// Run of 5 >= 4, so it compresses to "!5@".
	if !strings.Contains(got, "#1!5@") {
		t.Fatalf("expected run-length %q in output, got %q", "#1!5@", got)
	}
	// Color 0 is absent from the band and must not be emitted as pixel data.
	if strings.Contains(got, "#0!") || strings.Contains(got, "#0?") {
		t.Fatalf("did not expect color 0 pixel data, got %q", got)
	}
}

func TestEncodeRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		pix  []byte
		w, h int
		pal  []RGB
	}{
		{"zero size", []byte{0}, 0, 0, []RGB{{0, 0, 0}}},
		{"short buffer", []byte{0}, 2, 2, []RGB{{0, 0, 0}}},
		{"empty palette", []byte{0, 0, 0, 0}, 2, 2, nil},
	}
	for _, c := range cases {
		if out := encode(c.pix, c.w, c.h, c.pal); out != "" {
			t.Errorf("%s: expected empty output, got %q", c.name, out)
		}
	}
}
