package validator

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
)

type validator struct{}

// NewValidator returns new Validator
func NewValidator() *validator {
	return new(validator{})
}

func (v *validator) Product(p *entity.Product) error {
	if p == nil {
		err := errors.New("the product is empty")
		return err
	}
	if p.Name == "" {
		err := errors.New("the product name is empty")
		return err
	}
	if p.Price <= 0 {
		err := errors.New("the product price must be positive")
		return err
	}
	return nil
}

func (v *validator) UUID(id string) (uuid.UUID, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, err
	}
	return uid, nil
}

// DecodeError checks for various possible request body decoding errors
func (v *validator) DecodeError(err error) error {
	var (
		syntaxError        *json.SyntaxError
		unmarshalTypeError *json.UnmarshalTypeError
	)

	switch {
	// Catch any syntax errors in the JSON
	case errors.As(err, &syntaxError):
		return fmt.Errorf("request body contains badly-formed JSON at position: %d", syntaxError.Offset)

	// Decode() can also return io.ErrUnexpectedEOF for JSON syntax errors
	case errors.Is(err, io.ErrUnexpectedEOF):
		return errors.New("request body contains badly-formed JSON")

	// Catch any type errors
	case errors.As(err, &unmarshalTypeError):
		return fmt.Errorf("invalid value for the %q field at position: %d",
			unmarshalTypeError.Field, unmarshalTypeError.Offset)

	// Catch the error caused by extra unexpected fields
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		fieldErr := strings.TrimPrefix(err.Error(), "json: ")
		return errors.New(fieldErr)

	// An io.EOF error is returned by Decode() if the request body is empty
	case errors.Is(err, io.EOF):
		return errors.New("request body must not be empty")

	// Otherwise return wrapped error
	default:
		return fmt.Errorf("error decoding JSON: %w", err)
	}
}
