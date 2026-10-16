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

ALTER TABLE decision_audit
    ADD COLUMN reason_code text REFERENCES reason_codes (code),
    ADD COLUMN reason text;

-- Restore NOT NULL with placeholder for any existing audit rows.
UPDATE decision_audit SET reason_code = 'OK', reason = 'legacy restore' WHERE reason_code IS NULL;

ALTER TABLE decision_audit
    ALTER COLUMN reason_code SET NOT NULL,
    ALTER COLUMN reason SET NOT NULL;
