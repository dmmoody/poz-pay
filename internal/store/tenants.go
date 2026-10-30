package store

import (
	"context"
	"errors"
)

// Account is one account under a financial institution and party.
type Account struct {
	ID                 string
	FIID               string
	PartyID            string
	Active             bool
	PositivePayEnabled bool
}

// Customer is the institution and party an upload is filed under.
// RoutingNumber is that institution's stored number. Empty means the column is null.
type Customer struct {
	FIActive      bool
	PartyActive   bool
	RoutingNumber string
}

var (
	// ErrInstitutionNotFound means the financial institution id is not stored.
	ErrInstitutionNotFound = errors.New("financial institution not found")

	// ErrPartyNotFound means the party is not stored under that institution.
	ErrPartyNotFound = errors.New("party not found")
)

// Tenants reads institutions, parties, and accounts.
type Tenants interface {
	FIExists(ctx context.Context, fiID string) (bool, error)
	PartyBelongsToFI(ctx context.Context, fiID, partyID string) (bool, error)
	GetAccount(ctx context.Context, fiID, accountID string) (*Account, error)
	Customer(ctx context.Context, fiID, partyID string) (Customer, error)
	Accounts(ctx context.Context, fiID, partyID string) (map[string]string, error)
	AccountOnParty(ctx context.Context, fiID, partyID, accountID string) (bool, error)
}
