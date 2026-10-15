-- Issued check register: upload batch + items. Hot path: (fi_id, account_id, check_number).

CREATE TABLE check_register_batches (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    fi_id       uuid NOT NULL REFERENCES financial_institutions (id) ON DELETE RESTRICT,
    party_id    uuid NOT NULL,
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    item_count  integer NOT NULL DEFAULT 0,
    source      text NOT NULL DEFAULT 'json',
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT check_register_batches_party_fi_fk
        FOREIGN KEY (party_id, fi_id) REFERENCES parties (id, fi_id) ON DELETE RESTRICT,
    CONSTRAINT check_register_batches_item_count_nonneg CHECK (item_count >= 0)
);

CREATE INDEX check_register_batches_fi_party_idx
    ON check_register_batches (fi_id, party_id, uploaded_at DESC);

CREATE TABLE check_register_items (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id        uuid NOT NULL REFERENCES check_register_batches (id) ON DELETE CASCADE,
    fi_id           uuid NOT NULL,
    party_id        uuid NOT NULL,
    account_id      uuid NOT NULL,
    check_number    text NOT NULL,
    amount          bigint NOT NULL,
    payee           text NOT NULL,
    payee_normalized text NOT NULL,
    issue_date      date,
    voided_at       timestamptz,
    paid_at         timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT check_register_items_amount_nonneg CHECK (amount >= 0),
    CONSTRAINT check_register_items_check_number_nonempty CHECK (length(trim(check_number)) > 0),
    CONSTRAINT check_register_items_payee_nonempty CHECK (length(trim(payee)) > 0),
    CONSTRAINT check_register_items_account_fi_fk
        FOREIGN KEY (account_id, fi_id) REFERENCES accounts (id, fi_id) ON DELETE RESTRICT,
    CONSTRAINT check_register_items_party_fi_fk
        FOREIGN KEY (party_id, fi_id) REFERENCES parties (id, fi_id) ON DELETE RESTRICT
);

-- One active (non-void) item per account check number.
CREATE UNIQUE INDEX check_register_items_active_uq
    ON check_register_items (fi_id, account_id, check_number)
    WHERE voided_at IS NULL;

CREATE INDEX check_register_items_lookup_idx
    ON check_register_items (fi_id, account_id, check_number);

CREATE INDEX check_register_items_batch_id_idx
    ON check_register_items (batch_id);
