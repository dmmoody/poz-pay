package checkposting

import postingsvc "github.com/dmmoody/poz-pay/internal/checkposting"

// Handler serves the record that the core posted a check.
type Handler struct {
	Postings *postingsvc.Service
}
