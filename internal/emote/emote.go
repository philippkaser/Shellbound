// Package emote is Shellbound's gesture system: a small catalog of reactions a
// player can play on their avatar, drawn as a hand-pixelled icon in a callout
// bubble above the head (in the same monochrome style as everything else) and,
// for some, a little body motion — a hop, a sway, a crouch. Emotes are
// broadcast through the hub so everyone nearby sees them, and fade after Dur.
package emote

import (
	"math"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// Dur is how long an emote stays visible after it's played.
const Dur = 3.5 // seconds

// motion is the optional body gesture an emote adds to the avatar.
type motion int

const (
	mNone   motion = iota
	mHop           // little bounces (happy, laugh)
	mSway          // side-to-side (dance)
	mShake         // fast jitter (angry)
	mCrouch        // settle low (sleep)
)

// Emote is one gesture. Verb is the third-person phrasing used in /help.
type Emote struct {
	Key    string
	Verb   string
	motion motion
	icon   func(c *canvas.Canvas, cx, cy int, t float64)
}

// catalog is the full set, in menu order (the index is the quick-key number).
var catalog = []Emote{
	{Key: "wave", Verb: "waves", motion: mNone, icon: iconWave},
	{Key: "happy", Verb: "smiles", motion: mHop, icon: iconSmile},
	{Key: "laugh", Verb: "laughs", motion: mHop, icon: iconLaugh},
	{Key: "heart", Verb: "loves it", motion: mNone, icon: iconHeart},
	{Key: "cry", Verb: "cries", motion: mNone, icon: iconCry},
	{Key: "angry", Verb: "fumes", motion: mShake, icon: iconAngry},
	{Key: "sleep", Verb: "dozes off", motion: mCrouch, icon: iconSleep},
	{Key: "dance", Verb: "dances", motion: mSway, icon: iconNote},
}

var byKey = func() map[string]Emote {
	m := make(map[string]Emote, len(catalog))
	for _, e := range catalog {
		m[e.Key] = e
	}
	return m
}()

// All returns the catalog in menu order.
func All() []Emote { return catalog }

// Valid reports whether key names a real emote.
func Valid(key string) bool { _, ok := byKey[key]; return ok }

// Monochrome bubble tones.
const (
	paper = canvas.Color(0xF2F2F2) // bubble fill
	ink   = canvas.Color(0x2A2A2A) // outline / icon
	mid   = canvas.Color(0x8A8A8A) // soft accents
)

// Offset returns the avatar's body gesture displacement at time t — applied to
// the sprite so the figure hops, sways or crouches while the emote plays.
func Offset(key string, t float64) (dx, dy int) {
	e, ok := byKey[key]
	if !ok {
		return 0, 0
	}
	switch e.motion {
	case mHop:
		dy = -int(math.Abs(math.Sin(t*5)) * 3)
	case mSway:
		dx = int(math.Round(math.Sin(t*5) * 2))
	case mShake:
		dx = int(math.Round(math.Sin(t * 22)))
	case mCrouch:
		dy = 3
	}
	return dx, dy
}

// DrawBubble paints the emote's callout above (hx, hy) — the point just above
// the avatar's head — bobbing gently. Unknown keys draw nothing.
func DrawBubble(c *canvas.Canvas, hx, hy int, key string, t float64) {
	e, ok := byKey[key]
	if !ok || e.icon == nil {
		return
	}
	bob := int(math.Round(math.Sin(t * 3)))
	cx := hx
	cy := hy - 11 + bob
	const hw, hh = 8, 6
	// Body and a 1px outline, with a short tail dropping toward the head.
	c.FillRect(cx-hw, cy-hh, 2*hw+1, 2*hh+1, paper)
	c.Rect(cx-hw, cy-hh, 2*hw+1, 2*hh+1, ink)
	c.FillRect(cx-1, cy+hh, 3, 2, paper)
	c.Set(cx-2, cy+hh+1, ink)
	c.Set(cx+2, cy+hh+1, ink)
	c.Set(cx, cy+hh+2, ink)
	e.icon(c, cx, cy-1, t)
}

// --- icons (drawn centered at (cx, cy), ~±4px) ---

func iconWave(c *canvas.Canvas, cx, cy int, t float64) {
	c.FillRect(cx-2, cy-1, 5, 4, ink) // palm
	c.Set(cx-2, cy-2, ink)
	c.Set(cx, cy-2, ink)
	c.Set(cx+2, cy-2, ink) // fingers
	c.Set(cx-3, cy, ink)   // thumb
}

func iconSmile(c *canvas.Canvas, cx, cy int, t float64) {
	c.Set(cx-2, cy-2, ink)
	c.Set(cx+2, cy-2, ink)
	c.Set(cx-2, cy+1, ink)
	c.Set(cx-1, cy+2, ink)
	c.Set(cx, cy+2, ink)
	c.Set(cx+1, cy+2, ink)
	c.Set(cx+2, cy+1, ink)
}

func iconLaugh(c *canvas.Canvas, cx, cy int, t float64) {
	c.Set(cx-2, cy-2, ink)
	c.Set(cx+2, cy-2, ink)
	c.FillRect(cx-2, cy+1, 5, 2, ink) // wide open grin
	c.Set(cx-2, cy, ink)
	c.Set(cx+2, cy, ink)
}

func iconHeart(c *canvas.Canvas, cx, cy int, t float64) {
	c.Set(cx-2, cy-1, ink)
	c.Set(cx-1, cy-2, ink)
	c.Set(cx+1, cy-2, ink)
	c.Set(cx+2, cy-1, ink)
	c.HLine(cx-2, cx+2, cy, ink)
	c.HLine(cx-1, cx+1, cy+1, ink)
	c.Set(cx, cy+2, ink)
}

func iconCry(c *canvas.Canvas, cx, cy int, t float64) {
	c.Set(cx-2, cy-2, ink)
	c.Set(cx+2, cy-2, ink)
	// frown
	c.Set(cx-2, cy+2, ink)
	c.Set(cx-1, cy+1, ink)
	c.Set(cx, cy+1, ink)
	c.Set(cx+1, cy+1, ink)
	c.Set(cx+2, cy+2, ink)
	// a tear dripping from the left eye, looping
	ty := -1 + int(math.Mod(t*5, 4))
	c.Set(cx-2, cy+ty, mid)
}

func iconAngry(c *canvas.Canvas, cx, cy int, t float64) {
	c.Set(cx, cy, ink)
	c.Set(cx-2, cy-2, ink)
	c.Set(cx-1, cy-1, ink)
	c.Set(cx+2, cy-2, ink)
	c.Set(cx+1, cy-1, ink)
	c.Set(cx-2, cy+2, ink)
	c.Set(cx-1, cy+1, ink)
	c.Set(cx+2, cy+2, ink)
	c.Set(cx+1, cy+1, ink)
}

func iconSleep(c *canvas.Canvas, cx, cy int, t float64) {
	// A big "Z".
	c.HLine(cx-2, cx+1, cy-2, ink)
	c.Set(cx, cy-1, ink)
	c.Set(cx-1, cy, ink)
	c.HLine(cx-2, cx+1, cy+1, ink)
	// A small rising "z".
	zy := cy - 3 - int(math.Mod(t*3, 3))
	c.Set(cx+2, zy, mid)
	c.Set(cx+3, zy, mid)
	c.Set(cx+2, zy+1, mid)
}

func iconNote(c *canvas.Canvas, cx, cy int, t float64) {
	c.FillCircle(cx-1, cy+2, 1, ink) // note head
	c.VLine(cx, cy-3, cy+2, ink)     // stem
	c.HLine(cx, cx+2, cy-3, ink)     // flag
	c.Set(cx+2, cy-2, ink)
}
