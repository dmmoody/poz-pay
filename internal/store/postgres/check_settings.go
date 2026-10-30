package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dmmoody/poz-pay/internal/store"
)

// CheckSettings loads FI/party check positive-pay policy.
type CheckSettings struct {
	db *sql.DB
}

func NewCheckSettings(db *sql.DB) *CheckSettings {
	return &CheckSettings{db: db}
}

// ResolvePolicy returns party override when present, else FI settings, else defaults.
func (s *CheckSettings) ResolvePolicy(ctx context.Context, fiID, partyID string) (store.Policy, error) {
	var pol store.Policy
	if partyID != "" {
		err := s.db.QueryRowContext(ctx, `
			SELECT return_on_mismatch, max_age_days
			FROM party_check_settings
			WHERE party_id = $1::uuid AND fi_id = $2::uuid
		`, partyID, fiID).Scan(&pol.ReturnOnMismatch, &pol.MaxAgeDays)
		if err == nil {
			return pol, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return pol, fmt.Errorf("party check settings: %w", err)
		}
	}

	err := s.db.QueryRowContext(ctx, `
		SELECT return_on_mismatch, max_age_days
		FROM fi_check_settings
		WHERE fi_id = $1::uuid
	`, fiID).Scan(&pol.ReturnOnMismatch, &pol.MaxAgeDays)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Policy{MaxAgeDays: store.DefaultMaxAgeDays}, nil
	}
	if err != nil {
		return pol, fmt.Errorf("fi check settings: %w", err)
	}
	return pol, nil
}
