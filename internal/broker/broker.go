// Package broker is the message broker for this process. RabbitMQ is the engine.
// Publisher and Listener are the seam another broker can implement.
package broker

import "context"

// Publisher delivers one durable message.
type Publisher interface {
	Publish(ctx context.Context, topic string, body []byte) error
}

// Listener consumes one topic.
// A non-nil handler error asks for redelivery.
// Nil acknowledges the delivery, including messages the handler discards.
type Listener interface {
	Listen(ctx context.Context, topic string, handle func(context.Context, []byte) error) error
}

var (
	_ Publisher = (*Client)(nil)
	_ Listener  = (*Client)(nil)
)
