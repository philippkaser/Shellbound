module github.com/shellbound/shellbound

// Go 1.22+ required by the spec; 1.23 is pinned because recent Charm
// releases declare it as their minimum. `go mod tidy` will resolve the
// full dependency graph and generate go.sum on first build.
go 1.23.0

require (
	github.com/charmbracelet/bubbles v0.21.0
	github.com/charmbracelet/bubbletea v1.3.4
	github.com/charmbracelet/harmonica v0.2.0
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/charmbracelet/log v0.4.0
	github.com/charmbracelet/ssh v0.0.0-20240301204039-e79ff702f5b3
	github.com/charmbracelet/wish v1.4.6
	github.com/muesli/termenv v0.15.2
	golang.org/x/crypto v0.31.0
	modernc.org/sqlite v1.33.1
)
