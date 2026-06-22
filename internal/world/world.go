// Package world defines the portal-world plugin surface: the World
// interface that future mini-games implement, the registry they are looked
// up in, and the scoped persistence APIs handed to a running world.
//
// Isolation model: a world never sees a database handle or another
// player's data. It receives a SaveStore and InventoryAPI already bound to
// (player, world key) via closures, so cross-world or cross-player access
// is impossible by construction.
package world

import (
	"fmt"
	"io"
	"sort"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/render/canvas"
)

// PlayerInfo identifies the player inside a world, without exposing
// anything sensitive.
type PlayerInfo struct {
	ID       int64
	Username string
	Color    string // hex personal color
}

// SaveStore is a single save slot scoped to one (player, world) pair.
type SaveStore interface {
	// Load returns the saved blob, or an empty non-nil slice if no save
	// exists yet.
	Load() ([]byte, error)
	// Save overwrites the slot with data.
	Save(data []byte) error
}

// InventoryAPI lets a world grant items to the player it is bound to.
type InventoryAPI interface {
	// Grant awards qty of an item; itemKey should be stable and prefixed
	// by convention with the world key (e.g. "chess.gold_pawn").
	Grant(itemKey, name string, qty int) error
}

// Render carries the shared Sixel rendering collaborators a graphical world
// needs: the session's fixed palette, the synchronized session writer (the
// world writes whole Sixel frames to it, the same way the plaza does), and the
// best-known terminal cell size in pixels. Text-only worlds can ignore it.
type Render struct {
	Palette      *canvas.Palette
	Out          io.Writer
	CellW, CellH int
}

// Context is everything a world receives when a player enters it.
type Context struct {
	Player    PlayerInfo
	Save      SaveStore
	Inventory InventoryAPI
	Render    Render
	// Exit returns the player to the plaza. Safe to call multiple times;
	// calls after the first are no-ops.
	Exit func()
}

// World is a portal destination: a self-contained bubbletea experience.
type World interface {
	// Key is the stable identifier ("bomberman"); it scopes saves.
	Key() string
	// Name is the display name shown on the portal pedestal.
	Name() string
	// Init builds the bubbletea model that runs the game for one player.
	Init(ctx Context) tea.Model
}

// Registry holds the registered worlds, keyed by World.Key.
type Registry struct {
	mu     sync.RWMutex
	worlds map[string]World
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{worlds: make(map[string]World)}
}

// Register adds a world; registering a duplicate key is an error.
func (r *Registry) Register(w World) error {
	if w == nil || w.Key() == "" {
		return fmt.Errorf("world: cannot register nil or keyless world")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.worlds[w.Key()]; exists {
		return fmt.Errorf("world: duplicate key %q", w.Key())
	}
	r.worlds[w.Key()] = w
	return nil
}

// Get looks up a world by key.
func (r *Registry) Get(key string) (World, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.worlds[key]
	return w, ok
}

// Keys returns the registered keys, sorted.
func (r *Registry) Keys() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.worlds))
	for k := range r.worlds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
