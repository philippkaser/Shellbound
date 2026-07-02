package overworld

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shellbound/shellbound/internal/hub"
	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/ui/listpanel"
)

// TestRenderPlazaPreview dumps PNG frames of the plaza when
// PLAZA_PREVIEW_DIR is set, so the shared space can be eyeballed without an
// SSH session. It is a no-op in normal CI runs.
func TestRenderPlazaPreview(t *testing.T) {
	dir := os.Getenv("PLAZA_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PLAZA_PREVIEW_DIR to render plaza previews")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	world := plaza.Load()
	pal := canvas.DefaultPalette(plaza.PaletteAccents())
	r := NewRenderer(pal, nil, world)

	players := []playerSnapshot{
		{id: 1, name: "ada", color: "#FF5FAF", x: world.SpawnX, y: world.SpawnY * 2, dir: hub.DirDown, cosmetic: "tophat"},
		{id: 2, name: "bob", color: "#60A5FA", x: world.SpawnX - 4, y: (world.SpawnY - 2) * 2, dir: hub.DirRight, moving: true},
		{id: 3, name: "cyd", color: "#4ADE80", x: world.SpawnX + 5, y: (world.SpawnY + 1) * 2, dir: hub.DirLeft, emote: "wave"},
	}
	snap := frameSnapshot{
		termW: 100, termH: 32, cellW: 8, cellH: 16,
		players: players, selfID: 1, coins: 42,
	}
	r.Submit(snap)
	r.snapPNG(t, dir, "plaza_spawn", snap, 0.7)

	// A modal panel with a selection bar over the dimmed plaza.
	snap.panel = listpanel.Content{
		Title: "Shop  ✦42",
		Lines: []string{
			"* Beanie          Common    owned",
			"  Bow             Common    30✦",
			"- Wizard Hat      Epic      160✦",
		},
		Cursor: 1,
		Footer: "up/down select  Enter buy  Esc close",
	}
	r.Submit(snap)
	r.snapPNG(t, dir, "plaza_panel", snap, 0.7)
	snap.panel = listpanel.Content{Cursor: -1}

	// The fountain, centered, to check the pool's symmetry around the statue.
	for i := range players {
		players[i].x = 38 + (i-1)*4
		players[i].y = (19 + 4) * 2
	}
	snap.players = players
	r.Submit(snap)
	r.snapPNG(t, dir, "plaza_fountain", snap, 1.2)

	// A second shot near a portal for the gateway/glow art.
	if len(plaza.Portals) > 0 {
		px, py := plaza.Portals[0].Center()
		for i := range players {
			players[i].x = int(px) + (i-1)*3
			players[i].y = int(py*2) + i
		}
		snap.players = players
		r.Submit(snap)
		r.snapPNG(t, dir, "plaza_portal", snap, 1.9)
	}
}

// snapPNG builds one frame at world time tt and saves it.
func (r *Renderer) snapPNG(t *testing.T, dir, name string, snap frameSnapshot, tt float64) {
	r.primed = false
	r.build(snap, 1.0/renderFPS, tt, time.Now())
	img := image.NewRGBA(image.Rect(0, 0, r.screen.W, r.screen.H))
	for i, c := range r.screen.Pixels() {
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
