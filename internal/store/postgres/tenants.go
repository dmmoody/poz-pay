package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dmmoody/poz-pay/internal/store"
)

// Tenants reads FI / account rows for decide and upload.
type Tenants struct {
	db *sql.DB
}

func NewTenants(db *sql.DB) *Tenants {
	return &Tenants{db: db}
}

func (s *Tenants) FIExists(ctx context.Context, fiID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM financial_institutions WHERE id = $1::uuid)
	`, fiID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("fi exists: %w", err)
	}
	return exists, nil
}

func (s *Tenants) PartyBelongsToFI(ctx context.Context, fiID, partyID string) (bool, error) {
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
func (s *Tenants) GetAccount(ctx context.Context, fiID, accountID string) (*store.Account, error) {
	var a store.Account
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

// Customer returns the institution and party an upload is filed under.
func (s *Tenants) Customer(ctx context.Context, fiID, partyID string) (store.Customer, error) {
	var (
		fiActive bool
		routing  sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT active, routing_number
		FROM financial_institutions
		WHERE id = $1::uuid
	`, fiID).Scan(&fiActive, &routing)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Customer{}, store.ErrInstitutionNotFound
	}
	if err != nil {
		return store.Customer{}, fmt.Errorf("load financial institution: %w", err)
	}

	var partyActive bool
	err = s.db.QueryRowContext(ctx, `
		SELECT active FROM parties WHERE id = $1::uuid AND fi_id = $2::uuid
	`, partyID, fiID).Scan(&partyActive)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Customer{}, store.ErrPartyNotFound
	}
	if err != nil {
		return store.Customer{}, fmt.Errorf("load party: %w", err)
	}
	out := store.Customer{FIActive: fiActive, PartyActive: partyActive}
	if routing.Valid {
		out.RoutingNumber = routing.String
	}
	return out, nil
}

// Accounts maps external account numbers to account ids for one customer.
func (s *Tenants) Accounts(ctx context.Context, fiID, partyID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT external_account_number, id::text
		FROM accounts
		WHERE fi_id = $1::uuid AND party_id = $2::uuid
		  AND external_account_number IS NOT NULL
	`, fiID, partyID)
	if err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var number, id string
		if err := rows.Scan(&number, &id); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		out[number] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}
	return out, nil
}

// AccountOnParty checks account belongs to party under FI.
func (s *Tenants) AccountOnParty(ctx context.Context, fiID, partyID, accountID string) (bool, error) {
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
