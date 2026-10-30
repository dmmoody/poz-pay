package registerupload

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dmmoody/poz-pay/internal/store"
)

func TestAccept_rejectsTenantAndHeader(t *testing.T) {
	t.Parallel()
	tenants := &fakeTenants{customer: Customer{FIActive: true, PartyActive: true, RoutingNumber: "011000015"}}
	records := &fakeRecords{}
	svc := NewService(tenants, records, 1<<20)

	if _, err := svc.Accept(context.Background(), AcceptInput{Body: strings.NewReader("nope\n")}); !errors.Is(err, ErrInvalidCSV) {
		t.Fatalf("header err = %v", err)
	}
	if records.created {
		t.Fatal("invalid header was stored")
	}

	tenants.customer.FIActive = false
	if _, err := svc.Accept(context.Background(), AcceptInput{Body: strings.NewReader(sampleCSV)}); !errors.Is(err, ErrFIInactive) {
		t.Fatalf("inactive = %v", err)
	}

	tenants.customer.FIActive = true
	tenants.customer.RoutingNumber = ""
	if _, err := svc.Accept(context.Background(), AcceptInput{Body: strings.NewReader(sampleCSV)}); !errors.Is(err, ErrRoutingNumberRequired) {
		t.Fatalf("routing = %v", err)
	}
}

