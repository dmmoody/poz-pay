-- One append-only audit stream per check. Presentment and posted are kinds of the same row.

CREATE TABLE check_audit_kinds (
    code       text PRIMARY KEY,
    label      text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO check_audit_kinds (code, label) VALUES
    ('presentment', 'Check presented for a decision'),
    ('posted', 'Check posted by the paying system');

ALTER TABLE decision_audit
    ADD COLUMN kind text NOT NULL DEFAULT 'presentment' REFERENCES check_audit_kinds (code),
    ADD COLUMN paid_at timestamptz,
    ALTER COLUMN decision DROP NOT NULL;

ALTER TABLE decision_audit
    ADD CONSTRAINT decision_audit_presentment_shape CHECK (
        kind <> 'presentment'
        OR (decision IS NOT NULL AND paid_at IS NULL)
    ),
    ADD CONSTRAINT decision_audit_posted_shape CHECK (
        kind <> 'posted'
        OR (
            decision IS NULL
            AND match IS NULL
            AND paid_at IS NOT NULL
            AND register_item_id IS NOT NULL
            AND account_id IS NOT NULL
            AND check_number IS NOT NULL
        )
    );

COMMENT ON COLUMN decision_audit.kind IS
    'presentment or posted. Both are events on one check, ordered by decided_at.';

COMMENT ON COLUMN decision_audit.paid_at IS
    'Paying system settlement time. Set only on kind=posted. decided_at is when this service recorded the event.';
