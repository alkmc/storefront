package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	appJSON  = "application/json"
	encError = "error encoding data"
)

// respond replies to the request with the specified payload and HTTP code
func respond(w http.ResponseWriter, httpCode int, payload any) {
	w.Header().Set("Content-Type", appJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	w.WriteHeader(httpCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, encError, http.StatusInternalServerError)
	}
}

// respondError replies to the request with an error message and its HTTP code
func respondError(w http.ResponseWriter, code int, msg string) {
	respond(w, code, map[string]string{"message": msg})
}

// decodeBody decodes request body to given struct and translates errors
func decodeBody(r io.ReadCloser, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return mapDecodeError(err)
	}
	return nil
}

func mapDecodeError(err error) error {
	var (
		syntaxError        *json.SyntaxError
		unmarshalTypeError *json.UnmarshalTypeError
	)

	switch {
	case errors.As(err, &syntaxError):
		return fmt.Errorf("request body contains badly-formed JSON at position: %d", syntaxError.Offset)
	case errors.Is(err, io.ErrUnexpectedEOF):
		return errors.New("request body contains badly-formed JSON")
	case errors.As(err, &unmarshalTypeError):
		return fmt.Errorf("invalid value for the %q field at position: %d",
			unmarshalTypeError.Field, unmarshalTypeError.Offset)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		fieldErr := strings.TrimPrefix(err.Error(), "json: ")
		return errors.New(fieldErr)
	case errors.Is(err, io.EOF):
		return errors.New("request body must not be empty")
	default:
		return fmt.Errorf("error decoding JSON: %w", err)
	}
}
