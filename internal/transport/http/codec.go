package http

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const (
	MediaTypeJSON = "application/json"

	msgEncodeFailed    = "error encoding data"
	msgBodyTooLarge    = "request body too large"
	msgEmptyBody       = "request body must not be empty"
	msgMalformedJSON   = "request body contains malformed JSON"
	msgInvalidBody     = "invalid request body"
	msgInternalError   = "internal server error"
	msgUnavailable     = "service temporarily unavailable"
	msgInvalidQuantity = "quantity must be between 1 and 10000"

	headerIdempotencyKey      = "Idempotency-Key"
	headerIdempotencyReplayed = "Idempotency-Replayed"
	msgIdempotencyMismatch    = "idempotency key reused with different payload"
	msgIdempotencyRequired    = "Idempotency-Key header is required"
)

type messageResponse struct {
	Message string `json:"message"`
}

// respond replies to the request with the specified payload and HTTP code
func respond(w http.ResponseWriter, httpCode int, payload any) {
	w.Header().Set("Content-Type", MediaTypeJSON)
	w.WriteHeader(httpCode)
	if err := json.MarshalWrite(w, payload); err != nil {
		http.Error(w, msgEncodeFailed, http.StatusInternalServerError)
	}
}

// respondError replies to the request with an error message and its HTTP code
func respondError(w http.ResponseWriter, code int, msg string) {
	respond(w, code, messageResponse{Message: msg})
}

// respondDecodeError responds to a decoder error
func respondDecodeError(w http.ResponseWriter, err error) {
	msg, status := mapDecodeError(err)
	respondError(w, status, msg)
}

// decodeBody decodes request body to given struct
func decodeBody(r io.ReadCloser, v any) error {
	return json.UnmarshalRead(r, v, json.RejectUnknownMembers(true))
}

// mapDecodeError returns the client-facing message and HTTP status for a decoder error
func mapDecodeError(err error) (msg string, status int) {
	if mbe, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return fmt.Sprintf("%s: max %d bytes", msgBodyTooLarge, mbe.Limit), http.StatusRequestEntityTooLarge
	}
	return describeDecodeError(err), http.StatusBadRequest
}

// describeDecodeError describes a decoder error in terms the client can act on
func describeDecodeError(err error) string {
	if _, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
		return msgMalformedJSON
	}
	if se, ok := errors.AsType[*json.SemanticError](err); ok {
		switch field := se.JSONPointer.LastToken(); {
		case errors.Is(se.Err, json.ErrUnknownName):
			return fmt.Sprintf("unknown field %q", field)
		case field != "":
			return fmt.Sprintf("invalid value for the %q field", field)
		}
	}
	return msgInvalidBody
}
