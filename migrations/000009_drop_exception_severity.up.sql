-- Severity is pipeline control flow, not a wire/lookup field.

ALTER TABLE exception_codes
    DROP COLUMN IF EXISTS severity;

DROP TABLE IF EXISTS exception_severities;
