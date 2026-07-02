// Package iso projects Shellbound's grid world into a 2:1 isometric screen
// space and draws the primitive volumes the plaza is built from (ground
// diamonds and extruded cubes). Game logic — movement, collision, the
// multiplayer protocol — stays in plain grid coordinates; only rendering is
// isometric, so this package is pure render-space math plus a few canvas
// drawing helpers.
package iso

import "github.com/shellbound/shellbound/internal/render/canvas"

// Tile dimensions in pixels. A 2:1 diamond (TileW == 2*TileH) is the classic
// isometric footprint. Half-extents are the common case in the math below.
// Larger tiles pull the camera closer so avatars read clearly.
const (
	TileW = 48
	TileH = 24
	HW    = TileW / 2
	HH    = TileH / 2
)

// Project maps grid (gx, gy) — which may be fractional — to the top vertex of
// that cell's ground diamond in screen space.
func Project(gx, gy float64) (sx, sy float64) {
	return (gx - gy) * HW, (gx + gy) * HH
}

// Unproject is the inverse of Project.
func Unproject(sx, sy float64) (gx, gy float64) {
	gx = sx/TileW + sy/TileH
	gy = sy/TileH - sx/TileW
	return
}

// Depth is the painter's-algorithm key for a cell: larger means nearer the
// camera, so draw in ascending order for correct overlap.
func Depth(gx, gy int) int { return gx + gy }

// VisibleCellRange returns the inclusive grid rectangle that can project into a
// canvasW×canvasH viewport whose top-left sits at screen-space (originSx,
// originSy). marginCells pads the rectangle so tall cubes near the edges (and
// objects whose bases sit just off-screen) are not clipped early. Callers
// clamp the result to the map bounds.
func VisibleCellRange(originSx, originSy float64, canvasW, canvasH, marginCells int) (gx0, gy0, gx1, gy1 int) {
	corners := [4][2]float64{
		{originSx, originSy},
		{originSx + float64(canvasW), originSy},
		{originSx, originSy + float64(canvasH)},
		{originSx + float64(canvasW), originSy + float64(canvasH)},
	}
	first := true
	var minGX, minGY, maxGX, maxGY float64
	for _, c := range corners {
		gx, gy := Unproject(c[0], c[1])
		if first {
			minGX, maxGX, minGY, maxGY = gx, gx, gy, gy
			first = false
			continue
		}
		minGX, maxGX = minf(minGX, gx), maxf(maxGX, gx)
		minGY, maxGY = minf(minGY, gy), maxf(maxGY, gy)
	}
	return int(minGX) - marginCells, int(minGY) - marginCells,
		int(maxGX) + marginCells, int(maxGY) + marginCells
}

// rowHalf returns the half-width of the tile diamond at row dy in [0, TileH)
// (linear taper to the waist).
func rowHalf(dy int) int {
	if dy <= HH {
		return dy * HW / HH
	}
	return (TileH - dy) * HW / HH
}

// DrawDiamond fills the ground diamond whose top vertex is at screen pixel
// (cx, cy), with an optional edge color (pass fill == edge for no outline).
func DrawDiamond(c *canvas.Canvas, cx, cy int, fill, edge canvas.Color) {
	for dy := 0; dy < TileH; dy++ {
		half := rowHalf(dy)
		y := cy + dy
		if edge != fill {
			c.Set(cx-half, y, edge)
			c.Set(cx+half, y, edge)
			if half > 0 {
				c.HSpan(cx-half+1, cx+half-1, y, fill)
			}
			continue
		}
		c.HSpan(cx-half, cx+half, y, fill)
	}
}

// DrawDiamondTextured fills the ground diamond, choosing each pixel's color
// via tex(dx, dy) where dx is the signed offset from the center column and dy
// the row within [0, TileH). It is the hook for per-tile grain — cobble
// speckle, grass tufts, sand ripples — without every caller re-deriving the
// diamond's row geometry.
func DrawDiamondTextured(c *canvas.Canvas, cx, cy int, tex func(dx, dy int) canvas.Color) {
	for dy := 0; dy < TileH; dy++ {
		half := rowHalf(dy)
		y := cy + dy
		for dx := -half; dx <= half; dx++ {
			c.Set(cx+dx, y, tex(dx, dy))
		}
	}
}

// DrawCube draws an extruded tile of pixel height h whose ground diamond top
// vertex is at (cx, cy): two shaded side faces and a lit top diamond. Heights
// of 0 degenerate to a flat top diamond.
func DrawCube(c *canvas.Canvas, cx, cy, h int, top, left, right canvas.Color) {
	if h < 0 {
		h = 0
	}
	// Left face: columns from the left vertex to the bottom vertex; each
	// column is a vertical bar of height h hanging off the lower-left edge.
	for dx := -HW; dx <= 0; dx++ {
		yEdge := cy + HH + (dx+HW)/2 // lower-left edge of the ground diamond
		c.VLine(cx+dx, yEdge-h, yEdge, left)
	}
	// Right face: from the bottom vertex to the right vertex.
	for dx := 1; dx <= HW; dx++ {
		yEdge := cy + TileH - dx/2 // lower-right edge
		c.VLine(cx+dx, yEdge-h, yEdge, right)
	}
	// Top face sits h pixels above the ground diamond.
	DrawDiamond(c, cx, cy-h, top, top)
}

// DrawCubeShaded draws an extruded tile like DrawCube with two extra depth
// cues that make volumes sit in the scene instead of floating: the side faces
// darken toward the ground (a cheap ambient-occlusion gradient), and a thin
// darker seam runs along the base of each face where it meets the floor.
func DrawCubeShaded(c *canvas.Canvas, cx, cy, h int, top, left, right canvas.Color) {
	if h <= 0 {
		DrawDiamond(c, cx, cy, top, top)
		return
	}
	shade := func(base canvas.Color, y, yTop, yEdge int) canvas.Color {
		if y == yEdge {
			return base.Scale(0.55) // contact seam
		}
		if h <= 2 {
			return base
		}
		// Darken the lower third toward the ground.
		v := float64(y-yTop) / float64(h)
		if v > 0.66 {
			return base.Scale(1 - 0.28*(v-0.66)/0.34)
		}
		return base
	}
	for dx := -HW; dx <= 0; dx++ {
		yEdge := cy + HH + (dx+HW)/2
		for y := yEdge - h; y <= yEdge; y++ {
			c.Set(cx+dx, y, shade(left, y, yEdge-h, yEdge))
		}
	}
	for dx := 1; dx <= HW; dx++ {
		yEdge := cy + TileH - dx/2
		for y := yEdge - h; y <= yEdge; y++ {
			c.Set(cx+dx, y, shade(right, y, yEdge-h, yEdge))
		}
	}
	DrawDiamond(c, cx, cy-h, top, top)
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
