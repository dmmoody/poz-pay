// Package checkposting records that the paying system posted a check.
// A pay decision does not post. A return decision means the core should not post.
package checkposting

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/dmmoody/poz-pay/internal/store"
)

// Accounts resolves the institution and the account a posting is applied to.
type Accounts interface {
	FIExists(ctx context.Context, fiID string) (bool, error)
	GetAccount(ctx context.Context, fiID, accountID string) (*Account, error)
}

// Register records the paid fact on the issued item.
type Register interface {
	RecordPayment(ctx context.Context, rec PaymentRecord) (Payment, error)
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

// Service records settlement for a register item.
type Service struct {
	accounts Accounts
	register Register
}

func NewService(accounts Accounts, register Register) *Service {
	return &Service{accounts: accounts, register: register}
}

// ErrFINotFound is returned when fi_id is unknown (HTTP 404).
var ErrFINotFound = store.ErrInstitutionNotFound

// ErrAccountNotFound is returned when the account is unknown under the FI.
var ErrAccountNotFound = errors.New("account not found")

// ErrCheckNotInRegister is returned when a posting names no register row.
var ErrCheckNotInRegister = store.ErrNotInRegister

// ErrCheckVoided is returned when a posting targets a voided register row.
var ErrCheckVoided = store.ErrVoided

// ErrCheckNumberRequired is returned when a check number is blank.
var ErrCheckNumberRequired = errors.New("check number is required")

// ErrPaidAtRequired is returned when a posting omits the paying system's timestamp.
var ErrPaidAtRequired = errors.New("paid_at is required")

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
