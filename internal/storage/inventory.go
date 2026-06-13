package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// Item is one inventory entry. In 1.0 nothing grants items yet; the schema
// and API exist so future portal worlds can award them.
type Item struct {
	ID         int64
	PlayerID   int64
	WorldKey   string // which world granted it ("plaza" for system grants)
	ItemKey    string // stable machine key, e.g. "bomberman.trophy"
	Name       string // display name
	Qty        int
	AcquiredAt time.Time
}

// Inventory is the repository for the inventory table.
type Inventory struct {
	db *sql.DB
}

// Items returns the player's inventory, newest first.
func (r *Inventory) Items(playerID int64) ([]Item, error) {
	rows, err := r.db.Query(
		`SELECT id, player_id, world_key, item_key, name, qty, acquired_at
		 FROM inventory WHERE player_id = ? ORDER BY id DESC`,
		playerID,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: list inventory: %w", err)
	}
	defer rows.Close()

	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.PlayerID, &it.WorldKey, &it.ItemKey, &it.Name, &it.Qty, &it.AcquiredAt); err != nil {
			return nil, fmt.Errorf("storage: scan item: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: iterate inventory: %w", err)
	}
	return out, nil
}

// Grant awards qty of an item to a player on behalf of a world. qty must be
// positive.
func (r *Inventory) Grant(playerID int64, worldKey, itemKey, name string, qty int) error {
	if qty <= 0 {
		return fmt.Errorf("storage: grant qty must be positive, got %d", qty)
	}
	_, err := r.db.Exec(
		`INSERT INTO inventory (player_id, world_key, item_key, name, qty) VALUES (?, ?, ?, ?, ?)`,
		playerID, worldKey, itemKey, name, qty,
	)
	if err != nil {
		return fmt.Errorf("storage: grant item: %w", err)
	}
	return nil
}
