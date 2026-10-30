package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrNotInRegister means no register row matches the check number.
	ErrNotInRegister = errors.New("check not in register")

	// ErrVoided means the matching register row is voided.
	ErrVoided = errors.New("check voided")
)

// RegisterItem is an issued check row used for matching.
type RegisterItem struct {
	ID                string
	CheckNumber       string
	Amount            int64
	Payee             string
	PayeeNormalized   string
	IssueDate         *time.Time
	VoidedAt          *time.Time
	PaidAt            *time.Time
	PaidCorrelationID string
}

// UploadItem is one register line to insert.
type UploadItem struct {
	AccountID   string
	CheckNumber string
	Amount      int64
	Payee       string
	IssueDate   *time.Time
	Void        bool
}

// UploadBatchResult is the outcome of inserting a JSON register batch.
type UploadBatchResult struct {
	BatchID  string
	Accepted int
	Rejected int
	Errors   []UploadItemError
}

// UploadItemError is a per-row reject from a JSON register batch.
type UploadItemError struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// Payment is the posting recorded against a register item.
type Payment struct {
	RegisterItemID string    `json:"register_item_id"`
	CheckNumber    string    `json:"check_number"`
	PaidAt         time.Time `json:"paid_at"`
	CorrelationID  string    `json:"correlation_id,omitempty"`
	// AlreadyPaid reports that this call found an existing posting.
	AlreadyPaid bool `json:"-"`
}

// PaymentRecord is the posting written with its audit event in one transaction.
type PaymentRecord struct {
	FIID          string
	PartyID       string
	AccountID     string
	CheckNumber   string
	CorrelationID string
	PaidAt        time.Time
	Request       any
}

// CheckRegister loads issued checks and writes batches and postings.
type CheckRegister interface {
	FindItem(ctx context.Context, fiID, accountID, checkNumber string) (*RegisterItem, error)
	InsertBatch(ctx context.Context, fiID, partyID string, items []UploadItem, payeeNorm func(string) string) (UploadBatchResult, error)
	RecordPayment(ctx context.Context, rec PaymentRecord) (Payment, error)
}

// CheckRegister persists and loads check register rows.
type checkRegister struct {
	db *sql.DB
}

func NewCheckRegister(db *sql.DB) CheckRegister {
	return &checkRegister{db: db}
}

