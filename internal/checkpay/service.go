package checkpay

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dmmoody/poz-pay/internal/decision"
	"github.com/dmmoody/poz-pay/internal/store"
)

// AccountReader resolves accounts for decide/upload.
type AccountReader interface {
	FIExists(ctx context.Context, fiID string) (bool, error)
	PartyBelongsToFI(ctx context.Context, fiID, partyID string) (bool, error)
	GetAccount(ctx context.Context, fiID, accountID string) (*Account, error)
	AccountOnParty(ctx context.Context, fiID, partyID, accountID string) (bool, error)
}

// Register loads register items and persists batches and postings.
type Register interface {
	FindItem(ctx context.Context, fiID, accountID, checkNumber string) (*RegisterItem, error)
	InsertBatch(ctx context.Context, fiID, partyID string, items []UploadItem, payeeNorm func(string) string) (UploadBatchResult, error)
	RecordPayment(ctx context.Context, rec PaymentRecord) (Payment, error)
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

// PostInput records that the core posted a check.
type PostInput struct {
	FIID          string
	AccountID     string
	CheckNumber   string
	CorrelationID string
	PaidAt        time.Time
	RawRequest    any
}

// CheckAudit is every presentment and the posting for one check, oldest first.
type CheckAudit struct {
	CheckNumber    string            `json:"check_number"`
	RegisterItemID string            `json:"register_item_id,omitempty"`
	Events         []CheckAuditEvent `json:"events"`
}

// Service orchestrates check positive pay decide and register upload.
type Service struct {
	accounts  AccountReader
	register  Register
	policy    PolicyResolver
	decisions DecisionLog
}

func NewService(
	accounts AccountReader,
	register Register,
	policy PolicyResolver,
	decisions DecisionLog,
) *Service {
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

// ErrPartyNotFound is returned when party is unknown under FI.
var ErrPartyNotFound = store.ErrPartyNotFound

// ErrAccountNotFound is returned when the account is unknown under the FI.
var ErrAccountNotFound = fmt.Errorf("account not found")

// ErrCheckNotInRegister is returned when a posting names no register row.
var ErrCheckNotInRegister = store.ErrNotInRegister

// ErrCheckVoided is returned when a posting targets a voided register row.
var ErrCheckVoided = store.ErrVoided

// ErrCheckNumberRequired is returned when a check number is blank.
var ErrCheckNumberRequired = fmt.Errorf("check number is required")

// ErrPaidAtRequired is returned when a posting omits the paying system's timestamp.
var ErrPaidAtRequired = fmt.Errorf("paid_at is required")

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

// Post records settlement for a register item. A repeated post returns the original posting.
func (s *Service) Post(ctx context.Context, in PostInput) (Payment, error) {
	in.CheckNumber = strings.TrimSpace(in.CheckNumber)
	in.CorrelationID = strings.TrimSpace(in.CorrelationID)
	if in.CheckNumber == "" {
		return Payment{}, ErrCheckNumberRequired
	}
	if in.PaidAt.IsZero() {
		return Payment{}, ErrPaidAtRequired
	}

	acct, err := s.requireAccount(ctx, in.FIID, in.AccountID)
	if err != nil {
		return Payment{}, err
	}

	payment, err := s.register.RecordPayment(ctx, PaymentRecord{
		FIID:          in.FIID,
		PartyID:       acct.PartyID,
		AccountID:     acct.ID,
		CheckNumber:   in.CheckNumber,
		CorrelationID: in.CorrelationID,
		PaidAt:        in.PaidAt.UTC(),
		Request:       in.RawRequest,
	})
	if err != nil {
		return Payment{}, err
	}
	payment.CheckNumber = in.CheckNumber
	payment.PaidAt = payment.PaidAt.UTC()
	return payment, nil
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

// Upload validates ownership and inserts a register batch.
func (s *Service) Upload(ctx context.Context, fiID, partyID string, items []UploadItem) (UploadBatchResult, error) {
	ok, err := s.accounts.FIExists(ctx, fiID)
	if err != nil {
		return UploadBatchResult{}, err
	}
	if !ok {
		return UploadBatchResult{}, ErrFINotFound
	}
	ok, err = s.accounts.PartyBelongsToFI(ctx, fiID, partyID)
	if err != nil {
		return UploadBatchResult{}, err
	}
	if !ok {
		return UploadBatchResult{}, ErrPartyNotFound
	}

	var (
		prepared  []UploadItem
		origIndex []int
		preErrors []UploadItemError
	)
	for i, it := range items {
		if it.AccountID == "" || it.CheckNumber == "" || it.Payee == "" {
			preErrors = append(preErrors, UploadItemError{Index: i, Message: "account_id, check_number, and payee are required"})
			continue
		}
		if it.Amount < 0 {
			preErrors = append(preErrors, UploadItemError{Index: i, Message: "amount must be >= 0"})
			continue
		}
		belongs, err := s.accounts.AccountOnParty(ctx, fiID, partyID, it.AccountID)
		if err != nil {
			return UploadBatchResult{}, err
		}
		if !belongs {
			preErrors = append(preErrors, UploadItemError{Index: i, Message: "account does not belong to party"})
			continue
		}
		prepared = append(prepared, it)
		origIndex = append(origIndex, i)
	}

	dbResult, err := s.register.InsertBatch(ctx, fiID, partyID, prepared, NormalizePayee)
	if err != nil {
		return UploadBatchResult{}, err
	}

	out := UploadBatchResult{
		BatchID:  dbResult.BatchID,
		Accepted: dbResult.Accepted,
		Rejected: dbResult.Rejected + len(preErrors),
		Errors:   append([]UploadItemError{}, preErrors...),
	}
	for _, e := range dbResult.Errors {
		idx := e.Index
		if e.Index >= 0 && e.Index < len(origIndex) {
			idx = origIndex[e.Index]
		}
		out.Errors = append(out.Errors, UploadItemError{Index: idx, Message: e.Message})
	}
	return out, nil
}
