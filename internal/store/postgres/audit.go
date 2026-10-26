package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/dmmoody/poz-pay/internal/checkpay"
	"github.com/dmmoody/poz-pay/internal/decide"
)

// AuditStore appends decision_audit rows.
type AuditStore struct {
	db *sql.DB
}

func NewAuditStore(db *sql.DB) *AuditStore {
	return &AuditStore{db: db}
}

// WriteCheckDecision persists one check presentment. partyID/accountID may be empty before the account resolves.
func (s *AuditStore) WriteCheckDecision(ctx context.Context, entry checkpay.CheckAuditEntry) (string, error) {
	excs, err := json.Marshal(entry.Result.Exceptions)
	if err != nil {
		return "", fmt.Errorf("marshal exceptions: %w", err)
	}
	var matchJSON any
	if entry.Result.Match != nil {
		b, err := json.Marshal(entry.Result.Match)
		if err != nil {
			return "", fmt.Errorf("marshal match: %w", err)
		}
		matchJSON = b
	}
	reqJSON, err := json.Marshal(entry.Request)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	var party any
	if entry.PartyID != "" {
		party = entry.PartyID
	}
	var account any
	if entry.AccountID != "" {
		account = entry.AccountID
	}
	var corr any
	if entry.CorrelationID != "" {
		corr = entry.CorrelationID
	}
	var registerItem any
	if entry.RegisterItemID != "" {
		registerItem = entry.RegisterItemID
	}

	var id string
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO decision_audit (
			fi_id, party_id, account_id, rail, decision,
			match, exceptions, request, correlation_id, latency_ms,
			check_number, register_item_id
		) VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4, $5,
			$6::jsonb, $7::jsonb, $8::jsonb, $9, $10,
			$11, $12::uuid
		)
		RETURNING id::text
	`, entry.FIID, party, account, decide.RailCheck, entry.Result.Decision,
		matchJSON, excs, reqJSON, corr, entry.Result.LatencyMs,
		entry.CheckNumber, registerItem).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert decision_audit: %w", err)
	}
	return id, nil
}

// ListCheckDecisions returns presentments of one check, oldest first.
func (s *AuditStore) ListCheckDecisions(ctx context.Context, fiID, accountID, checkNumber string) ([]checkpay.RecordedDecision, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, decision, match, exceptions, request,
		       correlation_id, register_item_id::text, decided_at, latency_ms
		FROM decision_audit
		WHERE fi_id = $1::uuid
		  AND account_id = $2::uuid
		  AND rail = 'check'
		  AND check_number = $3
		ORDER BY decided_at ASC, id ASC
	`, fiID, accountID, checkNumber)
	if err != nil {
		return nil, fmt.Errorf("list decision_audit: %w", err)
	}
	defer rows.Close()

	out := []checkpay.RecordedDecision{}
	for rows.Next() {
		var (
			rec      checkpay.RecordedDecision
			matchRaw []byte
			excRaw   []byte
			reqRaw   []byte
			corr     sql.NullString
			regID    sql.NullString
		)
		if err := rows.Scan(
			&rec.ID,
			&rec.Decision,
			&matchRaw,
			&excRaw,
			&reqRaw,
			&corr,
			&regID,
			&rec.DecidedAt,
			&rec.LatencyMs,
		); err != nil {
			return nil, fmt.Errorf("scan decision_audit: %w", err)
		}
		if len(matchRaw) > 0 {
			var match decide.Match
			if err := json.Unmarshal(matchRaw, &match); err != nil {
				return nil, fmt.Errorf("unmarshal match: %w", err)
			}
			rec.Match = &match
		}
		rec.Exceptions = []decide.Exception{}
		if len(excRaw) > 0 {
			if err := json.Unmarshal(excRaw, &rec.Exceptions); err != nil {
				return nil, fmt.Errorf("unmarshal exceptions: %w", err)
			}
			if rec.Exceptions == nil {
				rec.Exceptions = []decide.Exception{}
			}
		}
		rec.Request = reqRaw
		if corr.Valid {
			rec.CorrelationID = corr.String
		}
		if regID.Valid {
			rec.RegisterItemID = regID.String
		}
		rec.DecidedAt = rec.DecidedAt.UTC()
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list decision_audit: %w", err)
	}
	return out, nil
}

var _ checkpay.DecisionLog = (*AuditStore)(nil)