func TestAccept_storesCanonicalFile(t *testing.T) {
	t.Parallel()
	tenants := &fakeTenants{customer: Customer{FIActive: true, PartyActive: true, RoutingNumber: "011000015"}}
	records := &fakeRecords{}
	svc := NewService(tenants, records, 1<<20)
	got, err := svc.Accept(context.Background(), AcceptInput{
		FIID:     "11111111-1111-1111-1111-111111111111",
		PartyID:  "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		Filename: `..\Acme Register.CSV`,
		Body:     strings.NewReader(sampleCSV),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == "" || got.ID != records.intakeID || records.filename != "Acme Register.CSV" || records.topic != TopicUploadAccepted {
		t.Fatalf("upload = %+v filename %q topic %q", got, records.filename, records.topic)
	}
	if !bytes.Equal(records.body, []byte(sampleCSV)) {
		t.Fatalf("stored body = %q", records.body)
	}
}

func TestIngest_batchesSkipsAndClassifies(t *testing.T) {
	t.Parallel()
	records := &fakeRecords{
		status:        "accepted",
		routingNumber: "011000015",
		rowsProcessed: 1,
		file: []byte(strings.Join([]string{
			"routing_number,account_number,check_number,amount,payee,issue_date,void",
			"011000015,100200300,100,1.00,SKIP ME,2026-10-01,false",
			"011000015,100200300,101,2.00,KEEP,2026-10-01,false",
			"021000021,100200300,102,3.00,OTHER BANK,2026-10-01,false",
			"011000015,999,103,4.00,UNKNOWN,2026-10-01,false",
		}, "\n") + "\n"),
	}
	tenants := &fakeTenants{accounts: map[string]string{"100200300": "acct-1"}}
	svc := NewService(tenants, records, 1<<20)
	svc.batchSize = 2

	if err := svc.Ingest(context.Background(), "11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatal(err)
	}
	if !records.completed || records.failed != "" {
		t.Fatalf("completed=%v failed=%q", records.completed, records.failed)
	}
	if len(records.batches) != 2 {
		t.Fatalf("batches = %+v", records.batches)
	}
	first := records.batches[0]
	if first.through != 3 || len(first.items) != 1 || len(first.rejections) != 1 {
		t.Fatalf("first batch = %+v", first)
	}
	if first.items[0].CheckNumber != "101" {
		t.Fatalf("kept row = %+v", first.items[0])
	}
	if first.rejections[0].Code != CodeRoutingMismatch {
		t.Fatalf("mismatch = %+v", first.rejections[0])
	}
	second := records.batches[1]
	if second.through != 4 || len(second.rejections) != 1 || second.rejections[0].Code != CodeUnknownAccount {
		t.Fatalf("second batch = %+v", second)
	}
}

func TestIngest_brokenFileFailsWithoutRetry(t *testing.T) {
	t.Parallel()
	records := &fakeRecords{
		status:        "accepted",
		routingNumber: "011000015",
		file:          []byte("routing_number,account_number,check_number,amount,payee,issue_date,void\n\"unterminated\n"),
	}
	svc := NewService(&fakeTenants{}, records, 1<<20)
	if err := svc.Ingest(context.Background(), "11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatal(err)
	}
	if records.failed == "" || records.completed {
		t.Fatalf("failed=%q completed=%v", records.failed, records.completed)
	}
}

func TestIngest_busyAndFinished(t *testing.T) {
	t.Parallel()
	svc := NewService(&fakeTenants{}, &fakeRecords{busy: true}, 1)
	if err := svc.Ingest(context.Background(), "11111111-1111-1111-1111-111111111111"); !errors.Is(err, ErrBusy) {
		t.Fatalf("busy = %v", err)
	}
	svc = NewService(&fakeTenants{}, &fakeRecords{status: "completed"}, 1)
	if err := svc.Ingest(context.Background(), "11111111-1111-1111-1111-111111111111"); !errors.Is(err, ErrAlreadyComplete) {
		t.Fatalf("complete = %v", err)
	}
}

func TestConsume_acksPoisonAndFinished(t *testing.T) {
	t.Parallel()
	lis := &fakeListener{}
	svc := NewService(&fakeTenants{}, &fakeRecords{status: "completed"}, 1)
	if err := Consume(context.Background(), lis, svc); err != nil {
		t.Fatal(err)
	}
	if err := lis.handle(context.Background(), []byte(`{"upload_id":"nope"}`)); err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-1111-1111-111111111111"
	if err := lis.handle(context.Background(), []byte(`{"upload_id":"`+id+`"}`)); err != nil {
		t.Fatal(err)
	}

	svc = NewService(&fakeTenants{}, &fakeRecords{busy: true}, 1)
	lis = &fakeListener{}
	if err := Consume(context.Background(), lis, svc); err != nil {
		t.Fatal(err)
	}
	if err := lis.handle(context.Background(), []byte(`{"upload_id":"`+id+`"}`)); !errors.Is(err, ErrBusy) {
		t.Fatalf("redeliver = %v", err)
	}
}

const sampleCSV = "routing_number,account_number,check_number,amount,payee,issue_date,void\n" +
	"011000015,100200300,100,1.00,ACME,2026-10-01,false\n"

type fakeTenants struct {
	customer Customer
	accounts map[string]string
}

func (f *fakeTenants) Customer(context.Context, string, string) (Customer, error) {
	return f.customer, nil
}

func (f *fakeTenants) Accounts(context.Context, string, string) (map[string]string, error) {
	return f.accounts, nil
}

type savedBatch struct {
	items      []store.UploadItem
	rejections []Rejection
	through    int
}

type fakeRecords struct {
	created       bool
	filename      string
	topic         string
	body          []byte
	intakeID      string
	busy          bool
	status        string
	routingNumber string
	rowsProcessed int
	file          []byte
	chunkRead     bool
	completed     bool
	failed        string
	items         []store.UploadItem
	rejections    []Rejection
	batches       []savedBatch
}

func (f *fakeRecords) WithinTx(ctx context.Context, fn func(context.Context, Tx) error) error {
	return fn(ctx, f)
}

func (f *fakeRecords) WithinConn(ctx context.Context, fn func(context.Context, Conn) error) error {
	return fn(ctx, f)
}

func (f *fakeRecords) Find(context.Context, string, string, string) (Upload, error) {
	return Upload{}, ErrUploadNotFound
}

func (f *fakeRecords) InsertIntake(_ context.Context, row Intake) (time.Time, error) {
	f.created = true
	f.filename = row.Filename
	f.intakeID = row.ID
	return time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC), nil
}

func (f *fakeRecords) InsertChunk(_ context.Context, _ string, _ int, data []byte) error {
	f.body = append(f.body, data...)
	return nil
}

func (f *fakeRecords) SetBytes(context.Context, string, int64, []byte) error { return nil }

func (f *fakeRecords) InsertMessage(_ context.Context, topic string, _ []byte) error {
	f.topic = topic
	return nil
}

func (f *fakeRecords) LockIntake(context.Context, string) (IntakeRow, error) {
	status := f.status
	if status == "" {
		status = "accepted"
	}
	return IntakeRow{
		RoutingNumber: f.routingNumber,
		Status:        status,
		RowsProcessed: f.rowsProcessed,
	}, nil
}

func (f *fakeRecords) InsertBatch(context.Context, string, string, string) (string, error) {
	return "batch-1", nil
}

func (f *fakeRecords) SetIngesting(context.Context, string, string) error { return nil }

func (f *fakeRecords) RejectionCount(context.Context, string) (int, error) { return 0, nil }

func (f *fakeRecords) InsertItem(_ context.Context, _, _, _ string, item store.UploadItem, _ string) error {
	f.items = append(f.items, item)
	return nil
}

func (f *fakeRecords) InsertRejection(_ context.Context, _ string, rej Rejection) error {
	f.rejections = append(f.rejections, rej)
	return nil
}

func (f *fakeRecords) Advance(_ context.Context, _ string, _, _, rowsProcessed int) error {
	f.batches = append(f.batches, savedBatch{
		items:      f.items,
		rejections: f.rejections,
		through:    rowsProcessed,
	})
	f.items = nil
	f.rejections = nil
	return nil
}

func (f *fakeRecords) TryLock(context.Context, string) (bool, error) { return !f.busy, nil }

func (f *fakeRecords) Unlock(context.Context, string) error { return nil }

func (f *fakeRecords) ReadChunk(context.Context, string, int) ([]byte, bool, error) {
	if f.chunkRead {
		return nil, false, nil
	}
	f.chunkRead = true
	if len(f.file) == 0 {
		return nil, false, nil
	}
	return f.file, true, nil
}

func (f *fakeRecords) MarkCompleted(context.Context, string) error {
	f.completed = true
	return nil
}

func (f *fakeRecords) MarkFailed(_ context.Context, _, reason string) error {
	f.failed = reason
	return nil
}

type fakeListener struct {
	handle func(context.Context, []byte) error
}

func (f *fakeListener) Listen(_ context.Context, topic string, handle func(context.Context, []byte) error) error {
	if topic != TopicUploadAccepted {
		return errors.New("unexpected topic")
	}
	f.handle = handle
	return nil
}
