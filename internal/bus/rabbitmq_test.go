//go:build integration

package bus

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
	"uuid"

	"github.com/alkmc/storefront/internal/event"
	"github.com/rabbitmq/rabbitmq-amqp-go-client/pkg/rabbitmqamqp"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

const (
	// keep in sync with compose.yaml
	rabbitmqImage = "rabbitmq:4.4-management"

	// a routing regression must fail the test instead of blocking until the binary times out
	receiveTimeout = 5 * time.Second
	// recoveryTimeout outlasts the library's first 5s reconnect delay plus jitter and dial.
	recoveryTimeout = 30 * time.Second
	// recoveryAttemptTimeout bounds one connect and publish like OUTBOX_PUBLISH_TIMEOUT does.
	recoveryAttemptTimeout = 2 * time.Second
	// recoveryPollInterval paces retries while the connection recovers.
	recoveryPollInterval = 100 * time.Millisecond
)

// testBroker is a fresh broker with an admin connection that plays the consumer's side.
type testBroker struct {
	container *rabbitmq.RabbitMQContainer
	url       string
	conn      *rabbitmqamqp.AmqpConnection
}

func setupTestContainerRabbit(t *testing.T) testBroker {
	t.Helper()
	ctx := t.Context()

	container, err := rabbitmq.Run(ctx, rabbitmqImage)
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("terminate rabbitmq container: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("run rabbitmq container: %v", err)
	}

	url, err := container.AmqpURL(ctx)
	if err != nil {
		t.Fatalf("rabbitmq url: %v", err)
	}
	conn, err := rabbitmqamqp.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial rabbitmq: %v", err)
	}
	// t.Context is canceled before cleanup runs, closing the conn needs a live ctx
	closeCtx := context.WithoutCancel(ctx)
	t.Cleanup(func() {
		if err := conn.Close(closeCtx); err != nil {
			t.Logf("close amqp connection: %v", err)
		}
	})

	return testBroker{container: container, url: url, conn: conn}
}

// connectPublisher returns a Publisher connected to url and closed on cleanup.
func connectPublisher(t *testing.T, url string) *Publisher {
	t.Helper()
	pub := NewPublisher(url)
	closeCtx := context.WithoutCancel(t.Context())
	t.Cleanup(func() {
		if err := pub.Close(closeCtx); err != nil {
			t.Logf("close publisher: %v", err)
		}
	})
	if err := pub.Connect(t.Context()); err != nil {
		t.Fatalf("connect publisher: %v", err)
	}
	return pub
}

// closeBrokerConnections drops every client connection the way a broker restart does.
func closeBrokerConnections(t *testing.T, b testBroker) {
	t.Helper()
	rabbitmqctl(t, b, "close_all_connections", "integration test")
}

// setConfigurePermission sets the regexp of resources the test user may declare.
func setConfigurePermission(t *testing.T, b testBroker, configure string) {
	t.Helper()
	rabbitmqctl(t, b, "set_permissions", b.container.AdminUsername, configure, ".*", ".*")
}

func rabbitmqctl(t *testing.T, b testBroker, args ...string) {
	t.Helper()
	code, out, err := b.container.Exec(t.Context(), append([]string{"rabbitmqctl"}, args...))
	if err != nil {
		t.Fatalf("exec rabbitmqctl: %v", err)
	}
	if code != 0 {
		msg, _ := io.ReadAll(out)
		t.Fatalf("rabbitmqctl exited %d: %s", code, msg)
	}
}

// watchStates subscribes to connection states, buffered because the library blocks on a full channel.
func watchStates(pub *Publisher) <-chan *rabbitmqamqp.StateChanged {
	states := make(chan *rabbitmqamqp.StateChanged, 16)
	pub.conn.NotifyStatusChange(states)
	return states
}

// waitReopened blocks until the library reports the connection open again, after topology recovery.
func waitReopened(t *testing.T, states <-chan *rabbitmqamqp.StateChanged) {
	t.Helper()
	timeout := time.After(recoveryTimeout)
	for {
		select {
		case s := <-states:
			if _, open := s.To.(*rabbitmqamqp.StateOpen); open {
				return
			}
		case <-timeout:
			t.Fatalf("connection not reopened after %v", recoveryTimeout)
		}
	}
}

