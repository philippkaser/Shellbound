package shellmon

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/shellbound/shellbound/internal/render/canvas"
	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// TestRenderAreaPreviews dumps a PNG of each area when SHELLMON_PREVIEW_DIR is
// set, so the overworld can be eyeballed without an SSH session. It is a no-op
// in normal CI runs.
func TestRenderAreaPreviews(t *testing.T) {
	dir := os.Getenv("SHELLMON_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set SHELLMON_PREVIEW_DIR to render area previews")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const pw, ph = 720, 420
	for _, key := range allAreas {
		m := &model{
			scr:      canvas.New(pw, ph),
			roster:   []*mon.Creature{mon.NewCreature("cindle", 5)},
			defeated: map[string]bool{},
			found:    map[string]bool{},
		}
		m.enterArea(key, -1, -1)
		m.route.px, m.route.py = m.route.w/2, m.route.h/2 // center the camera for the shot
		m.scr.Clear(canvas.Black)
		m.drawRoute(pw, ph, 0.6)

		img := image.NewRGBA(image.Rect(0, 0, pw, ph))
		px := m.scr.Pixels()
		for i, c := range px {
			img.Pix[i*4+0] = c.R()
			img.Pix[i*4+1] = c.G()
			img.Pix[i*4+2] = c.B()
			img.Pix[i*4+3] = 0xFF
		}

		f, err := os.Create(filepath.Join(dir, key+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			t.Fatal(err)
		}
		f.Close()
	}
}
