package checkpay

import (
	"context"
	"fmt"
	"time"

	"github.com/dmmoody/poz-pay/internal/decide"
)

// AccountReader resolves accounts for decide/upload.
type AccountReader interface {
	FIExists(ctx context.Context, fiID string) (bool, error)
	PartyBelongsToFI(ctx context.Context, fiID, partyID string) (bool, error)
	GetAccount(ctx context.Context, fiID, accountID string) (*Account, error)
	AccountOnParty(ctx context.Context, fiID, partyID, accountID string) (bool, error)
}

// RegisterReader loads register items for decide.
type RegisterReader interface {
	FindItem(ctx context.Context, fiID, accountID, checkNumber string) (*RegisterItem, error)
}

// RegisterWriter persists upload batches and paid marks.
type RegisterWriter interface {
	InsertBatch(ctx context.Context, fiID, partyID string, items []UploadItem, payeeNorm func(string) string) (UploadBatchResult, error)
	MarkPaid(ctx context.Context, fiID, accountID, checkNumber string) (bool, error)
}

// PolicyResolver loads check decide policy for an FI/party.
type PolicyResolver interface {
	ResolvePolicy(ctx context.Context, fiID, partyID string) (Policy, error)
}

// DecisionAuditor appends audit rows.
type DecisionAuditor interface {
	WriteCheckDecision(ctx context.Context, fiID, partyID, accountID, correlationID string, req any, result decide.Result) (string, error)
}

// UploadItem is a register line accepted by the service.
type UploadItem struct {
	AccountID   string
	CheckNumber string
	Amount      int64
	Payee       string
	IssueDate   *time.Time
	Void        bool
}

// UploadBatchResult is returned to HTTP after upload.
type UploadBatchResult struct {
	BatchID  string
	Accepted int
	Rejected int
	Errors   []UploadItemError
}

// UploadItemError is a per-row reject.
type UploadItemError struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// Service orchestrates check positive pay decide and register upload.
type Service struct {
	accounts AccountReader
	register RegisterReader
	writer   RegisterWriter
	policy   PolicyResolver
	audit    DecisionAuditor
}

func NewService(
	accounts AccountReader,
	register RegisterReader,
	writer RegisterWriter,
	policy PolicyResolver,
	audit DecisionAuditor,
) *Service {
	return &Service{
		accounts: accounts,
		register: register,
		writer:   writer,
		policy:   policy,
		audit:    audit,
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
var ErrFINotFound = fmt.Errorf("financial institution not found")

// ErrPartyNotFound is returned when party is unknown under FI.
var ErrPartyNotFound = fmt.Errorf("party not found")

// Decide evaluates a presented check, marks paid on pay, and audits the outcome.
func (s *Service) Decide(ctx context.Context, in DecideInput) (decide.Result, error) {
	start := time.Now()

	ok, err := s.accounts.FIExists(ctx, in.FIID)
	if err != nil {
		return decide.Result{}, err
	}
	if !ok {
		return decide.Result{}, ErrFINotFound
	}

	acct, err := s.accounts.GetAccount(ctx, in.FIID, in.AccountID)
	if err != nil {
		return decide.Result{}, err
	}

	var item *RegisterItem
	if acct != nil {
		item, err = s.register.FindItem(ctx, in.FIID, in.AccountID, in.Check.CheckNumber)
		if err != nil {
			return decide.Result{}, err
		}
	}

	pol := Policy{}
	if acct != nil && s.policy != nil {
		pol, err = s.policy.ResolvePolicy(ctx, in.FIID, acct.PartyID)
		if err != nil {
			return decide.Result{}, err
		}
	}

	asOf := time.Now().UTC()
	result := EvaluateCheck(acct, item, in.Check, pol, asOf)

	// Successful pay marks the register item so a second presentment is CHECK_ALREADY_PAID.
	if result.Decision == decide.DecisionPay && acct != nil {
		marked, err := s.writer.MarkPaid(ctx, in.FIID, in.AccountID, in.Check.CheckNumber)
		if err != nil {
			return decide.Result{}, fmt.Errorf("mark paid: %w", err)
		}
		if !marked {
			item, err = s.register.FindItem(ctx, in.FIID, in.AccountID, in.Check.CheckNumber)
			if err != nil {
				return decide.Result{}, err
			}
			result = EvaluateCheck(acct, item, in.Check, pol, asOf)
		}
	}

	result.CorrelationID = in.CorrelationID
	result.LatencyMs = time.Since(start).Milliseconds()

	partyID := ""
	accountID := ""
	if acct != nil {
		partyID = acct.PartyID
		accountID = acct.ID
	}

	if _, err := s.audit.WriteCheckDecision(ctx, in.FIID, partyID, accountID, in.CorrelationID, in.RawRequest, result); err != nil {
		return decide.Result{}, fmt.Errorf("audit decide: %w", err)
	}
	return result, nil
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

	dbResult, err := s.writer.InsertBatch(ctx, fiID, partyID, prepared, NormalizePayee)
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
