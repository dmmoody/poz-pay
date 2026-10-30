package checkpay

import "github.com/dmmoody/poz-pay/internal/store"

const DefaultMaxAgeDays = store.DefaultMaxAgeDays

const (
	AuditKindPresentment = store.KindPresentment
	AuditKindPosted      = store.KindPosted
)

type (
	Account           = store.Account
	RegisterItem      = store.RegisterItem
	UploadItem        = store.UploadItem
	UploadBatchResult = store.UploadBatchResult
	UploadItemError   = store.UploadItemError
	Payment           = store.Payment
	PaymentRecord     = store.PaymentRecord
	Policy            = store.Policy
	CheckAuditEntry   = store.CheckEntry
	RecordedDecision  = store.RecordedDecision
	CheckAuditEvent   = store.CheckEvent
	PresentmentAudit  = store.Presentment
	PostedAudit       = store.Posted
)
