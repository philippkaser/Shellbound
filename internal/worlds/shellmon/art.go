package shellmon

// This file holds the Shellmon overworld drawing helpers — pure canvas art
// split out of route.go, which keeps the world state and movement logic.

import (
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/iso"
	"github.com/shellbound/shellbound/internal/render/sprites"
)

func drawNameTag(c *canvas.Canvas, footX, footY int, name string, col canvas.Color) {
	w := canvas.TextWidth(name)
	c.DrawTextShadow(footX-w/2, footY-sprites.Height-canvas.LineH, name, col, 0x000000)
}

func groundGrass(c *canvas.Canvas, px, py, x, y int) {
	fill := routeGrass
	if (x+y)&1 == 0 {
		fill = routeGrasB
	}
	iso.DrawDiamond(c, px, py, fill, routeEdge)
}

// groundLawn paints manicured short grass: a flat green diamond with a light
// speckle but no tall swaying blades, so towns don't read as wild-encounter
// grass. Also used as the natural floor under trees, rocks and ledges.
func groundLawn(c *canvas.Canvas, px, py, x, y int) {
	fill := canvas.Color(0x1B1F1B)
	if (x+y)&1 == 0 {
		fill = canvas.Color(0x202420)
	}
	iso.DrawDiamond(c, px, py, fill, routeEdge)
	cx, cy := px, py+iso.HH
	seed := x*7 + y*13
	for i := 0; i < 4; i++ {
		bx := cx - 12 + ((seed + i*7) % 24)
		by := cy - 5 + ((seed + i*5) % 9)
		c.Set(bx, by, canvas.Color(0x2C302C))
	}
}

// groundSand paints a pale, grainy shore tile.
func groundSand(c *canvas.Canvas, px, py, x, y int) {
	fill := canvas.Color(0x2B2A26)
	if (x+y)&1 == 0 {
		fill = canvas.Color(0x33322C)
	}
	iso.DrawDiamond(c, px, py, fill, canvas.Color(0x1A1914))
	cx, cy := px, py+iso.HH
	seed := x*5 + y*11
	for i := 0; i < 3; i++ {
		gx := cx - 11 + ((seed + i*9) % 22)
		gy := cy - 4 + ((seed + i*6) % 8)
		c.Set(gx, gy, canvas.Color(0x3E3C34))
	}
}

// drawPierTile paints a wooden plank walkway sitting over water.
func drawPierTile(c *canvas.Canvas, px, py, x, y int) {
	iso.DrawDiamond(c, px, py, canvas.Color(0x141420), canvas.Color(0x0E0E16)) // water at the edges
	wood := canvas.Color(0x3A322A)
	if (x+y)&1 == 0 {
		wood = canvas.Color(0x453B30)
	}
	cx, cy := px, py+iso.HH
	for dy := -6; dy <= 6; dy++ {
		half := (iso.HW - 5) * (6 - absi(dy)) / 6
		for dx := -half; dx <= half; dx++ {
			c.Set(cx+dx, cy+dy, wood)
		}
	}
	c.HLine(cx-11, cx+11, cy-2, canvas.Color(0x2A241E)) // plank seams
	c.HLine(cx-11, cx+11, cy+2, canvas.Color(0x2A241E))
}

// drawWell paints a little stone wishing-well: a round rim over water with a
// shingled roof on two posts — Oakhaven's town-square centerpiece.
func drawWell(c *canvas.Canvas, footX, footY int) {
	drawContactShadow(c, footX, footY)
	for dy := -4; dy <= 3; dy++ { // stone rim
		w := int(11 * sqrtClamp(1-float64(dy*dy)/18.0))
		for dx := -w; dx <= w; dx++ {
			tone := canvas.Color(0x9A9A9A)
			if dx < -w/3 {
				tone = canvas.Color(0x686868)
			}
			c.Set(footX+dx, footY-5+dy, tone)
		}
	}
	for dy := -3; dy <= 1; dy++ { // water surface
		w := int(7 * sqrtClamp(1-float64(dy*dy)/10.0))
		for dx := -w; dx <= w; dx++ {
			c.Set(footX+dx, footY-6+dy, canvas.Color(0x39394A))
		}
	}
	c.Set(footX-2, footY-7, canvas.Color(0xC6C6E0)) // glint
	c.VLine(footX-9, footY-22, footY-7, canvas.Color(0x4A3F30))
	c.VLine(footX+9, footY-22, footY-7, canvas.Color(0x4A3F30))
	fillTriangle(c, [2]int{footX - 12, footY - 21}, [2]int{footX + 12, footY - 21}, [2]int{footX, footY - 31}, canvas.Color(0x8A6A4A))
	drawRidge(c, [2]int{footX, footY - 31}, [2]int{footX - 12, footY - 21}, canvas.Color(0xB09070))
}

