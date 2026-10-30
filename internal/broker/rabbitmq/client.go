// Package rabbitmq is the RabbitMQ broker used to wake check-register ingest.
// Publish and Listen are the seam another broker (for example Google Pub/Sub)
// can replace by implementing the same two methods.
package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Topology is the durable exchange, queue, and routing keys this process uses.
type Topology struct {
	Exchange    string
	Queue       string
	RoutingKeys []string
}

// Client is safe for one publisher and one consumer at a time.
type Client struct {
	url  string
	topo Topology

	mu   sync.Mutex
	conn *amqp.Connection
}

func New(url string, topo Topology) (*Client, error) {
	if url == "" {
		return nil, errors.New("rabbitmq url is required")
	}
	if topo.Exchange == "" || topo.Queue == "" || len(topo.RoutingKeys) == 0 {
		return nil, errors.New("rabbitmq topology is incomplete")
	}
	return &Client{url: url, topo: topo}, nil
}

// Publish sends one persistent message and waits for a broker confirm.
func (c *Client) Publish(ctx context.Context, topic string, body []byte) error {
	conn, err := c.connection(ctx)
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		c.invalidate(conn)
		return fmt.Errorf("rabbitmq channel: %w", err)
	}
	defer func() { _ = ch.Close() }()
	if err := c.declare(ch); err != nil {
		c.invalidate(conn)
		return err
	}
	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("rabbitmq confirm: %w", err)
	}
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	err = ch.PublishWithContext(ctx, c.topo.Exchange, topic, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		Body:         body,
	})
	if err != nil {
		c.invalidate(conn)
		return fmt.Errorf("rabbitmq publish: %w", err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case conf, ok := <-confirms:
		if !ok || !conf.Ack {
			return errors.New("rabbitmq nacked the message")
		}
		return nil
	}
}

// Listen delivers messages until ctx is cancelled. It reconnects with backoff
// when the broker drops the connection. A non-nil handler error is requeued
// after a short delay so a locked upload does not hot-loop.
func (c *Client) Listen(ctx context.Context, topic string, handle func(context.Context, []byte) error) error {
	if !c.topo.binds(topic) {
		return fmt.Errorf("rabbitmq topic %q is not bound", topic)
	}
	backoff := time.Second
	for {
		err := c.consumeOnce(ctx, handle)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Printf("rabbitmq: consume: %v", err)
		if !sleep(ctx, backoff) {
			return ctx.Err()
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// Close drops the shared connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (t Topology) binds(topic string) bool {
	for _, key := range t.RoutingKeys {
		if key == topic {
			return true
		}
	}
	return false
}

func (c *Client) consumeOnce(ctx context.Context, handle func(context.Context, []byte) error) error {
	conn, err := c.connection(ctx)
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		c.invalidate(conn)
		return fmt.Errorf("rabbitmq channel: %w", err)
	}
	defer func() { _ = ch.Close() }()
	if err := c.declare(ch); err != nil {
		c.invalidate(conn)
		return err
	}
	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("rabbitmq qos: %w", err)
	}
	deliveries, err := ch.Consume(c.topo.Queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("rabbitmq consume: %w", err)
	}
	closed := ch.NotifyClose(make(chan *amqp.Error, 1))
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case amqpErr := <-closed:
			if amqpErr == nil {
				return errors.New("rabbitmq channel closed")
			}
			c.invalidate(conn)
			return fmt.Errorf("rabbitmq channel closed: %v", amqpErr)
		case d, ok := <-deliveries:
			if !ok {
				c.invalidate(conn)
				return errors.New("rabbitmq deliveries closed")
			}
			if err := handle(ctx, d.Body); err != nil {
				if !sleep(ctx, 2*time.Second) {
					_ = d.Nack(false, true)
					return ctx.Err()
				}
				if nackErr := d.Nack(false, true); nackErr != nil {
					return fmt.Errorf("rabbitmq nack: %w", nackErr)
				}
				continue
			}
			if ackErr := d.Ack(false); ackErr != nil {
				return fmt.Errorf("rabbitmq ack: %w", ackErr)
			}
		}
	}
}

func (c *Client) connection(ctx context.Context) (*amqp.Connection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil && !c.conn.IsClosed() {
		return c.conn, nil
	}
	conn, err := amqp.DialConfig(c.url, amqp.Config{
		Heartbeat: 10 * time.Second,
		Dial: func(network, addr string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("rabbitmq dial: %w", err)
	}
	c.conn = conn
	return conn, nil
}

func (c *Client) invalidate(conn *amqp.Connection) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == conn {
		_ = c.conn.Close()
		c.conn = nil
	}
}

func (c *Client) declare(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(c.topo.Exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq exchange: %w", err)
	}
	if _, err := ch.QueueDeclare(c.topo.Queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq queue: %w", err)
	}
	for _, key := range c.topo.RoutingKeys {
		if err := ch.QueueBind(c.topo.Queue, key, c.topo.Exchange, false, nil); err != nil {
			return fmt.Errorf("rabbitmq bind: %w", err)
		}
	}
	return nil
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
