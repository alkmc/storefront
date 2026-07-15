package cache

import (
	"bytes"
	"testing"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

func TestClassifyStoredValue(t *testing.T) {
	t.Parallel()

	product := domain.Product{
		ID:      uuid.MustParse("0199e1a0-0000-7000-8000-000000000001"),
		Name:    "Keyboard",
		Stock:   7,
		Version: 3,
		Price:   domain.Money{MinorAmount: 12900, Currency: domain.CurrencyPLN},
	}
	payload, err := encodeProduct(product)
	if err != nil {
		t.Fatalf("encodeProduct: %v", err)
	}

	tests := []struct {
		name      string
		raw       []byte
		want      domain.Product
		wantHit   bool
		wantToken bool
	}{
		{name: "payload", raw: payload, want: product, wantHit: true},
		{name: "tombstone", raw: newTombstone(), wantToken: true},
		{name: "unknown tag", raw: []byte("xwhatever"), wantToken: true},
		{name: "corrupt payload", raw: []byte{tagPayload, '{'}, wantToken: true},
		{name: "empty", raw: nil, wantToken: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := classify(tt.raw)
			if got.Hit != tt.wantHit {
				t.Errorf("Hit = %v, want %v", got.Hit, tt.wantHit)
			}
			if got.Product != tt.want {
				t.Errorf("Product = %+v, want %+v", got.Product, tt.want)
			}
			if hasToken := got.token != nil; hasToken != tt.wantToken {
				t.Errorf("token present = %v, want %v", hasToken, tt.wantToken)
			}
			if tt.wantToken && !bytes.Equal(got.token, tt.raw) {
				t.Errorf("token = %q, want the raw value %q", got.token, tt.raw)
			}
		})
	}
}
