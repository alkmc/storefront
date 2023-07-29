package renderer

import (
	"encoding/json"
	"io"
	"net/http"
)

const (
	appJSON  = "application/json"
	encError = "error encoding data"
)

// JSON replies to the request with the specified payload and HTTP code
func JSON(w http.ResponseWriter, httpCode int, payload any) {
	w.Header().Set("Content-Type", appJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	w.WriteHeader(httpCode)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		http.Error(w, encError, http.StatusInternalServerError)
	}
}

// Decode decodes request body to given struct
func Decode(r io.ReadCloser, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return err
	}
	return nil
}
