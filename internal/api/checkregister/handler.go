package checkregister

import (
	"github.com/dmmoody/poz-pay/internal/registerbatch"
	upload "github.com/dmmoody/poz-pay/internal/registerupload"
)

// Handler serves check-register intake. JSON and CSV share one route.
type Handler struct {
	Batches        *registerbatch.Service
	Uploads        *upload.Service
	MaxUploadBytes int64
}
