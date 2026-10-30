// Package response writes the JSON bodies shared by the HTTP handlers.
// Those packages cannot import the parent api package.
package response

import (
	"encoding/json"
	"net/http"
)

type body struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// WriteJSON sets the JSON content type and encodes v.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes the API error envelope.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	var b body
	b.Error.Code = code
	b.Error.Message = message
	WriteJSON(w, status, b)
}

// LooksLikeUUID reports whether s has the 8-4-4-4-12 hex shape.
func LooksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
	}
	return true
}
