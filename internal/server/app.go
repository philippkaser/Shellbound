package server

import (
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"

	"github.com/shellbound/shellbound/internal/hub"
	"github.com/shellbound/shellbound/internal/plaza"
	"github.com/shellbound/shellbound/internal/storage"
	"github.com/shellbound/shellbound/internal/style"
	"github.com/shellbound/shellbound/internal/ui/login"
	"github.com/shellbound/shellbound/internal/ui/overworld"
	"github.com/shellbound/shellbound/internal/world"
	shellmonworld "github.com/shellbound/shellbound/internal/worlds/shellmon"
)

// appState is which top-level screen owns input.
type appState int

const (
	stateLogin appState = iota
	statePlaza
	stateWorld
)

// worldExitMsg is delivered when a portal world calls Context.Exit. The
// generation guards against a stale Exit closure firing after the player
// already left and re-entered.
type worldExitMsg struct{ gen int }

// appDeps are the session app's shared collaborators.
type appDeps struct {
	hub        *hub.Hub
	repos      *storage.Repos
	registry   *world.Registry
	plazaMap   *plaza.Map
	env        overworld.Env
	sessionID  string
	onTeardown func(func())
}

// app is the root bubbletea model for one SSH session. It is a pointer
// model: Update mutates in place.
type app struct {
	deps        appDeps
	theme       style.Theme
	fingerprint string

	state  appState
	login  login.Model
	over   overworld.Model
	joined bool
	handle *hub.Handle

	worldModel tea.Model
	worldGen   int

	// worldStop halts the current world's background renderer. One session
	// teardown reads it (under the mutex) instead of appending a teardown
	// closure per world entry, so re-entering portals doesn't accumulate
	// stale Stop funcs over dead models for the life of the session.
	worldStopMu sync.Mutex
	worldStop   func()

	// internal carries world-exit notifications; done unblocks the
	// listener when the session ends so its goroutine never leaks.
	internal chan tea.Msg
	done     chan struct{}

	lastSize  tea.WindowSizeMsg
	lastPixel overworld.PixelSizeMsg
	lastCell  overworld.CellSizeMsg
}

// newApp builds the session model. player is nil on first connect, which
// routes through the username picker.
func newApp(deps appDeps, theme style.Theme, fingerprint string, player *storage.Player) *app {
	a := &app{
		deps:        deps,
		theme:       theme,
		fingerprint: fingerprint,
		internal:    make(chan tea.Msg, 4),
		done:        make(chan struct{}),
	}
	closeOnce := func() {
		select {
		case <-a.done:
		default:
			close(a.done)
		}
	}
	deps.onTeardown(closeOnce)
	deps.onTeardown(a.stopWorld) // halt whichever world renderer is live at disconnect

	if player != nil {
		a.join(*player)
	} else {
		a.state = stateLogin
		a.login = login.New(theme, deps.repos.Players, fingerprint)
	}
	return a
}

// join registers with the hub and builds the overworld.
func (a *app) join(player storage.Player) {
	spawn := hub.Pos{X: a.deps.plazaMap.SpawnX, Y: a.deps.plazaMap.SpawnY*2 + 1}
	info := hub.PlayerInfo{ID: player.ID, Name: player.Username, Color: player.Color, Cosmetic: player.Cosmetic}
	handle, snapshot := a.deps.hub.Join(a.deps.sessionID, info, spawn)
	a.handle = handle
	a.over = overworld.New(a.theme, a.deps.plazaMap, a.deps.env, a.deps.repos, player, handle, snapshot)
	a.joined = true
	a.state = statePlaza
	// Ensure the overworld's background render goroutine is stopped with the
	// session (the renderer pointer is stable across model copies).
	a.deps.onTeardown(a.over.StopRenderer)
}

// listenInternal waits for the next internal message; the done channel
// guarantees the goroutine exits with the session.
func (a *app) listenInternal() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-a.internal:
			return msg
		case <-a.done:
			return nil
		}
	}
}

