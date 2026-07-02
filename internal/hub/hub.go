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

	"github.com/shellbound/shellbound/internal/shellmon"
)

// challengeTTL is how long a pending PvP challenge stays valid.
const challengeTTL = 30 * time.Second

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
	busy   bool // inside a portal world or a PvP battle — not challengeable
}

// pendingChallenge is a PvP challenge awaiting the target's response.
type pendingChallenge struct {
	fromID   int64
	fromSID  string
	fromInfo PlayerInfo
	team     []*shellmon.Creature
	at       time.Time
}

// Hub holds every online player. Create with New, start the movement
// sweeper with Run, and register sessions via Join.
type Hub struct {
	mu       sync.RWMutex
	sessions map[string]*session // by SSH session id
	byPlayer map[int64]string    // player id -> session id
	// challenges holds the pending PvP challenge per target player id (one at a
	// time; a fresh challenge replaces an older one).
	challenges map[int64]pendingChallenge
	// lastExpiry is when the sweep last pruned expired challenges.
	lastExpiry time.Time
}

// New creates an empty hub.
func New() *Hub {
	return &Hub{
		sessions:   make(map[string]*session),
		byPlayer:   make(map[int64]string),
		challenges: make(map[int64]pendingChallenge),
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
// It also lazily expires pending PvP challenges (about once a second) so an
// ignored challenge tells the challenger instead of dangling forever.
func (h *Hub) sweep() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if now := time.Now(); now.Sub(h.lastExpiry) >= time.Second {
		h.lastExpiry = now
		for target, pc := range h.challenges {
			if now.Sub(pc.at) <= challengeTTL {
				continue
			}
			delete(h.challenges, target)
			if ch, ok := h.sessions[pc.fromSID]; ok {
				ch.send(EvBattleDeclined{From: pc.fromInfo, Reason: "your challenge expired unanswered"})
			}
		}
	}

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

// trySend is send for events that must not be silently lost (battle starts,
// challenges): it reports whether the event was actually enqueued so the
// caller can back out of state changes instead of leaving a player half-in
// a battle that never reaches them. Callers must hold h.mu.
func (s *session) trySend(ev Event) bool {
	if s.closed {
		return false
	}
	select {
	case s.ch <- ev:
		return true
	default:
		return false
	}
}

// canSend reports whether the session's buffer has room. All sends happen
// under h.mu, so under the write lock this is a reliable reservation check.
func (s *session) canSend() bool {
	return !s.closed && len(s.ch) < cap(s.ch)
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
	h.dropChallengesFor(s.info.ID)
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

// emote broadcasts a gesture from a session to everyone (including the sender),
// so a single render path drives both self and peers.
func (h *Hub) emote(sessionID, kind string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.sessions[sessionID]
	if !ok {
		return
	}
	ev := EvEmote{PlayerID: s.info.ID, Kind: kind}
	for _, other := range h.sessions {
		other.send(ev)
	}
}

// setBusy flags a session as in-world/in-battle (so it can't be challenged).
func (h *Hub) setBusy(sessionID string, busy bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.sessions[sessionID]; ok {
		s.busy = busy
	}
}

// challengePvP records a challenge to a target player and notifies them. It
// reports whether the challenge was delivered, and if not, why.
func (h *Hub) challengePvP(fromSID string, toPlayerID int64, team []*shellmon.Creature) (bool, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	from, ok := h.sessions[fromSID]
	if !ok {
		return false, "you are not connected"
	}
	if from.info.ID == toPlayerID {
		return false, "you can't duel yourself"
	}
	toSID, ok := h.byPlayer[toPlayerID]
	if !ok {
		return false, "they're not online"
	}
	to := h.sessions[toSID]
	if to == nil || to.closed {
		return false, "they're not online"
	}
	if to.busy {
		return false, to.info.Name + " is busy"
	}
	// A fresh challenge replaces an older pending one; tell the displaced
	// challenger rather than letting them wait on nothing.
	if prev, ok := h.challenges[toPlayerID]; ok && prev.fromSID != fromSID {
		if prevS, ok := h.sessions[prev.fromSID]; ok {
			prevS.send(EvBattleDeclined{From: to.info, Reason: to.info.Name + " received another challenge"})
		}
	}
	pc := pendingChallenge{
		fromID: from.info.ID, fromSID: fromSID, fromInfo: from.info, team: team, at: time.Now(),
	}
	if !to.trySend(EvBattleChallenge{From: from.info}) {
		return false, to.info.Name + " is not responding"
	}
	h.challenges[toPlayerID] = pc
	return true, ""
}

// respondPvP resolves a pending challenge held by the responding player. On
// accept (with a non-empty team) it builds the shared match and drops both
// players into it; on decline it notifies the challenger.
func (h *Hub) respondPvP(responderSID string, challengerID int64, accept bool, team []*shellmon.Creature) {
	h.mu.Lock()
	defer h.mu.Unlock()
	resp, ok := h.sessions[responderSID]
	if !ok {
		return
	}
	pc, ok := h.challenges[resp.info.ID]
	if !ok || pc.fromID != challengerID || time.Since(pc.at) > challengeTTL {
		return // stale or mismatched
	}
	delete(h.challenges, resp.info.ID)

	chSID, ok := h.byPlayer[challengerID]
	chSession := h.sessions[chSID]
	if !accept || !ok || chSession == nil || chSession.closed {
		if chSession != nil {
			chSession.send(EvBattleDeclined{From: resp.info, Reason: resp.info.Name + " declined."})
		}
		return
	}
	// Re-validate the challenger: it must still be the same session that
	// issued the challenge and must not have gone busy (entered a portal
	// world or another battle) in the meantime — otherwise accepting would
	// yank a player out of whatever they're doing, or soft-lock them.
	if chSID != pc.fromSID || chSession.busy {
		resp.send(EvBattleDeclined{From: pc.fromInfo, Reason: pc.fromInfo.Name + " is no longer available"})
		return
	}

	// Build the single shared match (challenger = side A) and start both
	// sides. EvBattleStart must not be dropped — a player flagged busy who
	// never receives the match is soft-locked — so reserve room in both
	// buffers before flipping any state.
	if !chSession.canSend() || !resp.canSend() {
		resp.send(EvBattleDeclined{From: pc.fromInfo, Reason: pc.fromInfo.Name + " is not responding"})
		return
	}
	match := shellmon.NewMatch(pc.team, team, time.Now().UnixNano())
	resp.busy, chSession.busy = true, true
	chSession.send(EvBattleStart{Match: match, SideA: true, Opponent: resp.info})
	resp.send(EvBattleStart{Match: match, SideA: false, Opponent: pc.fromInfo})
}

// dropChallengesFor removes any pending challenge to or from a player (called on
// leave so a vanished player leaves no dangling challenge).
func (h *Hub) dropChallengesFor(playerID int64) {
	delete(h.challenges, playerID)
	for target, pc := range h.challenges {
		if pc.fromID == playerID {
			delete(h.challenges, target)
		}
	}
}

// whisper delivers a DM to one online player. It reports whether the
// recipient was online to receive it.
func (h *Hub) whisper(fromSID string, toPlayerID int64, text string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	from, ok := h.sessions[fromSID]
	if !ok || from.info.ID == toPlayerID {
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

// PlayEmote broadcasts a visual gesture (an emote key) to everyone nearby.
func (hd *Handle) PlayEmote(kind string) { hd.hub.emote(hd.sid, kind) }

// SetBusy marks this session as in a world/battle (or back in the plaza), which
// gates whether others can challenge it.
func (hd *Handle) SetBusy(busy bool) { hd.hub.setBusy(hd.sid, busy) }

// ChallengePvP challenges another player to a Shellmon duel, attaching this
// player's team. Returns whether it was sent and, if not, a short reason.
func (hd *Handle) ChallengePvP(toPlayerID int64, team []*shellmon.Creature) (bool, string) {
	return hd.hub.challengePvP(hd.sid, toPlayerID, team)
}

// RespondPvP accepts or declines a pending challenge from challengerID; on
// accept, team is this player's roster.
func (hd *Handle) RespondPvP(challengerID int64, accept bool, team []*shellmon.Creature) {
	hd.hub.respondPvP(hd.sid, challengerID, accept, team)
}

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
