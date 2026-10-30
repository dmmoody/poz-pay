package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Message is one unpublished broker payload.
type Message struct {
	ID    string
	Topic string
	Body  []byte
}

// Outbox is the unpublished-message queue stored with the business write.
type Outbox interface {
	Pending(ctx context.Context, limit int) ([]Message, error)
	MarkPublished(ctx context.Context, id string) error
	RecordFailure(ctx context.Context, id, reason string) error
}

// outboxErrorRunes bounds last_error. The column is unbounded text; the cap
// keeps a broker error from storing an arbitrarily large string.
const outboxErrorRunes = 500

// Outbox is the durable unpublished-message queue.
type outbox struct {
	db *sql.DB
}

func NewOutbox(db *sql.DB) Outbox {
	return &outbox{db: db}
}

func (s *outbox) Pending(ctx context.Context, limit int) ([]Message, error) {
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

func (s *outbox) MarkPublished(ctx context.Context, id string) error {
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

func (s *outbox) RecordFailure(ctx context.Context, id, reason string) error {
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
