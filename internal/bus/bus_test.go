package bus

import (
	"errors"
	"testing"

	"github.com/alkmc/storefront/internal/event"
	"github.com/rabbitmq/rabbitmq-amqp-go-client/pkg/rabbitmqamqp"
)

func TestOutcomeErr(t *testing.T) {
	tests := []struct {
		name       string
		outcome    rabbitmqamqp.DeliveryState
		wantPoison bool
	}{
		{
			name:       "accepted is success",
			outcome:    &rabbitmqamqp.StateAccepted{},
			wantPoison: false,
		},
		{
			name:       "released (no queue bound) is success",
			outcome:    &rabbitmqamqp.StateReleased{},
			wantPoison: false,
		},
		{
			name:       "rejected is poison",
			outcome:    &rabbitmqamqp.StateRejected{},
			wantPoison: true,
		},
		{
			name:       "unknown outcome is poison",
			outcome:    &rabbitmqamqp.StateModified{},
			wantPoison: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := outcomeErr(tt.outcome)
			if got := errors.Is(err, event.ErrUndeliverable); got != tt.wantPoison {
				t.Fatalf("err = %v, want poison %t", err, tt.wantPoison)
			}
			if !tt.wantPoison && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
		})
	}
}