// drawLighthouse paints a tall banded tower with a lit lantern room and a faint
// sweeping beam — Tidewell's seaside landmark.
func drawLighthouse(c *canvas.Canvas, footX, footY int, t float64) {
	drawContactShadow(c, footX, footY)
	const H = 54
	for y := 0; y <= H; y++ {
		f := float64(y) / float64(H)
		half := int(11 - 5*f) // taper toward the top
		tone := canvas.Color(0xCACACA)
		if (y/10)%2 == 0 {
			tone = canvas.Color(0x8E8E8E) // painted banding
		}
		for dx := -half; dx <= half; dx++ {
			col := tone
			if dx < -half/3 {
				col = col.Scale(0.7)
			}
			c.Set(footX+dx, footY-y, col)
		}
	}
	top := footY - H
	c.FillRect(footX-5, top-9, 11, 9, canvas.Color(0x363636)) // lantern room frame
	c.FillRect(footX-3, top-7, 7, 5, canvas.Color(0xF6F4CC))  // the light
	fillTriangle(c, [2]int{footX - 7, top - 9}, [2]int{footX + 7, top - 9}, [2]int{footX, top - 17}, canvas.Color(0x656565))
	bx := int(16 * sinf(t*1.1)) // slow sweeping beam
	c.HLine(footX, footX+bx, top-5, canvas.Color(0x585848))
}

// drawFence paints a low post-and-rail fence segment.
func drawFence(c *canvas.Canvas, footX, footY int) {
	c.VLine(footX-9, footY-11, footY-1, canvas.Color(0x5A4A38))
	c.VLine(footX+9, footY-11, footY-1, canvas.Color(0x5A4A38))
	c.HLine(footX-9, footX+9, footY-10, canvas.Color(0x6C5A44))
	c.HLine(footX-9, footX+9, footY-5, canvas.Color(0x6C5A44))
}

// drawLedge paints a low earthen ledge (a single-tile drop you can hop down).
func drawLedge(c *canvas.Canvas, px, py int) {
	iso.DrawCube(c, px, py, 7, canvas.Color(0x3A342A), canvas.Color(0x241F18), canvas.Color(0x2E2820))
	// A bright lip along the top-front edges reads as the drop.
	c.HLine(px-iso.HW+2, px-1, py+iso.HH-7, canvas.Color(0x5C5242))
	c.HLine(px+1, px+iso.HW-2, py+iso.HH-7, canvas.Color(0x5C5242))
}

// --- isometric route art (monochrome, top-lit like the plaza) ---

// drawTallGrass paints a lush, swaying clump of blades on a grass tile — taller
// and denser than mere texture, so "wild grass" reads as somewhere to search.
func drawTallGrass(c *canvas.Canvas, cx, cy, gx, gy int, t float64) {
	seed := gx*7 + gy*13
	for i := 0; i < 9; i++ {
		bx := cx - 16 + ((seed + i*9) % 32)
		base := cy - 2 + ((seed + i*5) % 7)
		hgt := 7 + (seed+i*3)%5
		sway := sinf(t*1.7 + float64(seed+i)*0.6)
		tone := canvas.Color(0x444444)
		if i%3 == 0 {
			tone = canvas.Color(0x303030)
		}
		// a curved blade: each segment leans a touch further with the wind
		for s := 0; s <= hgt; s++ {
			x := bx + int(sway*float64(s)*0.35)
			c.Set(x, base-s, tone)
		}
		c.Set(bx+int(sway*float64(hgt)*0.4), base-hgt-1, canvas.Color(0x5A5A5A)) // tip catch-light
	}
}

