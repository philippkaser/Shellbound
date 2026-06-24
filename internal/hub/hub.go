// Package hub is Shellbound's in-memory presence and broadcast core. Every
// connected session registers here; the hub fans out joins, leaves, chat
// and coalesced movement updates over per-session buffered channels.
//
// Locking model: all channel sends happen while holding at least the read
// lock; removal and channel close happen under the write lock. Because the
// two cannot overlap, closing a channel after removing the session from
// the maps can never race a send.
package hub

import (
	"context"
	"sort"
	"sync"
	"time"
)

// eventBuffer is the per-session channel depth. A session that stalls past
// this many pending events starts losing them (drop-oldest-first would be
// nicer but drop-newest is lock-free and the next sweep repairs positions).
const eventBuffer = 256

// broadcastEvery is the movement coalescing interval (~20 Hz).
const broadcastEvery = 50 * time.Millisecond

type session struct {
	sid    string // SSH session id (hub map key)
	info   PlayerInfo
	ch     chan Event
	state  PlayerState
	dirty  bool
	closed bool
}

// Hub holds every online player. Create with New, start the movement
// sweeper with Run, and register sessions via Join.
type Hub struct {
	mu       sync.RWMutex
	sessions map[string]*session // by SSH session id
	byPlayer map[int64]string    // player id -> session id
}

// New creates an empty hub.
func New() *Hub {
	return &Hub{
		sessions: make(map[string]*session),
		byPlayer: make(map[int64]string),
	}
}

// Run drives the 20 Hz movement broadcast sweep until ctx is canceled.
// Call it in its own goroutine.
func (h *Hub) Run(ctx context.Context) {
	ticker := time.NewTicker(broadcastEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.sweep()
		}
	}
}

// sweep broadcasts the states of all dirty sessions and clears the flags.
func (h *Hub) sweep() {
	h.mu.Lock()
	defer h.mu.Unlock()
	var states []PlayerState
	for _, s := range h.sessions {
		if s.dirty {
			states = append(states, s.state)
			s.dirty = false
		}
	}
	if len(states) == 0 {
		return
	}
	ev := EvMoves{States: states}
	for _, s := range h.sessions {
		s.send(ev)
	}
}

// send delivers ev without blocking; events are dropped if the session's
// buffer is full. Callers must hold h.mu (read or write).
func (s *session) send(ev Event) {
	if s.closed {
		return
	}
	select {
	case s.ch <- ev:
	default:
		// Buffer full: drop. Movement repairs itself on the next sweep and
		// chat loss under a 256-event backlog means the session is doomed
		// anyway.
	}
}

// Join registers a player session and returns a Handle plus a snapshot of
// everyone else currently online. If the same player is already connected,
// the old session is kicked (takeover semantics).
func (h *Hub) Join(sessionID string, info PlayerInfo, pos Pos) (*Handle, []PlayerState) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Takeover: boot any existing session for this player.
	if oldSID, ok := h.byPlayer[info.ID]; ok {
		if old, ok := h.sessions[oldSID]; ok {
			old.send(EvKick{Reason: "you connected from another terminal"})
			old.closed = true
			close(old.ch)
			delete(h.sessions, oldSID)
		}
		delete(h.byPlayer, info.ID)
	}

	s := &session{
		sid:  sessionID,
		info: info,
		ch:   make(chan Event, eventBuffer),
		state: PlayerState{
			Info: info,
			Pos:  pos,
			Dir:  DirDown,
		},
	}

	// Snapshot others before inserting, and announce the newcomer to them.
	var snapshot []PlayerState
	join := EvJoin{State: s.state}
	for _, other := range h.sessions {
		snapshot = append(snapshot, other.state)
		other.send(join)
	}

	h.sessions[sessionID] = s
	h.byPlayer[info.ID] = sessionID
	return &Handle{hub: h, sid: sessionID, ch: s.ch}, snapshot
}

// leave removes a session and notifies the others. Idempotent.
func (h *Hub) leave(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[sessionID]
	if !ok {
		return
	}
	delete(h.sessions, sessionID)
	if h.byPlayer[s.info.ID] == sessionID {
		delete(h.byPlayer, s.info.ID)
	}
	s.closed = true
	close(s.ch)
	ev := EvLeave{PlayerID: s.info.ID, Name: s.info.Name}
	for _, other := range h.sessions {
		other.send(ev)
	}
}

