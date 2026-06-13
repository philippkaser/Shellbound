// Package style centralizes the Shellbound look: the monochrome scale used
// for the world, the vibrant palette used for player identity, and a
// per-session lipgloss theme.
//
// Color discipline: the map itself is black, white and grey. The only
// saturated colors on screen are player names, chat usernames and portal
// shimmer.
package style

import (
	"crypto/sha256"

	"github.com/charmbracelet/lipgloss"
)

// Monochrome scale (hex strings usable as lipgloss.Color values).
const (
	Black     = "#000000"
	White     = "#FFFFFF"
	GreyLight = "#D4D4D4"
	GreyMid   = "#A1A1A1"
	GreyDim   = "#737373"
	GreyDark  = "#404040"
)

// Palette is the curated set of ~24 vibrant, high-contrast colors that read
// well on pure black. A player's color is picked deterministically from this
// slice via PlayerColor.
var Palette = []string{
	"#FF5FAF", // hot pink
	"#A855F7", // electric purple
	"#5EEAD4", // teal cyan
	"#A3E635", // lime
	"#FBBF24", // amber
	"#60A5FA", // sky blue
	"#F87171", // coral red
	"#34D399", // emerald
	"#F472B6", // pink
	"#C084FC", // lavender
	"#22D3EE", // cyan
	"#FACC15", // yellow
	"#FB923C", // orange
	"#4ADE80", // green
	"#818CF8", // indigo
	"#E879F9", // fuchsia
	"#2DD4BF", // turquoise
	"#FDE047", // lemon
	"#FF8FA3", // salmon
	"#93C5FD", // powder blue
	"#BEF264", // chartreuse
	"#FDA4AF", // rose
	"#67E8F9", // ice blue
	"#FCA5A5", // light coral
}

// PlayerColor deterministically maps an SSH public key fingerprint to a hex
// color from Palette. The same fingerprint always yields the same color.
func PlayerColor(fingerprint string) string {
	sum := sha256.Sum256([]byte(fingerprint))
	return Palette[int(sum[0])%len(Palette)]
}

// Theme bundles per-session lipgloss styles. All styles are built from a
// session-bound renderer so colors survive the SSH transport; styles built
// from the package-level lipgloss default would inherit the *server*
// process's color profile (usually no TTY, meaning colors get stripped).
type Theme struct {
	Renderer *lipgloss.Renderer

	// Text styles.
	Text   lipgloss.Style // plain white on black
	Dim    lipgloss.Style // mid grey, for secondary text
	Faded  lipgloss.Style // dim grey, for fading chat
	System lipgloss.Style // italic grey, for system/chat-command output
	Error  lipgloss.Style // white on black, bold (errors stay monochrome)

	// Chrome styles.
	PanelBorder lipgloss.Style // rounded white border box
	PanelTitle  lipgloss.Style // bold white
	InputBar    lipgloss.Style // rounded border for the chat input
	Toast       lipgloss.Style // inverse white box for transient notices
}

// NewTheme builds a Theme from a session-bound renderer.
func NewTheme(r *lipgloss.Renderer) Theme {
	return Theme{
		Renderer: r,
		Text:     r.NewStyle().Foreground(lipgloss.Color(White)),
		Dim:      r.NewStyle().Foreground(lipgloss.Color(GreyMid)),
		Faded:    r.NewStyle().Foreground(lipgloss.Color(GreyDark)),
		System:   r.NewStyle().Foreground(lipgloss.Color(GreyDim)).Italic(true),
		Error:    r.NewStyle().Foreground(lipgloss.Color(White)).Bold(true),
		PanelBorder: r.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(GreyLight)).
			Padding(0, 1),
		PanelTitle: r.NewStyle().Foreground(lipgloss.Color(White)).Bold(true),
		InputBar: r.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(GreyMid)).
			Padding(0, 1),
		Toast: r.NewStyle().
			Foreground(lipgloss.Color(Black)).
			Background(lipgloss.Color(White)).
			Padding(0, 1),
	}
}

// Colored returns a style with the given hex foreground, bound to the
// theme's renderer. Used for player-colored names.
func (t Theme) Colored(hex string) lipgloss.Style {
	return t.Renderer.NewStyle().Foreground(lipgloss.Color(hex))
}
