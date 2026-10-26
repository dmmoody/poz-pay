package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dmmoody/poz-pay/internal/checkpay"
)

// CheckRegisterStore persists and loads check register rows.
type CheckRegisterStore struct {
	db *sql.DB
}

func NewCheckRegisterStore(db *sql.DB) *CheckRegisterStore {
	return &CheckRegisterStore{db: db}
}

// FindItem loads the best matching register row for decide (active preferred over voided).
func (s *CheckRegisterStore) FindItem(ctx context.Context, fiID, accountID, checkNumber string) (*checkpay.RegisterItem, error) {
	var (
		item      checkpay.RegisterItem
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
func (s *CheckRegisterStore) InsertBatch(
	ctx context.Context,
	fiID, partyID string,
	items []checkpay.UploadItem,
	payeeNorm func(string) string,
) (checkpay.UploadBatchResult, error) {
	var out checkpay.UploadBatchResult
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
		var voided any
		if it.Void {
			voided = time.Now().UTC()
		}
		var issue any
		if it.IssueDate != nil {
			issue = it.IssueDate.UTC()
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO check_register_items (
				batch_id, fi_id, party_id, account_id,
				check_number, amount, payee, payee_normalized, issue_date, voided_at
			) VALUES (
				$1::uuid, $2::uuid, $3::uuid, $4::uuid,
				$5, $6, $7, $8, $9, $10
			)
		`, batchID, fiID, partyID, it.AccountID,
			it.CheckNumber, it.Amount, it.Payee, payeeNorm(it.Payee), issue, voided)
		if err != nil {
			out.Errors = append(out.Errors, checkpay.UploadItemError{
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
func (s *CheckRegisterStore) RecordPayment(ctx context.Context, fiID, accountID, checkNumber, correlationID string, postedAt time.Time) (checkpay.Payment, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return checkpay.Payment{}, fmt.Errorf("begin post: %w", err)
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
	`, fiID, accountID, checkNumber).Scan(&id, &voidedAt, &paidAt, &paidCorr)
	if errors.Is(err, sql.ErrNoRows) {
		return checkpay.Payment{}, checkpay.ErrCheckNotInRegister
	}
	if err != nil {
		return checkpay.Payment{}, fmt.Errorf("lock register item: %w", err)
	}
	if voidedAt.Valid {
		return checkpay.Payment{}, checkpay.ErrCheckVoided
	}
	if paidAt.Valid {
		payment := checkpay.Payment{
			RegisterItemID: id,
			PaidAt:         paidAt.Time.UTC(),
			AlreadyPaid:    true,
		}
		if paidCorr.Valid {
			payment.CorrelationID = paidCorr.String
		}
		if err := tx.Commit(); err != nil {
			return checkpay.Payment{}, fmt.Errorf("commit post: %w", err)
		}
		return payment, nil
	}

	var corr any
	if correlationID != "" {
		corr = correlationID
	}
	var posted time.Time
	err = tx.QueryRowContext(ctx, `
		UPDATE check_register_items
		SET paid_at = $2, paid_correlation_id = $3
		WHERE id = $1::uuid
		  AND voided_at IS NULL
		  AND paid_at IS NULL
		RETURNING paid_at
	`, id, postedAt.UTC(), corr).Scan(&posted)
	if err != nil {
		return checkpay.Payment{}, fmt.Errorf("record payment: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return checkpay.Payment{}, fmt.Errorf("commit post: %w", err)
	}
	return checkpay.Payment{
		RegisterItemID: id,
		PaidAt:         posted.UTC(),
		CorrelationID:  correlationID,
	}, nil
}

var (
	_ checkpay.RegisterReader = (*CheckRegisterStore)(nil)
	_ checkpay.RegisterWriter = (*CheckRegisterStore)(nil)
)
