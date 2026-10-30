-- One authorizing artifact on a clean pay. Absent when the decision is a gate or a mismatch.

CREATE TABLE match_codes (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    rail        text REFERENCES rail_types (code),
    created_at  timestamptz NOT NULL DEFAULT now()
);

INSERT INTO match_codes (code, label, rail) VALUES
    ('REGISTER_MATCH', 'Issued check authorized pay', 'check');

ALTER TABLE decision_audit
    ADD COLUMN match jsonb,
    ADD CONSTRAINT decision_audit_match_object
        CHECK (match IS NULL OR jsonb_typeof(match) = 'object');

COMMENT ON COLUMN decision_audit.match IS
    'Authorizing artifact (code + facts). NULL when the decision was not a clean pay';
