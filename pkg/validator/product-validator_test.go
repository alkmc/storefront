package validator

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/alkmc/restClean/pkg/entity"
)

const (
	emptyProduct  = "the product is empty"
	emptyName     = "the product name is empty"
	negativePrice = "the product price must be positive"
	uuidStr       = "98f67138-b104-4836-a216-2b2c27f4bbee"
	invalidLen    = "invalid UUID length: 35"
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

func TestValidateIncorrectUUID(t *testing.T) {
	idStr := strings.TrimSuffix(uuidStr, "e")
	testValidator := NewValidator()

	uid, err := testValidator.UUID(idStr)
	assert.Equal(t, uuid.Nil, uid)
	assert.EqualError(t, err, invalidLen)
}

func TestValidateCorrectUUID(t *testing.T) {
	testValidator := NewValidator()

	uid, err := testValidator.UUID(uuidStr)
	assert.Nil(t, err)
	assert.Equal(t, uid.String(), uuidStr)
}
