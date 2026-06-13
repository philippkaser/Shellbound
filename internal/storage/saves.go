package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

// Saves is the repository for per-(player, world) save blobs. The primary
// key on (player_id, world_key) guarantees a single slot per pair.
type Saves struct {
	db *sql.DB
}

// Load returns the save blob for (playerID, worldKey). If no save exists it
// returns an empty, non-nil slice and no error, matching the
// world.SaveStore contract.
func (r *Saves) Load(playerID int64, worldKey string) ([]byte, error) {
	var data []byte
	err := r.db.QueryRow(
		`SELECT data FROM saves WHERE player_id = ? AND world_key = ?`,
		playerID, worldKey,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return []byte{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage: load save: %w", err)
	}
	if data == nil {
		data = []byte{}
	}
	return data, nil
}

// Save upserts the save blob for (playerID, worldKey).
func (r *Saves) Save(playerID int64, worldKey string, data []byte) error {
	if data == nil {
		data = []byte{}
	}
	_, err := r.db.Exec(
		`INSERT INTO saves (player_id, world_key, data, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT (player_id, world_key) DO UPDATE SET data = excluded.data, updated_at = CURRENT_TIMESTAMP`,
		playerID, worldKey, data,
	)
	if err != nil {
		return fmt.Errorf("storage: write save: %w", err)
	}
	return nil
}
