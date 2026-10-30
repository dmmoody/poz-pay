-- CSV check-register intake. The file is stored in chunks, then a durable
-- outbox row wakes a broker consumer. Row ingest commits in batches so a large
-- file does not stay in one transaction. Column mapping is not stored here;
-- the file must already use the canonical header.

CREATE TABLE check_register_batch_sources (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

INSERT INTO check_register_batch_sources (code, label) VALUES
    ('json', 'JSON upload'),
    ('csv', 'CSV upload'),
    ('seed', 'Demo seed');

ALTER TABLE check_register_batches
    ADD CONSTRAINT check_register_batches_source_fk
        FOREIGN KEY (source) REFERENCES check_register_batch_sources (code);

ALTER TABLE financial_institutions
    ADD CONSTRAINT financial_institutions_routing_number_aba
        CHECK (routing_number IS NULL OR routing_number ~ '^[0-9]{9}$');

-- One ABA identifies one institution. Nulls stay allowed for fixtures that
-- predate a routing number; CSV intake refuses those institutions.
CREATE UNIQUE INDEX financial_institutions_routing_number_uq
    ON financial_institutions (routing_number)
    WHERE routing_number IS NOT NULL;

CREATE TABLE check_register_upload_statuses (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

INSERT INTO check_register_upload_statuses (code, label) VALUES
    ('accepted', 'File stored; ingest message not yet delivered'),
    ('ingesting', 'Worker is writing register items'),
    ('completed', 'Ingest finished'),
    ('failed', 'Ingest stopped before the file was fully read');

CREATE TABLE check_register_rejection_codes (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

INSERT INTO check_register_rejection_codes (code, label) VALUES
    ('invalid_row', 'Row shape does not match the canonical header'),
    ('invalid_routing', 'Routing number is not a valid ABA number'),
    ('invalid_account', 'Account number is missing or too long'),
    ('invalid_check_number', 'Check number is missing or too long'),
    ('invalid_amount', 'Amount is not a non-negative decimal amount'),
    ('invalid_payee', 'Payee is missing or too long'),
    ('invalid_date', 'Issue date is not YYYY-MM-DD'),
    ('invalid_void', 'Void flag is not a boolean'),
    ('routing_mismatch', 'Routing number does not match the financial institution'),
    ('unknown_account', 'Account number is not on this customer'),
    ('duplicate_check', 'Active check number is already issued');

CREATE TABLE check_register_uploads (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    fi_id            uuid NOT NULL REFERENCES financial_institutions (id) ON DELETE RESTRICT,
    party_id         uuid NOT NULL,
    routing_number   text NOT NULL,
    filename         text NOT NULL,
    byte_size        bigint NOT NULL,
    sha256           bytea NOT NULL,
    status           text NOT NULL REFERENCES check_register_upload_statuses (code),
    batch_id         uuid REFERENCES check_register_batches (id) ON DELETE RESTRICT,
    accepted_count   integer NOT NULL DEFAULT 0,
    rejected_count   integer NOT NULL DEFAULT 0,
    rows_processed   integer NOT NULL DEFAULT 0,
    failure          text,
    received_at      timestamptz NOT NULL DEFAULT now(),
    completed_at     timestamptz,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT check_register_uploads_party_fi_fk
        FOREIGN KEY (party_id, fi_id) REFERENCES parties (id, fi_id) ON DELETE RESTRICT,
    CONSTRAINT check_register_uploads_routing_aba
        CHECK (routing_number ~ '^[0-9]{9}$'),
    CONSTRAINT check_register_uploads_filename_len
        CHECK (length(trim(filename)) > 0 AND length(filename) <= 255),
    CONSTRAINT check_register_uploads_byte_size_nonneg
        CHECK (byte_size >= 0),
    CONSTRAINT check_register_uploads_sha256_len
        CHECK (octet_length(sha256) = 32),
    CONSTRAINT check_register_uploads_counts_nonneg
        CHECK (accepted_count >= 0 AND rejected_count >= 0 AND rows_processed >= 0),
    CONSTRAINT check_register_uploads_completed_ck
        CHECK ((status = 'completed') = (completed_at IS NOT NULL)),
    CONSTRAINT check_register_uploads_failure_ck
        CHECK ((status = 'failed') = (failure IS NOT NULL))
);

CREATE INDEX check_register_uploads_fi_party_idx
    ON check_register_uploads (fi_id, party_id, received_at DESC);

-- Chunked file bytes. Read sequentially by (upload_id, seq); do not load the
-- object in one value. Object storage can replace this table later.
CREATE TABLE check_register_upload_blobs (
    upload_id   uuid NOT NULL REFERENCES check_register_uploads (id) ON DELETE CASCADE,
    seq         integer NOT NULL CHECK (seq >= 0),
    data        bytea NOT NULL,
    PRIMARY KEY (upload_id, seq)
);

CREATE TABLE check_register_upload_rejections (
    upload_id   uuid NOT NULL REFERENCES check_register_uploads (id) ON DELETE CASCADE,
    row_number  integer NOT NULL CHECK (row_number > 0),
    code        text NOT NULL REFERENCES check_register_rejection_codes (code),
    message     text NOT NULL,
    PRIMARY KEY (upload_id, row_number)
);

-- One unpublished broker message. The upload row is the source of truth;
-- this row exists so a broker outage does not drop an accepted file.
CREATE TABLE message_outbox (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    topic         text NOT NULL,
    payload       jsonb NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    published_at  timestamptz,
    attempts      integer NOT NULL DEFAULT 0,
    last_error    text,
    CONSTRAINT message_outbox_topic_nonempty CHECK (length(trim(topic)) > 0),
    CONSTRAINT message_outbox_attempts_nonneg CHECK (attempts >= 0)
);

CREATE INDEX message_outbox_pending_idx
    ON message_outbox (created_at)
    WHERE published_at IS NULL;
