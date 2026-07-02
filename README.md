# Shellbound

A tiny MMO that lives entirely inside your terminal. Connect over SSH and
you're standing in a shared plaza: walk around, watch other players wander
past, chat, whisper, make friends. The plaza is drawn as **isometric pixel
art** using real terminal graphics (Sixel) — strict black-and-white, with
exactly three splashes of color (player names, chat usernames, and each portal
shimmering as an isometric pool of its world's own signature hue) and interactive lighting that
follows you and pools around the lamps. Fireflies drift and the fountain spits
droplets.

```
ssh -p 80 your-server
```

No account. No password. Your SSH key *is* your identity: the first visit
asks for a username; every visit after that drops you straight into the
plaza, wearing the color deterministically derived from your key.

## Running a server

Requires Go 1.22+ and nothing else (the SQLite driver is pure Go — no CGO).

```sh
make run           # listens on :80, creates ./shellbound.db and a host key
```

Configuration is via environment variables:

| Variable             | Default                       | Meaning                |
| -------------------- | ----------------------------- | ---------------------- |
| `SHELLBOUND_ADDR`    | `:80`                       | SSH listen address     |
| `SHELLBOUND_DB`      | `./shellbound.db`             | SQLite database file   |
| `SHELLBOUND_HOSTKEY` | `./.ssh/shellbound_ed25519`   | Host key (auto-created)|
| `SHELLBOUND_SIXEL`   | `on`                          | Set `off` to serve a "use a Sixel terminal" notice instead of graphics |
| `SHELLBOUND_CELL`    | `8x16`                        | Fallback cell size in pixels (`WxH`) used only when the client doesn't report pixel dimensions; otherwise the real size is derived from the PTY |

Other targets: `make build`, `make test`, `make vet`, `make hostkey`.

### Keeping it running after you log out

`make run` (and `go run`) runs in the foreground and dies when your SSH
session ends. To choose how to run it — and keep it up after logout — use
the launcher:

```sh
./scripts/run.sh          # interactive menu; default keeps it running, no service
```

The default (option 1) builds the binary and starts it **detached** with
`nohup`, so it survives logout without installing anything system-wide. It
writes `./shellbound.log` and a `./shellbound.pid`. Drive it directly too:

```sh
make start     # background, survives logout (no service)   ← the default
make status    # is it running?
make logs      # follow the log
make stop      # stop it
make run       # foreground (dev; stops on logout)
```

If you'd rather have it managed — surviving reboots and auto-restarting on
crash — install it as a **systemd service** (as root, from the checkout):

```sh
make service     # builds, installs a unit (from deploy/shellbound.service), enables + starts it
systemctl status shellbound
journalctl -u shellbound -f      # follow logs
make service                     # rebuild + restart to deploy code changes
make unservice                   # stop and remove the unit (keeps the database)
```

No systemd and prefer a terminal multiplexer? `tmux new -d -s shellbound
'make run'` also survives logout (reattach with `tmux attach -t
shellbound`).

Your terminal **must support Sixel graphics** and 24-bit color, and should use
a dark/black background. Known-good clients: WezTerm, foot, mlterm, Konsole,
contour, recent Windows Terminal, and `xterm -ti vt340`. Sixel support cannot
be reliably auto-detected over the SSH input path, so the server assumes it is
present; set `SHELLBOUND_SIXEL=off` for a deployment whose users lack it and
everyone is shown a short "connect with a Sixel terminal" notice instead.

## Controls

| Key            | Action                                  |
| -------------- | --------------------------------------- |
| WASD / arrows  | Walk (each press steps; hold to repeat) |
| `y` `u` `b` `n`| Walk diagonally (NW / NE / SW / SE)     |
| `Shift` + move | Run (two tiles per step)                |
| `Enter`        | Open chat — `Enter` sends, `Esc` cancels|
| `g`            | Emote picker (or `/wave`, `/dance`, …)  |
| `x`            | Greet/inspect a nearby player           |
| `i`            | Inventory                               |
| `c`            | Wardrobe (equip cosmetic headwear)      |
| `e`            | Shop (when standing by the plaza stall) |
| `f`            | Friends list (Enter to message someone) |
| `Esc`          | Close any panel                         |
| `q` / `Ctrl+C` | Disconnect                              |

Chat commands: `/help`, `/who`, `/w <user> <msg>`, `/friend add|remove|list`,
`/me <action>`, `/quit`. Emotes: `/wave`, `/happy`, `/laugh`, `/heart`,
`/cry`, `/angry`, `/sleep`, `/dance` (or press `g` for the picker).

Direct messages are sent and read in the chat console: `/w <user> <msg>`
whispers someone (delivered live if they're online, saved otherwise), and
`/w <user>` on its own prints your recent thread with them. The friends
panel (`f`) is an index of friends and people you've messaged — pressing
Enter on a name pre-fills a `/w` to them.

## Design notes

- **Identity.** `SHA256` fingerprint of the client's public key, mapped to
  a row in `players`. Colors come from `sha256(fingerprint)[0]` into a
  curated 24-color palette, so a player's color is stable forever.
- **Rendering.** The plaza is baked into a true RGB pixel framebuffer
  (`internal/render/canvas`) and shipped as one Sixel image per frame
  (`internal/render/sixel`, a fixed-palette run-length encoder). The world is
  projected to a 2:1 **isometric** screen space (`internal/render/iso`): the
  ground is 2×2 flagstone slabs with dark joints and baked ambient occlusion
  against the walls; walls, pillars and benches are depth-sorted extruded
  cubes with contact seams and an AO gradient so they sit on the floor; a
  detailed tiered **fountain** (rippling pool, spilling sheets, fine spray);
  and a **city skyline** of towers ringing the back edges, hazed by
  distance, with seed-picked rooflines (blinking antenna beacons, penthouse
  blocks) and windows that twinkle on their own slow clocks. The solid
  structures and the skyline are indexed and pre-sorted once at load, so the
  per-frame render only culls and draws — no allocation or sorting per frame.
  Avatars are procedural pixel art (`internal/render/sprites`) — hooded,
  cloaked figures with a 4-phase walk gait (the body dips on the passing
  frames), swinging arms, an idle breathing bob and a soft contact shadow;
  text
  (names, chat, HUD, panels) is baked with a 5×7 bitmap font so the entire frame
  composites in one place. **Interactive lighting** (`internal/render/light`)
  dims the plaza and lets the player and lamps reveal it, with blocky glow
  halos. Each portal is a gateway (`internal/plaza/portals.go`): a pool lying in
  the isometric ground plane as a 2:1 ellipse of chunky pixel-art blocks,
  ringed by eight worn standing stones (taller at the back) with a dithered
  column of its light rising high enough to navigate by from across the
  plaza. The pool's surface shimmers with drifting caustics like the
  fountain's water, motes of its light drift up and fade, and a smooth
  colored bloom plus a soft floor light wash the portal's glow gently onto
  the surrounding tiles. Its low-saturation hue is derived from a hash of
  its world key (memoized at load), so a world's color is stable forever.
- **The render loop.** Bubble Tea's line renderer can't host a Sixel image, so
  the plaza returns a constant `View` (keeping that renderer quiescent) and a
  dedicated background goroutine (`internal/ui/overworld/renderer.go`) produces
  frames on its own clock, writing to a mutex-guarded session writer
  (`internal/render/syncwriter`) shared with Bubble Tea. The event loop only
  publishes cheap state snapshots, so the heavy encode never causes input lag;
  player and camera motion are interpolated at the render rate for smoothness
  independent of the logic tick. Frames are produced on a steady 20 fps clock,
  and input additionally `Kick()`s an out-of-band frame so a keypress shows
  immediately instead of waiting up to a tick. The image is a **fixed
  viewW×viewH pixel viewport**, snapped down to whole cells and letterboxed:
  every player sees exactly the same slice of the world, and a larger terminal
  just gets wider margins. Snapping to cell boundaries makes centering exact;
  the cell size is derived from the PTY (or a terminal query) and is stable
  across resizes. The SSH layer pins sessions to TrueColor.
- **Camera & motion.** Movement runs on a steady tick so the walk pace is
  decoupled from the terminal's key-repeat (which has a long, OS-dependent
  initial delay and a variable rate — riding it directly makes a held key
  stutter and feel laggy). The first press steps instantly; while a direction
  key is held, the tick carries the walk one tile per `moveTickEvery`. A key is
  "held" only while fresh: a lone tap expires within `tapWindow` (so it moves
  exactly one tile), while a key whose repeats have begun stays live within
  `holdSteady` of the last repeat. Each step lands square on the floor grid; the
  renderer eases the avatar toward its target with short, snappy frame-rate-
  independent smoothing and keeps the camera centered on the local player.
  Diagonals have dedicated keys (key-repeat only repeats the last key); Shift
  runs (two tiles per step). Below 60×20 cells, a resize prompt is baked into
  the frame.
- **Multiplayer.** One in-memory hub holds all sessions. Input is local
  and immediate; position updates are flagged dirty and broadcast by a
  50 ms coalescing sweep (~20 Hz), so keypress spam never floods peers.
  Connecting the same key twice hands the avatar to the newest session.
  Critical events (battle starts) are reserve-then-commit: buffer room is
  checked for both players before either is flagged busy, so a stalled
  session can never soft-lock its opponent; pending challenges expire on
  the sweep with a notice to the challenger.
- **Emotes.** Gestures (`internal/emote`) broadcast through the hub like chat
  (the sender hears its own echo, so one render path drives self and peers).
  Each plays for a few seconds as a hand-pixelled icon in a callout bubble
  above the avatar's head — a wave, heart, laugh, tears, anger, a sleepy "Z",
  a music note — and some add a little body motion (a hop, a sway, a crouch).
  Trigger them with `/wave`-style commands or the `g` quick-picker.
- **Player interactions.** Walk up to someone and press `x` to open their card —
  name, what they're wearing, how long they've wandered Shellbound — with quick
  actions to whisper (`w`), friend (`f`) or challenge them to a Shellmon duel
  (`v`).
- **Shellmon PvP.** Challenging from the inspect card sends a duel invite through
  the hub; the target gets a modal accept/decline prompt. On accept, the hub
  loads both players' saved teams, heals them, builds one shared
  `shellmon.Match` and drops both sessions into it. Because every session runs
  in the same server process, the two clients hold the *same* mutex-guarded
  match: each submits its action for the turn, the turn resolves once both are
  in, and each side polls a race-free `Snapshot` at its render tick — no battle
  state crosses a wire, only the handshake. The duel uses copies of the rosters,
  so nobody's saved party is changed. Players in a world (or a duel) are flagged
  busy and can't be challenged.
- **Worlds.** Portals reference `world.World` implementations from a
  registry. A world receives a `Context` carrying save/inventory APIs
  pre-bound to `(player, world key)` — it cannot touch any other slot — plus
  a `Render` handle (the shared palette, the synchronized session writer and
  the cell size) so a world can ship full Sixel frames exactly like the
  plaza. The **Bomberman** portal leads to **The Vault** (`internal/worlds/
  bomber`): an isometric bomb arena whose wisps hunt you when you're close
  and flee a burning fuse, with camera shake on every detonation — clearing
  it unlocks the *Sparkforged Crown*. The **Doom** portal leads to a first-person **raycast shooter**
  (`internal/worlds/doom`): perspective walls with brick shading, billboarded
  imps whose eyes glow in the portal's hue (they fan out around you rather
  than stacking), hitscan firing with a crosshair hit-marker, held-key
  movement that glides on the tick instead of stuttering with the OS
  key-repeat, and a distance-attenuated muzzle flash — clearing the hall
  unlocks the *Hellbreaker Horns*. Both paint their accents in the
  portal's own colour, so a world and its gateway look like one place. The
  **Shellmon** portal leads to a **creature collector** (`internal/worlds/
  shellmon`): pick one of three starters, then set out across a little
  **overworld** — the starting town of **Oakhaven**, **Route 1**, and the
  seaside town of **Tidewell** at the far end — linked by edge warps, each with
  its own charm and an irregular, hand-carved treeline (no strict rectangles).
  Oakhaven is a cozy inland village around a stone wishing-well, with cottages, a
  fenced paddock and flower beds; Tidewell is a seaside port that opens straight
  onto the **open ocean** to the south — no treeline there, the sea is the map
  edge — fronted by a sandy shore, a wooden fishing pier and a striped
  lighthouse. The towns are paved and lawned (no wild grass, so nothing spawns in
  them), with rest/heal pads, signs to read and NPCs to bump for a line. Route 1
  is drawn in the Pokémon idiom: a dirt path threading short grass past defined
  patches of tall grass (the only tiles that hide wild Shellmon) and a ledge you
  can hop down but not climb back up. Three **trainers** challenge you — on
  contact or down their line of sight through the grass — each a real 6v6 battle
  that stays won once beaten; the toughest, a rumored ace who trains in the
  northeast grass, drops the *Wanderer's Halo*. Tidewell has the region's first
  **gym**, and it plays like one: a flooded hall where a one-way **water-current
  ring** (bottom sweeps east, right north, top west, left south) is the only way
  across the central pool — you ride a current and it carries you to the next
  stone island, past two junior trainers, up to **Leader Pearl**. Beating her
  plays an animated **badge award** and grants the **Coral Badge** (plus the
  *Captain's Cap* to wear). Earned badges are shown in a case on the party
  screen (`p`).
  **Secrets** are scattered about: a stray Frostnip on the route, a Wizard Hat
  buried in a far grass corner, and a Flower Crown glinting at the old well in
  Tidewell — collected once and persisted. Areas are stateless templates rebuilt
  on entry, so per-player progress (beaten trainers, found secrets) lives in the
  save, not the map. Battles use a turn-based **6v6 engine** (`internal/shellmon`) over an original
  18-creature roster across a Spark▸Bramble▸Tide type triangle — catching,
  leveling and a full Sixel battle screen. The **battle arena** is a stark
  black-and-white night stage (a near-black starfield, dark ridge silhouettes, a
  crisp horizon line and a faint pool of ground light), with sparse colour pops
  in each Shellmon's type hue — the only colour, just like the plaza reserves it
  for names and portals — on the creature names and the attack effects (eased HP
  bars, hit shakes, faint sinks, type-styled cast motes and impact bursts). The
  **overworld** is drawn in the same 2:1 isometric projection as the plaza —
  gabled cottages, a wishing-well, a lighthouse and a plank pier, tall-grass
  patches and earthen ledges, and the gym's flowing current lanes, all ringed by
  a depth-sorted forest (with an irregular, hand-carved treeline); the avatar,
  NPCs and trainers breathe and the avatar walks, like in the plaza.
  Entering a portal, an encounter, a duel or warping between areas all play a
  diamond-wipe transition (in the portal's own hue for the gateway). The party
  persists as a JSON roster in the per-world save slot, so no schema is
  involved.
- **The Shellmon engine.** `internal/shellmon` is pure, unit-tested game logic
  and data: the type triangle, an 11-move pool (typed damage plus status), 18
  species with role-varied stats and level-up learnsets, level-scaled stats with
  XP/leveling and HP-based catch odds, and a 6v6 battle engine (switches before
  attacks, speed order, KOs cancel queued moves, stat stages, forced switches,
  a move-choosing AI). Creatures are procedural monochrome sprites drawn as
  recognizable animal archetypes — a fox, a mouse, a snail, a shark, an owl,
  a hippo, a rabbit, a hedgehog, a bear, a songbird, a puppy, a tortoise, a
  frog, a crab, a seahorse, an anglerfish, a penguin and an octopus — so a
  species reads at a glance, the way a Pokémon does. The same engine
  will drive PvP.
- **Cosmetics.** Headwear worn over the avatar (`internal/cosmetic`), drawn in
  the same monochrome, overhead-lit style. Pieces come from three places:
  starters everyone has (cap, headband, top hat, antenna), rewards for clearing
  the worlds (crown, horns, halo, and the gym's captain's cap), and shop stock
  bought with coins (beanie, bow, visor, flower crown, wizard hat). Each carries
  a **rarity tier** —
  Common, Rare, Epic, Legendary (shown as a text label, since the strict color
  discipline reserves color for names, chat and portals) — surfaced in the shop,
  the wardrobe, and a player's inspect card. The wardrobe panel (`c`) lists what
  you own — starters ∪ inventory grants keyed `cosmetic.*`; the equipped piece
  is persisted on the player row and broadcast through the hub so everyone sees
  it.
- **Coins & the shop.** A soft currency earned passively: one coin every
  ~20 seconds you're in the plaza (time inside a portal world doesn't pay, so
  the reward is for hanging around the shared space), persisted as it lands and
  shown in the HUD purse. The plaza has a **cosmetics stall** (`H` in the map,
  rendered as a counter under a striped market awning); walk up to it and press
  `e` to open the shop (`internal/ui/shop`), which lists the buyable headwear
  with prices and your balance. A purchase atomically debits the price (a guard
  in the `UPDATE` makes check-and-charge a single statement, so no double-spend)
  and grants the cosmetic as a `cosmetic.*` inventory item — the same ownership
  channel the world rewards use, so it shows up in the wardrobe immediately.
- **Panels.** Every modal list (friends, wardrobe, shop, inventory, the
  emote picker, inspect cards) is built on one small kit
  (`internal/ui/listpanel`): panels own their data and hand the renderer a
  `Content{Title, Lines, Cursor, Footer}`; the renderer draws the chrome —
  title rule, inverse selection bar, footer hint, drop shadow — in one
  place, in pixels.
- **Previews.** The plaza, the Shellmon overworld/battles, the Vault and
  Doom all have PNG preview harnesses (`*_test.go`, gated behind
  `PLAZA_PREVIEW_DIR` / `SHELLMON_PREVIEW_DIR`), so the art can be
  eyeballed and iterated without an SSH session.
- **Persistence.** Pure-Go SQLite, single writer connection, in-code
  migrations on startup. Tables: `players` (with equipped cosmetic and coin
  balance), `friends`, `dms`, `inventory`, `saves`.

## Roadmap

- **Done** — three portal worlds: Bomberman → "The Vault" (bomb arena),
  Doom → a raycast FPS, and Shellmon → a creature collector with a 6v6
  battle engine, all granting progression through the real save/inventory
  pipelines.
- **Done** — Shellmon **PvP**: challenge a nearby player from their inspect card
  and fight 6v6 over the hub with the same engine.
- **Done** — a Shellmon **overworld**: two towns (one opening onto the ocean)
  and a wild Route 1 with line-of-sight trainer battles, ledges, a
  reward-dropping ace, hidden secrets, and the first **gym** (an indoor hall
  with junior trainers and Leader Pearl for the Coral Badge) — all linked by
  warps with per-player progress in the save.
- **Later** — more routes and towns and gyms for Shellmon; spectating duels;
  the Doom portal doing whatever a terminal can get away with; player-placed
  decorations; moderation tools.

## Repository layout

```
cmd/shellbound/      entry point
internal/server/     wish server, session app shell
internal/hub/        presence + broadcast
internal/auth/       fingerprints, username rules
internal/storage/    sqlite, migrations, repositories
internal/world/      World interface, registry, scoped stores + render handle
internal/shellmon/   creature-battler engine: types, moves, species, battles
internal/worlds/     world implementations (bomber, doom, shellmon, comingsoon)
internal/plaza/      map, isometric tiles, portals, cosmetics shop stall
internal/cosmetic/   wearable headwear catalog + rendering
internal/ui/         login, overworld, chat, inventory, cosmetics, shop, friends, toast, listpanel
internal/render/     canvas, sixel, iso, light, sprites, screen, syncwriter
internal/style/      palette, themes
internal/anim/       camera spring, flicker helpers
```
