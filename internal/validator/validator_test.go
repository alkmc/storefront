package validator

import (
	"strings"
	"testing"

	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
)

func TestValidator_Product(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		product *entity.Product
		wantErr string
	}{
		{
			name:    "empty product",
			product: nil,
			wantErr: "the product is empty",
		},
		{
			name:    "empty name",
			product: new(entity.Product{Name: "", Price: 1.1}),
			wantErr: "the product name is empty",
		},
		{
			name:    "negative price",
			product: new(entity.Product{Name: "Car", Price: -1.0}),
			wantErr: "the product price must be positive",
		},
		{
			name:    "success",
			product: new(entity.Product{Name: "Car", Price: 10.5}),
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Product(tt.product)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidator_UUID(t *testing.T) {
	v := NewValidator()
	validUUID := "98f67138-b104-4836-a216-2b2c27f4bbee"

	tests := []struct {
		name    string
		uuid    string
		wantErr bool
	}{
		{
			name:    "valid uuid",
			uuid:    validUUID,
			wantErr: false,
		},
		{
			name:    "invalid length",
			uuid:    strings.TrimSuffix(validUUID, "e"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uid, err := v.UUID(tt.uuid)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if uid != uuid.Nil {
					t.Errorf("got %v, want %v", uid, uuid.Nil)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if uid.String() != validUUID {
					t.Errorf("got %v, want %v", uid.String(), validUUID)
				}
			}
		})
	}
}
