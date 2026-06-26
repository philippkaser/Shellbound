package shellmon

import (
	"time"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// transition is a short screen wipe played when a Shellmon screen changes
// (entering the world, an encounter starting, returning to the route). It
// reveals the new screen through a growing 2:1 diamond with an opening flash —
// the same isometric motif as the world itself.
type transition struct {
	start time.Time
	dur   float64
}

// begin (re)starts the wipe.
func (tr *transition) begin(dur float64) {
	tr.start = time.Now()
	tr.dur = dur
}

// overlay draws the wipe over the finished frame; it's inert once complete.
func (tr *transition) overlay(c *canvas.Canvas, pw, ph int) {
	if tr.start.IsZero() {
		return
	}
	el := time.Since(tr.start).Seconds()
	if el >= tr.dur || tr.dur <= 0 {
		return
	}
	p := el / tr.dur

	// Opening flash.
	if p < 0.14 {
		k := (0.14 - p) / 0.14 * 0.7
		wash := canvas.RGB(220, 220, 230).Scale(k)
		for y := 0; y < ph; y++ {
			for x := 0; x < pw; x++ {
				c.Set(x, y, c.At(x, y).Lighten(wash))
			}
		}
	}

	// Diamond reveal: black outside a 2:1 diamond growing from the center.
	cx, cy := pw/2, ph/2
	maxR := pw/2 + ph + 8
	r := int(float64(maxR) * p)
	for y := 0; y < ph; y++ {
		dy2 := iabs(y-cy) * 2
		for x := 0; x < pw; x++ {
			if iabs(x-cx)+dy2 > r {
				c.Set(x, y, canvas.Black)
			}
		}
	}
}

func iabs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
