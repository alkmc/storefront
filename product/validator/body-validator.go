package validator

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
)

//Body checks for various possible request body decoding errors
func (v *productValidator) Body(err error) error {

	var syntaxError *json.SyntaxError
	var unmarshalTypeError *json.UnmarshalTypeError

	switch {
	// Catch any syntax errors in the JSON
	case errors.As(err, &syntaxError):
		return fmt.Errorf("request body contains badly-formed JSON at position: %d", syntaxError.Offset)
		// http.Error(w, msg, http.StatusBadRequest)

	// In some circumstances Decode() may also return an
	// io.ErrUnexpectedEOF error for syntax errors in the JSON
	case errors.Is(err, io.ErrUnexpectedEOF):
		return fmt.Errorf("request body contains badly-formed JSON")
		// http.Error(w, msg, http.StatusBadRequest)

	// Catch any type errors
	case errors.As(err, &unmarshalTypeError):
		return fmt.Errorf("invalid value for the %q field at position: %d",
			unmarshalTypeError.Field, unmarshalTypeError.Offset)
		// http.Error(w, msg, http.StatusBadRequest)

	// Catch the error caused by extra unexpected fields
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
		return fmt.Errorf("unknown field %s", fieldName)
		// http.Error(w, msg, http.StatusBadRequest)

	// An io.EOF error is returned by Decode() if the request body is empty
	case errors.Is(err, io.EOF):
		return errors.New("request body must not be empty")
		// http.Error(w, msg, http.StatusBadRequest)

	// Otherwise log the error
	default:
		log.Println(err.Error())
		return errors.New("error decoding JSON")
		// http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
