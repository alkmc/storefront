package cache

import (
	"testing"
	"uuid"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/go-cmp/cmp"
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
			want: Entry{Product: product, Hit: true, Found: true, exists: true},
		},
		{
			name: "missing marker",
			raw:  []byte{tagMissing},
			want: Entry{Hit: true, exists: true},
		},
		{
			name: "tombstone",
			raw:  tombstone,
			want: Entry{exists: true, token: tombstone},
		},
		{
			name: "unknown tag",
			raw:  unknownTag,
			want: Entry{exists: true, token: unknownTag},
		},
		{
			name: "corrupt payload",
			raw:  corrupt,
			want: Entry{exists: true, token: corrupt},
		},
		{
			name: "empty value",
			raw:  []byte{},
			want: Entry{exists: true, token: []byte{}},
		},
		{
			name: "empty value read back as nil",
			raw:  nil,
			want: Entry{exists: true},
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
