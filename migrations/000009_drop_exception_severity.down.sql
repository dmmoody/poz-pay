CREATE TABLE exception_severities (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

INSERT INTO exception_severities (code, label) VALUES
    ('hard', 'Hard (first-fail)'),
    ('soft', 'Soft (accumulate)');

ALTER TABLE exception_codes
    ADD COLUMN severity text REFERENCES exception_severities (code);

UPDATE exception_codes SET severity = 'hard' WHERE code IN (
    'ACCOUNT_NOT_OURS', 'ACCOUNT_INACTIVE',
    'CHECK_NOT_IN_REGISTER', 'CHECK_VOIDED', 'CHECK_ALREADY_PAID',
    'ORIGINATOR_DENIED', 'RULE_RETURN', 'APPROVE_MODE_STRICT'
);
UPDATE exception_codes SET severity = 'soft' WHERE severity IS NULL;

ALTER TABLE exception_codes
    ALTER COLUMN severity SET NOT NULL;
