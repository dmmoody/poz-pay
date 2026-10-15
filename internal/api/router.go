package api

import "net/http"

// NewRouter registers all HTTP routes on a ServeMux.
func NewRouter(h *Handlers) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.HandleFunc("POST /v1/decide", h.Decide)
	mux.HandleFunc("POST /v1/fis/{fi_id}/parties/{party_id}/check-registers", h.UploadCheckRegister)
	return mux
}
