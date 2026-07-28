package cache

import (
	"testing"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

func TestClassifyStoredValue(t *testing.T) {
	t.Parallel()

	product := domain.Product{
		ID:      domain.ProductID(uuid.MustParse("0199e1a0-0000-7000-8000-000000000001")),
		Name:    "Keyboard",
		Stock:   7,
		Version: 3,
		Price:   domain.Money{MinorAmount: 12900, Currency: domain.CurrencyPLN},
	}
	payload, err := encodeProduct(product)
	if err != nil {
		t.Fatalf("encodeProduct: %v", err)
	}
	tombstone := newTombstone()
	unknownTag := []byte("xwhatever")
	corrupt := []byte{tagPayload, '{'}

	tests := []struct {
		name string
		raw  []byte
		want Entry
	}{
		{
			name: "payload",
			raw:  payload,
			want: Entry{Product: product, Hit: true, Found: true},
		},
		{
			name: "missing marker",
			raw:  []byte{tagMissing},
			want: Entry{Hit: true},
		},
		{
			name: "tombstone",
			raw:  tombstone,
			want: Entry{token: tombstone},
		},
		{
			name: "unknown tag",
			raw:  unknownTag,
			want: Entry{token: unknownTag},
		},
		{
			name: "corrupt payload",
			raw:  corrupt,
			want: Entry{token: corrupt},
		},
		{
			name: "empty",
			raw:  nil,
			want: Entry{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Entry keeps the raw token unexported, so cmp needs explicit access to it
			if diff := cmp.Diff(tt.want, classify(tt.raw), cmp.AllowUnexported(Entry{})); diff != "" {
				t.Errorf("classify(%q) mismatch (-want +got):\n%s", tt.raw, diff)
			}
		})
	}
}
