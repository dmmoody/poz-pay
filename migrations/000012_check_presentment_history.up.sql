-- Decide stays repeatable. paid_at is the posting fact.
-- Each decide call is its own audit row, keyed by check number so repeats are queryable.

ALTER TABLE decision_audit
    ADD COLUMN check_number text,
    ADD COLUMN register_item_id uuid;

UPDATE decision_audit
SET check_number = NULLIF(btrim(request #>> '{check,check_number}'), '')
WHERE rail = 'check'
  AND check_number IS NULL;

UPDATE decision_audit AS d
SET register_item_id = i.id
FROM check_register_items AS i
WHERE d.rail = 'check'
  AND d.register_item_id IS NULL
  AND (d.match #>> '{facts,register_item_id}') ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
  AND i.id = (d.match #>> '{facts,register_item_id}')::uuid;

ALTER TABLE decision_audit
    ADD CONSTRAINT decision_audit_check_number_when_check
        CHECK (
            rail <> 'check'
            OR (check_number IS NOT NULL AND length(btrim(check_number)) > 0)
        ),
    ADD CONSTRAINT decision_audit_register_item_fk
        FOREIGN KEY (register_item_id) REFERENCES check_register_items (id) ON DELETE RESTRICT;

CREATE INDEX decision_audit_check_presentment_idx
    ON decision_audit (fi_id, account_id, check_number, decided_at, id)
    WHERE rail = 'check';

COMMENT ON COLUMN decision_audit.check_number IS
    'Presented check number. One row per decide call, so repeats of the same number stay in order.';

COMMENT ON COLUMN decision_audit.register_item_id IS
    'Register row resolved for this decide, including mismatch, void, and already-paid. NULL when unresolved.';

ALTER TABLE check_register_items
    ADD COLUMN paid_correlation_id text,
    ADD CONSTRAINT check_register_items_paid_correlation_requires_paid
        CHECK (paid_correlation_id IS NULL OR paid_at IS NOT NULL);

COMMENT ON COLUMN check_register_items.paid_at IS
    'Set when the item is posted. Decide does not write this.';

COMMENT ON COLUMN check_register_items.paid_correlation_id IS
    'correlation_id from the posting that first set paid_at. A repeated post leaves it unchanged.';

UPDATE exception_codes
SET label = 'Check already posted'
WHERE code = 'CHECK_ALREADY_PAID';
