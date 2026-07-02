package overworld

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
)

// TestRenderPlazaPreview dumps a plaza frame (with a center crosshair) when
// SHELLMON_PREVIEW_DIR is set, to eyeball where the local player sits.
func TestRenderPlazaPreview(t *testing.T) {
	dir := os.Getenv("SHELLMON_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set SHELLMON_PREVIEW_DIR to render the plaza preview")
	}
	const pw, ph = 720, 420
	world := plaza.Load()
	r := NewRenderer(nil, nil, world)
	r.screen = canvas.New(pw, ph)
	r.screen.Clear(canvas.Black)

	snap := frameSnapshot{
		termW: 100, termH: 40, cellW: 8, cellH: 16, selfID: 1,
		players: []playerSnapshot{{id: 1, name: "you", color: "#ffffff", x: world.SpawnX, y: world.SpawnY * 2}},
	}
	r.updateEntities(snap)
	r.interpolate(1)
	self := r.ents[snap.selfID]
	r.camX, r.camY = self.fx, self.fy

	r.screen.Clear(canvas.Black)
	csx, csy := iso.Project(r.camX, r.camY)
	// Same framing as the live renderer: focus on the tile centre.
	originSx := csx - float64(pw)/2
	originSy := csy + float64(iso.HH) - float64(ph)/2
	r.world.RenderIso(r.screen, originSx, originSy, 0.5)
	for _, p := range plaza.Portals {
		p.RenderIso(r.screen, 0.5, originSx, originSy)
	}
	r.drawPlayers(originSx, originSy, 0.5)
	mark := canvas.RGB(0xFF, 0x30, 0x30) // center crosshair
	r.screen.HLine(pw/2-16, pw/2+16, ph/2, mark)
	r.screen.VLine(pw/2, ph/2-16, ph/2+16, mark)

	img := image.NewRGBA(image.Rect(0, 0, pw, ph))
	for i, c := range r.screen.Pixels() {
		img.Pix[i*4+0] = c.R()
		img.Pix[i*4+1] = c.G()
		img.Pix[i*4+2] = c.B()
		img.Pix[i*4+3] = 0xFF
	}
	f, err := os.Create(filepath.Join(dir, "plaza_center.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
