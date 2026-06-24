package storage

import (
	"database/sql"
	"fmt"
)

// migrations is the ordered list of schema migrations. Append only; never
// edit an entry that has shipped.
var migrations = []string{
	// 1: players — fingerprint is the permanent SSH identity.
	`CREATE TABLE IF NOT EXISTS players (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		fingerprint TEXT NOT NULL UNIQUE,
		username    TEXT NOT NULL UNIQUE COLLATE NOCASE,
		color       TEXT NOT NULL,
		created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	// 2: friends — directed edges; (a follows b) is one row.
	`CREATE TABLE IF NOT EXISTS friends (
		player_id  INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
		friend_id  INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (player_id, friend_id)
	)`,
	// 3: dms — persisted direct messages.
	`CREATE TABLE IF NOT EXISTS dms (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		sender_id    INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
		recipient_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
		body         TEXT NOT NULL,
		created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_dms_pair ON dms (sender_id, recipient_id, id)`,
	// 4: inventory — empty in 1.0; future worlds grant items through the
	// world.InventoryAPI, scoped by world_key.
	`CREATE TABLE IF NOT EXISTS inventory (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		player_id   INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
		world_key   TEXT NOT NULL,
		item_key    TEXT NOT NULL,
		name        TEXT NOT NULL,
		qty         INTEGER NOT NULL DEFAULT 1,
		acquired_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_inventory_player ON inventory (player_id)`,
	// 5: saves — one opaque blob per (player, world). The unique pair
	// constraint is what makes world saves physically isolated.
	`CREATE TABLE IF NOT EXISTS saves (
		player_id  INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
		world_key  TEXT NOT NULL,
		data       BLOB NOT NULL,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (player_id, world_key)
	)`,
	// 6: cosmetics — the player's equipped headwear (empty = bare-headed).
	// Ownership of unlockable pieces is derived from the inventory.
	`ALTER TABLE players ADD COLUMN cosmetic TEXT NOT NULL DEFAULT ''`,
	// 7: coins — the soft currency earned passively while online and spent at
	// the plaza cosmetics shop.
	`ALTER TABLE players ADD COLUMN coins INTEGER NOT NULL DEFAULT 0`,
}

// Migrate applies all pending migrations inside the schema_migrations
// version ledger. It is idempotent and safe to call on every startup.
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("storage: create migrations table: %w", err)
	}

	var current int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("storage: read migration version: %w", err)
	}

	for i := current; i < len(migrations); i++ {
		version := i + 1
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("storage: begin migration %d: %w", version, err)
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("storage: apply migration %d: %w", version, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("storage: record migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("storage: commit migration %d: %w", version, err)
		}
	}
	return nil
}
