// Package overworld is the heart of the client experience: the plaza
// renderer, movement and input handling, multiplayer presence, chat and
// the panel overlays. One Model exists per SSH session.
package overworld

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/anim"
	"github.com/shellbound/shellbound/internal/hub"
	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/render/halfblock"
	"github.com/shellbound/shellbound/internal/render/shimmer"
	"github.com/shellbound/shellbound/internal/storage"
	"github.com/shellbound/shellbound/internal/style"
	"github.com/shellbound/shellbound/internal/ui/chat"
	"github.com/shellbound/shellbound/internal/ui/friends"
	"github.com/shellbound/shellbound/internal/ui/inventory"
	"github.com/shellbound/shellbound/internal/ui/toast"
)

// Tick rates: ambient animation at 10 FPS; movement steps once per tick
// while keys are held. The hub broadcasts at 20 Hz on its own clock. The
// move tick is deliberately unhurried so a step reads as a clean cell-to-
// cell hop rather than a sprint; heldWindow stays comfortably above it so
// a held key bridges the gap between key-repeat events.
const (
	animTickEvery = 100 * time.Millisecond
	moveTickEvery = 90 * time.Millisecond
	heldWindow    = 220 * time.Millisecond
)

// Minimum playable terminal size.
const (
	minTermW = 60
	minTermH = 20
)

// EnterPortalMsg asks the session app to switch to the world registered
// under Key. Emitted when the player walks into a portal mouth.
type EnterPortalMsg struct {
	Key  string
	Name string
}

// DisconnectMsg asks the session app to end the session.
type DisconnectMsg struct {
	Reason string
}

// Internal tick/event messages.
type animTickMsg time.Time
type moveTickMsg time.Time
type hubEventMsg struct{ ev hub.Event }
type hubClosedMsg struct{}

// Model is the per-session overworld state.
type Model struct {
	theme  style.Theme
	world  *plaza.Map
	base   *halfblock.Canvas // shared, read-only static plaza
	repos  *storage.Repos
	player storage.Player
	handle *hub.Handle

	// Local avatar: feet position as (cell column, half-block pixel row).
	px, py    int
	dir       hub.Dir
	moving    bool
	walkCount int
	onPortal  bool

	cam     *anim.Camera
	held    map[string]time.Time
	ticking bool // a moveTick chain is live

	remotes map[int64]hub.PlayerState

	chat    chat.Model
	inv     inventory.Model
	friends friends.Model
	toasts  toast.Model
	unread  map[int64]string // player id -> username with unseen DMs

	field *shimmer.Field
	start time.Time

	termW, termH int
	screen       *halfblock.Canvas
	sb           *strings.Builder
}

// New creates the overworld for a logged-in player. base must be the
// world-sized canvas produced by plazaMap.RenderBase (it is only ever
// read). The model joins the hub immediately; snapshot seeds the remote
// player set.
func New(
	theme style.Theme,
	plazaMap *plaza.Map,
	base *halfblock.Canvas,
	repos *storage.Repos,
	player storage.Player,
	handle *hub.Handle,
	snapshot []hub.PlayerState,
) Model {
	px := plazaMap.SpawnX
	py := plazaMap.SpawnY*2 + 1

	remotes := make(map[int64]hub.PlayerState, len(snapshot))
	for _, st := range snapshot {
		if st.Info.ID != player.ID {
			remotes[st.Info.ID] = st
		}
	}

	m := Model{
		theme:   theme,
		world:   plazaMap,
		base:    base,
		repos:   repos,
		player:  player,
		handle:  handle,
		px:      px,
		py:      py,
		dir:     hub.DirDown,
		cam:     anim.NewCamera(float64(px), float64(py)/2),
		held:    make(map[string]time.Time),
		remotes: remotes,
		chat:    chat.New(theme),
		inv:     inventory.New(theme),
		toasts:  toast.New(theme),
		unread:  make(map[int64]string),
		field:   shimmer.NewField(),
		start:   time.Now(),
		sb:      &strings.Builder{},
	}
	m.friends = friends.New(theme, repos, player, m.sendDM)
	m.chat.AddSystem("welcome to shellbound — /help for commands")
	return m
}

