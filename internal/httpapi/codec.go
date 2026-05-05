package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/alkmc/restClean/internal/serviceerr"
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
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		http.Error(w, encError, http.StatusInternalServerError)
	}
}

// respondError replies to the request with the service error and its mapped HTTP code
func respondError(w http.ResponseWriter, err *serviceerr.ServiceError) {
	codes := map[string]int{
		"product validation error": http.StatusUnprocessableEntity,
		"invalid input error":      http.StatusBadRequest,
		"request body error":       http.StatusUnprocessableEntity,
	}
	code, ok := codes[err.Code]
	if !ok {
		code = http.StatusInternalServerError
	}
	respond(w, code, err)
}

// decodeBody decodes request body to given struct
func decodeBody(r io.ReadCloser, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return err
	}
	return nil
}
