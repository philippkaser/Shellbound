package shellmon

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// TestRenderSpriteSheet dumps a labeled grid of every species when
// SHELLMON_PREVIEW_DIR is set, so the creature designs can be eyeballed and
// iterated without an SSH session. No-op in CI.
func TestRenderSpriteSheet(t *testing.T) {
	dir := os.Getenv("SHELLMON_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set SHELLMON_PREVIEW_DIR to render the sprite sheet")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	const (
		u     = 4   // battle "you" scale
		cellW = 130 // grid cell size in px
		cellH = 140
		cols  = 6
	)
	rows := (len(catalog) + cols - 1) / cols
	c := canvas.New(cols*cellW, rows*cellH)
	c.Clear(canvas.RGB(10, 10, 10))

	for i, sp := range catalog {
		gx, gy := i%cols, i/cols
		cx := gx*cellW + cellW/2
		cy := gy*cellH + cellH/2 - 8
		// A faint ground pad grounds each figure like the battle platform.
		c.FillEllipse(cx, cy+11*u-4, 44, 12, canvas.RGB(30, 30, 30))
		DrawCreature(c, cx, cy, u, sp.Key)
		label := sp.Name + " [" + sp.Type.String() + "]"
		c.DrawText(cx-canvas.TextWidth(label)/2, gy*cellH+cellH-12, label, canvas.RGB(160, 160, 160))
	}

	img := image.NewRGBA(image.Rect(0, 0, c.W, c.H))
	for i, col := range c.Pixels() {
		img.Pix[i*4+0] = col.R()
		img.Pix[i*4+1] = col.G()
		img.Pix[i*4+2] = col.B()
		img.Pix[i*4+3] = 0xFF
	}
	f, err := os.Create(filepath.Join(dir, "spritesheet.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
