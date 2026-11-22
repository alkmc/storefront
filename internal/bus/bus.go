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
	var (
		perMessageTarget rabbitmqamqp.ITargetAddress
		defaultOptions   rabbitmqamqp.IPublisherOptions
	)
	pub, err := conn.NewPublisher(ctx, perMessageTarget, defaultOptions)
	if err != nil {
		return nil, fmt.Errorf("bus new publisher: %w", err)
	}
	return new(Publisher{pub: pub}), nil
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

// outcomeErr accepts a release (no queue bound yet) and retries a rejection, which comes from broker state.
func outcomeErr(outcome rabbitmqamqp.DeliveryState) error {
	switch o := outcome.(type) {
	case *rabbitmqamqp.StateAccepted, *rabbitmqamqp.StateReleased:
		return nil
	case *rabbitmqamqp.StateRejected:
		return fmt.Errorf("bus publish: rejected: %w", o.Error)
	default:
		return fmt.Errorf("%w: bus publish not accepted (%T)", event.ErrUndeliverable, outcome)
	}
}
