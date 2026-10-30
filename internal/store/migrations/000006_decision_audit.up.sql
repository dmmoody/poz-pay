-- Append-only decide outcomes + human approvals (learning source).

CREATE TABLE reason_codes (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

INSERT INTO reason_codes (code, label) VALUES
    ('OK', 'No exceptions; pay');

INSERT INTO reason_codes (code, label)
SELECT code, label FROM exception_codes
ON CONFLICT DO NOTHING;

CREATE TABLE decision_audit (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    fi_id            uuid NOT NULL REFERENCES financial_institutions (id) ON DELETE RESTRICT,
    party_id         uuid,
    account_id       uuid,
    rail             text NOT NULL REFERENCES rail_types (code),
    decision         text NOT NULL REFERENCES decision_codes (code),
    reason_code      text NOT NULL REFERENCES reason_codes (code),
    reason           text NOT NULL,
    exceptions       jsonb NOT NULL DEFAULT '[]'::jsonb,
    request          jsonb NOT NULL,
    correlation_id   text,
    latency_ms       integer NOT NULL,
    decided_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT decision_audit_latency_nonneg CHECK (latency_ms >= 0),
    CONSTRAINT decision_audit_exceptions_array CHECK (jsonb_typeof(exceptions) = 'array'),
    CONSTRAINT decision_audit_request_object CHECK (jsonb_typeof(request) = 'object'),
    CONSTRAINT decision_audit_party_fi_fk
        FOREIGN KEY (party_id, fi_id) REFERENCES parties (id, fi_id) ON DELETE RESTRICT,
    CONSTRAINT decision_audit_account_fi_fk
        FOREIGN KEY (account_id, fi_id) REFERENCES accounts (id, fi_id) ON DELETE RESTRICT
);

CREATE INDEX decision_audit_fi_decided_idx
    ON decision_audit (fi_id, decided_at DESC);

CREATE INDEX decision_audit_correlation_idx
    ON decision_audit (fi_id, correlation_id)
    WHERE correlation_id IS NOT NULL;

CREATE INDEX decision_audit_account_idx
    ON decision_audit (fi_id, account_id, decided_at DESC)
    WHERE account_id IS NOT NULL;

CREATE TABLE approvals (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    fi_id                uuid NOT NULL REFERENCES financial_institutions (id) ON DELETE RESTRICT,
    party_id             uuid NOT NULL,
    decision_audit_id    uuid NOT NULL REFERENCES decision_audit (id) ON DELETE RESTRICT,
    action               text NOT NULL,
    propose_whitelist    boolean NOT NULL DEFAULT false,
    whitelist_entry_id   uuid REFERENCES ach_whitelist_entries (id) ON DELETE SET NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT approvals_action_check CHECK (action IN ('approve', 'reject')),
    CONSTRAINT approvals_party_fi_fk
        FOREIGN KEY (party_id, fi_id) REFERENCES parties (id, fi_id) ON DELETE RESTRICT
);

CREATE INDEX approvals_party_idx ON approvals (fi_id, party_id, created_at DESC);
CREATE INDEX approvals_audit_idx ON approvals (decision_audit_id);
