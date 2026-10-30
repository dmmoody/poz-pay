ALTER TABLE decision_audit
    DROP CONSTRAINT IF EXISTS decision_audit_posted_shape,
    DROP CONSTRAINT IF EXISTS decision_audit_presentment_shape;

DELETE FROM decision_audit WHERE kind = 'posted';

ALTER TABLE decision_audit
    ALTER COLUMN decision SET NOT NULL,
    DROP COLUMN IF EXISTS paid_at,
    DROP COLUMN IF EXISTS kind;

DROP TABLE IF EXISTS check_audit_kinds;
