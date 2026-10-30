package outbox

import (
	"context"
	"database/sql"
	"fmt"
)

// outboxErrorRunes bounds last_error. The column is unbounded text; the cap
// keeps a broker error from storing an arbitrarily large string.
const outboxErrorRunes = 500

// Queue is the durable unpublished-message queue.
type Queue struct {
	db *sql.DB
}

func NewQueue(db *sql.DB) *Queue {
	return &Queue{db: db}
}

func (s *Queue) Pending(ctx context.Context, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, topic, payload
		FROM message_outbox
		WHERE published_at IS NULL
		ORDER BY created_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list outbox: %w", err)
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.Topic, &msg.Body); err != nil {
			return nil, fmt.Errorf("scan outbox: %w", err)
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list outbox: %w", err)
	}
	return out, nil
}

func (s *Queue) MarkPublished(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE message_outbox
		SET published_at = now(), attempts = attempts + 1, last_error = NULL
		WHERE id = $1::uuid AND published_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}
	return nil
}

func (s *Queue) RecordFailure(ctx context.Context, id, reason string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE message_outbox
		SET attempts = attempts + 1, last_error = $2
		WHERE id = $1::uuid AND published_at IS NULL
	`, id, clip(reason, outboxErrorRunes))
	if err != nil {
		return fmt.Errorf("record outbox failure: %w", err)
	}
	return nil
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
