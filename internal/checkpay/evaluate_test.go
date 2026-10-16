package checkpay_test

import (
	"testing"
	"time"

	"github.com/dmmoody/poz-pay/internal/checkpay"
	"github.com/dmmoody/poz-pay/internal/decide"
)

func activeAcct() *checkpay.Account {
	return &checkpay.Account{
		ID:                 "acct-1",
		FIID:               "fi-1",
		PartyID:            "party-1",
		Active:             true,
		PositivePayEnabled: true,
	}
}

func issued(mod func(*checkpay.RegisterItem)) *checkpay.RegisterItem {
	item := &checkpay.RegisterItem{
		CheckNumber:     "1001",
		Amount:          10000,
		Payee:           "ACME SUPPLIES LLC",
		PayeeNormalized: "ACME SUPPLIES LLC",
	}
	if mod != nil {
		mod(item)
	}
	return item
}

func TestEvaluateCheck_HardFails(t *testing.T) {
	t.Parallel()
	presented := checkpay.PresentedCheck{CheckNumber: "1001", Amount: 10000, Payee: "ACME SUPPLIES LLC"}
	pol := checkpay.Policy{}

	t.Run("nil account", func(t *testing.T) {
		t.Parallel()
		r := checkpay.EvaluateCheck(nil, nil, presented, pol)
		assertHard(t, r, decide.CodeAccountNotOurs)
	})

	t.Run("inactive", func(t *testing.T) {
		t.Parallel()
		acct := activeAcct()
		acct.Active = false
		r := checkpay.EvaluateCheck(acct, issued(nil), presented, pol)
		assertHard(t, r, decide.CodeAccountInactive)
	})

	t.Run("not in register", func(t *testing.T) {
		t.Parallel()
		r := checkpay.EvaluateCheck(activeAcct(), nil, presented, pol)
		assertHard(t, r, decide.CodeCheckNotInRegister)
	})

	t.Run("voided", func(t *testing.T) {
		t.Parallel()
		now := time.Now().UTC()
		r := checkpay.EvaluateCheck(activeAcct(), issued(func(i *checkpay.RegisterItem) {
			i.VoidedAt = &now
		}), presented, pol)
		assertHard(t, r, decide.CodeCheckVoided)
	})

	t.Run("already paid", func(t *testing.T) {
		t.Parallel()
		now := time.Now().UTC()
		r := checkpay.EvaluateCheck(activeAcct(), issued(func(i *checkpay.RegisterItem) {
			i.PaidAt = &now
		}), presented, pol)
		assertHard(t, r, decide.CodeCheckAlreadyPaid)
	})
}

func TestEvaluateCheck_SoftAccumulate(t *testing.T) {
	t.Parallel()
	r := checkpay.EvaluateCheck(activeAcct(), issued(nil), checkpay.PresentedCheck{
		CheckNumber: "1001",
		Amount:      10050,
		Payee:       "ACME SUPPLIES",
	}, checkpay.Policy{})
	if r.Decision != decide.DecisionReview {
		t.Fatalf("decision=%s want review", r.Decision)
	}
	if r.ReasonCode != decide.CodeAmountMismatch {
		t.Fatalf("reason_code=%s want %s", r.ReasonCode, decide.CodeAmountMismatch)
	}
	if len(r.Exceptions) != 2 {
		t.Fatalf("exceptions=%d want 2: %+v", len(r.Exceptions), r.Exceptions)
	}
}

func TestEvaluateCheck_ReturnOnMismatch(t *testing.T) {
	t.Parallel()
	r := checkpay.EvaluateCheck(activeAcct(), issued(nil), checkpay.PresentedCheck{
		CheckNumber: "1001",
		Amount:      10050,
		Payee:       "ACME SUPPLIES LLC",
	}, checkpay.Policy{ReturnOnMismatch: true})
	if r.Decision != decide.DecisionReturn {
		t.Fatalf("decision=%s want return", r.Decision)
	}
	if r.ReasonCode != decide.CodeAmountMismatch {
		t.Fatalf("reason_code=%s", r.ReasonCode)
	}
}

func TestEvaluateCheck_Pay(t *testing.T) {
	t.Parallel()
	r := checkpay.EvaluateCheck(activeAcct(), issued(nil), checkpay.PresentedCheck{
		CheckNumber: "1001",
		Amount:      10000,
		Payee:       "  acme   supplies  llc ",
	}, checkpay.Policy{})
	if r.Decision != decide.DecisionPay || r.ReasonCode != decide.ReasonOK {
		t.Fatalf("got decision=%s reason=%s excs=%+v", r.Decision, r.ReasonCode, r.Exceptions)
	}
}

func TestNormalizePayee(t *testing.T) {
	t.Parallel()
	got := checkpay.NormalizePayee("  acme   supplies  llc ")
	if got != "ACME SUPPLIES LLC" {
		t.Fatalf("got %q", got)
	}
}

func assertHard(t *testing.T, r decide.Result, code string) {
	t.Helper()
	if r.Decision != decide.DecisionReturn {
		t.Fatalf("decision=%s want return", r.Decision)
	}
	if r.ReasonCode != code {
		t.Fatalf("reason_code=%s want %s", r.ReasonCode, code)
	}
	if len(r.Exceptions) != 1 || r.Exceptions[0].Severity != decide.SeverityHard {
		t.Fatalf("exceptions=%+v", r.Exceptions)
	}
}
