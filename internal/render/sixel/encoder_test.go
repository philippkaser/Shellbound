package sixel

import (
	"strings"
	"testing"
)

func TestEncoderReuseIsDeterministic(t *testing.T) {
	// The same Encoder must produce identical output across calls (its band
	// scratch must be fully reset between frames), including after encoding a
	// differently-sized frame in between.
	pal := []RGB{{0, 0, 0}, {255, 255, 255}, {255, 0, 0}}
	pixA := []byte{0, 1, 2, 1, 0, 2, 2, 1, 0, 0, 1, 2}
	pixB := []byte{2, 2, 1, 1, 0, 0}

	var e Encoder
	var first, mid, again strings.Builder
	e.Encode(&first, pixA, 4, 3, pal)
	e.Encode(&mid, pixB, 3, 2, pal)
	e.Encode(&again, pixA, 4, 3, pal)

	if first.String() != again.String() {
		t.Fatalf("encoder reuse changed output:\n first %q\n again %q", first.String(), again.String())
	}
	var oneShot strings.Builder
	Encode(&oneShot, pixA, 4, 3, pal)
	if first.String() != oneShot.String() {
		t.Fatalf("stateful and one-shot encoders disagree:\n stateful %q\n one-shot %q", first.String(), oneShot.String())
	}
}

func TestTrailingBlanksTrimmed(t *testing.T) {
	// Color 1 appears only in the left half; its plane must not carry '?'
	// padding out to the full width.
	pix := []byte{1, 1, 0, 0, 0, 0, 0, 0}
	pal := []RGB{{0, 0, 0}, {255, 255, 255}}
	var sb strings.Builder
	Encode(&sb, pix, 8, 1, pal)
	out := sb.String()

	// Color 1's data is exactly two '@' columns, no trailing '?' filler.
	if !strings.Contains(out, "#1@@") {
		t.Fatalf("expected color 1 plane %q in %q", "#1@@", out)
	}
	if strings.Contains(out, "@?") {
		t.Fatalf("expected trailing blanks to be trimmed, got %q", out)
	}
}