// Init implements tea.Model.
func (a *app) Init() tea.Cmd {
	cmds := []tea.Cmd{a.listenInternal()}
	switch a.state {
	case stateLogin:
		cmds = append(cmds, a.login.Init())
	case statePlaza:
		cmds = append(cmds, a.over.Init())
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.lastSize = msg
		// Everyone gets sizes so screens are correct the moment they show.
		var cmds []tea.Cmd
		if a.joined {
			var cmd tea.Cmd
			a.over, cmd = a.over.Update(msg)
			cmds = append(cmds, cmd)
		}
		if a.state == stateLogin {
			var cmd tea.Cmd
			a.login, cmd = a.login.Update(msg)
			cmds = append(cmds, cmd)
		}
		if a.worldModel != nil {
			var cmd tea.Cmd
			a.worldModel, cmd = a.worldModel.Update(msg)
			cmds = append(cmds, cmd)
		}
		return a, tea.Batch(cmds...)

	case overworld.PixelSizeMsg:
		// Only the plaza renderer cares about pixel dimensions.
		a.lastPixel = msg
		if a.joined {
			var cmd tea.Cmd
			a.over, cmd = a.over.Update(msg)
			return a, cmd
		}
		return a, nil

	case overworld.CellSizeMsg:
		a.lastCell = msg
		if a.joined {
			var cmd tea.Cmd
			a.over, cmd = a.over.Update(msg)
			return a, cmd
		}
		return a, nil

	case login.DoneMsg:
		if msg.Player == nil {
			return a, tea.Quit
		}
		a.join(*msg.Player)
		// Wipe the login screen before the plaza's Sixel frame paints over it.
		cmds := []tea.Cmd{tea.ClearScreen, a.over.Init()}
		if a.lastSize.Width > 0 {
			var cmd tea.Cmd
			a.over, cmd = a.over.Update(a.lastSize)
			cmds = append(cmds, cmd)
		}
		if a.lastPixel.W > 0 {
			var cmd tea.Cmd
			a.over, cmd = a.over.Update(a.lastPixel)
			cmds = append(cmds, cmd)
		}
		if a.lastCell.W > 0 {
			var cmd tea.Cmd
			a.over, cmd = a.over.Update(a.lastCell)
			cmds = append(cmds, cmd)
		}
		return a, tea.Batch(cmds...)

	case login.QuitMsg:
		return a, tea.Quit

	case overworld.DisconnectMsg:
		a.leave()
		return a, tea.Quit

	case overworld.EnterPortalMsg:
		return a.enterWorld(msg)

	case overworld.StartPvPMsg:
		return a.enterPvP(msg)

	case worldExitMsg:
		if msg.gen == a.worldGen && a.state == stateWorld {
			a.state = statePlaza
			a.stopWorld() // halt the world's renderer before we drop it
			a.worldModel = nil
			if a.handle != nil {
				a.handle.SetBusy(false)
			}
			a.over = a.over.ResumeFromWorld()
			// Clear the world's text screen, then repaint the plaza frame.
			var cmd tea.Cmd
			a.over, cmd = a.over.Update(a.lastSize)
			return a, tea.Batch(a.listenInternal(), tea.ClearScreen, cmd)
		}
		return a, a.listenInternal()

	case tea.KeyMsg:
		// Global escape hatch, regardless of focus.
		if msg.String() == "ctrl+c" {
			a.leave()
			return a, tea.Quit
		}
		return a.routeKey(msg)
	}

	// Ticks, hub events, cursor blinks: the overworld must stay current
	// even while a portal world is on screen.
	var cmds []tea.Cmd
	if a.joined {
		var cmd tea.Cmd
		a.over, cmd = a.over.Update(msg)
		cmds = append(cmds, cmd)
	}
	if a.state == stateLogin {
		var cmd tea.Cmd
		a.login, cmd = a.login.Update(msg)
		cmds = append(cmds, cmd)
	}
	if a.state == stateWorld && a.worldModel != nil {
		var cmd tea.Cmd
		a.worldModel, cmd = a.worldModel.Update(msg)
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...)
}

// routeKey sends a key press to whichever screen has focus.
func (a *app) routeKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch a.state {
	case stateLogin:
		var cmd tea.Cmd
		a.login, cmd = a.login.Update(key)
		return a, cmd
	case stateWorld:
		if a.worldModel != nil {
			var cmd tea.Cmd
			a.worldModel, cmd = a.worldModel.Update(key)
			return a, cmd
		}
		return a, nil
	default:
		var cmd tea.Cmd
		a.over, cmd = a.over.Update(key)
		return a, cmd
	}
}

// enterWorld swaps in the portal world's model.
func (a *app) enterWorld(msg overworld.EnterPortalMsg) (tea.Model, tea.Cmd) {
	w, ok := a.deps.registry.Get(msg.Key)
	if !ok {
		log.Warn("portal references unknown world", "key", msg.Key)
		return a, nil
	}
	player := a.overPlayer()
	a.worldGen++
	gen := a.worldGen
	exit := func() {
		select {
		case a.internal <- worldExitMsg{gen: gen}:
		default:
		}
	}
	cw, ch := a.cellSize()
	ctx := world.Context{
		Player:    player,
		Save:      world.NewSaveStore(a.deps.repos.Saves, player.ID, w.Key()),
		Inventory: world.NewInventoryAPI(a.deps.repos.Inventory, player.ID, w.Key()),
		Render:    world.Render{Palette: a.deps.env.Pal, Out: a.deps.env.Out, CellW: cw, CellH: ch},
		Exit:      exit,
	}
	a.worldModel = w.Init(ctx)
	// A graphical world renders on its own; track its Stop so the session
	// teardown (or the exit back to the plaza) halts it.
	a.setWorldStop(a.worldModel)
	a.state = stateWorld
	if a.handle != nil {
		a.handle.SetBusy(true) // can't be challenged while inside a world
	}
	// The plaza must stop painting Sixel frames while the world owns the
	// screen; clear its last frame so the world's text renders cleanly.
	a.over = a.over.SetActive(false)

	cmds := []tea.Cmd{tea.ClearScreen, a.worldModel.Init()}
	if a.lastSize.Width > 0 {
		var cmd tea.Cmd
		a.worldModel, cmd = a.worldModel.Update(a.lastSize)
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...)
}

