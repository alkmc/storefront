package repository

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapDBError(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		wantUnavailable bool
	}{
		{
			name:            "nil",
			err:             nil,
			wantUnavailable: false,
		},
		{
			name:            "context canceled",
			err:             context.Canceled,
			wantUnavailable: false,
		},
		{
			name:            "context deadline",
			err:             context.DeadlineExceeded,
			wantUnavailable: false,
		},
		{
			name:            "pg connection failure",
			err:             &pgconn.PgError{Code: "08006"},
			wantUnavailable: true,
		},
		{
			name:            "pg insufficient resources",
			err:             &pgconn.PgError{Code: "53300"},
			wantUnavailable: true,
		},
		{
			name:            "pg admin shutdown",
			err:             &pgconn.PgError{Code: "57P01"},
			wantUnavailable: true,
		},
		{
			name:            "pg unique violation",
			err:             &pgconn.PgError{Code: "23505"},
			wantUnavailable: false,
		},
		{
			name:            "net error",
			err:             &net.OpError{Op: "dial", Err: errors.New("connection refused")},
			wantUnavailable: true,
		},
		{
			name:            "plain error",
			err:             errors.New("boom"),
			wantUnavailable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapDBError(tt.err)
			gotUnavail := errors.Is(got, domain.ErrUnavailable)
			if gotUnavail != tt.wantUnavailable {
				t.Fatalf("mapDBError(%v): got unavailable=%v, want %v", tt.err, gotUnavail, tt.wantUnavailable)
			}
			if tt.err != nil && !errors.Is(got, tt.err) {
				t.Errorf("original error not preserved in chain: %v", got)
			}
		})
	}
}
