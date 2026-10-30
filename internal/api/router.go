package api

import (
	"net/http"

	payhttp "github.com/dmmoody/poz-pay/internal/api/checkpay"
	"github.com/dmmoody/poz-pay/internal/api/health"
	reghttp "github.com/dmmoody/poz-pay/internal/api/registerupload"
)

// Routes is the set of HTTP behavior this server mounts.
type Routes struct {
	Health    health.Handler
	Checks    payhttp.Handler
	Registers reghttp.Handler
}

// NewRouter registers all HTTP routes on a ServeMux.
func NewRouter(routes Routes) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", routes.Health.Healthz)
	mux.HandleFunc("POST /v1/decide", routes.Checks.Decide)
	mux.HandleFunc("POST /v1/fis/{fi_id}/accounts/{account_id}/check-postings", routes.Checks.PostCheck)
	mux.HandleFunc("GET /v1/fis/{fi_id}/accounts/{account_id}/checks/{check_number}/decisions", routes.Checks.ListCheckDecisions)
	mux.HandleFunc("GET /v1/fis/{fi_id}/accounts/{account_id}/checks/{check_number}/audit", routes.Checks.ListCheckAudit)
	mux.HandleFunc("POST /v1/fis/{fi_id}/parties/{party_id}/check-registers", routes.Registers.UploadCheckRegister)
	mux.HandleFunc("GET /v1/fis/{fi_id}/parties/{party_id}/check-register-uploads/{upload_id}", routes.Registers.GetCheckRegisterUpload)
	return mux
}
