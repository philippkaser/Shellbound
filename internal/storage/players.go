package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Player is a persisted Shellbound identity.
type Player struct {
	ID          int64
	Fingerprint string
	Username    string
	Color       string // hex, e.g. "#FF5FAF"
	CreatedAt   time.Time
}

// ErrUsernameTaken is returned by Players.Create when the requested
// username collides (case-insensitively) with an existing player.
var ErrUsernameTaken = errors.New("storage: username already taken")

// Players is the repository for the players table.
type Players struct {
	db *sql.DB
}

const playerCols = `id, fingerprint, username, color, created_at`

func scanPlayer(row *sql.Row) (*Player, error) {
	var p Player
	err := row.Scan(&p.ID, &p.Fingerprint, &p.Username, &p.Color, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage: scan player: %w", err)
	}
	return &p, nil
}

// ByFingerprint returns the player with the given SSH key fingerprint, or
// (nil, nil) if none exists.
func (r *Players) ByFingerprint(fp string) (*Player, error) {
	row := r.db.QueryRow(`SELECT `+playerCols+` FROM players WHERE fingerprint = ?`, fp)
	return scanPlayer(row)
}

// ByUsername returns the player with the given username (case-insensitive),
// or (nil, nil) if none exists.
func (r *Players) ByUsername(name string) (*Player, error) {
	row := r.db.QueryRow(`SELECT `+playerCols+` FROM players WHERE username = ?`, name)
	return scanPlayer(row)
}

// ByID returns the player with the given id, or (nil, nil) if none exists.
func (r *Players) ByID(id int64) (*Player, error) {
	row := r.db.QueryRow(`SELECT `+playerCols+` FROM players WHERE id = ?`, id)
	return scanPlayer(row)
}

// Create registers a new player. It returns ErrUsernameTaken if the
// username collides with an existing one.
func (r *Players) Create(fingerprint, username, color string) (*Player, error) {
	res, err := r.db.Exec(
		`INSERT INTO players (fingerprint, username, color) VALUES (?, ?, ?)`,
		fingerprint, username, color,
	)
	if err != nil {
		// modernc.org/sqlite surfaces constraint violations as errors whose
		// text contains "UNIQUE constraint failed"; we keep the check loose
		// rather than depending on driver-internal error types.
		if strings.Contains(err.Error(), "UNIQUE") && strings.Contains(err.Error(), "username") {
			return nil, ErrUsernameTaken
		}
		return nil, fmt.Errorf("storage: create player: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("storage: create player id: %w", err)
	}
	p, err := r.ByID(id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, fmt.Errorf("storage: created player %d not found", id)
	}
	return p, nil
}
