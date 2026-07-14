// Package bus publishes outbox records to the message broker.
package bus

import (
	"context"
	"fmt"

	"github.com/alkmc/storefront/internal/event"
	"github.com/rabbitmq/rabbitmq-amqp-go-client/pkg/rabbitmqamqp"
)

// Exchange is the topic exchange that carries product events.
const Exchange = "storefront.product"

// Publisher emits outbox records to a RabbitMQ topic exchange.
type Publisher struct {
	pub *rabbitmqamqp.Publisher
}

// NewPublisher declares the topic exchange and returns a Publisher.
func NewPublisher(ctx context.Context, conn *rabbitmqamqp.AmqpConnection,
) (*Publisher, error) {
	if _, err := conn.Management().DeclareExchange(
		ctx, &rabbitmqamqp.TopicExchangeSpecification{Name: Exchange},
	); err != nil {
		return nil, fmt.Errorf("bus declare exchange: %w", err)
	}
	pub, err := conn.NewPublisher(ctx, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("bus new publisher: %w", err)
	}
	return &Publisher{pub: pub}, nil
}

// Publish emits the record's payload and blocks until the broker settles it.
func (p *Publisher) Publish(ctx context.Context, r event.Record) error {
	msg, err := rabbitmqamqp.NewMessageWithAddress(
		r.Payload,
		&rabbitmqamqp.ExchangeAddress{Exchange: Exchange, Key: r.Type},
	)
	if err != nil {
		return fmt.Errorf("%w: bus address: %w", event.ErrUndeliverable, err)
	}
	msg.Properties.MessageID = r.MessageID.String()

	res, err := p.pub.Publish(ctx, msg)
	if err != nil {
		return fmt.Errorf("bus publish: %w", err)
	}
	return outcomeErr(res.Outcome)
}

// Close releases the publisher but not the connection, which the caller owns.
func (p *Publisher) Close(ctx context.Context) error {
	if err := p.pub.Close(ctx); err != nil {
		return fmt.Errorf("bus close: %w", err)
	}
	return nil
}

// outcomeErr classifies the settlement, released (no queue bound yet) is pub/sub success.
func outcomeErr(outcome rabbitmqamqp.DeliveryState) error {
	switch outcome.(type) {
	case *rabbitmqamqp.StateAccepted, *rabbitmqamqp.StateReleased:
		return nil
	default:
		return fmt.Errorf("%w: bus publish not accepted (%T)", event.ErrUndeliverable, outcome)
	}
}
