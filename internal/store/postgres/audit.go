package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/dmmoody/poz-pay/internal/decide"
)

// AuditStore appends decision_audit rows.
type AuditStore struct {
	db *sql.DB
}

func NewAuditStore(db *sql.DB) *AuditStore {
	return &AuditStore{db: db}
}

// WriteCheckDecision persists a check decide outcome. partyID/accountID may be empty on ACCOUNT_NOT_OURS before resolve.
func (s *AuditStore) WriteCheckDecision(
	ctx context.Context,
	fiID, partyID, accountID, correlationID string,
	req any,
	result decide.Result,
) (string, error) {
	excs, err := json.Marshal(result.Exceptions)
	if err != nil {
		return "", fmt.Errorf("marshal exceptions: %w", err)
	}
	var matchJSON any
	if result.Match != nil {
		b, err := json.Marshal(result.Match)
		if err != nil {
			return "", fmt.Errorf("marshal match: %w", err)
		}
		matchJSON = b
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	var party any
	if partyID != "" {
		party = partyID
	}
	var account any
	if accountID != "" {
		account = accountID
	}
	var corr any
	if correlationID != "" {
		corr = correlationID
	}

	var id string
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO decision_audit (
			fi_id, party_id, account_id, rail, decision,
			match, exceptions, request, correlation_id, latency_ms
		) VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4, $5,
			$6::jsonb, $7::jsonb, $8::jsonb, $9, $10
		)
		RETURNING id::text
	`, fiID, party, account, decide.RailCheck, result.Decision,
		matchJSON, excs, reqJSON, corr, result.LatencyMs).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert decision_audit: %w", err)
	}
	return id, nil
}
