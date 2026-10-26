package checkpay_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dmmoody/poz-pay/internal/api"
	"github.com/dmmoody/poz-pay/internal/checkpay"
	"github.com/dmmoody/poz-pay/internal/decide"
)

const (
	testFIID      = "11111111-1111-1111-1111-111111111111"
	testAccountID = "22222222-2222-2222-2222-222222222222"
)

var testPaidAt = time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC)

func TestDecide_staysPayUntilPostedAndKeepsHistory(t *testing.T) {
	t.Parallel()

	item := &checkpay.RegisterItem{
		ID:              "reg-1005",
		CheckNumber:     "1005",
		Amount:          12000,
		Payee:           "STAPLES INC",
		PayeeNormalized: "STAPLES INC",
	}
	reg := &memRegister{item: item}
	log := &memDecisions{}
	svc := newTestService(reg, log)
	in := checkpay.DecideInput{
		FIID:          testFIID,
		AccountID:     testAccountID,
		CorrelationID: "core-1",
		Check: checkpay.PresentedCheck{
			CheckNumber: "1005",
			Amount:      12000,
			Payee:       "STAPLES INC",
		},
		RawRequest: map[string]any{"check_number": "1005"},
	}

	ctx := context.Background()
	first, err := svc.Decide(ctx, in)
	if err != nil {
		t.Fatalf("first decide: %v", err)
	}
	second, err := svc.Decide(ctx, in)
	if err != nil {
		t.Fatalf("second decide: %v", err)
	}
	if first.Decision != decide.DecisionPay || second.Decision != decide.DecisionPay {
		t.Fatalf("decisions = %s, %s; both presentments before post must pay", first.Decision, second.Decision)
	}
	if reg.posts != 0 {
		t.Fatalf("decide posted %d times", reg.posts)
	}

	posted, err := svc.Post(ctx, checkpay.PostInput{
		FIID:          testFIID,
		AccountID:     testAccountID,
		CheckNumber:   "1005",
		CorrelationID: "core-1",
		PaidAt:        testPaidAt,
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if posted.AlreadyPaid || posted.RegisterItemID != "reg-1005" || posted.CorrelationID != "core-1" || !posted.PaidAt.Equal(testPaidAt) {
		t.Fatalf("post = %+v", posted)
	}

	again, err := svc.Post(ctx, checkpay.PostInput{
		FIID:          testFIID,
		AccountID:     testAccountID,
		CheckNumber:   "1005",
		CorrelationID: "core-2",
		PaidAt:        testPaidAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("repeat post: %v", err)
	}
	if !again.AlreadyPaid || again.CorrelationID != "core-1" || !again.PaidAt.Equal(posted.PaidAt) {
		t.Fatalf("repeat post = %+v, first paid_at %s", again, posted.PaidAt)
	}
	if reg.posts != 1 {
		t.Fatalf("posts = %d", reg.posts)
	}

	third, err := svc.Decide(ctx, in)
	if err != nil {
		t.Fatalf("third decide: %v", err)
	}
	if third.Decision != decide.DecisionReturn || len(third.Exceptions) != 1 || third.Exceptions[0].Code != decide.CodeCheckAlreadyPaid {
		t.Fatalf("after post = %+v", third)
	}

	history, err := svc.ListDecisions(ctx, testFIID, testAccountID, "1005")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if history.PaidAt == nil || history.PaidCorrelationID != "core-1" || history.RegisterItemID != "reg-1005" {
		t.Fatalf("history posting = %+v", history)
	}
	if len(history.Decisions) != 3 {
		t.Fatalf("history len = %d", len(history.Decisions))
	}
	if history.Decisions[0].Decision != decide.DecisionPay ||
		history.Decisions[1].Decision != decide.DecisionPay ||
		history.Decisions[2].Decision != decide.DecisionReturn {
		t.Fatalf("history decisions = %+v", history.Decisions)
	}
	if len(log.entries) != 3 {
		t.Fatalf("audit entries = %d", len(log.entries))
	}
	for i, entry := range log.entries {
		if entry.CheckNumber != "1005" || entry.RegisterItemID != "reg-1005" || entry.CorrelationID != "core-1" {
			t.Fatalf("entry %d = %+v", i, entry)
		}
	}
}

func TestPost_rejectsVoidedAndMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(&memRegister{}, &memDecisions{})
		_, err := svc.Post(ctx, checkpay.PostInput{FIID: testFIID, AccountID: testAccountID, CheckNumber: "9999", PaidAt: testPaidAt})
		if err != checkpay.ErrCheckNotInRegister {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("voided", func(t *testing.T) {
		t.Parallel()
		voidedAt := time.Now().UTC()
		svc := newTestService(&memRegister{item: &checkpay.RegisterItem{
			ID:       "reg-void",
			VoidedAt: &voidedAt,
		}}, &memDecisions{})
		_, err := svc.Post(ctx, checkpay.PostInput{FIID: testFIID, AccountID: testAccountID, CheckNumber: "1003", PaidAt: testPaidAt})
		if err != checkpay.ErrCheckVoided {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("unknown account", func(t *testing.T) {
		t.Parallel()
		reg := &memRegister{item: &checkpay.RegisterItem{ID: "reg-1"}}
		svc := newTestService(reg, &memDecisions{})
		_, err := svc.Post(ctx, checkpay.PostInput{FIID: testFIID, AccountID: "missing", CheckNumber: "1005", PaidAt: testPaidAt})
		if err != checkpay.ErrAccountNotFound {
			t.Fatalf("err = %v", err)
		}
		if reg.posts != 0 {
			t.Fatalf("posts = %d", reg.posts)
		}
	})
}

func TestHTTP_presentmentsStayInHistoryUntilPost(t *testing.T) {
	t.Parallel()

	reg := &memRegister{item: &checkpay.RegisterItem{
		ID:              "reg-1005",
		CheckNumber:     "1005",
		Amount:          12000,
		Payee:           "STAPLES INC",
		PayeeNormalized: "STAPLES INC",
	}}
	srv := httptest.NewServer(api.NewRouter(&api.Handlers{Checks: newTestService(reg, &memDecisions{})}))
	t.Cleanup(srv.Close)

	decideBody := []byte(`{
		"rail":"check",
		"fi_id":"` + testFIID + `",
		"account_id":"` + testAccountID + `",
		"correlation_id":"core-1",
		"check":{"check_number":"1005","amount":12000,"payee":"STAPLES INC"}
	}`)
	for range 2 {
		res := postJSON(t, srv.URL+"/v1/decide", decideBody)
		if res.Status != http.StatusOK || res.JSON["decision"] != "pay" {
			t.Fatalf("decide = %d %+v", res.Status, res.JSON)
		}
	}

	history := getJSON(t, srv.URL+"/v1/fis/"+testFIID+"/accounts/"+testAccountID+"/checks/1005/decisions")
	if history.Status != http.StatusOK || history.JSON["paid_at"] != nil {
		t.Fatalf("history before post = %d %+v", history.Status, history.JSON)
	}
	decisions, _ := history.JSON["decisions"].([]any)
	if len(decisions) != 2 {
		t.Fatalf("decisions = %d", len(decisions))
	}

	posted := postJSON(t, srv.URL+"/v1/fis/"+testFIID+"/accounts/"+testAccountID+"/check-postings", []byte(`{"check_number":"1005","correlation_id":"core-1","paid_at":"2026-10-26T18:00:00Z"}`))
	if posted.Status != http.StatusCreated || posted.JSON["correlation_id"] != "core-1" || posted.JSON["paid_at"] != "2026-10-26T18:00:00Z" {
		t.Fatalf("post = %d %+v", posted.Status, posted.JSON)
	}
	if _, ok := posted.JSON["already_paid"]; ok {
		t.Fatalf("post body includes already_paid: %+v", posted.JSON)
	}
	replay := postJSON(t, srv.URL+"/v1/fis/"+testFIID+"/accounts/"+testAccountID+"/check-postings", []byte(`{"check_number":"1005","correlation_id":"core-2","paid_at":"2026-10-26T19:00:00Z"}`))
	if replay.Status != http.StatusOK || replay.JSON["correlation_id"] != "core-1" || replay.JSON["paid_at"] != posted.JSON["paid_at"] {
		t.Fatalf("replay post = %d %+v", replay.Status, replay.JSON)
	}

	after := postJSON(t, srv.URL+"/v1/decide", decideBody)
	if after.Status != http.StatusOK || after.JSON["decision"] != "return" {
		t.Fatalf("decide after post = %d %+v", after.Status, after.JSON)
	}
}

type httpResult struct {
	Status int
	JSON   map[string]any
}

func postJSON(t *testing.T, url string, body []byte) httpResult {
	t.Helper()
	res, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	return readResult(t, res)
}

func getJSON(t *testing.T, url string) httpResult {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	return readResult(t, res)
}

func readResult(t *testing.T, res *http.Response) httpResult {
	t.Helper()
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json %s: %v", raw, err)
	}
	return httpResult{Status: res.StatusCode, JSON: payload}
}

type memAccounts struct {
	acct *checkpay.Account
}

func (m memAccounts) FIExists(context.Context, string) (bool, error) { return true, nil }

func (m memAccounts) PartyBelongsToFI(context.Context, string, string) (bool, error) {
	return true, nil
}

func (m memAccounts) GetAccount(_ context.Context, _, accountID string) (*checkpay.Account, error) {
	if m.acct == nil || m.acct.ID != accountID {
		return nil, nil
	}
	return m.acct, nil
}

func (m memAccounts) AccountOnParty(context.Context, string, string, string) (bool, error) {
	return true, nil
}

type memPolicy struct{}

func (memPolicy) ResolvePolicy(context.Context, string, string) (checkpay.Policy, error) {
	return checkpay.Policy{}, nil
}

type memRegister struct {
	item  *checkpay.RegisterItem
	posts int
}

func (m *memRegister) FindItem(context.Context, string, string, string) (*checkpay.RegisterItem, error) {
	return m.item, nil
}

func (m *memRegister) InsertBatch(context.Context, string, string, []checkpay.UploadItem, func(string) string) (checkpay.UploadBatchResult, error) {
	return checkpay.UploadBatchResult{}, nil
}

func (m *memRegister) RecordPayment(_ context.Context, _, _, _, correlationID string, paidAt time.Time) (checkpay.Payment, error) {
	if m.item == nil {
		return checkpay.Payment{}, checkpay.ErrCheckNotInRegister
	}
	if m.item.VoidedAt != nil {
		return checkpay.Payment{}, checkpay.ErrCheckVoided
	}
	if m.item.PaidAt != nil {
		return checkpay.Payment{
			RegisterItemID: m.item.ID,
			PaidAt:         *m.item.PaidAt,
			CorrelationID:  m.item.PaidCorrelationID,
			AlreadyPaid:    true,
		}, nil
	}
	stored := paidAt.UTC()
	m.item.PaidAt = &stored
	m.item.PaidCorrelationID = correlationID
	m.posts++
	return checkpay.Payment{
		RegisterItemID: m.item.ID,
		PaidAt:         stored,
		CorrelationID:  correlationID,
	}, nil
}

type memDecisions struct {
	entries []checkpay.CheckAuditEntry
	rows    []checkpay.RecordedDecision
}

func (m *memDecisions) WriteCheckDecision(_ context.Context, entry checkpay.CheckAuditEntry) (string, error) {
	m.entries = append(m.entries, entry)
	rec := checkpay.RecordedDecision{
		ID:             entry.CheckNumber,
		Decision:       entry.Result.Decision,
		Match:          entry.Result.Match,
		Exceptions:     entry.Result.Exceptions,
		CorrelationID:  entry.CorrelationID,
		RegisterItemID: entry.RegisterItemID,
		DecidedAt:      time.Now().UTC(),
		LatencyMs:      entry.Result.LatencyMs,
	}
	m.rows = append(m.rows, rec)
	return rec.ID, nil
}

func (m *memDecisions) ListCheckDecisions(context.Context, string, string, string) ([]checkpay.RecordedDecision, error) {
	out := make([]checkpay.RecordedDecision, len(m.rows))
	copy(out, m.rows)
	return out, nil
}

func newTestService(reg *memRegister, log *memDecisions) *checkpay.Service {
	return checkpay.NewService(
		memAccounts{acct: &checkpay.Account{
			ID:                 testAccountID,
			FIID:               testFIID,
			PartyID:            "party-1",
			Active:             true,
			PositivePayEnabled: true,
		}},
		reg,
		reg,
		memPolicy{},
		log,
	)
}
