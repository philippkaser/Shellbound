# Shellbound

A tiny MMO that lives entirely inside your terminal. Connect over SSH and
you're standing in a shared plaza: walk around, watch other players wander
past, chat, whisper, make friends. Strict black-and-white pixel art with
exactly three splashes of color — player names, chat usernames, and the
rainbow shimmer of the portals.

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

Other targets: `make build`, `make test`, `make vet`, `make hostkey`.

> **Note on go.sum** — this repository ships without a `go.sum`; run
> `go mod tidy` once before the first build. If a pinned version in
> `go.mod` has been yanked upstream, `go get <module>@latest` will move it
> forward; the code sticks to long-stable APIs of these libraries.

Your terminal should support 24-bit color (practically all modern
terminals do) and ideally use a dark/black background.

## Controls

| Key            | Action                                  |
| -------------- | --------------------------------------- |
| WASD / arrows  | Walk, one cell per step (no diagonals)  |
| `Enter`        | Open chat — `Enter` sends, `Esc` cancels|
| `i`            | Inventory                               |
| `f`            | Friends & messages                      |
| `Esc`          | Close any panel                         |
| `q` / `Ctrl+C` | Disconnect                              |

Chat commands: `/help`, `/who`, `/w <user> <msg>`, `/friend add|remove|list`,
`/me <action>`, `/quit`.

## Design notes

- **Identity.** `SHA256` fingerprint of the client's public key, mapped to
  a row in `players`. Colors come from `sha256(fingerprint)[0]` into a
  curated 24-color palette, so a player's color is stable forever.
- **Rendering.** The plaza is drawn on a half-block framebuffer
  (`internal/render/halfblock`): every terminal cell carries two stacked
  pixels via `▀` with independent fg/bg, giving 100×100 pixels on a
  100×50-cell map. A glyph layer on top carries walls, benches, text and
  name tags. A quadrant-block buffer (`internal/render/pixelbuf`) provides
  2×2 sub-pixels per cell for the drifting clouds. The hot path emits
  run-length-minimized raw SGR sequences; the SSH layer pins sessions to
  TrueColor so output is deterministic.
- **Camera.** A `harmonica` spring per axis follows the player with slight
  lag; the viewport crops when the terminal is smaller than the map and
  letterboxes when larger. Below 60×20 cells, a resize prompt shows.
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
internal/plaza/      map, tiles, portals
internal/ui/         login, overworld, chat, inventory, friends, toast
internal/render/     halfblock canvas, pixelbuf, sprites, shimmer
internal/style/      palette, themes
internal/anim/       camera spring, flicker helpers
```
