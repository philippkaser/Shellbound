package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// DM is a persisted direct message.
type DM struct {
	ID          int64
	SenderID    int64
	RecipientID int64
	Body        string
	CreatedAt   time.Time
}

// Partner summarizes one DM conversation counterpart for the conversation
// list in the friends panel.
type Partner struct {
	Player Player
	LastAt time.Time
}

// DMs is the repository for the dms table.
type DMs struct {
	db *sql.DB
}

// Save persists one direct message.
func (r *DMs) Save(senderID, recipientID int64, body string) error {
	_, err := r.db.Exec(
		`INSERT INTO dms (sender_id, recipient_id, body) VALUES (?, ?, ?)`,
		senderID, recipientID, body,
	)
	if err != nil {
		return fmt.Errorf("storage: save dm: %w", err)
	}
	return nil
}

// Conversation returns the most recent `limit` messages between players a
// and b, oldest first.
func (r *DMs) Conversation(a, b int64, limit int) ([]DM, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(
		`SELECT id, sender_id, recipient_id, body, created_at FROM (
			SELECT id, sender_id, recipient_id, body, created_at
			FROM dms
			WHERE (sender_id = ? AND recipient_id = ?) OR (sender_id = ? AND recipient_id = ?)
			ORDER BY id DESC LIMIT ?
		 ) ORDER BY id ASC`,
		a, b, b, a, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: load conversation: %w", err)
	}
	defer rows.Close()

	var out []DM
	for rows.Next() {
		var m DM
		if err := rows.Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.Body, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("storage: scan dm: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: iterate dms: %w", err)
	}
	return out, nil
}

// Partners returns the distinct players that playerID has exchanged DMs
// with, most recent conversation first.
func (r *DMs) Partners(playerID int64) ([]Partner, error) {
	rows, err := r.db.Query(
		`SELECT p.id, p.fingerprint, p.username, p.color, p.created_at, MAX(d.created_at) AS last_at
		 FROM dms d
		 JOIN players p ON p.id = CASE WHEN d.sender_id = ? THEN d.recipient_id ELSE d.sender_id END
		 WHERE d.sender_id = ? OR d.recipient_id = ?
		 GROUP BY p.id
		 ORDER BY last_at DESC`,
		playerID, playerID, playerID,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: list dm partners: %w", err)
	}
	defer rows.Close()

	var out []Partner
	for rows.Next() {
		var pt Partner
		if err := rows.Scan(&pt.Player.ID, &pt.Player.Fingerprint, &pt.Player.Username,
			&pt.Player.Color, &pt.Player.CreatedAt, &pt.LastAt); err != nil {
			return nil, fmt.Errorf("storage: scan dm partner: %w", err)
		}
		out = append(out, pt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: iterate dm partners: %w", err)
	}
	return out, nil
}
