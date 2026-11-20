package domain

import (
	"strings"
	"testing"
)

func TestProduct_Validate(t *testing.T) {
	price := Money{MinorAmount: 100, Currency: CurrencyPLN}

	tests := []struct {
		name    string
		product Product
		wantErr bool
	}{
		{
			name:    "multibyte name at the limit",
			product: Product{Name: strings.Repeat("ą", MaxProductNameLen), Price: price},
		},
		{
			name:    "multibyte name over the limit",
			product: Product{Name: strings.Repeat("ą", MaxProductNameLen+1), Price: price},
			wantErr: true,
		},
		{
			name:    "NUL in name",
			product: Product{Name: "a\x00b", Price: price},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.product.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
