// Package overworld is the heart of the client experience: the plaza
// renderer, movement and input handling, multiplayer presence, chat and
// the panel overlays. One Model exists per SSH session.
//
// Rendering is decoupled: the Model runs game logic and, whenever state
// changes, publishes a cheap snapshot to a background Renderer (renderer.go)
// that produces smooth, interpolated Sixel frames on its own clock. The heavy
// encode therefore never blocks bubbletea's input loop.
package overworld

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/cosmetic"
	"github.com/shellbound/shellbound/internal/emote"
	"github.com/shellbound/shellbound/internal/hub"
	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/render/canvas"
	"github.com/shellbound/shellbound/internal/render/syncwriter"
	mon "github.com/shellbound/shellbound/internal/shellmon"
	"github.com/shellbound/shellbound/internal/storage"
	"github.com/shellbound/shellbound/internal/style"
	"github.com/shellbound/shellbound/internal/ui/chat"
	"github.com/shellbound/shellbound/internal/ui/cosmetics"
	"github.com/shellbound/shellbound/internal/ui/friends"
	"github.com/shellbound/shellbound/internal/ui/inventory"
	"github.com/shellbound/shellbound/internal/ui/shop"
	"github.com/shellbound/shellbound/internal/ui/toast"
)

// Movement uses a steady tick so the walk pace is decoupled from the
// terminal's key-repeat (which has a long, OS-dependent initial delay and a
// variable rate — relying on it makes a held key stutter and feel laggy). The
// first press steps instantly; while a direction key is held, the tick carries
// the walk at moveTickEvery. A key counts as "held" only while fresh: a lone
// tap expires within tapWindow (so it moves exactly one tile), and a key whose
// repeats have begun stays live within holdSteady of the last repeat. The anim
// tick drives ambient animation and is a backstop that returns the walk pose to
// idle a beat after the last step.
const (
	animTickEvery = 100 * time.Millisecond
	idleAfter     = 250 * time.Millisecond // revert to idle pose this long after the last step

	moveTickEvery = 120 * time.Millisecond // one tile per tick while a direction is held
	tapWindow     = 110 * time.Millisecond // < moveTickEvery: a single tap walks exactly one tile
	holdSteady    = 170 * time.Millisecond // a key whose repeats have begun keeps the walk alive
)