// setCosmetic updates a session's equipped cosmetic and flags it dirty so the
// next sweep rebroadcasts the new identity to everyone.
func (h *Hub) setCosmetic(sessionID, key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[sessionID]
	if !ok {
		return
	}
	s.info.Cosmetic = key
	s.state.Info.Cosmetic = key
	s.dirty = true
}

// move records a position update; the sweep broadcasts it.
func (h *Hub) move(sessionID string, pos Pos, dir Dir, moving bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[sessionID]
	if !ok {
		return
	}
	s.state.Pos = pos
	s.state.Dir = dir
	s.state.Moving = moving
	s.dirty = true
}

// chat broadcasts a chat line (or emote) from a session to everyone,
// including the sender (a single render path for all chat).
func (h *Hub) chat(sessionID, text string, emote bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.sessions[sessionID]
	if !ok {
		return
	}
	ev := EvChat{From: s.info, Text: text, Emote: emote}
	for _, other := range h.sessions {
		other.send(ev)
	}
}

// whisper delivers a DM to one online player. It reports whether the
// recipient was online to receive it.
func (h *Hub) whisper(fromSID string, toPlayerID int64, text string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	from, ok := h.sessions[fromSID]
	if !ok {
		return false
	}
	toSID, ok := h.byPlayer[toPlayerID]
	if !ok {
		return false
	}
	to, ok := h.sessions[toSID]
	if !ok {
		return false
	}
	to.send(EvWhisper{From: from.info, Text: text})
	return true
}

// who returns a snapshot of all online player states, sorted by name.
func (h *Hub) who() []PlayerState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]PlayerState, 0, len(h.sessions))
	for _, s := range h.sessions {
		out = append(out, s.state)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Info.Name < out[j].Info.Name })
	return out
}

// onlineIDs returns the set of online player ids.
func (h *Hub) onlineIDs() map[int64]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[int64]bool, len(h.byPlayer))
	for id := range h.byPlayer {
		out[id] = true
	}
	return out
}

// Handle is a session's capability to talk to the hub. All methods are
// safe after Leave; they become no-ops.
type Handle struct {
	hub  *Hub
	sid  string
	ch   chan Event
	once sync.Once
}

// Events returns the channel the hub delivers this session's events on.
// It closes when the session leaves or is kicked.
func (hd *Handle) Events() <-chan Event { return hd.ch }

// Move reports the local player's new position/facing; broadcast is
// coalesced by the hub sweep.
func (hd *Handle) Move(pos Pos, dir Dir, moving bool) {
	hd.hub.move(hd.sid, pos, dir, moving)
}

// SetCosmetic updates the player's equipped headwear; peers see it on the
// next sweep.
func (hd *Handle) SetCosmetic(key string) { hd.hub.setCosmetic(hd.sid, key) }

// Chat broadcasts a global chat message.
func (hd *Handle) Chat(text string) { hd.hub.chat(hd.sid, text, false) }

// Emote broadcasts a /me action line.
func (hd *Handle) Emote(text string) { hd.hub.chat(hd.sid, text, true) }

// Whisper delivers a DM live if the recipient is online; persistence is
// the caller's job. Returns whether it was delivered.
func (hd *Handle) Whisper(toPlayerID int64, text string) bool {
	return hd.hub.whisper(hd.sid, toPlayerID, text)
}

// Who lists everyone online, sorted by name.
func (hd *Handle) Who() []PlayerState { return hd.hub.who() }

// OnlineIDs returns the set of online player ids (for friend presence).
func (hd *Handle) OnlineIDs() map[int64]bool { return hd.hub.onlineIDs() }

// Leave deregisters the session. Idempotent and safe to call from both the
// quit path and the SSH teardown middleware.
func (hd *Handle) Leave() {
	hd.once.Do(func() { hd.hub.leave(hd.sid) })
}

// LeaveBySession removes whatever session is registered under the SSH
// session id. Used by the server's cleanup middleware, which has no Handle.
func (h *Hub) LeaveBySession(sessionID string) { h.leave(sessionID) }
