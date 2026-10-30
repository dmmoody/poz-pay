-- Decide contract is decision + exceptions only; drop primary reason fields.

ALTER TABLE decision_audit
    DROP COLUMN IF EXISTS reason,
    DROP COLUMN IF EXISTS reason_code;

DROP TABLE IF EXISTS reason_codes;
