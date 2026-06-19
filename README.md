# Shellbound

A tiny MMO that lives entirely inside your terminal. Connect over SSH and
you're standing in a shared plaza: walk around, watch other players wander
past, chat, whisper, make friends. The plaza is drawn as **isometric pixel
art** using real terminal graphics (Sixel) — strict black-and-white, with
exactly three splashes of color (player names, chat usernames, and the rainbow
shimmer of the portals) and interactive lighting that follows you and pools
around the lamps.

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
| `SHELLBOUND_CELL`    | `8x16`                        | Your client's cell size in pixels (`WxH`) — sets where the image is centered; lower it if the image scrolls. The world view itself is pixel-capped (same for everyone). |

Other targets: `make build`, `make test`, `make vet`, `make hostkey`.

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
| WASD / arrows  | Walk (hold two for diagonals)           |
| `Enter`        | Open chat — `Enter` sends, `Esc` cancels|
| `i`            | Inventory                               |
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
  projected to a 2:1 **isometric** screen space (`internal/render/iso`): ground
  diamonds and depth-sorted extruded cubes for walls, pillars, benches and the
  fountain. Avatars are procedural pixel art (`internal/render/sprites`); text
  (names, chat, HUD, panels) is baked with a 5×7 bitmap font so the entire frame
  composites in one place. **Interactive lighting** (`internal/render/light`)
  dims the plaza and lets the player and lamps reveal it, with blocky glow
  halos. Bubble Tea's line renderer can't host a Sixel image, so the plaza
  returns a constant `View` (keeping that renderer quiescent) and writes frames
  itself to a mutex-guarded session writer (`internal/render/syncwriter`) shared
  with Bubble Tea; the SSH layer pins sessions to TrueColor.
- **Camera.** A `harmonica` spring per axis follows the player in grid space,
  advanced by real elapsed time so the follow speed is identical at any frame
  rate; that position is projected to isometric screen space and centered in
  the frame. The frame itself is a pixel canvas capped to a fixed maximum
  (~896×560) and centered in the terminal, so every player sees the same slice
  of the world and bigger windows just get letterbox. Below 60×20 cells, a
  resize prompt is baked in instead.
- **Multiplayer.** One in-memory hub holds all sessions. Input is local
  and immediate; position updates are flagged dirty and broadcast by a
  50 ms coalescing sweep (~20 Hz), so keypress spam never floods peers.
  Connecting the same key twice hands the avatar to the newest session.
- **Worlds.** Portals reference `world.World` implementations from a
  registry. A world receives a `Context` carrying save/inventory APIs
  pre-bound to `(player, world key)` — it cannot touch any other slot. In
  1.0 all three portals lead to the "coming soon" placeholder, which
  persists a visit counter through the real save pipeline.
- **Persistence.** Pure-Go SQLite, single writer connection, in-code
  migrations on startup. Tables: `players`, `friends`, `dms`, `inventory`,
  `saves`.

## Roadmap

- **1.x** — first real portal world (Bomberman), item grants on victory,
  inventory that actually fills up.
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
internal/world/      World interface, registry, scoped stores
internal/worlds/     world implementations (comingsoon)
internal/plaza/      map, isometric tiles, portals
internal/ui/         login, overworld, chat, inventory, friends, toast
internal/render/     canvas, sixel, iso, light, sprites, shimmer, syncwriter
internal/style/      palette, themes
internal/anim/       camera spring, flicker helpers
```
