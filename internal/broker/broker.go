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