// enterPvP swaps in a shared Shellmon duel when a challenge is accepted.
func (a *app) enterPvP(msg overworld.StartPvPMsg) (tea.Model, tea.Cmd) {
	a.worldGen++
	gen := a.worldGen
	exit := func() {
		select {
		case a.internal <- worldExitMsg{gen: gen}:
		default:
		}
	}
	cw, ch := a.cellSize()
	render := world.Render{Palette: a.deps.env.Pal, Out: a.deps.env.Out, CellW: cw, CellH: ch}
	a.worldModel = shellmonworld.NewPvP(render, msg.Match, msg.SideA, msg.Opponent, exit)
	a.setWorldStop(a.worldModel)
	a.state = stateWorld
	if a.handle != nil {
		a.handle.SetBusy(true)
	}
	a.over = a.over.SetActive(false)

	cmds := []tea.Cmd{tea.ClearScreen, a.worldModel.Init()}
	if a.lastSize.Width > 0 {
		var cmd tea.Cmd
		a.worldModel, cmd = a.worldModel.Update(a.lastSize)
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...)
}

// setWorldStop records the current world's Stop func (if it has one) for the
// exit path and the session teardown. Safe against the teardown goroutine.
func (a *app) setWorldStop(m tea.Model) {
	a.worldStopMu.Lock()
	defer a.worldStopMu.Unlock()
	if s, ok := m.(interface{ Stop() }); ok {
		a.worldStop = s.Stop
	} else {
		a.worldStop = nil
	}
}

// stopWorld halts the current world's renderer, if any, exactly once.
func (a *app) stopWorld() {
	a.worldStopMu.Lock()
	stop := a.worldStop
	a.worldStop = nil
	a.worldStopMu.Unlock()
	if stop != nil {
		stop()
	}
}

// overPlayer extracts the player identity for world contexts.
func (a *app) overPlayer() world.PlayerInfo {
	p := a.over.Player()
	return world.PlayerInfo{ID: p.ID, Username: p.Username, Color: p.Color}
}

// cellSize returns the best-known terminal cell size in pixels, preferring a
// consistent pixel report, then the CSI 16t query reply, then the env fallback.
func (a *app) cellSize() (int, int) {
	if a.lastPixel.W > 0 && a.lastPixel.H > 0 && a.lastPixel.Cols > 0 && a.lastPixel.Rows > 0 {
		return max(1, a.lastPixel.W/a.lastPixel.Cols), max(1, a.lastPixel.H/a.lastPixel.Rows)
	}
	if a.lastCell.W > 0 && a.lastCell.H > 0 {
		return a.lastCell.W, a.lastCell.H
	}
	return a.deps.env.CellW, a.deps.env.CellH
}

// leave deregisters from the hub (idempotent).
func (a *app) leave() {
	if a.handle != nil {
		a.handle.Leave()
	}
}

// View implements tea.Model.
func (a *app) View() string {
	switch a.state {
	case stateLogin:
		return a.login.View()
	case stateWorld:
		if a.worldModel != nil {
			return a.worldModel.View()
		}
		return ""
	default:
		return a.over.View()
	}
}

// noticeModel shows a single message and exits on any key (or after a
// grace period) — used for pre-login failures.
type noticeModel struct {
	theme style.Theme
	text  string
	w, h  int
}

// newNoticeModel builds a noticeModel.
func newNoticeModel(theme style.Theme, text string) noticeModel {
	return noticeModel{theme: theme, text: text}
}

type noticeTimeoutMsg struct{}

// Init implements tea.Model.
func (n noticeModel) Init() tea.Cmd {
	return tea.Tick(6*time.Second, func(time.Time) tea.Msg { return noticeTimeoutMsg{} })
}

// Update implements tea.Model.
func (n noticeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		n.w, n.h = msg.Width, msg.Height
		return n, nil
	case noticeTimeoutMsg:
		return n, tea.Quit
	case tea.KeyMsg:
		return n, tea.Quit
	}
	return n, nil
}

// View implements tea.Model.
func (n noticeModel) View() string {
	card := n.theme.PanelBorder.Render(n.theme.Text.Render(n.text))
	if n.w > 0 && n.h > 0 {
		return lipgloss.Place(n.w, n.h, lipgloss.Center, lipgloss.Center, card)
	}
	return card
}
