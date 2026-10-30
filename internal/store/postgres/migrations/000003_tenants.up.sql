-- FI → party → account. Composite (id, fi_id) uniques enable cross-table FI integrity.

CREATE TABLE financial_institutions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    routing_number text,
    active      boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT financial_institutions_name_nonempty CHECK (length(trim(name)) > 0)
);

CREATE TABLE parties (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    fi_id           uuid NOT NULL REFERENCES financial_institutions (id) ON DELETE RESTRICT,
    party_type      text NOT NULL REFERENCES party_types (code),
    name            text NOT NULL,
    active          boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT parties_name_nonempty CHECK (length(trim(name)) > 0),
    CONSTRAINT parties_id_fi_uq UNIQUE (id, fi_id)
);

CREATE INDEX parties_fi_id_idx ON parties (fi_id);

CREATE TABLE accounts (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    fi_id                    uuid NOT NULL REFERENCES financial_institutions (id) ON DELETE RESTRICT,
    party_id                 uuid NOT NULL,
    external_account_number  text,
    name                     text NOT NULL,
    currency                 char(3) NOT NULL DEFAULT 'USD',
    active                   boolean NOT NULL DEFAULT true,
    positive_pay_enabled     boolean NOT NULL DEFAULT true,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT accounts_name_nonempty CHECK (length(trim(name)) > 0),
    CONSTRAINT accounts_currency_len CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT accounts_party_fi_fk
        FOREIGN KEY (party_id, fi_id) REFERENCES parties (id, fi_id) ON DELETE RESTRICT,
    CONSTRAINT accounts_id_fi_uq UNIQUE (id, fi_id),
    CONSTRAINT accounts_fi_external_uq UNIQUE (fi_id, external_account_number)
);

CREATE INDEX accounts_fi_party_idx ON accounts (fi_id, party_id);
CREATE INDEX accounts_party_id_idx ON accounts (party_id);
