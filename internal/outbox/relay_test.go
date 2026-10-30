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
		{ID: "b", Topic: "check_register.upload.accepted", Body: []byte(`{"upload_id":"2"}`)},
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
	if len(store.failed) != 1 || store.failed[0].id != "b" {
		t.Fatalf("failed = %+v", store.failed)
	}
}

func TestRelay_runStopsOnCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	NewRelay(&fakeStore{}, &fakePub{}, time.Hour).Run(ctx)
}

type sent struct {
	ID   string
	Body []byte
}

type fakePub struct {
	sent   []sent
	failID string
}

func (f *fakePub) Publish(_ context.Context, _ string, body []byte) error {
	f.sent = append(f.sent, sent{Body: body})
	if f.failID != "" && len(f.sent) == 2 {
		return errors.New("broker down")
	}
	return nil
}

type failRec struct {
	id     string
	reason string
}

type fakeStore struct {
	msgs      []Message
	published []string
	failed    []failRec
}

func (f *fakeStore) Pending(context.Context, int) ([]Message, error) {
	return f.msgs, nil
}

func (f *fakeStore) MarkPublished(_ context.Context, id string) error {
	f.published = append(f.published, id)
	return nil
}

func (f *fakeStore) RecordFailure(_ context.Context, id, reason string) error {
	f.failed = append(f.failed, failRec{id: id, reason: reason})
	return nil
}