// sendDM persists an outgoing DM and delivers it live when possible. Used
// by both the friends panel and /w.
func (m Model) sendDM(to storage.Player, text string) error {
	if err := m.repos.DMs.Save(m.player.ID, to.ID, text); err != nil {
		return err
	}
	m.handle.Whisper(to.ID, text)
	return nil
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(listenHub(m.handle.Events()), animTick())
}

func animTick() tea.Cmd {
	return tea.Tick(animTickEvery, func(t time.Time) tea.Msg { return animTickMsg(t) })
}

func moveTick() tea.Cmd {
	return tea.Tick(moveTickEvery, func(t time.Time) tea.Msg { return moveTickMsg(t) })
}

func listenHub(ch <-chan hub.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return hubClosedMsg{}
		}
		return hubEventMsg{ev: ev}
	}
}

// Update implements tea.Model (with a concrete return type; the session
// app owns the tea.Model interface).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termW, m.termH = msg.Width, msg.Height
		m.screen = nil // rebuilt lazily at the new size
		return m, nil

	case animTickMsg:
		now := time.Time(msg)
		m.toasts.Tick(now)
		m.cam.Update(float64(m.px), float64(m.py)/2)
		return m, animTick()

	case moveTickMsg:
		return m.stepMovement()

	case hubEventMsg:
		next, cmd := m.applyEvent(msg.ev)
		return next, tea.Batch(cmd, listenHub(next.handle.Events()))

	case hubClosedMsg:
		return m, func() tea.Msg { return DisconnectMsg{Reason: "connection closed"} }

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward everything else (e.g. cursor blinks) to whichever input is
	// active.
	if m.chat.IsOpen() {
		cmd, _ := m.chat.Update(msg)
		return m, cmd
	}
	if m.friends.IsOpen() {
		return m, m.friends.Update(msg)
	}
	return m, nil
}

// handleKey routes a key press by UI focus.
func (m Model) handleKey(key tea.KeyMsg) (Model, tea.Cmd) {
	// Hard quit always works.
	if key.String() == "ctrl+c" {
		return m, func() tea.Msg { return DisconnectMsg{Reason: "bye"} }
	}

	if m.chat.IsOpen() {
		cmd, submitted := m.chat.Update(key)
		if submitted != "" {
			next, scmd := m.submitChat(submitted)
			return next, tea.Batch(cmd, scmd)
		}
		return m, cmd
	}

	if m.friends.IsOpen() {
		return m, m.friends.Update(key)
	}

	if m.inv.IsOpen() {
		switch key.String() {
		case "esc", "i", "q":
			m.inv.Close()
		}
		return m, nil
	}

	// Plaza focus.
	switch key.String() {
	case "q":
		return m, func() tea.Msg { return DisconnectMsg{Reason: "bye"} }
	case "enter":
		return m, m.chat.Open()
	case "i":
		items, err := m.repos.Inventory.Items(m.player.ID)
		m.inv.Open(items, err)
		return m, nil
	case "f":
		m.friends.Open(m.handle.OnlineIDs())
		m.unread = make(map[int64]string)
		return m, nil
	case "up", "down", "left", "right", "w", "a", "s", "d":
		m.held[normalizeKey(key.String())] = time.Now()
		if !m.ticking {
			m.ticking = true
			return m, moveTick()
		}
		return m, nil
	}
	return m, nil
}

// normalizeKey folds WASD onto the arrow names so the held-key map has one
// entry per direction.
func normalizeKey(k string) string {
	switch k {
	case "w":
		return "up"
	case "s":
		return "down"
	case "a":
		return "left"
	case "d":
		return "right"
	}
	return k
}

