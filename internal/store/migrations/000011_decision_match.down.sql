ALTER TABLE decision_audit
    DROP CONSTRAINT IF EXISTS decision_audit_match_object,
    DROP COLUMN IF EXISTS match;

DROP TABLE IF EXISTS match_codes;