// Coins accrue passively while the player is in the plaza: one coin every
// coinEvery, persisted as it lands. The cadence is deliberately slow so a
// cosmetic is a goal you earn over a session rather than something you grind out
// in a minute. Time spent inside a portal world doesn't pay (the baseline resets
// on return), so coins reward hanging around the shared plaza.
const (
	coinEvery     = 20 * time.Second
	coinsPerAward = 1
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

// StartPvPMsg asks the session app to drop the player into a shared Shellmon
// duel. The Match is the coordinator both players hold.
type StartPvPMsg struct {
	Match    *mon.Match
	SideA    bool
	Opponent string
}

// shellmonWorldKey scopes the saved Shellmon roster (matches the portal key).
const shellmonWorldKey = "shellmon"

// pvpChallenge is an incoming duel invite awaiting this player's response.
type pvpChallenge struct {
	fromID int64
	name   string
}

// Portal transition timing.
const (
	portalEnterDur = 0.5 // seconds the portal "swallows" the screen before the world loads
	portalExitDur  = 0.4 // seconds the plaza fades back in on return
)

// portalFX drives the colored diamond wipe when stepping through a portal (and
// the reverse fade when returning).
type portalFX struct {
	key     string
	name    string
	start   time.Time
	emitted bool // the EnterPortalMsg has been sent
	exiting bool // returning to the plaza
}

// PixelSizeMsg carries the terminal's cell grid AND drawable pixels together
// (from the SSH pty-req / window-change), so the cell size can be derived from
// a single consistent pair. Sent only when the client reports pixel
// dimensions. Carrying both avoids the resize race where cols/rows update
// before pixels and the derived cell size is briefly — and destructively —
// wrong.
type PixelSizeMsg struct{ Cols, Rows, W, H int }

// CellSizeMsg carries the terminal's character cell size in pixels, obtained
// by querying the terminal directly (CSI 16 t). It's the most reliable source
// of cell size — many terminals don't report pixel dimensions over SSH.
type CellSizeMsg struct{ W, H int }

// Internal tick/event messages.
type animTickMsg time.Time
type moveTickMsg time.Time
type hubEventMsg struct{ ev hub.Event }
type hubClosedMsg struct{}

// emoteState is one player's active gesture and when it began.
type emoteState struct {
	kind  string
	start time.Time
}

// inspectInfo is the snapshot of another player shown in the inspect panel.
type inspectInfo struct {
	id           int64
	name         string
	cosmeticName string
	cosmeticTier string // rarity label of the worn piece ("" for bare-headed)
	since        string
}

// interactRadius is how close (in cells, Chebyshev) another player must be to
// greet or inspect them.
const interactRadius = 2

// moveIntent is the direction the player currently wants to walk. seen is the
// last time a key for this direction arrived; count grows with key-repeats so
// the tick can tell a sustained hold (repeats flowing) from a single tap.
type moveIntent struct {
	dx, dy int
	run    bool
	seen   time.Time
	count  int
}

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

	renderer *Renderer

	// Local avatar: feet position as (cell column, half-block pixel row).
	px, py    int
	dir       hub.Dir
	moving    bool
	walkCount int
	onPortal  bool
	lastMove  time.Time // when the avatar last stepped, for idle detection

	intent  moveIntent // direction currently held
	ticking bool       // a moveTick chain is live

	remotes map[int64]hub.PlayerState

	chat      chat.Model
	inv       inventory.Model
	friends   friends.Model
	wardrobe  cosmetics.Model
	shop      shop.Model
	toasts    toast.Model
	unread    map[int64]string // player id -> username with unseen DMs
	emoteMenu bool             // the quick emote picker is open

	// inspecting holds the player whose card is open (nil = none).
	inspecting *inspectInfo

	// challenge holds an incoming PvP duel invite (nil = none).
	challenge *pvpChallenge

	// portal drives the portal-entry/exit transition (nil = none).
	portal *portalFX

	// emotes holds each player's in-flight gesture (including our own); entries
	// are pruned once older than emote.Dur.
	emotes map[int64]emoteState

	lastCoinAt time.Time // when the last passive coin was credited

	termW, termH int
	cellW, cellH int // pixels per terminal cell; best known estimate
}

// New creates the overworld for a logged-in player. env carries the shared
// Sixel palette, the session writer and the terminal cell size. The model
// joins the hub immediately; snapshot seeds the remote player set and the
// background renderer starts at once.
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

	r := NewRenderer(env.Pal, env.Out, plazaMap)
	r.Start()

	cellW, cellH := env.CellW, env.CellH
	if cellW <= 0 {
		cellW = 8
	}
	if cellH <= 0 {
		cellH = 16
	}

	m := Model{
		theme:    theme,
		world:    plazaMap,
		renderer: r,
		cellW:    cellW,
		cellH:    cellH,
		repos:    repos,
		player:   player,
		handle:   handle,
		px:       px,
		py:       py,
		dir:      hub.DirDown,
		remotes:  remotes,
		chat:     chat.New(theme),
		inv:      inventory.New(theme),
		wardrobe: cosmetics.New(theme),
		shop:     shop.New(theme),
		toasts:   toast.New(theme),
		unread:   make(map[int64]string),
		emotes:   make(map[int64]emoteState),

		lastCoinAt: time.Now(),
	}
	m.friends = friends.New(theme, repos, player)
	m.chat.AddSystem("welcome to shellbound — /help for commands")
	return m
}

// SetActive marks whether the overworld currently owns the screen. While a
// portal world is on top it is inactive and the renderer emits no frames.
func (m Model) SetActive(b bool) Model {
	m.renderer.SetActive(b)
	return m
}

// StopRenderer halts the background render goroutine; the session app wires
// this into teardown so the goroutine never outlives the session.
func (m Model) StopRenderer() { m.renderer.Stop() }

