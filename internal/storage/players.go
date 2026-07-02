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
	Cosmetic    string // equipped headwear key ("" = bare-headed)
	Coins       int    // soft currency earned online, spent at the shop
}

// ErrUsernameTaken is returned by Players.Create when the requested
// username collides (case-insensitively) with an existing player.
var ErrUsernameTaken = errors.New("storage: username already taken")

// Players is the repository for the players table.
type Players struct {
	db *sql.DB
}

const playerCols = `id, fingerprint, username, color, created_at, cosmetic, coins`

func scanPlayer(row *sql.Row) (*Player, error) {
	var p Player
	err := row.Scan(&p.ID, &p.Fingerprint, &p.Username, &p.Color, &p.CreatedAt, &p.Cosmetic, &p.Coins)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage: scan player: %w", err)
	}
	return &p, nil
}

// SetCosmetic stores the player's equipped headwear key ("" = bare-headed).
func (r *Players) SetCosmetic(id int64, key string) error {
	if _, err := r.db.Exec(`UPDATE players SET cosmetic = ? WHERE id = ?`, key, id); err != nil {
		return fmt.Errorf("storage: set cosmetic: %w", err)
	}
	return nil
}

// AddCoins credits n coins to a player and returns the new balance. n must be
// positive. The UPDATE ... RETURNING makes credit-and-read one atomic
// statement, so the reported balance is exactly the result of this credit.
func (r *Players) AddCoins(id int64, n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("storage: add coins must be positive, got %d", n)
	}
	var balance int
	err := r.db.QueryRow(
		`UPDATE players SET coins = coins + ? WHERE id = ? RETURNING coins`, n, id,
	).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("storage: add coins: %w", err)
	}
	return balance, nil
}

// SpendCoins atomically deducts cost coins if the player can afford it,
// reporting whether the charge went through and the resulting balance. The
// guard in the UPDATE makes the check-and-debit a single statement, so two
// concurrent purchases can never overdraw; RETURNING folds the balance read
// into the same statement.
func (r *Players) SpendCoins(id int64, cost int) (ok bool, balance int, err error) {
	if cost < 0 {
		return false, 0, fmt.Errorf("storage: spend cost must be non-negative, got %d", cost)
	}
	err = r.db.QueryRow(
		`UPDATE players SET coins = coins - ? WHERE id = ? AND coins >= ? RETURNING coins`,
		cost, id, cost,
	).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		// Guard failed (or no such player): nothing was charged.
		balance, err = r.coins(id)
		return false, balance, err
	}
	if err != nil {
		return false, 0, fmt.Errorf("storage: spend coins: %w", err)
	}
	return true, balance, nil
}

// coins reads a player's current coin balance.
func (r *Players) coins(id int64) (int, error) {
	var n int
	if err := r.db.QueryRow(`SELECT coins FROM players WHERE id = ?`, id).Scan(&n); err != nil {
		return 0, fmt.Errorf("storage: read coins: %w", err)
	}
	return n, nil
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
