package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DefaultMaxAgeDays is the stale window when check settings omit max age.
const DefaultMaxAgeDays = 180

// Policy is the check positive pay setting for an institution or party.
type Policy struct {
	// ReturnOnMismatch maps field mismatches to a return decision.
	ReturnOnMismatch bool
	// MaxAgeDays is days after the register issue date before a check is expired.
	// Zero or less means DefaultMaxAgeDays.
	MaxAgeDays int
}

// CheckSettings loads the positive-pay policy for an institution or party.
type CheckSettings interface {
	ResolvePolicy(ctx context.Context, fiID, partyID string) (Policy, error)
}

// CheckSettings loads FI/party check positive-pay policy.
type checkSettings struct {
	db *sql.DB
}

func NewCheckSettings(db *sql.DB) CheckSettings {
	return &checkSettings{db: db}
}

// ResolvePolicy returns party override when present, else FI settings, else defaults.
func (s *checkSettings) ResolvePolicy(ctx context.Context, fiID, partyID string) (Policy, error) {
	var pol Policy
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
		return Policy{MaxAgeDays: DefaultMaxAgeDays}, nil
	}
	if err != nil {
		return pol, fmt.Errorf("fi check settings: %w", err)
	}
	return pol, nil
}
