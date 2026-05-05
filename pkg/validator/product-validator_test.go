package validator

import (
	"strings"
	"testing"

	"github.com/alkmc/restClean/pkg/entity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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
			product: &entity.Product{Name: "", Price: 1.1},
			wantErr: "the product name is empty",
		},
		{
			name:    "negative price",
			product: &entity.Product{Name: "Car", Price: -1.0},
			wantErr: "the product price must be positive",
		},
		{
			name:    "success",
			product: &entity.Product{Name: "Car", Price: 10.5},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Product(tt.product)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tt.product.ID)
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
				assert.Error(t, err)
				assert.Equal(t, uuid.Nil, uid)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, validUUID, uid.String())
			}
		})
	}
}
