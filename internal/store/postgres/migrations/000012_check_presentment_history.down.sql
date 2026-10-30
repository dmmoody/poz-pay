UPDATE exception_codes
SET label = 'Check already paid/presented'
WHERE code = 'CHECK_ALREADY_PAID';

ALTER TABLE check_register_items
    DROP CONSTRAINT IF EXISTS check_register_items_paid_correlation_requires_paid,
    DROP COLUMN IF EXISTS paid_correlation_id;

DROP INDEX IF EXISTS decision_audit_check_presentment_idx;

ALTER TABLE decision_audit
    DROP CONSTRAINT IF EXISTS decision_audit_register_item_fk,
    DROP CONSTRAINT IF EXISTS decision_audit_check_number_when_check,
    DROP COLUMN IF EXISTS register_item_id,
    DROP COLUMN IF EXISTS check_number;
