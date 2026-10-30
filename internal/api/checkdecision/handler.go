package checkdecision

import decisionsvc "github.com/dmmoody/poz-pay/internal/checkdecision"

// Handler serves check decisioning and the history of those decisions.
type Handler struct {
	Decisions *decisionsvc.Service
}
