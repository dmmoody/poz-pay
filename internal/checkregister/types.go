package checkregister

import (
	"errors"
	"time"
)

var (
	// ErrNotInRegister means no register row matches the check number.
	ErrNotInRegister = errors.New("check not in register")

	// ErrVoided means the matching register row is voided.
	ErrVoided = errors.New("check voided")
)

// RegisterItem is an issued check row used for matching.
type RegisterItem struct {
	ID                string
	CheckNumber       string
	Amount            int64
	Payee             string
	PayeeNormalized   string
	IssueDate         *time.Time
	VoidedAt          *time.Time
	PaidAt            *time.Time
	PaidCorrelationID string
}

// UploadItem is one register line to insert.
type UploadItem struct {
	AccountID   string
	CheckNumber string
	Amount      int64
	Payee       string
	IssueDate   *time.Time
	Void        bool
}

// UploadBatchResult is the outcome of inserting a JSON register batch.
type UploadBatchResult struct {
	BatchID  string
	Accepted int
	Rejected int
	Errors   []UploadItemError
}

// UploadItemError is a per-row reject from a JSON register batch.
type UploadItemError struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// Payment is the posting recorded against a register item.
type Payment struct {
	RegisterItemID string    `json:"register_item_id"`
	CheckNumber    string    `json:"check_number"`
	PaidAt         time.Time `json:"paid_at"`
	CorrelationID  string    `json:"correlation_id,omitempty"`
	// AlreadyPaid reports that this call found an existing posting.
	AlreadyPaid bool `json:"-"`
}

// PaymentRecord is the posting written with its audit event in one transaction.
type PaymentRecord struct {
	FIID          string
	PartyID       string
	AccountID     string
	CheckNumber   string
	CorrelationID string
	PaidAt        time.Time
	Request       any
}
