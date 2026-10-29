-- Check positive-pay policy (FI default; party override when present).

CREATE TABLE fi_check_settings (
    fi_id                 uuid PRIMARY KEY REFERENCES financial_institutions (id) ON DELETE CASCADE,
    return_on_mismatch    boolean NOT NULL DEFAULT false,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE party_check_settings (
    party_id              uuid PRIMARY KEY,
    fi_id                 uuid NOT NULL,
    return_on_mismatch    boolean NOT NULL DEFAULT false,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT party_check_settings_party_fi_fk
        FOREIGN KEY (party_id, fi_id) REFERENCES parties (id, fi_id) ON DELETE CASCADE
);

COMMENT ON COLUMN fi_check_settings.return_on_mismatch IS
    'When true, soft check mismatches (amount/payee/date) decide return instead of review';
COMMENT ON COLUMN party_check_settings.return_on_mismatch IS
    'Party override for return_on_mismatch; when row exists, wins over FI';
