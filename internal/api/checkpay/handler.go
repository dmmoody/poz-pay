package checkpay

import paysvc "github.com/dmmoody/poz-pay/internal/checkpay"

// Handler serves check positive pay decide, posting, and history routes.
type Handler struct {
	Checks *paysvc.Service
}
