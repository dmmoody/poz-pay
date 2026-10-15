package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dmmoody/poz-pay/internal/checkpay"
)

// TenantStore reads FI / account rows for decide and upload.
type TenantStore struct {
	db *sql.DB
}

func NewTenantStore(db *sql.DB) *TenantStore {
	return &TenantStore{db: db}
}

func (s *TenantStore) FIExists(ctx context.Context, fiID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM financial_institutions WHERE id = $1::uuid)
	`, fiID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("fi exists: %w", err)
	}
	return exists, nil
}

func (s *TenantStore) PartyBelongsToFI(ctx context.Context, fiID, partyID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM parties WHERE id = $1::uuid AND fi_id = $2::uuid AND active
		)
	`, partyID, fiID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("party belongs: %w", err)
	}
	return exists, nil
}

// GetAccount returns the account for (fi_id, account_id) or nil if not found under that FI.
func (s *TenantStore) GetAccount(ctx context.Context, fiID, accountID string) (*checkpay.Account, error) {
	var a checkpay.Account
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, fi_id::text, party_id::text, active, positive_pay_enabled
		FROM accounts
		WHERE id = $1::uuid AND fi_id = $2::uuid
	`, accountID, fiID).Scan(&a.ID, &a.FIID, &a.PartyID, &a.Active, &a.PositivePayEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	return &a, nil
}

// AccountOnParty checks account belongs to party under FI.
func (s *TenantStore) AccountOnParty(ctx context.Context, fiID, partyID, accountID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM accounts
			WHERE id = $1::uuid AND party_id = $2::uuid AND fi_id = $3::uuid
		)
	`, accountID, partyID, fiID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("account on party: %w", err)
	}
	return exists, nil
}
