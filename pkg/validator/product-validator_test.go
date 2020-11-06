package validator

import (
	"testing"

	"github.com/alkmc/restClean/pkg/entity"
	"github.com/stretchr/testify/assert"
)

const (
	emptyProduct  = "the product is empty"
	emptyName     = "the product name is empty"
	negativePrice = "the product price must be positive"
)

func TestValidateEmptyProduct(t *testing.T) {
	testValidator := NewValidator()
	err := testValidator.Product(nil)

	assert.NotNil(t, err)
	assert.Equal(t, emptyProduct, err.Error())
}

func TestValidateEmptyName(t *testing.T) {
	p := entity.Product{Name: "", Price: 1.1}
	testValidator := NewValidator()

	err := testValidator.Product(&p)
	assert.NotNil(t, err)
	assert.Equal(t, emptyName, err.Error())
}

func TestValidateInvalidPrice(t *testing.T) {
	p := entity.Product{Name: "Car", Price: -1}
	testValidator := NewValidator()

	err := testValidator.Product(&p)
	assert.NotNil(t, err)
	assert.Equal(t, negativePrice, err.Error())
}
