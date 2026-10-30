// Package registerbatch issues a JSON check-register batch for one party.
// CSV file intake is a different process.
package registerbatch

import (
	"context"

	"github.com/dmmoody/poz-pay/internal/payee"
	"github.com/dmmoody/poz-pay/internal/store"
)

// Accounts checks that the institution, party, and account line up.
type Accounts interface {
	FIExists(ctx context.Context, fiID string) (bool, error)
	PartyBelongsToFI(ctx context.Context, fiID, partyID string) (bool, error)
	AccountOnParty(ctx context.Context, fiID, partyID, accountID string) (bool, error)
}

// Register inserts the accepted items.
type Register interface {
	InsertBatch(ctx context.Context, fiID, partyID string, items []UploadItem, payeeNorm func(string) string) (UploadBatchResult, error)
}

// Service validates ownership and inserts a register batch.
type Service struct {
	accounts Accounts
	register Register
}

func NewService(accounts Accounts, register Register) *Service {
	return &Service{accounts: accounts, register: register}
}

// ErrFINotFound is returned when fi_id is unknown (HTTP 404).
var ErrFINotFound = store.ErrInstitutionNotFound

// ErrPartyNotFound is returned when party is unknown under FI.
var ErrPartyNotFound = store.ErrPartyNotFound

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

	prepared, origIndex, preErrors, err := s.prepare(ctx, fiID, partyID, items)
	if err != nil {
		return UploadBatchResult{}, err
	}

	dbResult, err := s.register.InsertBatch(ctx, fiID, partyID, prepared, payee.Normalize)
	if err != nil {
		return UploadBatchResult{}, err
	}
	return mergeResult(dbResult, origIndex, preErrors), nil
}

func (s *Service) prepare(ctx context.Context, fiID, partyID string, items []UploadItem) ([]UploadItem, []int, []UploadItemError, error) {
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
			return nil, nil, nil, err
		}
		if !belongs {
			preErrors = append(preErrors, UploadItemError{Index: i, Message: "account does not belong to party"})
			continue
		}
		prepared = append(prepared, it)
		origIndex = append(origIndex, i)
	}
	return prepared, origIndex, preErrors, nil
}

func mergeResult(dbResult UploadBatchResult, origIndex []int, preErrors []UploadItemError) UploadBatchResult {
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
	return out
}
