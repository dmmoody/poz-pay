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

const (
	// maxAttempts is how many failed publishes a row gets before the relay
	// leaves it unpublished. A later row can still be claimed.
	maxAttempts = 8

	// claimLease hides a claimed row from a second relay. It is longer than
	// one broker confirm. A crash before mark-published makes the row due again.
	claimLease = 30 * time.Second

	backoffBase = time.Second
	backoffCap  = time.Minute
)

// Message is one unpublished broker payload.
type Message = store.Message

// Relay claims due rows and publishes them.
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
			// last claimed batch leave the database if the broker is still up.
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
	msgs, err := r.store.Claim(ctx, r.batch, claimLease, maxAttempts)
	if err != nil {
		log.Printf("outbox: claim: %v", err)
		return
	}
	for i, msg := range msgs {
		if err := ctx.Err(); err != nil {
			r.release(msgs[i:])
			return
		}
		if err := r.pub.Publish(ctx, msg.Topic, msg.Body); err != nil {
			log.Printf("outbox: publish %s: %v", msg.ID, err)
			r.recordFailure(msg, err)
			r.release(msgs[i+1:])
			return
		}
		if err := r.store.MarkPublished(ctx, msg.ID); err != nil {
			// The broker already has the message. Ingest is idempotent, so a
			// later claim may deliver a duplicate once the lease expires.
			log.Printf("outbox: mark published %s: %v", msg.ID, err)
			r.release(msgs[i+1:])
			return
		}
	}
}

func (r *Relay) recordFailure(msg Message, cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	retryAt := time.Now().Add(retryAfter(msg.Attempts))
	if err := r.store.RecordFailure(ctx, msg.ID, cause.Error(), retryAt); err != nil {
		log.Printf("outbox: record failure %s: %v", msg.ID, err)
	}
}

func (r *Relay) release(msgs []Message) {
	if len(msgs) == 0 {
		return
	}
	ids := make([]string, len(msgs))
	for i, msg := range msgs {
		ids[i] = msg.ID
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.store.Release(ctx, ids); err != nil {
		log.Printf("outbox: release: %v", err)
	}
}

func retryAfter(attempts int) time.Duration {
	if attempts < 0 {
		attempts = 0
	}
	if attempts >= 6 {
		return backoffCap
	}
	d := backoffBase << attempts
	if d > backoffCap {
		return backoffCap
	}
	return d
}
