package store

import "context"

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
