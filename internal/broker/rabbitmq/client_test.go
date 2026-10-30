package rabbitmq

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestPublishAndListen(t *testing.T) {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		t.Skip("RABBITMQ_URL not set")
	}
	id := strconv.FormatInt(time.Now().UnixNano(), 10)
	topic := "check_register.test." + id
	client, err := New(url, Topology{
		Exchange:    "pozpay.events.test",
		Queue:       "pozpay.check-register.test." + id,
		RoutingKeys: []string{topic},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	body := []byte(`{"upload_id":"11111111-1111-1111-1111-111111111111"}`)
	if err := client.Publish(ctx, topic, body); err != nil {
		t.Fatalf("publish: %v", err)
	}

	got := make(chan []byte, 1)
	go func() {
		_ = client.Listen(ctx, topic, func(_ context.Context, delivered []byte) error {
			cp := make([]byte, len(delivered))
			copy(cp, delivered)
			got <- cp
			return nil
		})
	}()
	select {
	case b := <-got:
		cancel()
		if string(b) != string(body) {
			t.Fatalf("body = %s", b)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for delivery")
	}
}
