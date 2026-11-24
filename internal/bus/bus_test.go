package bus

import (
	"errors"
	"testing"

	"github.com/Azure/go-amqp"
	"github.com/alkmc/storefront/internal/event"
	"github.com/rabbitmq/rabbitmq-amqp-go-client/pkg/rabbitmqamqp"
)

func TestOutcomeErr(t *testing.T) {
	tests := []struct {
		name       string
		outcome    rabbitmqamqp.DeliveryState
		wantErr    bool
		wantPoison bool
	}{
		{
			name:       "accepted is success",
			outcome:    &rabbitmqamqp.StateAccepted{},
			wantErr:    false,
			wantPoison: false,
		},
		{
			name:       "released (no queue bound) is success",
			outcome:    &rabbitmqamqp.StateReleased{},
			wantErr:    false,
			wantPoison: false,
		},
		{
			name:       "rejected is retried",
			outcome:    &rabbitmqamqp.StateRejected{},
			wantErr:    true,
			wantPoison: false,
		},
		{
			name:       "unknown outcome is poison",
			outcome:    &rabbitmqamqp.StateModified{},
			wantErr:    true,
			wantPoison: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := outcomeErr(tt.outcome)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want error %t", err, tt.wantErr)
			}
			if got := errors.Is(err, event.ErrUndeliverable); got != tt.wantPoison {
				t.Fatalf("err = %v, want poison %t", err, tt.wantPoison)
			}
		})
	}
}

func TestExchangeMissing(t *testing.T) {
	tests := []struct {
		name    string
		outcome rabbitmqamqp.DeliveryState
		want    bool
	}{
		{
			name:    "rejected for a missing exchange",
			outcome: &rabbitmqamqp.StateRejected{Error: &amqp.Error{Condition: amqp.ErrCondNotFound}},
			want:    true,
		},
		{
			name:    "rejected for another reason",
			outcome: &rabbitmqamqp.StateRejected{Error: &amqp.Error{Condition: amqp.ErrCondResourceLimitExceeded}},
			want:    false,
		},
		{
			name:    "rejected without details",
			outcome: &rabbitmqamqp.StateRejected{},
			want:    false,
		},
		{
			name:    "accepted",
			outcome: &rabbitmqamqp.StateAccepted{},
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exchangeMissing(tt.outcome); got != tt.want {
				t.Fatalf("exchangeMissing = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestPublishBeforeConnectIsTransient keeps rows pending while the broker has never been reached.
func TestPublishBeforeConnectIsTransient(t *testing.T) {
	err := NewPublisher("amqp://127.0.0.1:5672/").Publish(t.Context(), event.Record{})
	if err == nil || errors.Is(err, event.ErrUndeliverable) {
		t.Fatalf("err = %v, want a transient error so the relay retries", err)
	}
}
