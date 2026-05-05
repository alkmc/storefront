package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
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

// decodeBody decodes request body to given struct
func decodeBody(r io.ReadCloser, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return err
	}
	return nil
}
