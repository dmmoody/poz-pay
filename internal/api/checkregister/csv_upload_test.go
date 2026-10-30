package checkregister_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dmmoody/poz-pay/internal/api"
	"github.com/dmmoody/poz-pay/internal/api/checkregister"
	"github.com/dmmoody/poz-pay/internal/registerupload"
	"github.com/dmmoody/poz-pay/internal/store"
)

const (
	csvFIID    = "11111111-1111-1111-1111-111111111111"
	csvPartyID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

func TestUploadCSV_acceptsCanonicalFile(t *testing.T) {
	t.Parallel()
	repo := &memStore{}
	srv := httptest.NewServer(api.NewRouter(api.Routes{
		Registers: checkregister.Handler{
			Uploads:        registerupload.NewService(&memTenants{}, repo, 1<<20),
			MaxUploadBytes: 1 << 20,
		},
	}))
	t.Cleanup(srv.Close)

	res := postCSV(t, srv.URL, "acme.csv", canonicalCSV)
	if res.Status != http.StatusAccepted {
		t.Fatalf("status %d body %s", res.Status, res.Raw)
	}
	if res.JSON["upload_id"] == "" || res.JSON["status"] != "accepted" || res.JSON["routing_number"] != "011000015" {
		t.Fatalf("body = %+v", res.JSON)
	}
	if repo.filename != "acme.csv" || !bytes.Contains(repo.body, []byte("routing_number")) {
		t.Fatalf("stored filename %q body %q", repo.filename, repo.body)
	}
}

func TestUploadCSV_rejectsBadHeader(t *testing.T) {
	t.Parallel()
	repo := &memStore{}
	srv := httptest.NewServer(api.NewRouter(api.Routes{
		Registers: checkregister.Handler{
			Uploads: registerupload.NewService(&memTenants{}, repo, 1<<20),
		},
	}))
	t.Cleanup(srv.Close)

	res := postCSV(t, srv.URL, "bad.csv", "aba,account,amount\n")
	if res.Status != http.StatusBadRequest {
		t.Fatalf("status %d body %s", res.Status, res.Raw)
	}
	errObj, _ := res.JSON["error"].(map[string]any)
	if errObj["code"] != "VALIDATION_ERROR" || repo.created {
		t.Fatalf("body = %+v created %v", res.JSON, repo.created)
	}
}

func TestUploadCSV_unknownInstitution(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(api.NewRouter(api.Routes{
		Registers: checkregister.Handler{
			Uploads: registerupload.NewService(&memTenants{err: store.ErrInstitutionNotFound}, &memStore{}, 1<<20),
		},
	}))
	t.Cleanup(srv.Close)
	res := postCSV(t, srv.URL, "acme.csv", canonicalCSV)
	if res.Status != http.StatusNotFound {
		t.Fatalf("status %d body %s", res.Status, res.Raw)
	}
}

func TestGetCheckRegisterUpload(t *testing.T) {
	t.Parallel()
	repo := &memStore{got: registerupload.Upload{
		ID: "33333333-3333-3333-3333-333333333333", Status: "completed",
		FIID: csvFIID, PartyID: csvPartyID, RoutingNumber: "011000015",
		Filename: "acme.csv", Accepted: 2, Rejected: 1, RowsProcessed: 3,
		ReceivedAt: time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC),
		Rejections: []registerupload.Rejection{{Row: 2, Code: "unknown_account", Message: "missing"}},
	}}
	srv := httptest.NewServer(api.NewRouter(api.Routes{
		Registers: checkregister.Handler{
			Uploads: registerupload.NewService(&memTenants{}, repo, 1<<20),
		},
	}))
	t.Cleanup(srv.Close)
	res, err := http.Get(srv.URL + "/v1/fis/" + csvFIID + "/parties/" + csvPartyID + "/check-register-uploads/33333333-3333-3333-3333-333333333333")
	if err != nil {
		t.Fatal(err)
	}
	body := readHTTP(t, res)
	if body.Status != http.StatusOK || body.JSON["accepted"] != float64(2) || body.JSON["rejections_truncated"] != false {
		t.Fatalf("body = %+v", body.JSON)
	}
}

func TestJSONRegisterUpload_stillRejectedWithoutItems(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(api.NewRouter(api.Routes{}))
	t.Cleanup(srv.Close)
	res, err := http.Post(srv.URL+"/v1/fis/"+csvFIID+"/parties/"+csvPartyID+"/check-registers", "application/json", strings.NewReader(`{"format":"json","items":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	body := readHTTP(t, res)
	if body.Status != http.StatusBadRequest {
		t.Fatalf("status %d body %s", body.Status, body.Raw)
	}
}

func postCSV(t *testing.T, base, filename, content string) httpResult {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/v1/fis/"+csvFIID+"/parties/"+csvPartyID+"/check-registers", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return readHTTP(t, res)
}

type httpResult struct {
	Status int
	JSON   map[string]any
	Raw    []byte
}

func readHTTP(t *testing.T, res *http.Response) httpResult {
	t.Helper()
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json %s: %v", raw, err)
	}
	return httpResult{Status: res.StatusCode, JSON: payload, Raw: raw}
}

const canonicalCSV = "routing_number,account_number,check_number,amount,payee,issue_date,void\n" +
	"011000015,100200300,100,1.00,ACME,2026-10-01,false\n"

type memTenants struct {
	err error
}

func (m *memTenants) Customer(context.Context, string, string) (registerupload.Customer, error) {
	if m.err != nil {
		return registerupload.Customer{}, m.err
	}
	return registerupload.Customer{FIActive: true, PartyActive: true, RoutingNumber: "011000015"}, nil
}

func (m *memTenants) Accounts(context.Context, string, string) (map[string]string, error) {
	return map[string]string{}, nil
}

type memStore struct {
	created  bool
	filename string
	body     []byte
	got      registerupload.Upload
}

func (m *memStore) WithinTx(ctx context.Context, fn func(context.Context, registerupload.Tx) error) error {
	return fn(ctx, m)
}

func (m *memStore) WithinConn(context.Context, func(context.Context, registerupload.Conn) error) error {
	return registerupload.ErrUploadNotFound
}

func (m *memStore) Find(context.Context, string, string, string) (registerupload.Upload, error) {
	if m.got.ID == "" {
		return registerupload.Upload{}, registerupload.ErrUploadNotFound
	}
	return m.got, nil
}

func (m *memStore) InsertIntake(_ context.Context, row registerupload.Intake) (time.Time, error) {
	m.created = true
	m.filename = row.Filename
	return time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC), nil
}

func (m *memStore) InsertChunk(_ context.Context, _ string, _ int, data []byte) error {
	m.body = append(m.body, data...)
	return nil
}

func (m *memStore) SetBytes(context.Context, string, int64, []byte) error { return nil }

func (m *memStore) InsertMessage(context.Context, string, []byte) error { return nil }

func (m *memStore) LockIntake(context.Context, string) (registerupload.IntakeRow, error) {
	return registerupload.IntakeRow{}, registerupload.ErrUploadNotFound
}

func (m *memStore) InsertBatch(context.Context, string, string, string) (string, error) {
	return "", nil
}

func (m *memStore) SetIngesting(context.Context, string, string) error { return nil }

func (m *memStore) RejectionCount(context.Context, string) (int, error) { return 0, nil }

func (m *memStore) InsertItem(context.Context, string, string, string, store.UploadItem, string) error {
	return nil
}

func (m *memStore) InsertRejection(context.Context, string, registerupload.Rejection) error {
	return nil
}

func (m *memStore) Advance(context.Context, string, int, int, int) error { return nil }
