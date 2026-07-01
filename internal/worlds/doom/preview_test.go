package doom

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// TestRenderDoomPreviews dumps a few frames when SHELLMON_PREVIEW_DIR is set, so
// the Doom visuals can be eyeballed without an SSH session. No-op in CI.
func TestRenderDoomPreviews(t *testing.T) {
	dir := os.Getenv("SHELLMON_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set SHELLMON_PREVIEW_DIR to render doom previews")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const pw, ph = 720, 420

	frame := func(name string, setup func(m *model)) {
		m := &model{key: "doom", g: newGame(), scr: canvas.New(pw, ph), termW: 100, termH: 40, cellW: 8, cellH: 16}
		setup(m)
		m.scr.Resize(pw, ph)
		m.scr.Clear(canvas.Black)
		m.drawWorld(pw, ph, 0.7)
		m.drawSprites(pw, ph, 0.7)
		m.drawWeapon(pw, ph, 0.7)
		m.drawHUD(pw, ph)
		m.drawBanner(pw, ph)

		img := image.NewRGBA(image.Rect(0, 0, pw, ph))
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

	// Facing an imp down the hall, gun idle.
	frame("doom_scene", func(m *model) {
		m.g.posX, m.g.posY = 9.5, 7.5
		m.g.dirX, m.g.dirY = 1, 0
		m.g.planeX, m.g.planeY = 0, fov
	})
	// Firing: muzzle flash + one imp mid-dissolve.
	frame("doom_fire", func(m *model) {
		m.g.posX, m.g.posY = 9.5, 7.5
		m.g.dirX, m.g.dirY = 1, 0
		m.g.planeX, m.g.planeY = 0, fov
		m.g.muzzle = muzzleTicks
		m.g.steps = 3
		m.g.enemies[0].alive = false
		m.g.enemies[0].dying = deathTicks / 2
	})
	// Facing a wall torch.
	frame("doom_torch", func(m *model) {
		m.g.posX, m.g.posY = 3.5, 12.5
		m.g.dirX, m.g.dirY = -1, 0
		m.g.planeX, m.g.planeY = 0, -fov
	})
}
