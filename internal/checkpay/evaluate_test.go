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

func TestEvaluateCheck_GateFails(t *testing.T) {
	t.Parallel()
	presented := checkpay.PresentedCheck{CheckNumber: "1001", Amount: 10000, Payee: "ACME SUPPLIES LLC"}
	pol := checkpay.Policy{}
	asOf := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

	t.Run("nil account", func(t *testing.T) {
		t.Parallel()
		r := checkpay.EvaluateCheck(nil, nil, presented, pol, asOf)
		assertGate(t, r, decide.CodeAccountNotOurs)
	})

	t.Run("inactive", func(t *testing.T) {
		t.Parallel()
		acct := activeAcct()
		acct.Active = false
		r := checkpay.EvaluateCheck(acct, issued(nil), presented, pol, asOf)
		assertGate(t, r, decide.CodeAccountInactive)
	})

	t.Run("not in register", func(t *testing.T) {
		t.Parallel()
		r := checkpay.EvaluateCheck(activeAcct(), nil, presented, pol, asOf)
		assertGate(t, r, decide.CodeCheckNotInRegister)
	})

	t.Run("voided", func(t *testing.T) {
		t.Parallel()
		now := time.Now().UTC()
		r := checkpay.EvaluateCheck(activeAcct(), issued(func(i *checkpay.RegisterItem) {
			i.VoidedAt = &now
		}), presented, pol, asOf)
		assertGate(t, r, decide.CodeCheckVoided)
	})

	t.Run("already paid", func(t *testing.T) {
		t.Parallel()
		now := time.Now().UTC()
		r := checkpay.EvaluateCheck(activeAcct(), issued(func(i *checkpay.RegisterItem) {
			i.PaidAt = &now
		}), presented, pol, asOf)
		assertGate(t, r, decide.CodeCheckAlreadyPaid)
	})
}

func TestEvaluateCheck_Expired(t *testing.T) {
	t.Parallel()
	issue := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	item := issued(func(i *checkpay.RegisterItem) {
		i.IssueDate = &issue
	})
	presented := checkpay.PresentedCheck{CheckNumber: "1001", Amount: 10000, Payee: "ACME SUPPLIES LLC"}

	t.Run("default 180 past", func(t *testing.T) {
		t.Parallel()
		asOf := issue.AddDate(0, 0, 181)
		r := checkpay.EvaluateCheck(activeAcct(), item, presented, checkpay.Policy{}, asOf)
		assertGate(t, r, decide.CodeCheckExpired)
		if r.Exceptions[0].Facts["age_days"] != 181 || r.Exceptions[0].Facts["max_age_days"] != 180 {
			t.Fatalf("facts=%+v", r.Exceptions[0].Facts)
		}
	})

	t.Run("default 180 on boundary still pay", func(t *testing.T) {
		t.Parallel()
		asOf := issue.AddDate(0, 0, 180)
		r := checkpay.EvaluateCheck(activeAcct(), item, presented, checkpay.Policy{}, asOf)
		if r.Decision != decide.DecisionPay {
			t.Fatalf("decision=%s excs=%+v want pay", r.Decision, r.Exceptions)
		}
	})

	t.Run("fi 60 days", func(t *testing.T) {
		t.Parallel()
		asOf := issue.AddDate(0, 0, 61)
		r := checkpay.EvaluateCheck(activeAcct(), item, presented, checkpay.Policy{MaxAgeDays: 60}, asOf)
		assertGate(t, r, decide.CodeCheckExpired)
	})

	t.Run("no issue_date skips expiry", func(t *testing.T) {
		t.Parallel()
		asOf := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
		r := checkpay.EvaluateCheck(activeAcct(), issued(nil), presented, checkpay.Policy{MaxAgeDays: 1}, asOf)
		if r.Decision != decide.DecisionPay {
			t.Fatalf("decision=%s excs=%+v want pay", r.Decision, r.Exceptions)
		}
	})
}

func TestEvaluateCheck_MismatchAccumulate(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	r := checkpay.EvaluateCheck(activeAcct(), issued(nil), checkpay.PresentedCheck{
		CheckNumber: "1001",
		Amount:      10050,
		Payee:       "ACME SUPPLIES",
	}, checkpay.Policy{}, asOf)
	if r.Decision != decide.DecisionReview {
		t.Fatalf("decision=%s want review", r.Decision)
	}
	if len(r.Exceptions) != 2 {
		t.Fatalf("exceptions=%d want 2: %+v", len(r.Exceptions), r.Exceptions)
	}
	codes := map[string]bool{}
	for _, e := range r.Exceptions {
		codes[e.Code] = true
	}
	if !codes[decide.CodeAmountMismatch] || !codes[decide.CodePayeeMismatch] {
		t.Fatalf("missing mismatch codes: %+v", r.Exceptions)
	}
}

func TestEvaluateCheck_ReturnOnMismatch(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	r := checkpay.EvaluateCheck(activeAcct(), issued(nil), checkpay.PresentedCheck{
		CheckNumber: "1001",
		Amount:      10050,
		Payee:       "ACME SUPPLIES LLC",
	}, checkpay.Policy{ReturnOnMismatch: true}, asOf)
	if r.Decision != decide.DecisionReturn {
		t.Fatalf("decision=%s want return", r.Decision)
	}
	if len(r.Exceptions) != 1 || r.Exceptions[0].Code != decide.CodeAmountMismatch {
		t.Fatalf("exceptions=%+v", r.Exceptions)
	}
	f := r.Exceptions[0].Facts
	if f["presented_amount"] != int64(10050) || f["issued_amount"] != int64(10000) {
		t.Fatalf("facts=%+v", f)
	}
}

func TestEvaluateCheck_Pay(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	r := checkpay.EvaluateCheck(activeAcct(), issued(nil), checkpay.PresentedCheck{
		CheckNumber: "1001",
		Amount:      10000,
		Payee:       "  acme   supplies  llc ",
	}, checkpay.Policy{}, asOf)
	if r.Decision != decide.DecisionPay {
		t.Fatalf("got decision=%s excs=%+v", r.Decision, r.Exceptions)
	}
	if len(r.Exceptions) != 0 {
		t.Fatalf("exceptions should be empty: %+v", r.Exceptions)
	}
}

func TestNormalizePayee(t *testing.T) {
	t.Parallel()
	got := checkpay.NormalizePayee("  acme   supplies  llc ")
	if got != "ACME SUPPLIES LLC" {
		t.Fatalf("got %q", got)
	}
}

func assertGate(t *testing.T, r decide.Result, code string) {
	t.Helper()
	if r.Decision != decide.DecisionReturn {
		t.Fatalf("decision=%s want return", r.Decision)
	}
	if len(r.Exceptions) != 1 || r.Exceptions[0].Code != code {
		t.Fatalf("exceptions=%+v want %s", r.Exceptions, code)
	}
}
