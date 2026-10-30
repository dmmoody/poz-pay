package checkposting_test

import (
	"context"
	"testing"
	"time"

	"github.com/dmmoody/poz-pay/internal/checkposting"
)

const (
	testFIID      = "11111111-1111-1111-1111-111111111111"
	testAccountID = "22222222-2222-2222-2222-222222222222"
)

var testPaidAt = time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC)

func TestPost_rejectsVoidedAndMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		svc := newPostingService(&memRegister{})
		_, err := svc.Post(ctx, checkposting.PostInput{FIID: testFIID, AccountID: testAccountID, CheckNumber: "9999", PaidAt: testPaidAt})
		if err != checkposting.ErrCheckNotInRegister {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("voided", func(t *testing.T) {
		t.Parallel()
		voidedAt := time.Now().UTC()
		svc := newPostingService(&memRegister{id: "reg-void", voided: &voidedAt})
		_, err := svc.Post(ctx, checkposting.PostInput{FIID: testFIID, AccountID: testAccountID, CheckNumber: "1003", PaidAt: testPaidAt})
		if err != checkposting.ErrCheckVoided {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("unknown account", func(t *testing.T) {
		t.Parallel()
		reg := &memRegister{id: "reg-1"}
		svc := newPostingService(reg)
		_, err := svc.Post(ctx, checkposting.PostInput{FIID: testFIID, AccountID: "missing", CheckNumber: "1005", PaidAt: testPaidAt})
		if err != checkposting.ErrAccountNotFound {
			t.Fatalf("err = %v", err)
		}
		if reg.posts != 0 {
			t.Fatalf("posts = %d", reg.posts)
		}
	})
}

type memAccounts struct {
	id string
}

func (m memAccounts) FIExists(context.Context, string) (bool, error) { return true, nil }

func (m memAccounts) GetAccount(_ context.Context, _, accountID string) (*checkposting.Account, error) {
	if m.id == "" || m.id != accountID {
		return nil, nil
	}
	return &checkposting.Account{ID: m.id, FIID: testFIID, PartyID: "party-1", Active: true}, nil
}

type memRegister struct {
	id     string
	voided *time.Time
	posts  int
}

func (m *memRegister) RecordPayment(context.Context, checkposting.PaymentRecord) (checkposting.Payment, error) {
	if m.id == "" {
		return checkposting.Payment{}, checkposting.ErrCheckNotInRegister
	}
	if m.voided != nil {
		return checkposting.Payment{}, checkposting.ErrCheckVoided
	}
	m.posts++
	return checkposting.Payment{RegisterItemID: m.id}, nil
}

func newPostingService(reg *memRegister) *checkposting.Service {
	return checkposting.NewService(memAccounts{id: testAccountID}, reg)
}
