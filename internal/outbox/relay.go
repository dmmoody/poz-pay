// Package outbox publishes durable messages that were stored with the
// business write. The database is the source of truth; the broker is a wake-up.
package outbox

import (
	"context"
	"log"
	"time"

	"github.com/dmmoody/poz-pay/internal/broker"
	"github.com/dmmoody/poz-pay/internal/store"
)

// Message is one unpublished broker payload.
type Message = store.Message

// Relay polls the store and publishes. One failed publish stops the batch so
// a broker outage does not spin on every pending row; the next tick retries.
type Relay struct {
	store    store.Outbox
	pub      broker.Publisher
	interval time.Duration
	batch    int
}

func NewRelay(queue store.Outbox, pub broker.Publisher, interval time.Duration) *Relay {
	if interval <= 0 {
		interval = time.Second
	}
	return &Relay{store: queue, pub: pub, interval: interval, batch: 20}
}

// Run publishes until ctx is cancelled, then tries one last batch.
func (r *Relay) Run(ctx context.Context) {
	r.publishPending(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// The caller already cancelled. A short independent timeout lets the
			// last polled batch leave the database if the broker is still up.
			flushCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			r.publishPending(flushCtx)
			cancel()
			return
		case <-ticker.C:
			r.publishPending(ctx)
		}
	}
}

func (r *Relay) publishPending(ctx context.Context) {
	msgs, err := r.store.Pending(ctx, r.batch)
	if err != nil {
		log.Printf("outbox: list: %v", err)
		return
	}
	for _, msg := range msgs {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := r.pub.Publish(ctx, msg.Topic, msg.Body); err != nil {
			log.Printf("outbox: publish %s: %v", msg.ID, err)
			if recErr := r.store.RecordFailure(ctx, msg.ID, err.Error()); recErr != nil {
				log.Printf("outbox: record failure %s: %v", msg.ID, recErr)
			}
			return
		}
		if err := r.store.MarkPublished(ctx, msg.ID); err != nil {
			// The broker already has the message. Ingest is idempotent, so the
			// next tick may deliver a duplicate.
			log.Printf("outbox: mark published %s: %v", msg.ID, err)
			return
		}
	}
}
