// Package listpanel is the shared kit behind Shellbound's modal panels
// (friends, wardrobe, shop, inventory, emote picker, inspect cards). Each
// panel owns its data and produces a Content for the pixel renderer; the
// cursor movement and column padding that every list re-implemented live
// here once.
package listpanel

import "strings"

// Content is a panel as the pixel renderer draws it: a title, body lines, an
// optional highlighted line (Cursor indexes Lines; -1 means none) and a
// dim footer hint.
type Content struct {
	Title  string
	Lines  []string
	Cursor int // index into Lines to render with the selection bar; -1 = none
	Footer string
}

// Empty reports whether there is nothing to draw.
func (c Content) Empty() bool {
	return c.Title == "" && len(c.Lines) == 0 && c.Footer == ""
}

// Nav applies a list-navigation key to a cursor over n rows. It reports the
// new cursor and whether the key was a navigation key.
func Nav(key string, cursor, n int) (int, bool) {
	switch key {
	case "up", "k":
		if cursor > 0 {
			cursor--
		}
		return cursor, true
	case "down", "j":
		if cursor < n-1 {
			cursor++
		}
		return cursor, true
	}
	return cursor, false
}

// Pad right-pads s with spaces to at least n columns (rune-aware, so names
// with multi-byte characters don't shift the columns after them).
func Pad(s string, n int) string {
	w := 0
	for range s {
		w++
	}
	if w >= n {
		return s + " "
	}
	return s + strings.Repeat(" ", n-w)
}
