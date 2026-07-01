package shellmon

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shellbound/shellbound/internal/render/canvas"
	mon "github.com/shellbound/shellbound/internal/shellmon"
)

// savePNG writes the canvas to dir/name.png.
func savePNG(t *testing.T, dir, name string, scr *canvas.Canvas) {
	img := image.NewRGBA(image.Rect(0, 0, scr.W, scr.H))
	for i, c := range scr.Pixels() {
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
		savePNG(t, dir, key, m.scr)
	}

	// The badge award animation and the party screen showing an earned badge.
	m := &model{
		scr:      canvas.New(pw, ph),
		roster:   []*mon.Creature{mon.NewCreature("cindle", 12), mon.NewCreature("dripling", 9)},
		defeated: map[string]bool{},
		found:    map[string]bool{},
		badges:   map[string]bool{"coral": true},
	}
	m.awardBadge, m.awardAt = "coral", time.Now().Add(-1300*time.Millisecond)
	m.scr.Clear(canvas.Black)
	m.drawBadgeAward(pw, ph, 0.6)
	savePNG(t, dir, "badge_award", m.scr)

	m.scr.Clear(canvas.Black)
	m.drawParty(pw, ph, 0.6)
	savePNG(t, dir, "party_badges", m.scr)
}
