package api

import (
	"net/http"

	"github.com/dmmoody/poz-pay/internal/api/checkdecision"
	"github.com/dmmoody/poz-pay/internal/api/checkposting"
	"github.com/dmmoody/poz-pay/internal/api/checkregister"
	"github.com/dmmoody/poz-pay/internal/api/health"
)

// Routes is the set of HTTP behavior this server mounts.
type Routes struct {
	Health    health.Handler
	Decisions checkdecision.Handler
	Postings  checkposting.Handler
	Registers checkregister.Handler
}

// NewRouter registers all HTTP routes on a ServeMux.
func NewRouter(routes Routes) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", routes.Health.Healthz)
	mux.HandleFunc("POST /v1/decide", routes.Decisions.Decide)
	mux.HandleFunc("POST /v1/fis/{fi_id}/accounts/{account_id}/check-postings", routes.Postings.PostCheck)
	mux.HandleFunc("GET /v1/fis/{fi_id}/accounts/{account_id}/checks/{check_number}/decisions", routes.Decisions.ListCheckDecisions)
	mux.HandleFunc("GET /v1/fis/{fi_id}/accounts/{account_id}/checks/{check_number}/audit", routes.Decisions.ListCheckAudit)
	mux.HandleFunc("POST /v1/fis/{fi_id}/parties/{party_id}/check-registers", routes.Registers.UploadCheckRegister)
	mux.HandleFunc("GET /v1/fis/{fi_id}/parties/{party_id}/check-register-uploads/{upload_id}", routes.Registers.GetCheckRegisterUpload)
	return mux
}
