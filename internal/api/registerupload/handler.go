package registerupload

import (
	paysvc "github.com/dmmoody/poz-pay/internal/checkpay"
	upload "github.com/dmmoody/poz-pay/internal/registerupload"
)

// Handler serves check-register intake. JSON and CSV share one route.
type Handler struct {
	Checks         *paysvc.Service
	Uploads        *upload.Service
	MaxUploadBytes int64
}
