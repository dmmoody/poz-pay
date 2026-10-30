package api

import (
	"github.com/dmmoody/poz-pay/internal/checkpay"
	"github.com/dmmoody/poz-pay/internal/registerupload"
)

// Handlers holds the dependencies for every route in this package.
// Route methods live in the file for that route group.
type Handlers struct {
	Checks         *checkpay.Service
	Uploads        *registerupload.Service
	DB             DBPinger
	MaxUploadBytes int64
}
