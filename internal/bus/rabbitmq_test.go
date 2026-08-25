//go:build integration

package bus

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/alkmc/storefront/internal/event"
	"github.com/rabbitmq/rabbitmq-amqp-go-client/pkg/rabbitmqamqp"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

const (
	testImage = "rabbitmq:4.3-management-alpine"

	// a routing regression must fail the test instead of blocking until the binary times out
	receiveTimeout = 5 * time.Second
)

func setupTestContainerRabbit(t *testing.T) *rabbitmqamqp.AmqpConnection {
	t.Helper()
	ctx := t.Context()

	container, err := rabbitmq.Run(ctx, testImage)
	if err != nil {
		t.Fatalf("run rabbitmq container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("terminate rabbitmq container: %v", err)
		}
	})

	url, err := container.AmqpURL(ctx)
	if err != nil {
		t.Fatalf("rabbitmq url: %v", err)
	}
	conn, err := rabbitmqamqp.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial rabbitmq: %v", err)
	}
	// t.Context is canceled before cleanup runs, closing the connection still needs a live one
	closeCtx := context.WithoutCancel(ctx)
	t.Cleanup(func() {
		if err := conn.Close(closeCtx); err != nil {
			t.Logf("close amqp connection: %v", err)
		}
	})

	return conn
}

func bindQueue(t *testing.T, conn *rabbitmqamqp.AmqpConnection, queue, bindingKey string) {
	t.Helper()
	ctx := t.Context()

	if _, err := conn.Management().DeclareQueue(ctx,
		&rabbitmqamqp.ClassicQueueSpecification{Name: queue},
	); err != nil {
		t.Fatalf("declare queue %s: %v", queue, err)
	}
	if _, err := conn.Management().Bind(ctx, &rabbitmqamqp.ExchangeToQueueBindingSpecification{
		SourceExchange:   Exchange,
		DestinationQueue: queue,
		BindingKey:       bindingKey,
	}); err != nil {
		t.Fatalf("bind %s to %s: %v", queue, bindingKey, err)
	}
}

func testRecord(eventType string) event.Record {
	return event.Record{
		MessageID: uuid.NewV7(),
		Type:      eventType,
		Payload:   []byte(`{"quantity":2}`),
	}
}

func TestPublisher_UnroutedPublishSucceeds(t *testing.T) {
	t.Parallel()
	conn := setupTestContainerRabbit(t)
	ctx := t.Context()

	pub, err := NewPublisher(ctx, conn)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	// with nothing bound the broker releases the message, the relay must not retire it as poison
	if err := pub.Publish(ctx, testRecord(event.TypeCreated)); err != nil {
		t.Fatalf("publish without a binding: %v", err)
	}
}

func TestPublisher_PublishesToBoundQueue(t *testing.T) {
	t.Parallel()
	conn := setupTestContainerRabbit(t)
	ctx := t.Context()

	pub, err := NewPublisher(ctx, conn)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	const (
		wildcard = "storefront.test.wildcard"
		exact    = "storefront.test.exact"
	)
	// a direct exchange compares binding keys verbatim, so only a topic one routes this wildcard
	bindQueue(t, conn, wildcard, "product.*")
	bindQueue(t, conn, exact, event.TypeCreated)

	consumer, err := conn.NewConsumer(ctx, wildcard, nil)
	if err != nil {
		t.Fatalf("new consumer: %v", err)
	}

	want := testRecord(event.TypePurchased)
	if err := pub.Publish(ctx, want); err != nil {
		t.Fatalf("publish purchased: %v", err)
	}
	if err := pub.Publish(ctx, testRecord(event.TypeCreated)); err != nil {
		t.Fatalf("publish created: %v", err)
	}

	receiveCtx, cancel := context.WithTimeout(ctx, receiveTimeout)
	defer cancel()
	delivery, err := consumer.Receive(receiveCtx)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if got := string(delivery.Message().GetData()); got != string(want.Payload) {
		t.Errorf("payload = %s, want %s", got, want.Payload)
	}
	if got := delivery.Message().Properties.MessageID; got != want.MessageID.String() {
		t.Errorf("message id = %v, want %s", got, want.MessageID)
	}
	// exactly the created event, a fanout exchange or a hardcoded routing key lands a different count
	held, err := conn.Management().PurgeQueue(ctx, exact)
	if err != nil {
		t.Fatalf("purge %s: %v", exact, err)
	}
	if held != 1 {
		t.Errorf("queue bound to %s holds %d messages, want 1", event.TypeCreated, held)
	}
}

func TestPublisher_RejectedIsUndeliverable(t *testing.T) {
	t.Parallel()
	conn := setupTestContainerRabbit(t)
	ctx := t.Context()

	pub, err := NewPublisher(ctx, conn)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	// dropping the exchange is the only way to make the broker reject, which is what poison means here
	if err := conn.Management().DeleteExchange(ctx, Exchange); err != nil {
		t.Fatalf("delete exchange: %v", err)
	}

	err = pub.Publish(ctx, testRecord(event.TypeCreated))
	if !errors.Is(err, event.ErrUndeliverable) {
		t.Fatalf("publish to a missing exchange = %v, want %v", err, event.ErrUndeliverable)
	}
}

func TestPublisher_ClosedLinkStopsPublishing(t *testing.T) {
	t.Parallel()
	conn := setupTestContainerRabbit(t)
	ctx := t.Context()

	pub, err := NewPublisher(ctx, conn)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	if err := pub.Close(ctx); err != nil {
		t.Fatalf("close publisher: %v", err)
	}

	// a dead link is a transport failure, retiring the outbox row on it would drop the event
	if err := pub.Publish(ctx, testRecord(event.TypePurchased)); err == nil ||
		errors.Is(err, event.ErrUndeliverable) {
		t.Errorf("publish over a closed link = %v, want a retryable error", err)
	}
}
