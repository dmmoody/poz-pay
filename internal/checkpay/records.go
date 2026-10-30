package checkpay

import (
	"github.com/dmmoody/poz-pay/internal/audit"
	"github.com/dmmoody/poz-pay/internal/checkregister"
	"github.com/dmmoody/poz-pay/internal/checksettings"
	"github.com/dmmoody/poz-pay/internal/tenant"
)

const DefaultMaxAgeDays = checksettings.DefaultMaxAgeDays

const (
	AuditKindPresentment = audit.KindPresentment
	AuditKindPosted      = audit.KindPosted
)

type (
	Account           = tenant.Account
	RegisterItem      = checkregister.RegisterItem
	UploadItem        = checkregister.UploadItem
	UploadBatchResult = checkregister.UploadBatchResult
	UploadItemError   = checkregister.UploadItemError
	Payment           = checkregister.Payment
	PaymentRecord     = checkregister.PaymentRecord
	Policy            = checksettings.Policy
	CheckAuditEntry   = audit.CheckEntry
	RecordedDecision  = audit.RecordedDecision
	CheckAuditEvent   = audit.CheckEvent
	PresentmentAudit  = audit.Presentment
	PostedAudit       = audit.Posted
)

var (
	_ AccountReader  = (*tenant.Store)(nil)
	_ Register       = (*checkregister.Store)(nil)
	_ PolicyResolver = (*checksettings.Store)(nil)
	_ DecisionLog    = (*audit.Store)(nil)
)
