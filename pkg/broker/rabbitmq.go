// Package broker carries the event bus: one topic exchange, one event
// envelope, and publisher/consumer helpers built on RabbitMQ.
package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ExchangeName is the single topic exchange every service publishes to.
const ExchangeName = "eproc.events"

// Routing keys / event names from the domain event catalogue (see docs/events.md).
const (
	EventVendorCreated     = "vendor.created"
	EventVendorUpdated     = "vendor.updated"
	EventTenderPublished   = "tender.published"
	EventTenderClosed      = "tender.closed"
	EventBidSubmitted      = "bid.submitted"
	EventBidSelected       = "bid.selected"
	EventPOCreated         = "po.created"
	EventApprovalRequested = "approval.requested"
	EventApprovalDecided   = "approval.decided"
	EventPOApproved        = "po.approved"
	EventPOIssued          = "po.issued"
	EventPORejected        = "po.rejected"
	EventPOClosed          = "po.closed"
	EventAuditRecorded     = "audit.recorded"
)

// Event is the envelope published for every domain event. Payload stays opaque
// so a consumer never needs the producer's structs.
type Event struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	TenantID   string          `json:"tenant_id,omitempty"`
	ActorID    string          `json:"actor_id,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}

// NewEvent builds an envelope with a fresh id and the current timestamp.
func NewEvent(name, tenantID, actorID string, payload any) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("marshal event payload: %w", err)
	}
	return Event{
		ID:         uuid.NewString(),
		Name:       name,
		TenantID:   tenantID,
		ActorID:    actorID,
		OccurredAt: time.Now().UTC(),
		Payload:    raw,
	}, nil
}

// Publisher publishes events; it is safe for concurrent use.
type Publisher struct {
	conn *amqp.Connection
	ch   *amqp.Channel
	log  *slog.Logger
}

// NewPublisher dials RabbitMQ, declares the exchange and returns a publisher.
func NewPublisher(url string, log *slog.Logger) (*Publisher, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq channel: %w", err)
	}
	if err := ch.ExchangeDeclare(ExchangeName, "topic", true, false, false, false, nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("declare exchange %s: %w", ExchangeName, err)
	}
	return &Publisher{conn: conn, ch: ch, log: log}, nil
}

// Publish sends one event to the topic exchange, keyed by evt.Name.
func (p *Publisher) Publish(ctx context.Context, evt Event) error {
	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	err = p.ch.PublishWithContext(ctx, ExchangeName, evt.Name, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    evt.ID,
		Timestamp:    evt.OccurredAt,
		Body:         body,
	})
	if err != nil {
		return fmt.Errorf("publish %s: %w", evt.Name, err)
	}
	if p.log != nil {
		p.log.DebugContext(ctx, "event published", slog.String("event", evt.Name), slog.String("event_id", evt.ID))
	}
	return nil
}

// Close releases the channel and the connection.
func (p *Publisher) Close() error {
	if p.ch != nil {
		_ = p.ch.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// Handler processes one delivery. Returning an error nacks the message.
type Handler func(ctx context.Context, evt Event) error

// Consumer subscribes a durable queue to the topic exchange.
type Consumer struct {
	conn  *amqp.Connection
	ch    *amqp.Channel
	queue string
	log   *slog.Logger
}

// NewConsumer binds a durable queue named queue to the given routing keys.
func NewConsumer(url, queue string, routingKeys []string, log *slog.Logger) (*Consumer, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq channel: %w", err)
	}
	if err := ch.ExchangeDeclare(ExchangeName, "topic", true, false, false, false, nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("declare exchange: %w", err)
	}
	if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("declare queue %s: %w", queue, err)
	}
	for _, key := range routingKeys {
		if err := ch.QueueBind(queue, key, ExchangeName, false, nil); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("bind %s to %s: %w", queue, key, err)
		}
	}
	return &Consumer{conn: conn, ch: ch, queue: queue, log: log}, nil
}

// Run consumes until ctx is cancelled or the channel closes. Messages are
// acked on success and nacked (without requeue) on handler error, so a poison
// message cannot loop forever.
func (c *Consumer) Run(ctx context.Context, handler Handler) error {
	deliveries, err := c.ch.Consume(c.queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", c.queue, err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery channel for %s closed", c.queue)
			}
			var evt Event
			if err := json.Unmarshal(delivery.Body, &evt); err != nil {
				if c.log != nil {
					c.log.Error("dropping malformed event", slog.String("error", err.Error()))
				}
				_ = delivery.Nack(false, false)
				continue
			}
			if err := handler(ctx, evt); err != nil {
				if c.log != nil {
					c.log.Error("event handler failed",
						slog.String("event", evt.Name), slog.String("event_id", evt.ID), slog.String("error", err.Error()))
				}
				_ = delivery.Nack(false, false)
				continue
			}
			_ = delivery.Ack(false)
		}
	}
}

// Close releases the consumer's channel and connection.
func (c *Consumer) Close() error {
	if c.ch != nil {
		_ = c.ch.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
