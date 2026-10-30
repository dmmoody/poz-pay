package registerupload_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/dmmoody/poz-pay/internal/postgres"
	"github.com/dmmoody/poz-pay/internal/registerintake"
	"github.com/dmmoody/poz-pay/internal/registerupload"
	"github.com/dmmoody/poz-pay/internal/tenant"
)

func TestService_ingestAndResume(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	if err := purgeCSVTestBank(ctx, db); err != nil {
		t.Fatalf("purge: %v", err)
	}
	t.Cleanup(func() {
		if err := purgeCSVTestBank(context.Background(), db); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})

	var fiID, partyID, accountID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO financial_institutions (name, routing_number)
		VALUES ('CSV Test Bank', '123456780')
		RETURNING id::text
	`).Scan(&fiID); err != nil {
		t.Fatalf("fi: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
		INSERT INTO parties (fi_id, party_type, name)
		VALUES ($1::uuid, 'business', 'CSV Customer')
		RETURNING id::text
	`, fiID).Scan(&partyID); err != nil {
		t.Fatalf("party: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
		INSERT INTO accounts (fi_id, party_id, external_account_number, name)
		VALUES ($1::uuid, $2::uuid, '555000111', 'CSV Operating')
		RETURNING id::text
	`, fiID, partyID).Scan(&accountID); err != nil {
		t.Fatalf("account: %v", err)
	}

	records := registerintake.New(db)
	tenants := tenant.New(db)
	svc := registerupload.NewService(tenants, records, 1<<20)
	body := strings.Join([]string{
		"routing_number,account_number,check_number,amount,payee,issue_date,void",
		"123456780,555000111,90001,10.00,Vendor One,2026-10-01,false",
		"123456780,555000111,90002,12.50,Vendor Two,2026-10-02,true",
		"011000015,555000111,90003,1.00,Other Bank,2026-10-03,false",
		"011000016,555000111,90004,1.00,Bad Routing,2026-10-03,false",
		"123456780,000,90005,1.00,Unknown Account,2026-10-03,false",
		"123456780,555000111,90001,3.00,Duplicate,2026-10-04,false",
	}, "\n") + "\n"

	upload, err := svc.Accept(ctx, registerupload.AcceptInput{
		FIID: fiID, PartyID: partyID, Filename: "registers.csv", Body: strings.NewReader(body),
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if upload.Status != "accepted" || upload.RoutingNumber != "123456780" || upload.ByteSize != int64(len(body)) {
		t.Fatalf("upload = %+v", upload)
	}

	var pending int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM message_outbox
		WHERE published_at IS NULL AND payload->>'upload_id' = $1
	`, upload.ID).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("outbox pending = %d, %v", pending, err)
	}

	if err := svc.Ingest(ctx, upload.ID); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	got, err := svc.Get(ctx, fiID, partyID, upload.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "completed" || got.Accepted != 2 || got.Rejected != 4 || got.RowsProcessed != 6 {
		t.Fatalf("status = %+v", got)
	}
	if got.RejectionsTruncated() || len(got.Rejections) != 4 {
		t.Fatalf("rejections = %+v", got.Rejections)
	}
	codes := map[string]int{}
	for _, rej := range got.Rejections {
		codes[rej.Code]++
	}
	if codes[registerupload.CodeRoutingMismatch] != 2 ||
		codes[registerupload.CodeUnknownAccount] != 1 || codes[registerupload.CodeDuplicateCheck] != 1 {
		t.Fatalf("codes = %v", codes)
	}

	var amount int64
	var voided bool
	var payee string
	err = db.QueryRowContext(ctx, `
		SELECT amount, voided_at IS NOT NULL, payee_normalized
		FROM check_register_items
		WHERE fi_id = $1::uuid AND account_id = $2::uuid AND check_number = '90001'
	`, fiID, accountID).Scan(&amount, &voided, &payee)
	if err != nil || amount != 1000 || voided || payee != "VENDOR ONE" {
		t.Fatalf("90001 amount=%d voided=%v payee=%q err=%v", amount, voided, payee, err)
	}
	err = db.QueryRowContext(ctx, `
		SELECT amount, voided_at IS NOT NULL
		FROM check_register_items
		WHERE fi_id = $1::uuid AND account_id = $2::uuid AND check_number = '90002'
	`, fiID, accountID).Scan(&amount, &voided)
	if err != nil || amount != 1250 || !voided {
		t.Fatalf("90002 amount=%d voided=%v err=%v", amount, voided, err)
	}

	if _, err := db.ExecContext(ctx, `
		UPDATE check_register_uploads
		SET status = 'ingesting', completed_at = NULL, updated_at = now()
		WHERE id = $1::uuid
	`, upload.ID); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := svc.Ingest(ctx, upload.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	got, err = svc.Get(ctx, fiID, partyID, upload.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "completed" || got.Accepted != 2 || got.Rejected != 4 {
		t.Fatalf("after resume = %+v", got)
	}
	if err := svc.Ingest(ctx, upload.ID); !errors.Is(err, registerupload.ErrAlreadyComplete) {
		t.Fatalf("second pass = %v", err)
	}

	small := registerupload.NewService(tenants, records, 32)
	_, err = small.Accept(ctx, registerupload.AcceptInput{
		FIID: fiID, PartyID: partyID, Filename: "too-big.csv", Body: strings.NewReader(body),
	})
	if !errors.Is(err, registerupload.ErrTooLarge) {
		t.Fatalf("too large = %v", err)
	}
	var leftover int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM check_register_uploads WHERE filename = 'too-big.csv'
	`).Scan(&leftover); err != nil || leftover != 0 {
		t.Fatalf("leftover too-big rows = %d, %v", leftover, err)
	}
}

func testDatabase(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := postgres.Open(context.Background(), url)
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var ready bool
	if err := db.QueryRow(`SELECT to_regclass('public.check_register_uploads') IS NOT NULL`).Scan(&ready); err != nil || !ready {
		t.Skip("migration 000014 is not applied")
	}
	return db
}

func purgeCSVTestBank(ctx context.Context, db *sql.DB) error {
	const fi = `SELECT id FROM financial_institutions WHERE name = 'CSV Test Bank'`
	stmts := []string{
		`DELETE FROM message_outbox WHERE payload->>'upload_id' IN (
			SELECT id::text FROM check_register_uploads WHERE fi_id IN (` + fi + `)
		)`,
		`DELETE FROM check_register_uploads WHERE fi_id IN (` + fi + `)`,
		`DELETE FROM check_register_batches WHERE fi_id IN (` + fi + `)`,
		`DELETE FROM accounts WHERE fi_id IN (` + fi + `)`,
		`DELETE FROM parties WHERE fi_id IN (` + fi + `)`,
		`DELETE FROM financial_institutions WHERE name = 'CSV Test Bank'`,
	}
	for _, q := range stmts {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
