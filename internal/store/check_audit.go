package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dmmoody/poz-pay/internal/decision"
)

const (
	// KindPresentment is a decide call on the check's audit stream.
	KindPresentment = "presentment"

	// KindPosted is the paying system's settlement on that same stream.
	KindPosted = "posted"
)

// CheckEntry is one presentment attempt written to decision history.
type CheckEntry struct {
	FIID           string
	PartyID        string
	AccountID      string
	CorrelationID  string
	CheckNumber    string
	RegisterItemID string
	Request        any
	Result         decision.Result
}

// RecordedDecision is one stored presentment, oldest first when listed.
type RecordedDecision struct {
	ID             string               `json:"id"`
	Decision       string               `json:"decision"`
	Match          *decision.Match      `json:"match"`
	Exceptions     []decision.Exception `json:"exceptions"`
	CorrelationID  string               `json:"correlation_id,omitempty"`
	RegisterItemID string               `json:"register_item_id,omitempty"`
	Request        json.RawMessage      `json:"request"`
	DecidedAt      time.Time            `json:"decided_at"`
	LatencyMs      int64                `json:"latency_ms"`
}

// CheckEvent is one row on the check's audit stream.
type CheckEvent struct {
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	RecordedAt     time.Time       `json:"recorded_at"`
	CorrelationID  string          `json:"correlation_id,omitempty"`
	RegisterItemID string          `json:"register_item_id,omitempty"`
	Request        json.RawMessage `json:"request"`
	Presentment    *Presentment    `json:"presentment,omitempty"`
	Posted         *Posted         `json:"posted,omitempty"`
}

// Presentment is the decide outcome stored on a presentment event.
type Presentment struct {
	Decision   string               `json:"decision"`
	Match      *decision.Match      `json:"match"`
	Exceptions []decision.Exception `json:"exceptions"`
	LatencyMs  int64                `json:"latency_ms"`
}

// Posted is the settlement stored on a posted event.
type Posted struct {
	PaidAt time.Time `json:"paid_at"`
}

// CheckAudit appends presentments and lists one check's history.
type CheckAudit interface {
	WriteCheckDecision(ctx context.Context, entry CheckEntry) (string, error)
	ListCheckDecisions(ctx context.Context, fiID, accountID, checkNumber string) ([]RecordedDecision, error)
	ListCheckAudit(ctx context.Context, fiID, accountID, checkNumber string) ([]CheckEvent, error)
}

// CheckAudit appends decision_audit rows.
type checkAudit struct {
	db *sql.DB
}

func NewCheckAudit(db *sql.DB) CheckAudit {
	return &checkAudit{db: db}
}

