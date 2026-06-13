// Package storage provides Shellbound's SQLite persistence: connection
// management, in-code migrations and one small repository per table group.
// It uses modernc.org/sqlite, a pure-Go driver, so no CGO is needed.
package storage

import (
	"database/sql"
	"fmt"

	// Register the pure-Go "sqlite" database/sql driver.
	_ "modernc.org/sqlite"
)

// Open opens (creating if necessary) the SQLite database at path, applies
// pragmas suited to a single-process server, and runs all pending
// migrations. The returned handle is safe for concurrent use.
func Open(path string) (*sql.DB, error) {
	// busy_timeout guards against transient lock contention; WAL gives us
	// concurrent readers with the single writer.
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open %s: %w", path, err)
	}
	// A single writer connection sidesteps SQLITE_BUSY entirely. Shellbound's
	// write volume (logins, chat DMs, saves) is tiny; this is not a bottleneck.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: ping %s: %w", path, err)
	}
	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Repos bundles all repositories over one database handle.
type Repos struct {
	Players   *Players
	Friends   *Friends
	DMs       *DMs
	Inventory *Inventory
	Saves     *Saves
}

// NewRepos constructs the repository set for db.
func NewRepos(db *sql.DB) *Repos {
	return &Repos{
		Players:   &Players{db: db},
		Friends:   &Friends{db: db},
		DMs:       &DMs{db: db},
		Inventory: &Inventory{db: db},
		Saves:     &Saves{db: db},
	}
}
