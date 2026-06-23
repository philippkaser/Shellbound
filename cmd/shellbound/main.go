// Command shellbound runs the Shellbound SSH MMO server: a shared
// black-and-white plaza reachable with nothing but an ssh client.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"

	"github.com/shellbound/shellbound/internal/hub"
	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/server"
	"github.com/shellbound/shellbound/internal/storage"
	"github.com/shellbound/shellbound/internal/world"
	"github.com/shellbound/shellbound/internal/worlds/bomber"
	"github.com/shellbound/shellbound/internal/worlds/comingsoon"
	"github.com/shellbound/shellbound/internal/worlds/doom"
)

// envOr returns the environment variable or a default.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	addr := envOr("SHELLBOUND_ADDR", ":80")
	dbPath := envOr("SHELLBOUND_DB", "./shellbound.db")
	hostKey := envOr("SHELLBOUND_HOSTKEY", "./.ssh/shellbound_ed25519")

	if dir := filepath.Dir(hostKey); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			log.Fatal("create host key dir", "dir", dir, "err", err)
		}
	}

	db, err := storage.Open(dbPath)
	if err != nil {
		log.Fatal("open database", "path", dbPath, "err", err)
	}
	defer db.Close()
	repos := storage.NewRepos(db)
	log.Info("database ready", "path", dbPath)

	// Register the worlds behind the portals. "bomberman" leads to the arena
	// game (The Vault) and "doom" to the raycast shooter; the rest are still the
	// "coming soon" placeholder. Adding a mini-game means registering it here.
	registry := world.NewRegistry()
	for _, p := range plaza.Portals {
		var w world.World = comingsoon.New(p.Key, p.Name)
		switch p.Key {
		case "bomberman":
			w = bomber.New(p.Key, p.Name)
		case "doom":
			w = doom.New(p.Key, p.Name)
		}
		if err := registry.Register(w); err != nil {
			log.Fatal("register world", "key", p.Key, "err", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	h := hub.New()
	go h.Run(ctx)

	srv, err := server.New(server.Config{
		Addr:        addr,
		HostKeyPath: hostKey,
		Hub:         h,
		Repos:       repos,
		Registry:    registry,
	})
	if err != nil {
		log.Fatal("create server", "err", err)
	}

	go func() {
		log.Info("shellbound is listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Error("server stopped", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", "err", err)
	}
}