// drawFlowers paints a small cluster of blossoms on a decorative tile.
func drawFlowers(c *canvas.Canvas, cx, cy, gx, gy int, t float64) {
	seed := gx*5 + gy*11
	for i := 0; i < 3; i++ {
		fx := cx - 10 + ((seed + i*13) % 20)
		fy := cy - 2 + ((seed + i*7) % 6)
		c.VLine(fx, fy, fy+4, canvas.Color(0x3A3A3A)) // stem
		// four petals around a bright center
		c.Set(fx, fy-2, canvas.Color(0xCFCFCF))
		c.Set(fx-1, fy-1, canvas.Color(0x9A9A9A))
		c.Set(fx+1, fy-1, canvas.Color(0x9A9A9A))
		c.Set(fx, fy, canvas.Color(0xF2F2F2))
		c.Set(fx, fy-1, canvas.Color(0xEDEDED))
	}
}

// drawRock paints a small top-lit boulder sitting on the ground point.
func drawRock(c *canvas.Canvas, baseX, baseY int) {
	for dy := -10; dy <= 0; dy++ {
		w := int(float64(12) * sqrtClamp(1-float64(dy*dy)/float64(11*11)))
		v := float64(dy+10) / 12
		tone := canvas.Color(0x8A8A8A)
		switch {
		case v < 0.3:
			tone = canvas.Color(0xAEAEAE)
		case v > 0.7:
			tone = canvas.Color(0x5E5E5E)
		}
		for dx := -w; dx <= w; dx++ {
			col := tone
			if dx < -w/3 {
				col = canvas.Color(0x5E5E5E)
			}
			c.Set(baseX+dx, baseY-2+dy, col)
		}
	}
	// a couple of cracks
	c.Set(baseX-1, baseY-6, canvas.Color(0x4A4A4A))
	c.Set(baseX, baseY-5, canvas.Color(0x4A4A4A))
	c.Set(baseX+1, baseY-7, canvas.Color(0x4A4A4A))
}

// drawWaterTile paints a shimmering pond diamond.
func drawWaterTile(c *canvas.Canvas, px, py, x, y int, t float64) {
	shade := 0.5 + 0.4*sinf(float64(x*5+y*7)+t*2.2)
	g := uint8(26 + 32*shade)
	iso.DrawDiamond(c, px, py, canvas.RGB(g, g, g+10), canvas.Color(0x0E0E16))
	if shade > 0.78 { // a drifting glint
		c.Set(px, py+iso.HH-2, canvas.Color(0xC8C8E0))
	}
}

// drawCurrentTile paints a water tile with animated chevrons drifting in the
// flow direction — the gym's current lanes that sweep the player along.
func drawCurrentTile(c *canvas.Canvas, px, py, x, y int, t byte, tm float64) {
	drawWaterTile(c, px, py, x, y, tm)
	cdx, cdy := currentDir(t)
	cx, cy := px, py+iso.HH
	// Two chevrons drifting along the flow, wrapping every ~10px.
	for i := 0; i < 2; i++ {
		drift := int((tm*10 + float64(i*5))) % 10
		ox := cdx * (drift - 5)
		oy := cdy * (drift - 5)
		bx, by := cx+ox, cy+oy
		tone := canvas.Color(0xBFBFD0)
		// A small ">"-style chevron pointing along (cdx, cdy).
		for k := -2; k <= 2; k++ {
			ax := bx + cdy*k + cdx*(-abs2(k))
			ay := by + cdx*k + cdy*(-abs2(k))
			c.Set(ax, ay, tone)
		}
	}
}