// WriteCheckDecision persists one check presentment. partyID/accountID may be empty before the account resolves.
func (s *checkAudit) WriteCheckDecision(ctx context.Context, entry CheckEntry) (string, error) {
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
			fi_id, party_id, account_id, rail, kind, decision,
			match, exceptions, request, correlation_id, latency_ms,
			check_number, register_item_id
		) VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4, $5, $6,
			$7::jsonb, $8::jsonb, $9::jsonb, $10, $11,
			$12, $13::uuid
		)
		RETURNING id::text
	`, entry.FIID, party, account, decision.RailCheck, KindPresentment, entry.Result.Decision,
		matchJSON, excs, reqJSON, corr, entry.Result.LatencyMs,
		entry.CheckNumber, registerItem).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert decision_audit: %w", err)
	}
	return id, nil
}

// ListCheckDecisions returns presentments of one check, oldest first.
func (s *checkAudit) ListCheckDecisions(ctx context.Context, fiID, accountID, checkNumber string) ([]RecordedDecision, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, decision, match, exceptions, request,
		       correlation_id, register_item_id::text, decided_at, latency_ms
		FROM decision_audit
		WHERE fi_id = $1::uuid
		  AND account_id = $2::uuid
		  AND rail = 'check'
		  AND kind = 'presentment'
		  AND check_number = $3
		ORDER BY decided_at ASC, id ASC
	`, fiID, accountID, checkNumber)
	if err != nil {
		return nil, fmt.Errorf("list decision_audit: %w", err)
	}
	defer rows.Close()

	out := []RecordedDecision{}
	for rows.Next() {
		var (
			rec      RecordedDecision
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
			var match decision.Match
			if err := json.Unmarshal(matchRaw, &match); err != nil {
				return nil, fmt.Errorf("unmarshal match: %w", err)
			}
			rec.Match = &match
		}
		rec.Exceptions = []decision.Exception{}
		if len(excRaw) > 0 {
			if err := json.Unmarshal(excRaw, &rec.Exceptions); err != nil {
				return nil, fmt.Errorf("unmarshal exceptions: %w", err)
			}
			if rec.Exceptions == nil {
				rec.Exceptions = []decision.Exception{}
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

// ListCheckAudit returns presentments and the posting for one check, oldest recorded first.
func (s *checkAudit) ListCheckAudit(ctx context.Context, fiID, accountID, checkNumber string) ([]CheckEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, kind, decision, match, exceptions, request,
		       correlation_id, register_item_id::text, decided_at, latency_ms, paid_at
		FROM decision_audit
		WHERE fi_id = $1::uuid
		  AND account_id = $2::uuid
		  AND rail = 'check'
		  AND check_number = $3
		ORDER BY decided_at ASC, id ASC
	`, fiID, accountID, checkNumber)
	if err != nil {
		return nil, fmt.Errorf("list check audit: %w", err)
	}
	defer rows.Close()

	out := []CheckEvent{}
	for rows.Next() {
		var (
			event    CheckEvent
			stored   sql.NullString
			matchRaw []byte
			excRaw   []byte
			reqRaw   []byte
			corr     sql.NullString
			regID    sql.NullString
			latency  int64
			paidAt   sql.NullTime
		)
		if err := rows.Scan(
			&event.ID,
			&event.Kind,
			&stored,
			&matchRaw,
			&excRaw,
			&reqRaw,
			&corr,
			&regID,
			&event.RecordedAt,
			&latency,
			&paidAt,
		); err != nil {
			return nil, fmt.Errorf("scan check audit: %w", err)
		}
		event.RecordedAt = event.RecordedAt.UTC()
		event.Request = reqRaw
		if corr.Valid {
			event.CorrelationID = corr.String
		}
		if regID.Valid {
			event.RegisterItemID = regID.String
		}
		switch event.Kind {
		case KindPosted:
			if paidAt.Valid {
				t := paidAt.Time.UTC()
				event.Posted = &Posted{PaidAt: t}
			}
		default:
			presentment := Presentment{
				Exceptions: []decision.Exception{},
				LatencyMs:  latency,
			}
			if stored.Valid {
				presentment.Decision = stored.String
			}
			if len(matchRaw) > 0 {
				var match decision.Match
				if err := json.Unmarshal(matchRaw, &match); err != nil {
					return nil, fmt.Errorf("unmarshal match: %w", err)
				}
				presentment.Match = &match
			}
			if len(excRaw) > 0 {
				if err := json.Unmarshal(excRaw, &presentment.Exceptions); err != nil {
					return nil, fmt.Errorf("unmarshal exceptions: %w", err)
				}
				if presentment.Exceptions == nil {
					presentment.Exceptions = []decision.Exception{}
				}
			}
			event.Presentment = &presentment
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list check audit: %w", err)
	}
	return out, nil
}

type postedCheck struct {
	FIID          string
	PartyID       string
	AccountID     string
	CheckNumber   string
	CorrelationID string
	Request       any
}

func insertPosted(ctx context.Context, tx *sql.Tx, rec postedCheck, registerItemID string, paidAt time.Time, latencyMs int64) error {
	if latencyMs < 0 {
		latencyMs = 0
	}
	reqJSON := []byte(`{}`)
	if rec.Request != nil {
		b, err := json.Marshal(rec.Request)
		if err != nil {
			return fmt.Errorf("marshal post request: %w", err)
		}
		reqJSON = b
	}
	var corr any
	if rec.CorrelationID != "" {
		corr = rec.CorrelationID
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO decision_audit (
			fi_id, party_id, account_id, rail, kind,
			exceptions, request, correlation_id, latency_ms,
			check_number, register_item_id, paid_at
		) VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4, $5,
			'[]'::jsonb, $6::jsonb, $7, $8,
			$9, $10::uuid, $11
		)
	`, rec.FIID, rec.PartyID, rec.AccountID, decision.RailCheck, KindPosted,
		reqJSON, corr, latencyMs, rec.CheckNumber, registerItemID, paidAt.UTC())
	if err != nil {
		return fmt.Errorf("insert posted audit: %w", err)
	}
	return nil
}
