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
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/light"
	"github.com/shellbound/shellbound/internal/render/shimmer"
	"github.com/shellbound/shellbound/internal/render/syncwriter"
	"github.com/shellbound/shellbound/internal/storage"
	"github.com/shellbound/shellbound/internal/style"
	"github.com/shellbound/shellbound/internal/ui/chat"
	"github.com/shellbound/shellbound/internal/ui/friends"
	"github.com/shellbound/shellbound/internal/ui/inventory"
	"github.com/shellbound/shellbound/internal/ui/toast"
)

// Tick rates: ambient animation at 10 FPS; movement stepping at 20 Hz
// while keys are held. The hub broadcasts at 20 Hz on its own clock.
const (
	animTickEvery = 100 * time.Millisecond
	moveTickEvery = 50 * time.Millisecond
	heldWindow    = 140 * time.Millisecond
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

// frameDoneMsg is returned after a Sixel frame has been written to the
// session; it carries no state and exists only to complete the render Cmd.
type frameDoneMsg struct{}

// Env carries the per-session rendering collaborators the server builds once
// and hands to every overworld: the shared Sixel palette, the synchronized
// session writer, and the probed terminal cell size in pixels.
type Env struct {
	Pal          *canvas.Palette
	Out          *syncwriter.Writer
	CellW, CellH int
}

// Model is the per-session overworld state.
type Model struct {
	theme  style.Theme
	world  *plaza.Map
	repos  *storage.Repos
	player storage.Player
	handle *hub.Handle

	// Rendering: full frames are baked into a pixel canvas and shipped as one
	// Sixel image straight to the session. active gates whether this model
	// currently owns the screen (false while a portal world is on top).
	pal          *canvas.Palette
	out          *syncwriter.Writer
	cellW, cellH int
	active       bool

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

	field  *shimmer.Field
	lights *light.Field
	start  time.Time

	termW, termH int
	screen       *canvas.Canvas
	sb           *strings.Builder
}

// New creates the overworld for a logged-in player. env carries the shared
// Sixel palette, the session writer and the terminal cell size. The model
// joins the hub immediately; snapshot seeds the remote player set.
func New(
	theme style.Theme,
	plazaMap *plaza.Map,
	env Env,
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
		pal:     env.Pal,
		out:     env.Out,
		cellW:   env.CellW,
		cellH:   env.CellH,
		active:  true,
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
		lights:  light.NewField(),
		start:   time.Now(),
		screen:  canvas.New(1, 1),
		sb:      &strings.Builder{},
	}
	m.friends = friends.New(theme, repos, player)
	m.chat.AddSystem("welcome to shellbound — /help for commands")
	return m
}

// SetActive marks whether the overworld currently owns the screen. While a
// portal world is on top it is inactive and emits no frames.
func (m Model) SetActive(b bool) Model {
	m.active = b
	return m
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
		if m.active {
			return m, m.writeFrameCmd(m.renderFrame())
		}
		return m, nil

	case animTickMsg:
		now := time.Time(msg)
		m.toasts.Tick(now)
		m.cam.Update(float64(m.px), float64(m.py)/2)
		if m.active {
			return m, tea.Batch(animTick(), m.writeFrameCmd(m.renderFrame()))
		}
		return m, animTick()

	case moveTickMsg:
		return m.stepMovement()

	case frameDoneMsg:
		return m, nil

	case hubEventMsg:
		next, cmd := m.applyEvent(msg.ev)
		return next, tea.Batch(cmd, listenHub(next.handle.Events()))

	case hubClosedMsg:
		return m, func() tea.Msg { return DisconnectMsg{Reason: "connection closed"} }

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward everything else (e.g. cursor blinks) to the chat input — the
	// only live text field; the friends panel is keyboard-driven only.
	if m.chat.IsOpen() {
		cmd, _ := m.chat.Update(msg)
		return m, cmd
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
		return m.updateFriends(key)
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

// updateFriends routes a key to the friends panel, then turns a row
// selection into a /w command pre-filled in the chat console.
func (m Model) updateFriends(msg tea.Msg) (Model, tea.Cmd) {
	cmd := m.friends.Update(msg)
	if target, ok := m.friends.TakeSelected(); ok {
		return m, tea.Batch(cmd, m.chat.OpenWith("/w "+target.Username+" "))
	}
	return m, cmd
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
// with axis-separated collision so walls let you slide along them.
func (m Model) stepMovement() (Model, tea.Cmd) {
	now := time.Now()
	heldDir := func(name string) bool {
		t, ok := m.held[name]
		return ok && now.Sub(t) <= heldWindow
	}
	dx, dy := 0, 0
	if heldDir("left") {
		dx--
	}
	if heldDir("right") {
		dx++
	}
	if heldDir("up") {
		dy--
	}
	if heldDir("down") {
		dy++
	}

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
		// Whispers land in the chat console; the HUD badge nudges the player
		// if they're busy in a panel or the message scrolls off.
		m.chat.Add(chat.Entry{Kind: chat.KindWhisperIn, Name: ev.From.Name, Color: ev.From.Color, Text: ev.Text})
		m.unread[ev.From.ID] = ev.From.Name

	case hub.EvKick:
		return m, func() tea.Msg { return DisconnectMsg{Reason: ev.Reason} }
	}
	return m, nil
}

// ResumeFromWorld is called by the session app when the player exits a
// portal world: it re-snaps the camera, re-arms rendering and refreshes
// presence-dependent UI.
func (m Model) ResumeFromWorld() Model {
	m.cam.Snap(float64(m.px), float64(m.py)/2)
	m.active = true
	return m
}

// writeFrameCmd writes a pre-built Sixel frame to the session off the update
// goroutine. The frame string is immutable and the writer is mutex-guarded,
// so this never races the model or bubbletea's renderer.
func (m Model) writeFrameCmd(frame string) tea.Cmd {
	if frame == "" || m.out == nil {
		return nil
	}
	out := m.out
	return func() tea.Msg {
		_, _ = out.WriteString(frame)
		return frameDoneMsg{}
	}
}

// Player returns the logged-in player this overworld belongs to.
func (m Model) Player() storage.Player { return m.player }
