package audit

import (
	"encoding/json"
	"time"

	"github.com/dmmoody/poz-pay/internal/decision"
)

const (
	// KindPresentment is a decide call on the check's audit stream.
	KindPresentment = "presentment"

	// KindPosted is the paying system's settlement on that same stream.
	KindPosted = "posted"
)

// CheckEntry is one presentment attempt written to decision history.
type CheckEntry struct {
	FIID           string
	PartyID        string
	AccountID      string
	CorrelationID  string
	CheckNumber    string
	RegisterItemID string
	Request        any
	Result         decision.Result
}

// RecordedDecision is one stored presentment, oldest first when listed.
type RecordedDecision struct {
	ID             string               `json:"id"`
	Decision       string               `json:"decision"`
	Match          *decision.Match      `json:"match"`
	Exceptions     []decision.Exception `json:"exceptions"`
	CorrelationID  string               `json:"correlation_id,omitempty"`
	RegisterItemID string               `json:"register_item_id,omitempty"`
	Request        json.RawMessage      `json:"request"`
	DecidedAt      time.Time            `json:"decided_at"`
	LatencyMs      int64                `json:"latency_ms"`
}

// CheckEvent is one row on the check's audit stream.
type CheckEvent struct {
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	RecordedAt     time.Time       `json:"recorded_at"`
	CorrelationID  string          `json:"correlation_id,omitempty"`
	RegisterItemID string          `json:"register_item_id,omitempty"`
	Request        json.RawMessage `json:"request"`
	Presentment    *Presentment    `json:"presentment,omitempty"`
	Posted         *Posted         `json:"posted,omitempty"`
}

// Presentment is the decide outcome stored on a presentment event.
type Presentment struct {
	Decision   string               `json:"decision"`
	Match      *decision.Match      `json:"match"`
	Exceptions []decision.Exception `json:"exceptions"`
	LatencyMs  int64                `json:"latency_ms"`
}

// Posted is the settlement stored on a posted event.
type Posted struct {
	PaidAt time.Time `json:"paid_at"`
}

// PostedCheck is the settlement written in the same transaction as the register posting.
type PostedCheck struct {
	FIID          string
	PartyID       string
	AccountID     string
	CheckNumber   string
	CorrelationID string
	Request       any
}