// publish hands the renderer a fresh, immutable snapshot of everything it
// draws. Cheap enough to call on every state change.
func (m *Model) publish() {
	players := make([]playerSnapshot, 0, len(m.remotes)+1)
	for _, st := range m.remotes {
		players = append(players, playerSnapshot{
			id: st.Info.ID, name: st.Info.Name, color: st.Info.Color,
			x: st.Pos.X, y: st.Pos.Y, dir: st.Dir, moving: st.Moving,
			cosmetic: st.Info.Cosmetic, emote: m.activeEmote(st.Info.ID),
		})
	}
	players = append(players, playerSnapshot{
		id: m.player.ID, name: m.player.Username, color: m.player.Color,
		x: m.px, y: m.py, dir: m.dir, moving: m.moving,
		cosmetic: m.player.Cosmetic, emote: m.activeEmote(m.player.ID),
	})

	var panel []string
	switch {
	case m.friends.IsOpen():
		panel = m.friends.Lines()
	case m.inv.IsOpen():
		panel = m.inv.Lines()
	case m.wardrobe.IsOpen():
		panel = m.wardrobe.Lines()
	case m.shop.IsOpen():
		panel = m.shop.Lines()
	case m.emoteMenu:
		panel = emoteMenuLines()
	case m.inspecting != nil:
		panel = m.inspectLines()
	case m.challenge != nil:
		panel = []string{
			"Battle Challenge", "",
			m.challenge.name + " wants to duel!",
			"(your full team, healed)", "",
			"y accept   n decline",
		}
	}

	// Contextual bottom-of-screen nudges, only when nothing holds focus.
	idle := !m.chat.IsOpen() && len(panel) == 0
	shopPrompt := idle && m.world.NearShop(m.px, m.py/2)
	interactPrompt := ""
	if idle {
		if st, ok := m.nearestPlayer(); ok {
			interactPrompt = "x  greet " + st.Info.Name
		}
	}

	var unreadName string
	var unreadN int
	if len(m.unread) > 0 && !m.friends.IsOpen() {
		for _, n := range m.unread {
			unreadName = n
			break
		}
		unreadN = len(m.unread) - 1
	}

	chatInput := ""
	if m.chat.IsOpen() {
		chatInput = m.chat.InputLine()
	}

	// Portal wipe parameters (entry swallow / exit fade).
	var pActive, pExit bool
	var pProgress float64
	var pColor canvas.Color
	if m.portal != nil {
		pActive, pExit = true, m.portal.exiting
		dur := portalEnterDur
		if pExit {
			dur = portalExitDur
		}
		pProgress = time.Since(m.portal.start).Seconds() / dur
		if pProgress > 1 {
			pProgress = 1
		}
		pColor = plaza.AccentColor(m.portal.key, 0.55)
	}

	m.renderer.Submit(frameSnapshot{
		termW: m.termW, termH: m.termH,
		cellW: m.cellW, cellH: m.cellH,
		players: players, selfID: m.player.ID,
		chat:           append([]chat.Entry(nil), m.chat.History()...),
		toast:          m.toasts.Message(),
		panelLines:     panel,
		chatInput:      chatInput,
		chatOpen:       m.chat.IsOpen(),
		unreadName:     unreadName,
		unreadN:        unreadN,
		coins:          m.player.Coins,
		shopPrompt:     shopPrompt,
		interactPrompt: interactPrompt,
		portalActive:   pActive,
		portalProgress: pProgress,
		portalExiting:  pExit,
		portalColor:    pColor,
	})
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
		m.publish()
		return m, nil

	case PixelSizeMsg:
		// Derive cell size from the consistent (cols/rows, pixels) pair. Cell
		// size is stable across window resizes, so this won't fight the
		// separate WindowSizeMsg.
		if msg.Cols > 0 && msg.Rows > 0 && msg.W > 0 && msg.H > 0 {
			m.cellW, m.cellH = max(1, msg.W/msg.Cols), max(1, msg.H/msg.Rows)
		}
		m.publish()
		return m, nil

	case CellSizeMsg:
		if msg.W > 0 && msg.H > 0 {
			m.cellW, m.cellH = msg.W, msg.H
		}
		m.publish()
		return m, nil

	case animTickMsg:
		m.toasts.Tick(time.Time(msg))
		// Flip back to the idle pose a beat after the last step, and tell peers.
		if m.moving && time.Since(m.lastMove) > idleAfter {
			m.moving = false
			m.handle.Move(hub.Pos{X: m.px, Y: m.py}, m.dir, false)
		}
		m.accrueCoins()
		m.pruneEmotes()
		// Drive the portal transition: once the entry wipe has covered the
		// screen, fire EnterPortalMsg; clear a finished exit fade.
		if m.portal != nil {
			if m.portal.exiting {
				if time.Since(m.portal.start).Seconds() >= portalExitDur {
					m.portal = nil
				}
			} else if !m.portal.emitted && time.Since(m.portal.start).Seconds() >= portalEnterDur {
				m.portal.emitted = true
				key, name := m.portal.key, m.portal.name
				m.publish()
				return m, tea.Batch(animTick(), func() tea.Msg { return EnterPortalMsg{Key: key, Name: name} })
			}
		}
		m.publish()
		return m, animTick()

	case moveTickMsg:
		next, cmd := m.tickMove()
		next.publish()
		next.renderer.Kick()
		return next, cmd

	case hubEventMsg:
		next, cmd := m.applyEvent(msg.ev)
		next.publish()
		return next, tea.Batch(cmd, listenHub(next.handle.Events()))

	case hubClosedMsg:
		return m, func() tea.Msg { return DisconnectMsg{Reason: "connection closed"} }

	case tea.KeyMsg:
		next, cmd := m.handleKey(msg)
		next.publish()
		next.renderer.Kick() // render the result of the input now, not on the next tick
		return next, cmd
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

	// An incoming duel challenge is modal: answer it before anything else.
	if m.challenge != nil {
		return m.updateChallenge(key)
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

	if m.wardrobe.IsOpen() {
		return m.updateWardrobe(key)
	}

	if m.shop.IsOpen() {
		return m.updateShop(key)
	}

	if m.emoteMenu {
		return m.updateEmoteMenu(key)
	}

	if m.inspecting != nil {
		return m.updateInspect(key)
	}

	// Plaza focus.
	switch key.String() {
	case "q":
		return m, func() tea.Msg { return DisconnectMsg{Reason: "bye"} }
	case "g":
		m.emoteMenu = true
		return m, nil
	case "x":
		if st, ok := m.nearestPlayer(); ok {
			m.openInspect(st)
		}
		return m, nil
	case "e":
		if m.world.NearShop(m.px, m.py/2) {
			m.shop.Open(m.ownedCosmetics(), m.player.Coins)
		}
		return m, nil
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
	case "c":
		m.wardrobe.Open(m.ownedCosmetics(), m.player.Cosmetic)
		return m, nil
	}
	// Movement. The press updates the held intent; pressMove steps instantly on
	// a fresh press and the moveTick chain carries a hold at a steady pace.
	// Diagonals have dedicated keys since key-repeat only repeats the last key;
	// Shift (or a capital letter) runs.
	if dx, dy, run, ok := parseMove(key.String()); ok {
		return m.pressMove(dx, dy, run)
	}
	return m, nil
}

// pressMove handles a movement key event. The first press of a fresh walk steps
// at once (so input feels instant) and starts the tick chain; a key-repeat of
// the current direction only refreshes liveness (the tick paces the walk, so
// speed never depends on the OS repeat rate); a change of direction responds
// immediately.
func (m Model) pressMove(dx, dy int, run bool) (Model, tea.Cmd) {
	now := time.Now()
	switch {
	case !m.ticking:
		// Fresh walk: step now (instant) and start the tick chain.
		m.intent = moveIntent{dx: dx, dy: dy, run: run, seen: now, count: 1}
		m.ticking = true
		next, cmd := m.step(dx, dy, run)
		if next.ticking { // a portal step ends the walk; don't start a chain
			cmd = tea.Batch(cmd, moveTick())
		}
		return next, cmd
	case dx == m.intent.dx && dy == m.intent.dy:
		// Key-repeat of the held direction: refresh liveness; the tick paces the
		// walk, so don't step here (keeps speed off the OS repeat rate).
		m.intent.seen = now
		m.intent.run = run
		m.intent.count++
		return m, nil
	default:
		// Direction change: respond at once; the tick chain is already live.
		m.intent = moveIntent{dx: dx, dy: dy, run: run, seen: now, count: 1}
		return m.step(dx, dy, run)
	}
}

// tickMove is one beat of the held-walk chain: it steps in the current
// direction while the key is still live, and otherwise stops and goes idle.
func (m Model) tickMove() (Model, tea.Cmd) {
	if !m.ticking {
		return m, nil
	}
	// A panel or the chat console stole focus — stop walking.
	if m.chat.IsOpen() || m.inv.IsOpen() || m.friends.IsOpen() || m.shop.IsOpen() || m.emoteMenu || m.inspecting != nil || m.challenge != nil {
		return m.stopWalk(), nil
	}
	window := tapWindow
	if m.intent.count >= 2 {
		window = holdSteady // repeats have begun: this is a real hold
	}
	if time.Since(m.intent.seen) > window {
		return m.stopWalk(), nil // released (or the tap is done)
	}
	next, cmd := m.step(m.intent.dx, m.intent.dy, m.intent.run)
	if !next.ticking {
		return next, cmd
	}
	return next, tea.Batch(cmd, moveTick())
}

// stopWalk ends the tick chain and broadcasts the idle pose.
func (m Model) stopWalk() Model {
	m.ticking = false
	if m.moving {
		m.moving = false
		m.handle.Move(hub.Pos{X: m.px, Y: m.py}, m.dir, false)
	}
	return m
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

// updateWardrobe routes a key to the wardrobe panel and, when the player
// equips something, persists it locally, in storage and to the hub.
func (m Model) updateWardrobe(msg tea.Msg) (Model, tea.Cmd) {
	cmd := m.wardrobe.Update(msg)
	if key, ok := m.wardrobe.TakeEquipped(); ok {
		m.player.Cosmetic = key
		_ = m.repos.Players.SetCosmetic(m.player.ID, key)
		m.handle.SetCosmetic(key)
		m.toasts.Show("now wearing: " + cosmetic.Name(key))
	}
	return m, cmd
}

// updateShop routes a key to the shop panel and, when the player buys
// something, charges their coins, grants the cosmetic and refreshes the panel.
func (m Model) updateShop(msg tea.Msg) (Model, tea.Cmd) {
	cmd := m.shop.Update(msg)
	if key, ok := m.shop.TakePurchase(); ok {
		m.buyCosmetic(key)
		m.shop.SetState(m.ownedCosmetics(), m.player.Coins)
	}
	return m, cmd
}

// buyCosmetic performs a purchase: it re-checks ownership, atomically debits the
// price (so a slow connection can't double-spend), grants the cosmetic as an
// inventory item and reports the outcome through a toast.
func (m *Model) buyCosmetic(key string) {
	if !cosmetic.Valid(key) {
		return
	}
	if m.ownedCosmetics()[key] {
		m.toasts.Show("you already own that")
		return
	}
	price := cosmetic.Price(key)
	if price <= 0 {
		m.toasts.Show("not for sale")
		return
	}
	ok, balance, err := m.repos.Players.SpendCoins(m.player.ID, price)
	if err != nil {
		m.toasts.Show("the till is jammed — try again")
		return
	}
	m.player.Coins = balance
	if !ok {
		m.toasts.Show("not enough coins for " + cosmetic.Name(key))
		return
	}
	if err := m.repos.Inventory.Grant(m.player.ID, "plaza", cosmetic.InventoryPrefix+key, cosmetic.Name(key), 1); err != nil {
		// The coins are already gone; refund so the player isn't out of pocket.
		if bal, rerr := m.repos.Players.AddCoins(m.player.ID, price); rerr == nil {
			m.player.Coins = bal
		}
		m.toasts.Show("the shelf was empty — refunded")
		return
	}
	m.toasts.Show("bought " + cosmetic.Name(key) + " — wear it with c")
}

// accrueCoins credits the slow passive income for being in the plaza, persisting
// each coin as it lands. It awards every whole coinEvery that has elapsed (so a
// brief stall doesn't lose income) and keeps the local balance in sync for the
// HUD and shop.
func (m *Model) accrueCoins() {
	awarded := 0
	for time.Since(m.lastCoinAt) >= coinEvery {
		m.lastCoinAt = m.lastCoinAt.Add(coinEvery)
		awarded += coinsPerAward
	}
	if awarded == 0 {
		return
	}
	if balance, err := m.repos.Players.AddCoins(m.player.ID, awarded); err == nil {
		m.player.Coins = balance
	} else {
		m.player.Coins += awarded // keep the HUD honest even if the write hiccups
	}
}

// activeEmote returns a player's in-flight gesture key, or "" if none is
// playing or it has expired.
func (m Model) activeEmote(id int64) string {
	e, ok := m.emotes[id]
	if !ok || time.Since(e.start).Seconds() > emote.Dur {
		return ""
	}
	return e.kind
}

// pruneEmotes drops gestures that have finished, so the map can't grow without
// bound as players come and go.
func (m *Model) pruneEmotes() {
	for id, e := range m.emotes {
		if time.Since(e.start).Seconds() > emote.Dur {
			delete(m.emotes, id)
		}
	}
}

// playEmote starts a gesture locally and broadcasts it. The hub echoes it back,
// but recording it now makes our own emote show instantly.
func (m *Model) playEmote(kind string) {
	if !emote.Valid(kind) {
		return
	}
	m.emotes[m.player.ID] = emoteState{kind: kind, start: time.Now()}
	m.handle.PlayEmote(kind)
}

// updateEmoteMenu handles the quick emote picker: a number plays that gesture
// and closes the menu; Esc/g closes it.
func (m Model) updateEmoteMenu(key tea.KeyMsg) (Model, tea.Cmd) {
	s := key.String()
	switch s {
	case "esc", "g", "q":
		m.emoteMenu = false
		return m, nil
	}
	all := emote.All()
	if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		if i := int(s[0] - '1'); i < len(all) {
			m.playEmote(all[i].Key)
			m.emoteMenu = false
		}
	}
	return m, nil
}

// emoteMenuLines is the quick-picker panel content for the renderer.
func emoteMenuLines() []string {
	out := []string{"Emotes", ""}
	for i, e := range emote.All() {
		out = append(out, "  "+strconv.Itoa(i+1)+"  "+e.Verb)
	}
	return append(out, "", "1-8 play  Esc close")
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// nearestPlayer returns the closest remote player within interactRadius cells of
// the local avatar (Chebyshev distance), for greeting/inspecting.
func (m Model) nearestPlayer() (hub.PlayerState, bool) {
	sx, sy := m.px, m.py/2
	best := interactRadius + 1
	var found hub.PlayerState
	ok := false
	for _, st := range m.remotes {
		dx, dy := abs(sx-st.Pos.X), abs(sy-st.Pos.Y/2)
		d := dx
		if dy > d {
			d = dy
		}
		if d <= interactRadius && d < best {
			best, found, ok = d, st, true
		}
	}
	return found, ok
}

// openInspect builds the inspect card for a remote player, pulling their join
// date from storage (only their public identity is broadcast live).
func (m *Model) openInspect(st hub.PlayerState) {
	key := st.Info.Cosmetic
	card := &inspectInfo{
		id:           st.Info.ID,
		name:         st.Info.Name,
		cosmeticName: cosmetic.Name(key),
	}
	if cosmetic.Valid(key) && key != "" && key != "none" {
		card.cosmeticTier = cosmetic.RarityOf(key).Label()
	}
	if p, err := m.repos.Players.ByID(st.Info.ID); err == nil && p != nil {
		card.since = p.CreatedAt.Format("Jan 2006")
	}
	m.inspecting = card
}

// updateInspect handles keys while a player's card is open: w pre-fills a
// whisper, f friends them, v challenges them to a Shellmon duel, Esc/x closes.
func (m Model) updateInspect(key tea.KeyMsg) (Model, tea.Cmd) {
	card := m.inspecting
	switch key.String() {
	case "esc", "x", "q":
		m.inspecting = nil
	case "w":
		m.inspecting = nil
		return m, m.chat.OpenWith("/w " + card.name + " ")
	case "f":
		if err := m.repos.Friends.Add(m.player.ID, card.id); err == nil {
			m.toasts.Show("✓ friend added: " + card.name)
		} else {
			m.toasts.Show("could not add friend")
		}
		m.inspecting = nil
	case "v":
		m.sendChallenge(card.id, card.name)
		m.inspecting = nil
	}
	return m, nil
}

// shellmonTeam loads this player's saved Shellmon roster.
func (m Model) shellmonTeam() []*mon.Creature {
	data, err := m.repos.Saves.Load(m.player.ID, shellmonWorldKey)
	if err != nil {
		return nil
	}
	return mon.UnmarshalParty(data)
}

// sendChallenge invites another player to a Shellmon duel, attaching our team.
func (m *Model) sendChallenge(toID int64, name string) {
	team := m.shellmonTeam()
	if len(team) == 0 {
		m.toasts.Show("catch some Shellmon first — visit the portal")
		return
	}
	if ok, reason := m.handle.ChallengePvP(toID, team); ok {
		m.toasts.Show("challenge sent to " + name)
	} else {
		m.toasts.Show(reason)
	}
}

// updateChallenge handles the incoming-duel prompt: y accepts, n/esc declines.
func (m Model) updateChallenge(key tea.KeyMsg) (Model, tea.Cmd) {
	ch := m.challenge
	switch key.String() {
	case "y", "enter":
		team := m.shellmonTeam()
		if len(team) == 0 {
			m.toasts.Show("you have no Shellmon to battle with")
			m.handle.RespondPvP(ch.fromID, false, nil)
		} else {
			m.handle.RespondPvP(ch.fromID, true, team)
		}
		m.challenge = nil
	case "n", "esc", "q":
		m.handle.RespondPvP(ch.fromID, false, nil)
		m.challenge = nil
	}
	return m, nil
}

// inspectLines is the card content for the renderer.
func (m Model) inspectLines() []string {
	c := m.inspecting
	out := []string{c.name, ""}
	wearing := "wearing: " + c.cosmeticName
	if c.cosmeticTier != "" {
		wearing += " (" + c.cosmeticTier + ")"
	}
	out = append(out, wearing)
	if c.since != "" {
		out = append(out, "wandering since "+c.since)
	}
	return append(out, "", "w whisper · f friend · v duel · Esc close")
}

// ownedCosmetics returns the set of unlockable cosmetic keys the player owns,
// derived from inventory items granted under the "cosmetic." prefix. Starter
// pieces are always available and need not appear here.
func (m Model) ownedCosmetics() map[string]bool {
	owned := make(map[string]bool)
	items, err := m.repos.Inventory.Items(m.player.ID)
	if err != nil {
		return owned
	}
	for _, it := range items {
		if strings.HasPrefix(it.ItemKey, cosmetic.InventoryPrefix) {
			owned[strings.TrimPrefix(it.ItemKey, cosmetic.InventoryPrefix)] = true
		}
	}
	return owned
}

// parseMove maps a key string to a movement vector in whole grid tiles (dx, dy
// each in {-1, 0, 1}) and whether to run. step() converts the tile delta into
// the half-row coordinate space. Cardinals are WASD/arrows, diagonals are the
// roguelike y/u/b/n cluster, and Shift (reported as "shift+…" or an uppercase
// letter) runs. ok is false for non-movement keys.
func parseMove(s string) (dx, dy int, run, ok bool) {
	if strings.HasPrefix(s, "shift+") {
		run = true
		s = strings.TrimPrefix(s, "shift+")
	}
	lower := strings.ToLower(s)
	if s != lower {
		run = true // an uppercase letter means Shift was held
	}
	switch lower {
	case "up", "w":
		dy = -1
	case "down", "s":
		dy = 1
	case "left", "a":
		dx = -1
	case "right", "d":
		dx = 1
	case "y":
		dx, dy = -1, -1
	case "u":
		dx, dy = 1, -1
	case "b":
		dx, dy = -1, 1
	case "n":
		dx, dy = 1, 1
	default:
		return 0, 0, false, false
	}
	return dx, dy, run, true
}

// step applies one movement input immediately, moving a whole tile at a time
// (up to two tiles when running) so the avatar always lands square on the floor
// grid the plaza is drawn on. Collision is axis-separated so walls let you
// slide along them; it faces the direction of effort even when blocked, and
// fires the portal trigger on entering a mouth.
func (m Model) step(dx, dy int, run bool) (Model, tea.Cmd) {
	tiles := 1
	if run {
		tiles = 2
	}
	// py is measured in half-rows (two per cell row), so one whole tile of
	// vertical travel is two units. Stepping by a full tile keeps the avatar
	// centered on a floor tile instead of straddling two.
	stepY := dy * 2
	moved := false
	for i := 0; i < tiles; i++ {
		adv := false
		if dx != 0 && !m.world.Blocked(m.px+dx, m.py/2) {
			m.px += dx
			adv = true
		}
		if stepY != 0 && !m.world.Blocked(m.px, (m.py+stepY)/2) {
			m.py += stepY
			adv = true
		}
		if !adv {
			break // ran into a wall; stop short
		}
		moved = true
	}

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

	if !moved {
		return m, nil // blocked: faced the wall, didn't budge
	}

	m.walkCount++
	m.moving = true
	m.lastMove = time.Now()
	m.handle.Move(hub.Pos{X: m.px, Y: m.py}, m.dir, true)

	// Portal trigger: fires on entering a mouth, not while standing in one (so
	// returning from a world doesn't immediately re-enter).
	if p, ok := plaza.PortalAt(m.px, m.py/2); ok {
		if !m.onPortal {
			m.onPortal = true
			m.moving = false
			m.ticking = false // entering a world ends the walk; let the chain die
			m.toasts.Show("✦ " + p.Name + " ✦")
			// Start the portal wipe; the EnterPortalMsg fires once it has
			// swallowed the screen (driven by the anim tick).
			m.portal = &portalFX{key: p.Key, name: p.Name, start: time.Now()}
		}
	} else {
		m.onPortal = false
	}
	return m, nil
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

	case hub.EvEmote:
		if emote.Valid(ev.Kind) {
			m.emotes[ev.PlayerID] = emoteState{kind: ev.Kind, start: time.Now()}
		}

	case hub.EvBattleChallenge:
		m.challenge = &pvpChallenge{fromID: ev.From.ID, name: ev.From.Name}
		m.toasts.Show(ev.From.Name + " challenges you!")

	case hub.EvBattleDeclined:
		m.toasts.Show(ev.Reason)

	case hub.EvBattleStart:
		m.challenge = nil
		match, sideA, opp := ev.Match, ev.SideA, ev.Opponent.Name
		return m, func() tea.Msg {
			return StartPvPMsg{Match: match, SideA: sideA, Opponent: opp}
		}

	case hub.EvKick:
		return m, func() tea.Msg { return DisconnectMsg{Reason: ev.Reason} }
	}
	return m, nil
}

// ResumeFromWorld is called by the session app when the player exits a portal
// world: it re-arms rendering (re-priming interpolation) and refreshes
// presence-dependent UI.
func (m Model) ResumeFromWorld() Model {
	m.renderer.SetActive(true)
	// Fade the plaza back in from the portal's colour.
	key := ""
	if m.portal != nil {
		key = m.portal.key
	}
	m.portal = &portalFX{key: key, exiting: true, start: time.Now()}
	// Don't pay out a lump for time spent inside the world; restart the clock.
	m.lastCoinAt = time.Now()
	// The player may have just earned a reward cosmetic in there; pick up the
	// fresh coin balance too in case another session credited it.
	if p, err := m.repos.Players.ByID(m.player.ID); err == nil && p != nil {
		m.player.Coins = p.Coins
	}
	return m
}

// Player returns the logged-in player this overworld belongs to.
func (m Model) Player() storage.Player { return m.player }
