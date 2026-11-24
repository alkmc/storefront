// Package bus publishes outbox records to the message broker.
package bus

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Azure/go-amqp"
	"github.com/alkmc/storefront/internal/event"
	"github.com/rabbitmq/rabbitmq-amqp-go-client/pkg/rabbitmqamqp"
)

// Exchange is the topic exchange that carries product events.
const Exchange = "storefront.product"

// Publisher emits outbox records to a RabbitMQ topic exchange.
type Publisher struct {
	env  *rabbitmqamqp.Environment
	conn *rabbitmqamqp.AmqpConnection
	pub  *rabbitmqamqp.Publisher
}

// NewPublisher returns a Publisher that dials on Connect, so startup does not wait for the broker.
func NewPublisher(url string) *Publisher {
	return new(Publisher{env: rabbitmqamqp.NewEnvironment(url, &rabbitmqamqp.AmqpConnOptions{
		RecoveryConfiguration: &rabbitmqamqp.RecoveryConfiguration{
			ActiveRecovery:           true,
			BackOffReconnectInterval: 5 * time.Second,
			MaxReconnectAttempts:     math.MaxInt,
		},
		TopologyRecoveryOptions: rabbitmqamqp.TopologyRecoveryAllEnabled,
	})})
}

// Connect redials and redeclares the exchange unless the connection is alive, releasing a dead one first.
func (p *Publisher) Connect(ctx context.Context) error {
	if p.alive() {
		return nil
	}
	_ = p.Close(ctx)
	conn, err := p.env.NewConnection(ctx)
	if err != nil {
		return fmt.Errorf("bus connect: %w", err)
	}
	pub, err := openPublisher(ctx, conn)
	if err != nil {
		_ = conn.Close(ctx)
		return err
	}
	p.conn, p.pub = conn, pub

	return nil
}

// Publish emits the record's payload and blocks until the broker settles it.
func (p *Publisher) Publish(ctx context.Context, r event.Record) error {
	if p.pub == nil {
		return errors.New("bus publish: not connected")
	}
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
	if exchangeMissing(res.Outcome) {
		_ = p.Close(ctx)
	}

	return outcomeErr(res.Outcome)
}

// Close closes and drops the connection and its publisher, so the next Connect redials.
func (p *Publisher) Close(ctx context.Context) error {
	if p.conn == nil {
		return nil
	}
	err := p.conn.Close(ctx)
	p.conn, p.pub = nil, nil
	if err != nil {
		return fmt.Errorf("bus close: %w", err)
	}

	return nil
}

// alive reports whether the connection is open or still recovering on its own.
func (p *Publisher) alive() bool {
	if p.conn == nil {
		return false
	}
	_, closed := p.conn.State().(*rabbitmqamqp.StateClosed)

	return !closed
}

// exchangeMissing reports a missing exchange, which needs a fresh connection to redeclare.
func exchangeMissing(outcome rabbitmqamqp.DeliveryState) bool {
	rejected, ok := outcome.(*rabbitmqamqp.StateRejected)
	return ok && rejected.Error != nil && rejected.Error.Condition == amqp.ErrCondNotFound
}

// openPublisher declares the topic exchange and opens a publisher on conn.
func openPublisher(ctx context.Context, conn *rabbitmqamqp.AmqpConnection,
) (*rabbitmqamqp.Publisher, error) {
	if _, err := conn.Management().DeclareExchange(
		ctx, &rabbitmqamqp.TopicExchangeSpecification{Name: Exchange},
	); err != nil {
		return nil, fmt.Errorf("bus declare exchange: %w", err)
	}
	var (
		anonymousTarget rabbitmqamqp.ITargetAddress
		defaultOptions  rabbitmqamqp.IPublisherOptions
	)
	pub, err := conn.NewPublisher(ctx, anonymousTarget, defaultOptions)
	if err != nil {
		return nil, fmt.Errorf("bus new publisher: %w", err)
	}

	return pub, nil
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
