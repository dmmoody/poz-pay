package checkdecision_test

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
	decisionhttp "github.com/dmmoody/poz-pay/internal/api/checkdecision"
	postinghttp "github.com/dmmoody/poz-pay/internal/api/checkposting"
	"github.com/dmmoody/poz-pay/internal/checkdecision"
	"github.com/dmmoody/poz-pay/internal/checkposting"
	"github.com/dmmoody/poz-pay/internal/decision"
)

const (
	testFIID      = "11111111-1111-1111-1111-111111111111"
	testAccountID = "22222222-2222-2222-2222-222222222222"
)

var testPaidAt = time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC)

func TestDecide_staysPayUntilPostedAndKeepsHistory(t *testing.T) {
	t.Parallel()

	item := &checkdecision.RegisterItem{
		ID:              "reg-1005",
		CheckNumber:     "1005",
		Amount:          12000,
		Payee:           "STAPLES INC",
		PayeeNormalized: "STAPLES INC",
	}
	reg := &memRegister{item: item}
	log := &memDecisions{}
	decisions, postings := newTestServices(reg, log)
	in := checkdecision.DecideInput{
		FIID:          testFIID,
		AccountID:     testAccountID,
		CorrelationID: "core-1",
		Check: checkdecision.PresentedCheck{
			CheckNumber: "1005",
			Amount:      12000,
			Payee:       "STAPLES INC",
		},
		RawRequest: map[string]any{"check_number": "1005"},
	}

	ctx := context.Background()
	first, err := decisions.Decide(ctx, in)
	if err != nil {
		t.Fatalf("first decide: %v", err)
	}
	second, err := decisions.Decide(ctx, in)
	if err != nil {
		t.Fatalf("second decide: %v", err)
	}
	if first.Decision != decision.DecisionPay || second.Decision != decision.DecisionPay {
		t.Fatalf("decisions = %s, %s; both presentments before post must pay", first.Decision, second.Decision)
	}
	if reg.posts != 0 {
		t.Fatalf("decide posted %d times", reg.posts)
	}

	posted, err := postings.Post(ctx, checkposting.PostInput{
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

	again, err := postings.Post(ctx, checkposting.PostInput{
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

	third, err := decisions.Decide(ctx, in)
	if err != nil {
		t.Fatalf("third decide: %v", err)
	}
	if third.Decision != decision.DecisionReturn || len(third.Exceptions) != 1 || third.Exceptions[0].Code != decision.CodeCheckAlreadyPaid {
		t.Fatalf("after post = %+v", third)
	}

	history, err := decisions.ListDecisions(ctx, testFIID, testAccountID, "1005")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if history.PaidAt == nil || history.PaidCorrelationID != "core-1" || history.RegisterItemID != "reg-1005" {
		t.Fatalf("history posting = %+v", history)
	}
	if len(history.Decisions) != 3 {
		t.Fatalf("history len = %d", len(history.Decisions))
	}
	if history.Decisions[0].Decision != decision.DecisionPay ||
		history.Decisions[1].Decision != decision.DecisionPay ||
		history.Decisions[2].Decision != decision.DecisionReturn {
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

	audit, err := decisions.ListAudit(ctx, testFIID, testAccountID, "1005")
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	got := make([]string, len(audit.Events))
	for i, event := range audit.Events {
		got[i] = event.Kind
	}
	want := []string{
		checkdecision.AuditKindPresentment,
		checkdecision.AuditKindPresentment,
		checkdecision.AuditKindPosted,
		checkdecision.AuditKindPresentment,
	}
	if len(got) != len(want) {
		t.Fatalf("audit kinds = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("audit kinds = %v", got)
		}
	}
	postedEvent := audit.Events[2]
	if postedEvent.Posted == nil || !postedEvent.Posted.PaidAt.Equal(testPaidAt) || postedEvent.CorrelationID != "core-1" {
		t.Fatalf("posted event = %+v", postedEvent)
	}
}

func TestHTTP_presentmentsStayInHistoryUntilPost(t *testing.T) {
	t.Parallel()

	reg := &memRegister{item: &checkdecision.RegisterItem{
		ID:              "reg-1005",
		CheckNumber:     "1005",
		Amount:          12000,
		Payee:           "STAPLES INC",
		PayeeNormalized: "STAPLES INC",
	}}
	log := &memDecisions{}
	decisions, postings := newTestServices(reg, log)
	srv := httptest.NewServer(api.NewRouter(api.Routes{
		Decisions: decisionhttp.Handler{Decisions: decisions},
		Postings:  postinghttp.Handler{Postings: postings},
	}))
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
	decided, _ := history.JSON["decisions"].([]any)
	if len(decided) != 2 {
		t.Fatalf("decisions = %d", len(decided))
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

	audit := getJSON(t, srv.URL+"/v1/fis/"+testFIID+"/accounts/"+testAccountID+"/checks/1005/audit")
	events, _ := audit.JSON["events"].([]any)
	if audit.Status != http.StatusOK || len(events) != 4 {
		t.Fatalf("audit = %d %+v", audit.Status, audit.JSON)
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
	acct *checkdecision.Account
}

func (m memAccounts) FIExists(context.Context, string) (bool, error) { return true, nil }

func (m memAccounts) GetAccount(_ context.Context, _, accountID string) (*checkdecision.Account, error) {
	if m.acct == nil || m.acct.ID != accountID {
		return nil, nil
	}
	return m.acct, nil
}

type memPolicy struct{}

func (memPolicy) ResolvePolicy(context.Context, string, string) (checkdecision.Policy, error) {
	return checkdecision.Policy{}, nil
}

type memRegister struct {
	item  *checkdecision.RegisterItem
	posts int
	audit *memDecisions
}

func (m *memRegister) FindItem(context.Context, string, string, string) (*checkdecision.RegisterItem, error) {
	return m.item, nil
}

func (m *memRegister) RecordPayment(_ context.Context, rec checkposting.PaymentRecord) (checkposting.Payment, error) {
	if m.item == nil {
		return checkposting.Payment{}, checkposting.ErrCheckNotInRegister
	}
	if m.item.VoidedAt != nil {
		return checkposting.Payment{}, checkposting.ErrCheckVoided
	}
	if m.item.PaidAt != nil {
		return checkposting.Payment{
			RegisterItemID: m.item.ID,
			PaidAt:         *m.item.PaidAt,
			CorrelationID:  m.item.PaidCorrelationID,
			AlreadyPaid:    true,
		}, nil
	}
	stored := rec.PaidAt.UTC()
	m.item.PaidAt = &stored
	m.item.PaidCorrelationID = rec.CorrelationID
	m.posts++
	if m.audit != nil {
		m.audit.events = append(m.audit.events, checkdecision.CheckAuditEvent{
			ID:             "posted",
			Kind:           checkdecision.AuditKindPosted,
			RecordedAt:     time.Now().UTC(),
			CorrelationID:  rec.CorrelationID,
			RegisterItemID: m.item.ID,
			Posted:         &checkdecision.PostedAudit{PaidAt: stored},
		})
	}
	return checkposting.Payment{
		RegisterItemID: m.item.ID,
		PaidAt:         stored,
		CorrelationID:  rec.CorrelationID,
	}, nil
}

type memDecisions struct {
	entries []checkdecision.CheckAuditEntry
	rows    []checkdecision.RecordedDecision
	events  []checkdecision.CheckAuditEvent
}

func (m *memDecisions) WriteCheckDecision(_ context.Context, entry checkdecision.CheckAuditEntry) (string, error) {
	m.entries = append(m.entries, entry)
	rec := checkdecision.RecordedDecision{
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
	m.events = append(m.events, checkdecision.CheckAuditEvent{
		ID:             rec.ID,
		Kind:           checkdecision.AuditKindPresentment,
		RecordedAt:     rec.DecidedAt,
		CorrelationID:  rec.CorrelationID,
		RegisterItemID: rec.RegisterItemID,
		Presentment: &checkdecision.PresentmentAudit{
			Decision:   rec.Decision,
			Match:      rec.Match,
			Exceptions: rec.Exceptions,
			LatencyMs:  rec.LatencyMs,
		},
	})
	return rec.ID, nil
}

func (m *memDecisions) ListCheckAudit(context.Context, string, string, string) ([]checkdecision.CheckAuditEvent, error) {
	out := make([]checkdecision.CheckAuditEvent, len(m.events))
	copy(out, m.events)
	return out, nil
}

func (m *memDecisions) ListCheckDecisions(context.Context, string, string, string) ([]checkdecision.RecordedDecision, error) {
	out := make([]checkdecision.RecordedDecision, len(m.rows))
	copy(out, m.rows)
	return out, nil
}

func newTestServices(reg *memRegister, log *memDecisions) (*checkdecision.Service, *checkposting.Service) {
	reg.audit = log
	accounts := memAccounts{acct: &checkdecision.Account{
		ID:                 testAccountID,
		FIID:               testFIID,
		PartyID:            "party-1",
		Active:             true,
		PositivePayEnabled: true,
	}}
	return checkdecision.NewService(accounts, reg, memPolicy{}, log), checkposting.NewService(accounts, reg)
}
