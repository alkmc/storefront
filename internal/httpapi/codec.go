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
	appJSON = "application/json"

	msgEncodeFailed  = "error encoding data"
	msgBodyTooLarge  = "request body too large"
	msgEmptyBody     = "request body must not be empty"
	msgMalformedJSON = "request body contains malformed JSON"
	msgInvalidBody   = "invalid request body"
)

type errorResponse struct {
	Message string `json:"message"`
}

// respond replies to the request with the specified payload and HTTP code
func respond(w http.ResponseWriter, httpCode int, payload any) {
	w.Header().Set("Content-Type", appJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	w.WriteHeader(httpCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, msgEncodeFailed, http.StatusInternalServerError)
	}
}

// respondError replies to the request with an error message and its HTTP code
func respondError(w http.ResponseWriter, code int, msg string) {
	respond(w, code, errorResponse{Message: msg})
}

// respondDecodeError responds to a decoder error
func respondDecodeError(w http.ResponseWriter, err error) {
	msg, status := mapDecodeError(err)
	respondError(w, status, msg)
}

// decodeBody decodes request body to given struct
func decodeBody(r io.ReadCloser, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// mapDecodeError returns the client-facing message and HTTP status for a decoder error
func mapDecodeError(err error) (msg string, status int) {
	if mbe, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return fmt.Sprintf("%s: max %d bytes", msgBodyTooLarge, mbe.Limit), http.StatusRequestEntityTooLarge
	}
	if _, ok := errors.AsType[*json.SyntaxError](err); ok {
		return msgMalformedJSON, http.StatusUnprocessableEntity
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return msgMalformedJSON, http.StatusUnprocessableEntity
	}
	if ute, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return fmt.Sprintf("invalid value for the %q field", ute.Field), http.StatusUnprocessableEntity
	}
	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		return strings.TrimPrefix(err.Error(), "json: "), http.StatusUnprocessableEntity
	}
	if errors.Is(err, io.EOF) {
		return msgEmptyBody, http.StatusBadRequest
	}
	return msgInvalidBody, http.StatusUnprocessableEntity
}
