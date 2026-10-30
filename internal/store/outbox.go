package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Message is one unpublished broker payload.
// Attempts is how many publishes have already failed.
type Message struct {
	ID       string
	Topic    string
	Body     []byte
	Attempts int
}

// Outbox is the unpublished-message queue stored with the business write.
type Outbox interface {
	Claim(ctx context.Context, limit int, lease time.Duration, maxAttempts int) ([]Message, error)
	MarkPublished(ctx context.Context, id string) error
	RecordFailure(ctx context.Context, id, reason string, retryAt time.Time) error
	Release(ctx context.Context, ids []string) error
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

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// InsertMessage writes one outbox row on the caller's transaction.
func InsertMessage(ctx context.Context, tx execer, topic string, payload []byte) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO message_outbox (topic, payload) VALUES ($1, $2::jsonb)
	`, topic, string(payload)); err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}
	return nil
}

// Claim leases up to limit rows that are due and still under maxAttempts.
// A second relay skips rows this claim already locked.
func (s *outbox) Claim(ctx context.Context, limit int, lease time.Duration, maxAttempts int) ([]Message, error) {
	if limit <= 0 {
		limit = 20
	}
	seconds := int64(lease / time.Second)
	if seconds < 1 {
		return nil, fmt.Errorf("outbox lease must be at least one second")
	}
	if maxAttempts < 1 {
		return nil, fmt.Errorf("outbox max attempts must be positive")
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH due AS (
			SELECT id
			FROM message_outbox
			WHERE published_at IS NULL
			  AND available_at <= now()
			  AND attempts < $2
			ORDER BY created_at
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		),
		claimed AS (
			UPDATE message_outbox AS m
			SET available_at = now() + ($1 * interval '1 second')
			FROM due
			WHERE m.id = due.id
			RETURNING m.id::text, m.topic, m.payload, m.attempts, m.created_at
		)
		SELECT id, topic, payload, attempts
		FROM claimed
		ORDER BY created_at
	`, seconds, maxAttempts, limit)
	if err != nil {
		return nil, fmt.Errorf("claim outbox: %w", err)
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.Topic, &msg.Body, &msg.Attempts); err != nil {
			return nil, fmt.Errorf("scan outbox: %w", err)
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim outbox: %w", err)
	}
	return out, nil
}

func (s *outbox) MarkPublished(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE message_outbox
		SET published_at = now(), last_error = NULL
		WHERE id = $1::uuid AND published_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}
	return nil
}

func (s *outbox) RecordFailure(ctx context.Context, id, reason string, retryAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE message_outbox
		SET attempts = attempts + 1, last_error = $2, available_at = $3
		WHERE id = $1::uuid AND published_at IS NULL
	`, id, clip(reason, outboxErrorRunes), retryAt.UTC())
	if err != nil {
		return fmt.Errorf("record outbox failure: %w", err)
	}
	return nil
}

func (s *outbox) Release(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE message_outbox
		SET available_at = now()
		WHERE id::text = ANY($1::text[]) AND published_at IS NULL
	`, ids)
	if err != nil {
		return fmt.Errorf("release outbox: %w", err)
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
