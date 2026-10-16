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
		payeeNorm sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT check_number, amount, payee, payee_normalized, issue_date, voided_at, paid_at
		FROM check_register_items
		WHERE fi_id = $1::uuid AND account_id = $2::uuid AND check_number = $3
		ORDER BY voided_at NULLS FIRST, created_at DESC
		LIMIT 1
	`, fiID, accountID, checkNumber).Scan(
		&item.CheckNumber,
		&item.Amount,
		&item.Payee,
		&payeeNorm,
		&issueDate,
		&voidedAt,
		&paidAt,
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
		t := paidAt.Time
		item.PaidAt = &t
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

// MarkPaid sets paid_at on the active (non-void) register item if still unpaid.
// Returns true when this call marked the row; false if already paid, voided, or missing.
func (s *CheckRegisterStore) MarkPaid(ctx context.Context, fiID, accountID, checkNumber string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE check_register_items
		SET paid_at = now()
		WHERE fi_id = $1::uuid
		  AND account_id = $2::uuid
		  AND check_number = $3
		  AND voided_at IS NULL
		  AND paid_at IS NULL
	`, fiID, accountID, checkNumber)
	if err != nil {
		return false, fmt.Errorf("mark paid: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("mark paid rows: %w", err)
	}
	return n > 0, nil
}
