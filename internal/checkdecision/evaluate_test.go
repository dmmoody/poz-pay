package checkdecision_test

import (
	"testing"
	"time"

	"github.com/dmmoody/poz-pay/internal/checkdecision"
	"github.com/dmmoody/poz-pay/internal/decision"
)

func activeAcct() *checkdecision.Account {
	return &checkdecision.Account{
		ID:                 "acct-1",
		FIID:               "fi-1",
		PartyID:            "party-1",
		Active:             true,
		PositivePayEnabled: true,
	}
}

func issued(mod func(*checkdecision.RegisterItem)) *checkdecision.RegisterItem {
	item := &checkdecision.RegisterItem{
		ID:              "reg-1001",
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
	presented := checkdecision.PresentedCheck{CheckNumber: "1001", Amount: 10000, Payee: "ACME SUPPLIES LLC"}
	pol := checkdecision.Policy{}
	asOf := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

	t.Run("nil account", func(t *testing.T) {
		t.Parallel()
		r := checkdecision.EvaluateCheck(nil, nil, presented, pol, asOf)
		assertGate(t, r, decision.CodeAccountNotOurs)
	})

	t.Run("inactive", func(t *testing.T) {
		t.Parallel()
		acct := activeAcct()
		acct.Active = false
		r := checkdecision.EvaluateCheck(acct, issued(nil), presented, pol, asOf)
		assertGate(t, r, decision.CodeAccountInactive)
	})

	t.Run("not in register", func(t *testing.T) {
		t.Parallel()
		r := checkdecision.EvaluateCheck(activeAcct(), nil, presented, pol, asOf)
		assertGate(t, r, decision.CodeCheckNotInRegister)
	})

	t.Run("voided", func(t *testing.T) {
		t.Parallel()
		now := time.Now().UTC()
		r := checkdecision.EvaluateCheck(activeAcct(), issued(func(i *checkdecision.RegisterItem) {
			i.VoidedAt = &now
		}), presented, pol, asOf)
		assertGate(t, r, decision.CodeCheckVoided)
	})

	t.Run("already paid", func(t *testing.T) {
		t.Parallel()
		now := time.Now().UTC()
		r := checkdecision.EvaluateCheck(activeAcct(), issued(func(i *checkdecision.RegisterItem) {
			i.PaidAt = &now
		}), presented, pol, asOf)
		assertGate(t, r, decision.CodeCheckAlreadyPaid)
	})
}

func TestEvaluateCheck_Expired(t *testing.T) {
	t.Parallel()
	issue := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	item := issued(func(i *checkdecision.RegisterItem) {
		i.IssueDate = &issue
	})
	presented := checkdecision.PresentedCheck{CheckNumber: "1001", Amount: 10000, Payee: "ACME SUPPLIES LLC"}

	t.Run("default 180 past", func(t *testing.T) {
		t.Parallel()
		asOf := issue.AddDate(0, 0, 181)
		r := checkdecision.EvaluateCheck(activeAcct(), item, presented, checkdecision.Policy{}, asOf)
		assertGate(t, r, decision.CodeCheckExpired)
		if r.Exceptions[0].Facts["age_days"] != 181 || r.Exceptions[0].Facts["max_age_days"] != 180 {
			t.Fatalf("facts=%+v", r.Exceptions[0].Facts)
		}
	})

	t.Run("default 180 on boundary still pay", func(t *testing.T) {
		t.Parallel()
		asOf := issue.AddDate(0, 0, 180)
		r := checkdecision.EvaluateCheck(activeAcct(), item, presented, checkdecision.Policy{}, asOf)
		assertRegisterMatch(t, r, "reg-1001")
	})

	t.Run("fi 60 days", func(t *testing.T) {
		t.Parallel()
		asOf := issue.AddDate(0, 0, 61)
		r := checkdecision.EvaluateCheck(activeAcct(), item, presented, checkdecision.Policy{MaxAgeDays: 60}, asOf)
		assertGate(t, r, decision.CodeCheckExpired)
	})

	t.Run("no issue_date skips expiry", func(t *testing.T) {
		t.Parallel()
		asOf := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
		r := checkdecision.EvaluateCheck(activeAcct(), issued(nil), presented, checkdecision.Policy{MaxAgeDays: 1}, asOf)
		assertRegisterMatch(t, r, "reg-1001")
	})
}

func TestEvaluateCheck_MismatchAccumulate(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	r := checkdecision.EvaluateCheck(activeAcct(), issued(nil), checkdecision.PresentedCheck{
		CheckNumber: "1001",
		Amount:      10050,
		Payee:       "ACME SUPPLIES",
	}, checkdecision.Policy{}, asOf)
	if r.Decision != decision.DecisionReview {
		t.Fatalf("decision=%s want review", r.Decision)
	}
	if len(r.Exceptions) != 2 {
		t.Fatalf("exceptions=%d want 2: %+v", len(r.Exceptions), r.Exceptions)
	}
	codes := map[string]bool{}
	for _, e := range r.Exceptions {
		codes[e.Code] = true
	}
	if !codes[decision.CodeAmountMismatch] || !codes[decision.CodePayeeMismatch] {
		t.Fatalf("missing mismatch codes: %+v", r.Exceptions)
	}
	assertNoMatch(t, r)
}

func TestEvaluateCheck_ReturnOnMismatch(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	r := checkdecision.EvaluateCheck(activeAcct(), issued(nil), checkdecision.PresentedCheck{
		CheckNumber: "1001",
		Amount:      10050,
		Payee:       "ACME SUPPLIES LLC",
	}, checkdecision.Policy{ReturnOnMismatch: true}, asOf)
	if r.Decision != decision.DecisionReturn {
		t.Fatalf("decision=%s want return", r.Decision)
	}
	if len(r.Exceptions) != 1 || r.Exceptions[0].Code != decision.CodeAmountMismatch {
		t.Fatalf("exceptions=%+v", r.Exceptions)
	}
	f := r.Exceptions[0].Facts
	if f["presented_amount"] != int64(10050) || f["issued_amount"] != int64(10000) {
		t.Fatalf("facts=%+v", f)
	}
	assertNoMatch(t, r)
}

func TestEvaluateCheck_Pay(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	r := checkdecision.EvaluateCheck(activeAcct(), issued(nil), checkdecision.PresentedCheck{
		CheckNumber: "1001",
		Amount:      10000,
		Payee:       "  acme   supplies  llc ",
	}, checkdecision.Policy{}, asOf)
	assertRegisterMatch(t, r, "reg-1001")
}

func assertGate(t *testing.T, r decision.Result, code string) {
	t.Helper()
	if r.Decision != decision.DecisionReturn {
		t.Fatalf("decision=%s want return", r.Decision)
	}
	if len(r.Exceptions) != 1 || r.Exceptions[0].Code != code {
		t.Fatalf("exceptions=%+v want %s", r.Exceptions, code)
	}
	assertNoMatch(t, r)
}

func assertRegisterMatch(t *testing.T, r decision.Result, registerItemID string) {
	t.Helper()
	if r.Decision != decision.DecisionPay {
		t.Fatalf("decision=%s excs=%+v want pay", r.Decision, r.Exceptions)
	}
	if len(r.Exceptions) != 0 {
		t.Fatalf("exceptions should be empty: %+v", r.Exceptions)
	}
	if r.Match == nil || r.Match.Code != decision.CodeRegisterMatch {
		t.Fatalf("match=%+v want %s", r.Match, decision.CodeRegisterMatch)
	}
	if r.Match.Facts["register_item_id"] != registerItemID {
		t.Fatalf("facts=%+v", r.Match.Facts)
	}
}

func assertNoMatch(t *testing.T, r decision.Result) {
	t.Helper()
	if r.Match != nil {
		t.Fatalf("match=%+v want none", r.Match)
	}
}