func abs2(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// drawHealPad paints the well/rest pad: a pale diamond with a bright cross.
func drawHealPad(c *canvas.Canvas, px, py int) {
	iso.DrawDiamond(c, px, py, canvas.Color(0x2E2E2E), canvas.Color(0x565656))
	cx, cy := px, py+iso.HH
	c.FillRect(cx-1, cy-4, 3, 9, canvas.Color(0xE6E6E6))
	c.FillRect(cx-4, cy-1, 9, 3, canvas.Color(0xE6E6E6))
}

// drawHouse paints an isometric cottage whose ground-diamond top vertex is at
// (vx, vy): an extruded box with two shaded wall faces and a hip roof. The door
// and windows are painted onto the wall faces — skewed to follow each face's
// slant — so they sit flat on a wall instead of warping across the front corner.
func drawHouse(c *canvas.Canvas, vx, vy int) {
	const (
		wallH = 28 // wall height in pixels
		roofH = 18 // roof peak above the wall top
	)
	// Body: the isometric box (top diamond will be hidden by the roof).
	iso.DrawCube(c, vx, vy, wallH,
		canvas.Color(0x9C9C9C), // top
		canvas.Color(0x6C6C6C), // left face (shaded)
		canvas.Color(0x909090)) // right face (lit)

	// Bottom-edge y of each wall face at horizontal offset dx (matches DrawCube).
	rightEdge := func(dx int) int { return vy + iso.TileH - dx/2 }      // dx in [1, HW]
	leftEdge := func(dx int) int { return vy + iso.HH + (dx+iso.HW)/2 } // dx in [-HW, 0]

	// Door on the lit right face, centered and resting on the ground edge.
	for dx := 8; dx <= 16; dx++ {
		b := rightEdge(dx)
		for y := b - 16; y < b; y++ {
			c.Set(vx+dx, y, canvas.Color(0x2A2421))
		}
		c.Set(vx+dx, b-16, canvas.Color(0x4A3F38)) // lintel
	}
	c.Set(vx+15, rightEdge(15)-8, canvas.Color(0xE0E0E0)) // knob

	// A lit window on each face, up near the eaves, framed with a cross bar.
	rightWindow := func(cx int) {
		for dx := cx - 2; dx <= cx+2; dx++ {
			top := rightEdge(dx) - wallH + 6
			for y := top; y <= top+6; y++ {
				c.Set(vx+dx, y, canvas.Color(0xDDDDDD))
			}
			c.Set(vx+dx, top+3, canvas.Color(0x4A4A4A)) // horizontal bar
		}
		c.VLine(vx+cx, rightEdge(cx)-wallH+6, rightEdge(cx)-wallH+12, canvas.Color(0x4A4A4A))
	}
	leftWindow := func(cx int) {
		for dx := cx - 2; dx <= cx+2; dx++ {
			top := leftEdge(dx) - wallH + 6
			for y := top; y <= top+6; y++ {
				c.Set(vx+dx, y, canvas.Color(0xB6B6B6)) // dimmer on the shaded face
			}
			c.Set(vx+dx, top+3, canvas.Color(0x3A3A3A))
		}
		c.VLine(vx+cx, leftEdge(cx)-wallH+6, leftEdge(cx)-wallH+12, canvas.Color(0x3A3A3A))
	}
	rightWindow(20)
	leftWindow(-12)

	// Hip roof: two visible triangular faces rising from the wall-top diamond's
	// front edges to a ridge point, plus a small eave overhang.
	const e = 3
	topY := vy - wallH                                    // wall-top diamond top vertex
	bm := [2]int{vx, topY + iso.TileH + e}                // front (bottom) corner, with eave
	lf := [2]int{vx - iso.HW - e, topY + iso.HH}          // left corner
	rt := [2]int{vx + iso.HW + e, topY + iso.HH}          // right corner
	apex := [2]int{vx, topY + iso.HH - roofH}             // ridge point above the centre
	fillTriangle(c, lf, bm, apex, canvas.Color(0x9E9E9E)) // left roof face (shaded)
	fillTriangle(c, bm, rt, apex, canvas.Color(0xC6C6C6)) // right roof face (lit)
	// Ridge lines from the apex down the two front hips.
	drawRidge(c, apex, bm, canvas.Color(0xECECEC))
	drawRidge(c, apex, lf, canvas.Color(0x6E6E6E))
	drawRidge(c, apex, rt, canvas.Color(0xE2E2E2))
}

// drawWall paints an interior stone wall block (for gym rooms) whose ground
// diamond top vertex is at (vx, vy): a plain isometric cube in cool stone tones.
func drawWall(c *canvas.Canvas, vx, vy int) {
	iso.DrawCube(c, vx, vy, 24,
		canvas.Color(0x8C8C94), // top
		canvas.Color(0x50505A), // left face
		canvas.Color(0x6C6C76)) // right face
}

// drawGym paints a prominent civic building: a tall isometric hall with a
// pediment gable and a banner over the door — Tidewell's gym.
func drawGym(c *canvas.Canvas, vx, vy int) {
	const wallH = 40
	iso.DrawCube(c, vx, vy, wallH,
		canvas.Color(0xB0B0B0), // top
		canvas.Color(0x707070), // left face
		canvas.Color(0x969696)) // right face

	// Wide double doors on the lit right face, resting on the ground edge.
	rightEdge := func(dx int) int { return vy + iso.TileH - dx/2 }
	for dx := 6; dx <= 18; dx++ {
		b := rightEdge(dx)
		for y := b - 20; y < b; y++ {
			c.Set(vx+dx, y, canvas.Color(0x24201C))
		}
	}
	c.VLine(vx+12, rightEdge(12)-20, rightEdge(12)-1, canvas.Color(0x0E0C0A)) // door split

	// A big gabled roof with a pediment, plus a banner ridge.
	topY := vy - wallH
	bm := [2]int{vx, topY + iso.TileH + 4}
	lf := [2]int{vx - iso.HW - 4, topY + iso.HH}
	rt := [2]int{vx + iso.HW + 4, topY + iso.HH}
	apex := [2]int{vx, topY + iso.HH - 22}
	fillTriangle(c, lf, bm, apex, canvas.Color(0x9A9A9A))
	fillTriangle(c, bm, rt, apex, canvas.Color(0xC8C8C8))
	drawRidge(c, apex, bm, canvas.Color(0xF0F0F0))
	drawRidge(c, apex, lf, canvas.Color(0x707070))
	drawRidge(c, apex, rt, canvas.Color(0xE6E6E6))
	// A banner hanging from the eave above the doors.
	c.FillRect(vx+6, topY+iso.HH+6, 12, 8, canvas.Color(0x3A3A3A))
	c.HLine(vx+7, vx+16, topY+iso.HH+9, canvas.Color(0xD0D0D0))
}

// fillTriangle scanline-fills the triangle (a, b, c) with col.
func fillTriangle(cv *canvas.Canvas, a, b, c [2]int, col canvas.Color) {
	minY := mini(a[1], mini(b[1], c[1]))
	maxY := maxi(a[1], maxi(b[1], c[1]))
	edges := [3][2][2]int{{a, b}, {b, c}, {c, a}}
	for y := minY; y <= maxY; y++ {
		xs := xs2(edges[:], y)
		if len(xs) < 2 {
			continue
		}
		lo, hi := xs[0], xs[len(xs)-1]
		for x := lo; x <= hi; x++ {
			cv.Set(x, y, col)
		}
	}
}

// xs2 returns the x crossings of scanline y against the given edges, sorted.
func xs2(edges [][2][2]int, y int) []int {
	var xs []int
	for _, e := range edges {
		y0, y1 := e[0][1], e[1][1]
		if y0 == y1 {
			continue
		}
		if (y >= y0 && y < y1) || (y >= y1 && y < y0) {
			x0, x1 := e[0][0], e[1][0]
			x := x0 + (x1-x0)*(y-y0)/(y1-y0)
			xs = append(xs, x)
		}
	}
	if len(xs) == 2 && xs[0] > xs[1] {
		xs[0], xs[1] = xs[1], xs[0]
	}
	return xs
}

// drawRidge draws a 1px line between two points (Bresenham).
func drawRidge(cv *canvas.Canvas, a, b [2]int, col canvas.Color) {
	x0, y0, x1, y1 := a[0], a[1], b[0], b[1]
	dx, dy := absi(x1-x0), -absi(y1-y0)
	sx, sy := sgn(x1-x0), sgn(y1-y0)
	err := dx + dy
	for {
		cv.Set(x0, y0, col)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func mini(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func absi(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
func sgn(a int) int {
	switch {
	case a > 0:
		return 1
	case a < 0:
		return -1
	default:
		return 0
	}
}

// drawSign paints a small wooden signpost.
func drawSign(c *canvas.Canvas, footX, footY int) {
	c.FillRect(footX-1, footY-13, 2, 13, canvas.Color(0x554636))  // post
	c.FillRect(footX-7, footY-22, 15, 10, canvas.Color(0x8A7654)) // board
	c.Rect(footX-7, footY-22, 15, 10, canvas.Color(0x3C3026))
	c.HLine(footX-4, footX+4, footY-18, canvas.Color(0x3C3026)) // "text"
	c.HLine(footX-4, footX+2, footY-15, canvas.Color(0x3C3026))
}

// drawItemBall paints a small bobbing pickup sphere with a band and glint.
func drawItemBall(c *canvas.Canvas, footX, footY int, t float64) {
	cy := footY - 6 + int(sinf(t*3)*1.5)
	for dy := -5; dy <= 5; dy++ {
		w := int(5 * sqrtClamp(1-float64(dy*dy)/25.0))
		tone := canvas.Color(0xD2D2D2)
		if dy > 1 {
			tone = canvas.Color(0x808080)
		}
		for dx := -w; dx <= w; dx++ {
			c.Set(footX+dx, cy+dy, tone)
		}
	}
	c.HLine(footX-5, footX+5, cy, canvas.Color(0x2E2E2E)) // band
	c.Set(footX, cy, canvas.Color(0xF4F4F4))              // button
	c.Set(footX-2, cy-3, canvas.Color(0xFFFFFF))          // glint
}

// drawIsoTree draws a tree rising from the ground point (baseX, baseY): a trunk
// and a top-lit canopy, with a soft ground shadow.
func drawIsoTree(c *canvas.Canvas, baseX, baseY, seed int) {
	// Ground shadow.
	for dy := -4; dy <= 4; dy++ {
		w := int(16 * sqrtClamp(1-float64(dy*dy)/16.0))
		yy := baseY + dy - 1
		for dx := -w; dx <= w; dx++ {
			c.Set(baseX+dx, yy, c.At(baseX+dx, yy).Scale(0.6))
		}
	}
	c.FillRect(baseX-2, baseY-18, 4, 18, canvas.Color(0x3A2F2A)) // trunk
	// Canopy: two overlapping top-lit blobs for a fuller silhouette.
	canopy(c, baseX-5, baseY-26, 12)
	canopy(c, baseX+5, baseY-24, 11)
	canopy(c, baseX, baseY-32, 13)
}

func canopy(c *canvas.Canvas, cx, cy, r int) {
	for dy := -r; dy <= r; dy++ {
		w := int(float64(r) * sqrtClamp(1-float64(dy*dy)/float64(r*r)))
		v := float64(dy+r) / float64(2*r)
		tone := canvas.Color(0x5E5E5E)
		switch {
		case v < 0.28:
			tone = canvas.Color(0x7A7A7A)
		case v > 0.72:
			tone = canvas.Color(0x3C3C3C)
		}
		for dx := -w; dx <= w; dx++ {
			col := tone
			if dx < -w/3 {
				col = canvas.Color(0x3C3C3C)
			}
			c.Set(cx+dx, cy+dy, col)
		}
	}
}

func drawContactShadow(c *canvas.Canvas, footX, footY int) {
	for dy := -3; dy <= 3; dy++ {
		w := int(9 * sqrtClamp(1-float64(dy*dy)/9.0))
		yy := footY + dy - 1
		for dx := -w; dx <= w; dx++ {
			c.Set(footX+dx, yy, c.At(footX+dx, yy).Scale(0.5))
		}
	}
}
