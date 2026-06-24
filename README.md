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
go mod tidy        # first time only: resolves deps, writes go.sum
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

> **Note on go.sum** — this repository ships without a `go.sum`; run
> `go mod tidy` once before the first build. If a pinned version in
> `go.mod` has been yanked upstream, `go get <module>@latest` will move it
> forward; the code sticks to long-stable APIs of these libraries.

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
| `i`            | Inventory                               |
| `c`            | Wardrobe (equip cosmetic headwear)      |
| `e`            | Shop (when standing by the plaza stall) |
| `f`            | Friends list (Enter to message someone) |
| `Esc`          | Close any panel                         |
| `q` / `Ctrl+C` | Disconnect                              |

Chat commands: `/help`, `/who`, `/w <user> <msg>`, `/friend add|remove|list`,
`/me <action>`, `/quit`.

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
  projected to a 2:1 **isometric** screen space (`internal/render/iso`): paved
  ground diamonds and depth-sorted extruded cubes for walls, pillars and
  benches, a detailed tiered **fountain** (rippling pool, spilling sheets, fine
  spray) and a **city skyline** of towers ringing the back edges. The solid
  structures and the skyline are indexed and pre-sorted once at load, so the
  per-frame render only culls and draws — no allocation or sorting per frame.
  Avatars are procedural pixel art (`internal/render/sprites`) — hooded, cloaked
  figures with swinging arms, an idle breathing bob and a soft contact shadow;
  text
  (names, chat, HUD, panels) is baked with a 5×7 bitmap font so the entire frame
  composites in one place. **Interactive lighting** (`internal/render/light`)
  dims the plaza and lets the player and lamps reveal it, with blocky glow
  halos. Each portal is a pool (`internal/plaza/portals.go`) lying in the
  isometric ground plane as a 2:1 ellipse of chunky pixel-art blocks: its
  surface shimmers with drifting caustics like the fountain's water, motes of
  its light drift up and fade, and a smooth colored bloom plus a soft floor
  light wash the portal's glow gently onto the surrounding tiles. Its
  low-saturation hue is derived from a hash of its world key, so a world's color
  is stable forever.
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
- **Worlds.** Portals reference `world.World` implementations from a
  registry. A world receives a `Context` carrying save/inventory APIs
  pre-bound to `(player, world key)` — it cannot touch any other slot — plus
  a `Render` handle (the shared palette, the synchronized session writer and
  the cell size) so a world can ship full Sixel frames exactly like the
  plaza. The **Bomberman** portal leads to **The Vault** (`internal/worlds/
  bomber`): an isometric bomb arena that unlocks the *Sparkforged Crown* when
  cleared. The **Doom** portal leads to a first-person **raycast shooter**
  (`internal/worlds/doom`): perspective walls with brick shading, billboarded
  imps whose eyes glow in the portal's hue, hitscan firing, unlocking the
  *Hellbreaker Horns* for clearing the hall. Both paint their accents in the
  portal's own colour, so a world and its gateway look like one place. Chess
  is still the text "coming soon" placeholder.
- **Cosmetics.** Headwear worn over the avatar (`internal/cosmetic`), drawn in
  the same monochrome, overhead-lit style. Pieces come from three places:
  starters everyone has (cap, headband, top hat, antenna), rewards for clearing
  the worlds (crown, horns, halo), and shop stock bought with coins (beanie,
  bow, visor, flower crown, wizard hat). The wardrobe panel (`c`) lists what you
  own — starters ∪ inventory grants keyed `cosmetic.*`; the equipped piece is
  persisted on the player row and broadcast through the hub so everyone sees it.
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
- **Persistence.** Pure-Go SQLite, single writer connection, in-code
  migrations on startup. Tables: `players` (with equipped cosmetic and coin
  balance), `friends`, `dms`, `inventory`, `saves`.

## Roadmap

- **Done** — two portal worlds: Bomberman → "The Vault" (bomb arena) and
  Doom → a raycast FPS, both granting a victory item through the real
  inventory pipeline.
- **1.x** — Chess; richer enemy behaviour; inventory the plaza shows off.
- **Later** — Chess (with correspondence via DMs?), the Doom portal doing
  whatever a terminal can get away with, player-placed decorations,
  moderation tools.

## Repository layout

```
cmd/shellbound/      entry point
internal/server/     wish server, session app shell
internal/hub/        presence + broadcast
internal/auth/       fingerprints, username rules
internal/storage/    sqlite, migrations, repositories
internal/world/      World interface, registry, scoped stores + render handle
internal/worlds/     world implementations (bomber, doom, comingsoon)
internal/plaza/      map, isometric tiles, portals, cosmetics shop stall
internal/cosmetic/   wearable headwear catalog + rendering
internal/ui/         login, overworld, chat, inventory, cosmetics, shop, friends, toast
internal/render/     canvas, sixel, iso, light, sprites, screen, syncwriter
internal/style/      palette, themes
internal/anim/       camera spring, flicker helpers
```
