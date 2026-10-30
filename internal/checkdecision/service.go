// Package checkdecision evaluates a presented check and reads that check's history.
// A decision does not pay the item. Posting is a separate process.
package checkdecision

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dmmoody/poz-pay/internal/decision"
	"github.com/dmmoody/poz-pay/internal/store"
)

// Accounts resolves the institution and the account a check is drawn on.
type Accounts interface {
	FIExists(ctx context.Context, fiID string) (bool, error)
	GetAccount(ctx context.Context, fiID, accountID string) (*Account, error)
}

// Register loads the issued item a presentment is compared to.
type Register interface {
	FindItem(ctx context.Context, fiID, accountID, checkNumber string) (*RegisterItem, error)
}

// PolicyResolver loads check decide policy for an FI/party.
type PolicyResolver interface {
	ResolvePolicy(ctx context.Context, fiID, partyID string) (Policy, error)
}

// DecisionLog appends presentments and lists the check's audit stream.
type DecisionLog interface {
	WriteCheckDecision(ctx context.Context, entry CheckAuditEntry) (string, error)
	ListCheckDecisions(ctx context.Context, fiID, accountID, checkNumber string) ([]RecordedDecision, error)
	ListCheckAudit(ctx context.Context, fiID, accountID, checkNumber string) ([]CheckAuditEvent, error)
}

// CheckHistory is every decide for one check number, plus the posting if any.
type CheckHistory struct {
	CheckNumber       string             `json:"check_number"`
	RegisterItemID    string             `json:"register_item_id,omitempty"`
	PaidAt            *time.Time         `json:"paid_at"`
	PaidCorrelationID string             `json:"paid_correlation_id,omitempty"`
	Decisions         []RecordedDecision `json:"decisions"`
}

// CheckAudit is every presentment and the posting for one check, oldest first.
type CheckAudit struct {
	CheckNumber    string            `json:"check_number"`
	RegisterItemID string            `json:"register_item_id,omitempty"`
	Events         []CheckAuditEvent `json:"events"`
}

// Service evaluates presentments and reads the resulting history.
type Service struct {
	accounts  Accounts
	register  Register
	policy    PolicyResolver
	decisions DecisionLog
}

func NewService(accounts Accounts, register Register, policy PolicyResolver, decisions DecisionLog) *Service {
	return &Service{
		accounts:  accounts,
		register:  register,
		policy:    policy,
		decisions: decisions,
	}
}

// DecideInput is the check rail decide request (already validated).
type DecideInput struct {
	FIID          string
	AccountID     string
	CorrelationID string
	Check         PresentedCheck
	RawRequest    any
}

// ErrFINotFound is returned when fi_id is unknown (HTTP 404).
var ErrFINotFound = store.ErrInstitutionNotFound

// ErrAccountNotFound is returned when the account is unknown under the FI.
var ErrAccountNotFound = errors.New("account not found")

// ErrCheckNumberRequired is returned when a check number is blank.
var ErrCheckNumberRequired = errors.New("check number is required")

// Decide evaluates a presented check and appends one history row. It does not post the item.
func (s *Service) Decide(ctx context.Context, in DecideInput) (decision.Result, error) {
	start := time.Now()

	ok, err := s.accounts.FIExists(ctx, in.FIID)
	if err != nil {
		return decision.Result{}, err
	}
	if !ok {
		return decision.Result{}, ErrFINotFound
	}

	acct, err := s.accounts.GetAccount(ctx, in.FIID, in.AccountID)
	if err != nil {
		return decision.Result{}, err
	}

	var item *RegisterItem
	if acct != nil {
		item, err = s.register.FindItem(ctx, in.FIID, in.AccountID, in.Check.CheckNumber)
		if err != nil {
			return decision.Result{}, err
		}
	}

	pol := Policy{}
	if acct != nil && s.policy != nil {
		pol, err = s.policy.ResolvePolicy(ctx, in.FIID, acct.PartyID)
		if err != nil {
			return decision.Result{}, err
		}
	}

	asOf := time.Now().UTC()
	result := EvaluateCheck(acct, item, in.Check, pol, asOf)
	result.CorrelationID = in.CorrelationID
	result.LatencyMs = time.Since(start).Milliseconds()

	partyID := ""
	accountID := ""
	if acct != nil {
		partyID = acct.PartyID
		accountID = acct.ID
	}
	registerItemID := ""
	if item != nil {
		registerItemID = item.ID
	}

	if _, err := s.decisions.WriteCheckDecision(ctx, CheckAuditEntry{
		FIID:           in.FIID,
		PartyID:        partyID,
		AccountID:      accountID,
		CorrelationID:  in.CorrelationID,
		CheckNumber:    in.Check.CheckNumber,
		RegisterItemID: registerItemID,
		Request:        in.RawRequest,
		Result:         result,
	}); err != nil {
		return decision.Result{}, fmt.Errorf("audit decide: %w", err)
	}
	return result, nil
}

// ListDecisions returns every decide for the check, oldest first, and the posting when present.
func (s *Service) ListDecisions(ctx context.Context, fiID, accountID, checkNumber string) (CheckHistory, error) {
	checkNumber = strings.TrimSpace(checkNumber)
	if checkNumber == "" {
		return CheckHistory{}, ErrCheckNumberRequired
	}
	if _, err := s.requireAccount(ctx, fiID, accountID); err != nil {
		return CheckHistory{}, err
	}

	item, err := s.register.FindItem(ctx, fiID, accountID, checkNumber)
	if err != nil {
		return CheckHistory{}, err
	}
	decisions, err := s.decisions.ListCheckDecisions(ctx, fiID, accountID, checkNumber)
	if err != nil {
		return CheckHistory{}, err
	}
	if decisions == nil {
		decisions = []RecordedDecision{}
	}

	history := CheckHistory{
		CheckNumber: checkNumber,
		Decisions:   decisions,
	}
	if item != nil {
		history.RegisterItemID = item.ID
		history.PaidCorrelationID = item.PaidCorrelationID
		if item.PaidAt != nil {
			paidAt := item.PaidAt.UTC()
			history.PaidAt = &paidAt
		}
	}
	return history, nil
}

// ListAudit returns presentments and the posting for one check, oldest recorded first.
func (s *Service) ListAudit(ctx context.Context, fiID, accountID, checkNumber string) (CheckAudit, error) {
	checkNumber = strings.TrimSpace(checkNumber)
	if checkNumber == "" {
		return CheckAudit{}, ErrCheckNumberRequired
	}
	if _, err := s.requireAccount(ctx, fiID, accountID); err != nil {
		return CheckAudit{}, err
	}

	item, err := s.register.FindItem(ctx, fiID, accountID, checkNumber)
	if err != nil {
		return CheckAudit{}, err
	}
	events, err := s.decisions.ListCheckAudit(ctx, fiID, accountID, checkNumber)
	if err != nil {
		return CheckAudit{}, err
	}
	if events == nil {
		events = []CheckAuditEvent{}
	}
	audit := CheckAudit{
		CheckNumber: checkNumber,
		Events:      events,
	}
	if item != nil {
		audit.RegisterItemID = item.ID
	}
	return audit, nil
}

func (s *Service) requireAccount(ctx context.Context, fiID, accountID string) (*Account, error) {
	ok, err := s.accounts.FIExists(ctx, fiID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrFINotFound
	}
	acct, err := s.accounts.GetAccount(ctx, fiID, accountID)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return nil, ErrAccountNotFound
	}
	return acct, nil
}