// stepMovement advances the avatar one step based on recently-held keys,
// with axis-separated collision so walls let you slide along them. A step
// is a full cell on both axes: horizontally that is one column, vertically
// two half-block pixel rows (py is in half-blocks). Diagonal movement is
// intentionally disabled — when both axes are held we keep only the more
// recently pressed one, so the avatar always travels straight along a row
// or a column.
func (m Model) stepMovement() (Model, tea.Cmd) {
	now := time.Now()
	heldAt := func(name string) (time.Time, bool) {
		t, ok := m.held[name]
		if ok && now.Sub(t) <= heldWindow {
			return t, true
		}
		return time.Time{}, false
	}
	dx, dy := 0, 0
	var hTime, vTime time.Time // newest press on each axis
	note := func(t time.Time, axis *time.Time) {
		if t.After(*axis) {
			*axis = t
		}
	}
	if t, ok := heldAt("left"); ok {
		dx--
		note(t, &hTime)
	}
	if t, ok := heldAt("right"); ok {
		dx++
		note(t, &hTime)
	}
	if t, ok := heldAt("up"); ok {
		dy--
		note(t, &vTime)
	}
	if t, ok := heldAt("down"); ok {
		dy++
		note(t, &vTime)
	}

	// No diagonals: keep the axis whose key was pressed most recently.
	if dx != 0 && dy != 0 {
		if hTime.After(vTime) {
			dy = 0
		} else {
			dx = 0
		}
	}

	// A vertical step covers a whole cell — two half-block pixel rows.
	dy *= 2

	if dx == 0 && dy == 0 {
		// Keys released: stop the tick chain and broadcast the idle pose.
		m.ticking = false
		if m.moving {
			m.moving = false
			m.handle.Move(hub.Pos{X: m.px, Y: m.py}, m.dir, false)
		}
		return m, nil
	}

	moved := false
	if dx != 0 && !m.world.Blocked(m.px+dx, m.py/2) {
		m.px += dx
		moved = true
	}
	if dy != 0 && !m.world.Blocked(m.px, (m.py+dy)/2) {
		m.py += dy
		moved = true
	}

	// Face the direction of effort even when blocked.
	switch {
	case dx < 0:
		m.dir = hub.DirLeft
	case dx > 0:
		m.dir = hub.DirRight
	case dy < 0:
		m.dir = hub.DirUp
	case dy > 0:
		m.dir = hub.DirDown
	}

	if moved {
		m.walkCount++
		m.moving = true
		m.cam.Update(float64(m.px), float64(m.py)/2)
		m.handle.Move(hub.Pos{X: m.px, Y: m.py}, m.dir, true)

		// Portal trigger: fires on the transition into a mouth, not while
		// standing in one (so returning from a world doesn't re-enter).
		if p, ok := plaza.PortalAt(m.px, m.py/2); ok {
			if !m.onPortal {
				m.onPortal = true
				m.toasts.Show("✦ " + p.Name + " ✦")
				return m, tea.Batch(moveTick(), func() tea.Msg {
					return EnterPortalMsg{Key: p.Key, Name: p.Name}
				})
			}
		} else {
			m.onPortal = false
		}
	} else if m.moving {
		// Pushing into a wall: stand still rather than pantomime walking.
		m.moving = false
		m.handle.Move(hub.Pos{X: m.px, Y: m.py}, m.dir, false)
	}
	return m, moveTick()
}

// applyEvent folds one hub event into local state.
func (m Model) applyEvent(ev hub.Event) (Model, tea.Cmd) {
	switch ev := ev.(type) {
	case hub.EvJoin:
		m.remotes[ev.State.Info.ID] = ev.State
		m.chat.AddSystem(ev.State.Info.Name + " appeared")

	case hub.EvLeave:
		delete(m.remotes, ev.PlayerID)
		m.chat.AddSystem(ev.Name + " left")

	case hub.EvMoves:
		for _, st := range ev.States {
			if st.Info.ID != m.player.ID {
				m.remotes[st.Info.ID] = st
			}
		}

	case hub.EvChat:
		kind := chat.KindChat
		if ev.Emote {
			kind = chat.KindEmote
		}
		m.chat.Add(chat.Entry{Kind: kind, Name: ev.From.Name, Color: ev.From.Color, Text: ev.Text})

	case hub.EvWhisper:
		m.chat.Add(chat.Entry{Kind: chat.KindWhisperIn, Name: ev.From.Name, Color: ev.From.Color, Text: ev.Text})
		if !m.friends.NotifyIncoming(ev.From.ID, ev.Text) {
			m.unread[ev.From.ID] = ev.From.Name
		}

	case hub.EvKick:
		return m, func() tea.Msg { return DisconnectMsg{Reason: ev.Reason} }
	}
	return m, nil
}

// ResumeFromWorld is called by the session app when the player exits a
// portal world: it re-snaps the camera and refreshes presence-dependent
// UI.
func (m Model) ResumeFromWorld() Model {
	m.cam.Snap(float64(m.px), float64(m.py)/2)
	return m
}

// Player returns the logged-in player this overworld belongs to.
func (m Model) Player() storage.Player { return m.player }
