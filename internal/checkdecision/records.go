package checkdecision

import "github.com/dmmoody/poz-pay/internal/store"

const DefaultMaxAgeDays = store.DefaultMaxAgeDays

const (
	AuditKindPresentment = store.KindPresentment
	AuditKindPosted      = store.KindPosted
)

type (
	Account          = store.Account
	RegisterItem     = store.RegisterItem
	Policy           = store.Policy
	CheckAuditEntry  = store.CheckEntry
	RecordedDecision = store.RecordedDecision
	CheckAuditEvent  = store.CheckEvent
	PresentmentAudit = store.Presentment
	PostedAudit      = store.Posted
)
