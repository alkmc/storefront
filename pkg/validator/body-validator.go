package validator

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
)

// Body checks for various possible request body decoding errors
func (v *productValidator) Body(err error) error {

	var syntaxError *json.SyntaxError
	var unmarshalTypeError *json.UnmarshalTypeError

	switch {
	// Catch any syntax errors in the JSON
	case errors.As(err, &syntaxError):
		return fmt.Errorf("request body contains badly-formed JSON at position: %d", syntaxError.Offset)

	// In some circumstances Decode() may also return an
	// io.ErrUnexpectedEOF error for syntax errors in the JSON
	case errors.Is(err, io.ErrUnexpectedEOF):
		return fmt.Errorf("request body contains badly-formed JSON")

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

	// Otherwise log the error
	default:
		log.Println(err.Error())
		return errors.New("error decoding JSON")
		//http.StatusInternalServerError)
	}
}
