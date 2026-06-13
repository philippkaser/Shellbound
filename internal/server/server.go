// Package server hosts the SSH endpoint: a Wish server that authenticates
// by public key only, hands every session a bubbletea program, and tears
// down hub presence when the connection ends (gracefully or not).
package server

import (
	"context"
	"fmt"
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
	"github.com/shellbound/shellbound/internal/render/halfblock"
	"github.com/shellbound/shellbound/internal/storage"
	"github.com/shellbound/shellbound/internal/style"
	"github.com/shellbound/shellbound/internal/world"
)

// Config wires the server's collaborators.
type Config struct {
	Addr        string // listen address, e.g. ":2222"
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
	base     *halfblock.Canvas

	// teardowns holds per-session cleanup funcs (close internal channels,
	// etc.) run by the cleanup middleware after the program exits.
	mu        sync.Mutex
	teardowns map[string][]func()
}

// New builds the server: it loads and pre-renders the plaza once (shared,
// read-only across sessions) and assembles the middleware chain.
func New(cfg Config) (*Server, error) {
	plazaMap := plaza.Load()
	base := halfblock.New(plazaMap.W, plazaMap.H)
	plazaMap.RenderBase(base)

	s := &Server{
		cfg:       cfg,
		plazaMap:  plazaMap,
		base:      base,
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
			bm.MiddlewareWithColorProfile(s.teaHandler, termenv.TrueColor),
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

// teaHandler builds the per-session bubbletea model.
func (s *Server) teaHandler(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
	opts := []tea.ProgramOption{tea.WithAltScreen()}

	// Styles must come from a session-bound renderer; we force TrueColor to
	// match both the middleware profile and the canvas's raw RGB output.
	renderer := lipgloss.NewRenderer(sess, termenv.WithProfile(termenv.TrueColor))
	theme := style.NewTheme(renderer)

	fp, err := auth.Fingerprint(sess.PublicKey())
	if err != nil {
		log.Warn("session without public key", "remote", sess.RemoteAddr())
		return newNoticeModel(theme, "no public key presented — shellbound identifies you by your ssh key"), opts
	}

	player, err := s.cfg.Repos.Players.ByFingerprint(fp)
	if err != nil {
		log.Error("player lookup failed", "fingerprint", fp, "err", err)
		return newNoticeModel(theme, "storage trouble — please try again in a moment"), opts
	}

	sid := sess.Context().SessionID()
	a := newApp(appDeps{
		hub:       s.cfg.Hub,
		repos:     s.cfg.Repos,
		registry:  s.cfg.Registry,
		plazaMap:  s.plazaMap,
		base:      s.base,
		sessionID: sid,
		onTeardown: func(fn func()) {
			s.addTeardown(sid, fn)
		},
	}, theme, fp, player)
	log.Info("session started", "fingerprint", fp, "known", player != nil)
	return a, opts
}
