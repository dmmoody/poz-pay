package outbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRelay_publishesAndStopsOnBrokerError(t *testing.T) {
	t.Parallel()
	store := &fakeStore{msgs: []Message{
		{ID: "a", Topic: "check_register.upload.accepted", Body: []byte(`{"upload_id":"1"}`)},
		{ID: "b", Topic: "check_register.upload.accepted", Body: []byte(`{"upload_id":"2"}`), Attempts: 1},
		{ID: "c", Topic: "check_register.upload.accepted", Body: []byte(`{"upload_id":"3"}`)},
	}}
	pub := &fakePub{failID: "b"}
	relay := NewRelay(store, pub, time.Hour)
	relay.publishPending(context.Background())

	if len(pub.sent) != 2 || string(pub.sent[1].Body) != `{"upload_id":"2"}` {
		t.Fatalf("sent = %+v", pub.sent)
	}
	if len(store.published) != 1 || store.published[0] != "a" {
		t.Fatalf("published = %v", store.published)
	}
	if len(store.failed) != 1 || store.failed[0].id != "b" || !store.failed[0].retryAt.After(time.Now()) {
		t.Fatalf("failed = %+v", store.failed)
	}
	if len(store.released) != 1 || store.released[0] != "c" {
		t.Fatalf("released = %v", store.released)
	}
	if store.lease != claimLease || store.maxAttempts != maxAttempts {
		t.Fatalf("claim lease %s max %d", store.lease, store.maxAttempts)
	}
}

func TestRelay_cancelReleasesClaim(t *testing.T) {
	t.Parallel()
	store := &fakeStore{msgs: []Message{{ID: "a", Topic: "t", Body: []byte("x")}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	NewRelay(store, &fakePub{}, time.Hour).publishPending(ctx)
	if len(store.released) != 1 || store.released[0] != "a" {
		t.Fatalf("released = %v", store.released)
	}
}

func TestRelay_runStopsOnCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	NewRelay(&fakeStore{}, &fakePub{}, time.Hour).Run(ctx)
}

func TestRetryAfter(t *testing.T) {
	t.Parallel()
	if retryAfter(0) != time.Second || retryAfter(1) != 2*time.Second || retryAfter(2) != 4*time.Second {
		t.Fatalf("backoff = %s %s %s", retryAfter(0), retryAfter(1), retryAfter(2))
	}
	if retryAfter(6) != time.Minute || retryAfter(20) != time.Minute {
		t.Fatalf("cap = %s %s", retryAfter(6), retryAfter(20))
	}
}

type sent struct {
	Body []byte
}

type fakePub struct {
	sent   []sent
	failID string
}

func (f *fakePub) Publish(_ context.Context, topic string, body []byte) error {
	f.sent = append(f.sent, sent{Body: body})
	if f.failID != "" && topic != "" && len(f.sent) == 2 {
		return errors.New("broker down")
	}
	return nil
}

type failRec struct {
	id      string
	reason  string
	retryAt time.Time
}

type fakeStore struct {
	msgs        []Message
	published   []string
	failed      []failRec
	released    []string
	lease       time.Duration
	maxAttempts int
}

func (f *fakeStore) Claim(_ context.Context, _ int, lease time.Duration, maxAttempts int) ([]Message, error) {
	f.lease = lease
	f.maxAttempts = maxAttempts
	return f.msgs, nil
}

func (f *fakeStore) MarkPublished(_ context.Context, id string) error {
	f.published = append(f.published, id)
	return nil
}

func (f *fakeStore) RecordFailure(_ context.Context, id, reason string, retryAt time.Time) error {
	f.failed = append(f.failed, failRec{id: id, reason: reason, retryAt: retryAt})
	return nil
}

func (f *fakeStore) Release(_ context.Context, ids []string) error {
	f.released = append(f.released, ids...)
	return nil
}
