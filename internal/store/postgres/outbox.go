package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dmmoody/poz-pay/internal/outbox"
)

// outboxErrorRunes bounds last_error. The column is unbounded text; the cap
// keeps a broker error from storing an arbitrarily large string.
const outboxErrorRunes = 500

// OutboxStore is the durable unpublished-message queue.
type OutboxStore struct {
	db *sql.DB
}

func NewOutboxStore(db *sql.DB) *OutboxStore {
	return &OutboxStore{db: db}
}

func (s *OutboxStore) Pending(ctx context.Context, limit int) ([]outbox.Message, error) {
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
	var out []outbox.Message
	for rows.Next() {
		var msg outbox.Message
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

func (s *OutboxStore) MarkPublished(ctx context.Context, id string) error {
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

func (s *OutboxStore) RecordFailure(ctx context.Context, id, reason string) error {
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

var _ outbox.Store = (*OutboxStore)(nil)