// waitPublishSucceeds runs the relay's connect and publish until the broker accepts rec.
func waitPublishSucceeds(t *testing.T, pub *Publisher, rec event.Record) {
	t.Helper()
	deadline := time.Now().Add(recoveryTimeout)
	for {
		err := connectAndPublish(t.Context(), pub, rec)
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("publish still failing after %v: %v", recoveryTimeout, err)
		}
		time.Sleep(recoveryPollInterval)
	}
}

func connectAndPublish(ctx context.Context, pub *Publisher, rec event.Record) error {
	ctx, cancel := context.WithTimeout(ctx, recoveryAttemptTimeout)
	defer cancel()
	if err := pub.Connect(ctx); err != nil {
		return err
	}
	return pub.Publish(ctx, rec)
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
	b := setupTestContainerRabbit(t)
	ctx := t.Context()

	pub := connectPublisher(t, b.url)

	// with nothing bound the broker releases the message, the relay must not retire it as poison
	if err := pub.Publish(ctx, testRecord(event.TypeCreated)); err != nil {
		t.Fatalf("publish without a binding: %v", err)
	}
}

func TestPublisher_PublishesToBoundQueue(t *testing.T) {
	t.Parallel()
	b := setupTestContainerRabbit(t)
	conn := b.conn
	ctx := t.Context()

	pub := connectPublisher(t, b.url)

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

// TestPublisher_RejectedIsRetryable keeps the row on a missing exchange and redeclares it on Connect.
func TestPublisher_RejectedIsRetryable(t *testing.T) {
	t.Parallel()
	b := setupTestContainerRabbit(t)
	conn := b.conn
	ctx := t.Context()

	pub := connectPublisher(t, b.url)

	if err := conn.Management().DeleteExchange(ctx, Exchange); err != nil {
		t.Fatalf("delete exchange: %v", err)
	}

	err := pub.Publish(ctx, testRecord(event.TypeCreated))
	if err == nil || errors.Is(err, event.ErrUndeliverable) {
		t.Fatalf("publish to a missing exchange = %v, want a retryable error", err)
	}
	waitPublishSucceeds(t, pub, testRecord(event.TypeCreated))
}

// TestPublisher_ReconnectsAfterClose pins that Connect replaces a connection that is closed for good.
func TestPublisher_ReconnectsAfterClose(t *testing.T) {
	t.Parallel()
	b := setupTestContainerRabbit(t)
	ctx := t.Context()

	pub := connectPublisher(t, b.url)
	if err := pub.Close(ctx); err != nil {
		t.Fatalf("close publisher: %v", err)
	}

	if err := pub.Publish(ctx, testRecord(event.TypePurchased)); err == nil ||
		errors.Is(err, event.ErrUndeliverable) {
		t.Fatalf("publish over a closed connection = %v, want a retryable error", err)
	}
	waitPublishSucceeds(t, pub, testRecord(event.TypePurchased))
}

// TestPublisher_RedeclaresLostExchangeOnReconnect pins that recovery restores an exchange the broker lost.
func TestPublisher_RedeclaresLostExchangeOnReconnect(t *testing.T) {
	t.Parallel()
	b := setupTestContainerRabbit(t)
	ctx := t.Context()

	pub := connectPublisher(t, b.url)
	states := watchStates(pub)
	if err := b.conn.Management().DeleteExchange(ctx, Exchange); err != nil {
		t.Fatalf("delete exchange: %v", err)
	}

	closeBrokerConnections(t, b)
	waitReopened(t, states)
	if err := pub.Publish(ctx, testRecord(event.TypeCreated)); err != nil {
		t.Fatalf("first publish after recovery = %v, want the exchange redeclared", err)
	}
}

// TestPublisher_RedialsWhenRecoveryLosesExchange pins that a recovery without the exchange heals.
func TestPublisher_RedialsWhenRecoveryLosesExchange(t *testing.T) {
	t.Parallel()
	b := setupTestContainerRabbit(t)
	ctx := t.Context()

	pub := connectPublisher(t, b.url)
	states := watchStates(pub)
	if err := b.conn.Management().DeleteExchange(ctx, Exchange); err != nil {
		t.Fatalf("delete exchange: %v", err)
	}
	setConfigurePermission(t, b, "^$")
	closeBrokerConnections(t, b)
	waitReopened(t, states)
	setConfigurePermission(t, b, ".*")

	if err := pub.Publish(ctx, testRecord(event.TypeCreated)); err == nil ||
		errors.Is(err, event.ErrUndeliverable) {
		t.Fatalf("publish after a failed recovery = %v, want a retryable error", err)
	}
	waitPublishSucceeds(t, pub, testRecord(event.TypeCreated))
}
