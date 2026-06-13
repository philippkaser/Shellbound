package storage

import (
	"database/sql"
	"fmt"
)

// Friends is the repository for the friends table. Friendships are stored
// as directed edges: Add(a, b) means a has added b.
type Friends struct {
	db *sql.DB
}

// Add records that playerID has added friendID. Adding an existing friend
// is a no-op.
func (r *Friends) Add(playerID, friendID int64) error {
	_, err := r.db.Exec(
		`INSERT OR IGNORE INTO friends (player_id, friend_id) VALUES (?, ?)`,
		playerID, friendID,
	)
	if err != nil {
		return fmt.Errorf("storage: add friend: %w", err)
	}
	return nil
}

// Remove deletes the friendship edge from playerID to friendID. Removing a
// non-existent friend is a no-op.
func (r *Friends) Remove(playerID, friendID int64) error {
	_, err := r.db.Exec(
		`DELETE FROM friends WHERE player_id = ? AND friend_id = ?`,
		playerID, friendID,
	)
	if err != nil {
		return fmt.Errorf("storage: remove friend: %w", err)
	}
	return nil
}

// IsFriend reports whether playerID has added friendID.
func (r *Friends) IsFriend(playerID, friendID int64) (bool, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM friends WHERE player_id = ? AND friend_id = ?`,
		playerID, friendID,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("storage: is friend: %w", err)
	}
	return n > 0, nil
}

// List returns the players that playerID has added, ordered by username.
func (r *Friends) List(playerID int64) ([]Player, error) {
	rows, err := r.db.Query(
		`SELECT p.id, p.fingerprint, p.username, p.color, p.created_at
		 FROM friends f JOIN players p ON p.id = f.friend_id
		 WHERE f.player_id = ?
		 ORDER BY p.username COLLATE NOCASE`,
		playerID,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: list friends: %w", err)
	}
	defer rows.Close()

	var out []Player
	for rows.Next() {
		var p Player
		if err := rows.Scan(&p.ID, &p.Fingerprint, &p.Username, &p.Color, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("storage: scan friend: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: iterate friends: %w", err)
	}
	return out, nil
}
