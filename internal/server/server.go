// Package server hosts the SSH endpoint: a Wish server that authenticates
// by public key only, hands every session a bubbletea program wired to ship
// Sixel graphics, and tears down hub presence when the connection ends
// (gracefully or not).
package server

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	bm "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	"github.com/muesli/termenv"

	"github.com/shellbound/shellbound/internal/auth"
	"github.com/shellbound/shellbound/internal/hub"
	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/syncwriter"
	"github.com/shellbound/shellbound/internal/storage"
	"github.com/shellbound/shellbound/internal/style"
	"github.com/shellbound/shellbound/internal/ui/overworld"
	"github.com/shellbound/shellbound/internal/world"
)

// Config wires the server's collaborators.
type Config struct {
	Addr        string // listen address, e.g. ":80"
	HostKeyPath string // ed25519 host key (created if missing)
	Hub         *hub.Hub
	Repos       *storage.Repos
	Registry    *world.Registry
}

// Server is the Shellbound SSH server.
type Server struct {
	cfg      Config
	ssh      *ssh.Server
	plazaMap *plaza.Map
	pal      *canvas.Palette // shared Sixel palette (read-only across sessions)

	// teardowns holds per-session cleanup funcs (close internal channels,
	// etc.) run by the cleanup middleware after the program exits.
	mu        sync.Mutex
	teardowns map[string][]func()
}

// New builds the server: it loads the plaza once and assembles the shared
// Sixel palette and middleware chain.
func New(cfg Config) (*Server, error) {
	plazaMap := plaza.Load()

	// The palette's only saturated registers are the curated player colors;
	// the rest is a grey ramp plus the shimmer hue ring.
	playerColors := make([]canvas.Color, len(style.Palette))
	for i, hex := range style.Palette {
		playerColors[i] = canvas.Hex(hex)
	}

	s := &Server{
		cfg:       cfg,
		plazaMap:  plazaMap,
		pal:       canvas.DefaultPalette(playerColors),
		teardowns: make(map[string][]func()),
	}

	srv, err := wish.NewServer(
		wish.WithAddress(cfg.Addr),
		wish.WithHostKeyPath(cfg.HostKeyPath),
		// Public keys only; any key is accepted — the key *is* the account.
		wish.WithPublicKeyAuth(func(_ ssh.Context, key ssh.PublicKey) bool {
			return key != nil
		}),
		wish.WithMiddleware(
			// Innermost first; wish runs the list back-to-front.
			bm.MiddlewareWithProgramHandler(s.programHandler, termenv.TrueColor),
			s.cleanupMiddleware,
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	s.ssh = srv
	return s, nil
}

// ListenAndServe blocks serving SSH connections.
func (s *Server) ListenAndServe() error { return s.ssh.ListenAndServe() }

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error { return s.ssh.Shutdown(ctx) }

// addTeardown registers fn to run when the session ends.
func (s *Server) addTeardown(sessionID string, fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teardowns[sessionID] = append(s.teardowns[sessionID], fn)
}

// runTeardowns executes and clears a session's teardown funcs.
func (s *Server) runTeardowns(sessionID string) {
	s.mu.Lock()
	fns := s.teardowns[sessionID]
	delete(s.teardowns, sessionID)
	s.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// cleanupMiddleware guarantees hub departure and goroutine shutdown no
// matter how the session ends (quit, drop, panic in the program).
func (s *Server) cleanupMiddleware(next ssh.Handler) ssh.Handler {
	return func(sess ssh.Session) {
		sid := sess.Context().SessionID()
		defer func() {
			s.runTeardowns(sid)
			s.cfg.Hub.LeaveBySession(sid)
		}()
		next(sess)
	}
}

// programHandler builds the per-session bubbletea program. Crucially it sets
// the program output to a synchronized writer over the session, so the plaza's
// own Sixel frame loop and bubbletea's renderer share one serialized stream.
func (s *Server) programHandler(sess ssh.Session) *tea.Program {
	// A session-bound renderer forced to TrueColor — both for lipgloss text
	// (login, panels-in-worlds) and to match the canvas's raw RGB output.
	renderer := lipgloss.NewRenderer(sess, termenv.WithProfile(termenv.TrueColor))
	theme := style.NewTheme(renderer)

	out := syncwriter.New(sess)
	baseOpts := []tea.ProgramOption{tea.WithInput(sess), tea.WithOutput(out), tea.WithAltScreen()}
	notice := func(text string) *tea.Program {
		return tea.NewProgram(newNoticeModel(theme, text), baseOpts...)
	}

	fp, err := auth.Fingerprint(sess.PublicKey())
	if err != nil {
		log.Warn("session without public key", "remote", sess.RemoteAddr())
		return notice("no public key presented — shellbound identifies you by your ssh key")
	}

	if !sixelEnabled() {
		return notice("Shellbound now renders with Sixel graphics.\r\n\r\n" +
			"Please connect from a Sixel-capable terminal — e.g. WezTerm, foot, " +
			"mlterm, Windows Terminal, or `xterm -ti vt340`.")
	}

	player, err := s.cfg.Repos.Players.ByFingerprint(fp)
	if err != nil {
		log.Error("player lookup failed", "fingerprint", fp, "err", err)
		return notice("storage trouble — please try again in a moment")
	}

	cw, ch := cellSize()
	sid := sess.Context().SessionID()
	a := newApp(appDeps{
		hub:       s.cfg.Hub,
		repos:     s.cfg.Repos,
		registry:  s.cfg.Registry,
		plazaMap:  s.plazaMap,
		env:       overworld.Env{Pal: s.pal, Out: out, CellW: cw, CellH: ch},
		sessionID: sid,
		onTeardown: func(fn func()) {
			s.addTeardown(sid, fn)
		},
	}, theme, fp, player)
	log.Info("session started", "fingerprint", fp, "known", player != nil)
	return tea.NewProgram(a, baseOpts...)
}

// sixelEnabled reports whether to serve the Sixel renderer. Auto-detecting
// Sixel support over the SSH/bubbletea input path is unreliable, so this is an
// explicit deployment switch: SHELLBOUND_SIXEL=off serves the notice screen to
// everyone; anything else (the default) serves graphics.
func sixelEnabled() bool {
	switch strings.ToLower(os.Getenv("SHELLBOUND_SIXEL")) {
	case "off", "0", "false", "no":
		return false
	default:
		return true
	}
}

// cellSize returns the assumed terminal cell size in pixels, overridable with
// SHELLBOUND_CELL="WxH". The frame is sized to the cell grid times this, so a
// value close to the client's real cell size makes the image fill the window.
func cellSize() (int, int) {
	// Conservative defaults: under-estimating a client's real cell size only
	// letterboxes the image, whereas over-estimating makes it taller than the
	// screen and scroll. Tune up with SHELLBOUND_CELL to fill the window.
	const defW, defH = 8, 16
	v := os.Getenv("SHELLBOUND_CELL")
	if v == "" {
		return defW, defH
	}
	parts := strings.SplitN(strings.ToLower(v), "x", 2)
	if len(parts) != 2 {
		return defW, defH
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return defW, defH
	}
	return w, h
}
