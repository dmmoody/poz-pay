package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dmmoody/poz-pay/internal/checkpay"
)

// CheckSettingsStore loads FI/party check positive-pay policy.
type CheckSettingsStore struct {
	db *sql.DB
}

func NewCheckSettingsStore(db *sql.DB) *CheckSettingsStore {
	return &CheckSettingsStore{db: db}
}

// ResolvePolicy returns party override when present, else FI settings, else defaults.
func (s *CheckSettingsStore) ResolvePolicy(ctx context.Context, fiID, partyID string) (checkpay.Policy, error) {
	var pol checkpay.Policy
	if partyID != "" {
		err := s.db.QueryRowContext(ctx, `
			SELECT return_on_mismatch
			FROM party_check_settings
			WHERE party_id = $1::uuid AND fi_id = $2::uuid
		`, partyID, fiID).Scan(&pol.ReturnOnMismatch)
		if err == nil {
			return pol, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return pol, fmt.Errorf("party check settings: %w", err)
		}
	}

	err := s.db.QueryRowContext(ctx, `
		SELECT return_on_mismatch
		FROM fi_check_settings
		WHERE fi_id = $1::uuid
	`, fiID).Scan(&pol.ReturnOnMismatch)
	if errors.Is(err, sql.ErrNoRows) {
		return checkpay.Policy{}, nil
	}
	if err != nil {
		return pol, fmt.Errorf("fi check settings: %w", err)
	}
	return pol, nil
}