// FindItem loads the best matching register row for decide (active preferred over voided).
func (s *checkRegister) FindItem(ctx context.Context, fiID, accountID, checkNumber string) (*RegisterItem, error) {
	var (
		item      RegisterItem
		issueDate sql.NullTime
		voidedAt  sql.NullTime
		paidAt    sql.NullTime
		paidCorr  sql.NullString
		payeeNorm sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, check_number, amount, payee, payee_normalized,
		       issue_date, voided_at, paid_at, paid_correlation_id
		FROM check_register_items
		WHERE fi_id = $1::uuid AND account_id = $2::uuid AND check_number = $3
		ORDER BY voided_at NULLS FIRST, created_at DESC
		LIMIT 1
	`, fiID, accountID, checkNumber).Scan(
		&item.ID,
		&item.CheckNumber,
		&item.Amount,
		&item.Payee,
		&payeeNorm,
		&issueDate,
		&voidedAt,
		&paidAt,
		&paidCorr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find register item: %w", err)
	}
	if payeeNorm.Valid {
		item.PayeeNormalized = payeeNorm.String
	}
	if issueDate.Valid {
		t := issueDate.Time
		item.IssueDate = &t
	}
	if voidedAt.Valid {
		t := voidedAt.Time
		item.VoidedAt = &t
	}
	if paidAt.Valid {
		t := paidAt.Time.UTC()
		item.PaidAt = &t
	}
	if paidCorr.Valid {
		item.PaidCorrelationID = paidCorr.String
	}
	return &item, nil
}

// InsertBatch writes a register batch and accepted items in one transaction.
func (s *checkRegister) InsertBatch(
	ctx context.Context,
	fiID, partyID string,
	items []UploadItem,
	payeeNorm func(string) string,
) (UploadBatchResult, error) {
	var out UploadBatchResult
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, fmt.Errorf("begin upload: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var batchID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO check_register_batches (fi_id, party_id, item_count, source)
		VALUES ($1::uuid, $2::uuid, 0, 'json')
		RETURNING id::text
	`, fiID, partyID).Scan(&batchID)
	if err != nil {
		return out, fmt.Errorf("insert batch: %w", err)
	}
	out.BatchID = batchID

	for i, it := range items {
		if err := insertItem(ctx, tx, batchID, fiID, partyID, it, payeeNorm(it.Payee)); err != nil {
			out.Errors = append(out.Errors, UploadItemError{
				Index:   i,
				Message: err.Error(),
			})
			out.Rejected++
			continue
		}
		out.Accepted++
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE check_register_batches SET item_count = $2 WHERE id = $1::uuid
	`, batchID, out.Accepted)
	if err != nil {
		return out, fmt.Errorf("update batch count: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return out, fmt.Errorf("commit upload: %w", err)
	}
	return out, nil
}

// RecordPayment sets paid_at on the active register item.
// A repeated call returns the original posting and does not move paid_at.
func (s *checkRegister) RecordPayment(ctx context.Context, rec PaymentRecord) (Payment, error) {
	started := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Payment{}, fmt.Errorf("begin post: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		id       string
		voidedAt sql.NullTime
		paidAt   sql.NullTime
		paidCorr sql.NullString
	)
	err = tx.QueryRowContext(ctx, `
		SELECT id::text, voided_at, paid_at, paid_correlation_id
		FROM check_register_items
		WHERE fi_id = $1::uuid AND account_id = $2::uuid AND check_number = $3
		ORDER BY voided_at NULLS FIRST, created_at DESC
		LIMIT 1
		FOR UPDATE
	`, rec.FIID, rec.AccountID, rec.CheckNumber).Scan(&id, &voidedAt, &paidAt, &paidCorr)
	if errors.Is(err, sql.ErrNoRows) {
		return Payment{}, ErrNotInRegister
	}
	if err != nil {
		return Payment{}, fmt.Errorf("lock register item: %w", err)
	}
	if voidedAt.Valid {
		return Payment{}, ErrVoided
	}
	if paidAt.Valid {
		payment := Payment{
			RegisterItemID: id,
			PaidAt:         paidAt.Time.UTC(),
			AlreadyPaid:    true,
		}
		if paidCorr.Valid {
			payment.CorrelationID = paidCorr.String
		}
		if err := tx.Commit(); err != nil {
			return Payment{}, fmt.Errorf("commit post: %w", err)
		}
		return payment, nil
	}

	var corr any
	if rec.CorrelationID != "" {
		corr = rec.CorrelationID
	}
	var posted time.Time
	err = tx.QueryRowContext(ctx, `
		UPDATE check_register_items
		SET paid_at = $2, paid_correlation_id = $3
		WHERE id = $1::uuid
		  AND voided_at IS NULL
		  AND paid_at IS NULL
		RETURNING paid_at
	`, id, rec.PaidAt.UTC(), corr).Scan(&posted)
	if err != nil {
		return Payment{}, fmt.Errorf("record payment: %w", err)
	}
	if err := insertPosted(ctx, tx, postedCheck{
		FIID:          rec.FIID,
		PartyID:       rec.PartyID,
		AccountID:     rec.AccountID,
		CheckNumber:   rec.CheckNumber,
		CorrelationID: rec.CorrelationID,
		Request:       rec.Request,
	}, id, posted, time.Since(started).Milliseconds()); err != nil {
		return Payment{}, err
	}
	if err := tx.Commit(); err != nil {
		return Payment{}, fmt.Errorf("commit post: %w", err)
	}
	return Payment{
		RegisterItemID: id,
		PaidAt:         posted.UTC(),
		CorrelationID:  rec.CorrelationID,
	}, nil
}

func insertItem(
	ctx context.Context,
	tx *sql.Tx,
	batchID, fiID, partyID string,
	it UploadItem,
	payeeNormalized string,
) error {
	var voided any
	if it.Void {
		voided = time.Now().UTC()
	}
	// Calendar date as text so the session TimeZone cannot shift the day.
	var issue any
	if it.IssueDate != nil {
		issue = it.IssueDate.UTC().Format("2006-01-02")
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO check_register_items (
			batch_id, fi_id, party_id, account_id,
			check_number, amount, payee, payee_normalized, issue_date, voided_at
		) VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4::uuid,
			$5, $6, $7, $8, $9::date, $10
		)
	`, batchID, fiID, partyID, it.AccountID,
		it.CheckNumber, it.Amount, it.Payee, payeeNormalized, issue, voided)
	return err
}
