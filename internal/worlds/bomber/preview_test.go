package bomber

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/light"
)

// TestRenderBomberPreviews dumps frames when SHELLMON_PREVIEW_DIR is set, so
// the Vault can be eyeballed without an SSH session. No-op in CI.
func TestRenderBomberPreviews(t *testing.T) {
	dir := os.Getenv("SHELLMON_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set SHELLMON_PREVIEW_DIR to render bomber previews")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const pw, ph = 720, 420

	frame := func(name string, setup func(m *model)) {
		m := &model{
			key: "bomberman", g: newGame(7), scr: canvas.New(pw, ph),
			sb: &strings.Builder{}, lights: light.NewField(),
			pal:   canvas.DefaultPalette(plaza.PaletteAccents()),
			termW: 100, termH: 40, cellW: 8, cellH: 16,
		}
		setup(m)
		if out := m.build(0.7, 0); out == "" {
			t.Fatalf("%s: empty frame", name)
		}
		img := image.NewRGBA(image.Rect(0, 0, m.scr.W, m.scr.H))
		for i, c := range m.scr.Pixels() {
			img.Pix[i*4+0] = c.R()
			img.Pix[i*4+1] = c.G()
			img.Pix[i*4+2] = c.B()
			img.Pix[i*4+3] = 0xFF
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}

	// Mid-game: a ticking bomb and the wisps roaming.
	frame("bomber_scene", func(m *model) {
		m.g.plant()
	})
	// A detonation lighting the arena (with camera shake active).
	frame("bomber_blast", func(m *model) {
		m.g.stampBlast(3, 1)
		m.g.stampBlast(2, 1)
		m.g.stampBlast(4, 1)
		m.g.stampBlast(3, 2)
		m.g.shake = shakeTicks
	})
}
