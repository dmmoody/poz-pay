-- Load the sample registers that ship already on the books.
-- acme-operating.csv is not loaded here. Upload that file (Postman or curl).
-- Requires scripts/seed.sql first, and the CSVs mounted at /registers.

CREATE TEMP TABLE register_csv (
    routing_number text,
    account_number text,
    check_number   text,
    amount         text,
    payee          text,
    issue_date     text,
    void           text
);

INSERT INTO check_register_batches (id, fi_id, party_id, item_count, source)
VALUES
    ('44444444-4444-4444-4444-444444444444',
     '11111111-1111-1111-1111-111111111111',
     'cccccccc-cccc-cccc-cccc-cccccccccccc',
     0, 'csv'),
    ('55555555-5555-5555-5555-555555555555',
     'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
     'dddddddd-dddd-dddd-dddd-dddddddddddd',
     0, 'csv')
ON CONFLICT (id) DO NOTHING;

\copy register_csv FROM '/registers/acme-treasury.csv' WITH (FORMAT csv, HEADER true)

INSERT INTO check_register_items (
    batch_id, fi_id, party_id, account_id,
    check_number, amount, payee, payee_normalized, issue_date, voided_at
)
SELECT
    '44444444-4444-4444-4444-444444444444'::uuid,
    a.fi_id,
    a.party_id,
    a.id,
    c.check_number,
    round(c.amount::numeric * 100)::bigint,
    c.payee,
    upper(regexp_replace(btrim(c.payee), '\s+', ' ', 'g')),
    NULLIF(btrim(c.issue_date), '')::date,
    CASE
        WHEN lower(btrim(c.void)) IN ('true', 'yes', 'y', '1') THEN now()
        ELSE NULL
    END
FROM register_csv AS c
JOIN financial_institutions AS fi ON fi.routing_number = c.routing_number
JOIN accounts AS a
    ON a.fi_id = fi.id
   AND a.external_account_number = c.account_number
WHERE NOT EXISTS (
    SELECT 1
    FROM check_register_items AS i
    WHERE i.fi_id = a.fi_id
      AND i.account_id = a.id
      AND i.check_number = c.check_number
);

DO $$
DECLARE
    n int;
BEGIN
    SELECT count(*) INTO n
    FROM check_register_items
    WHERE batch_id = '44444444-4444-4444-4444-444444444444';
    IF n = 0 THEN
        RAISE EXCEPTION 'acme-treasury.csv matched no accounts';
    END IF;
END $$;

TRUNCATE register_csv;

\copy register_csv FROM '/registers/beta-operating.csv' WITH (FORMAT csv, HEADER true)

INSERT INTO check_register_items (
    batch_id, fi_id, party_id, account_id,
    check_number, amount, payee, payee_normalized, issue_date, voided_at
)
SELECT
    '55555555-5555-5555-5555-555555555555'::uuid,
    a.fi_id,
    a.party_id,
    a.id,
    c.check_number,
    round(c.amount::numeric * 100)::bigint,
    c.payee,
    upper(regexp_replace(btrim(c.payee), '\s+', ' ', 'g')),
    NULLIF(btrim(c.issue_date), '')::date,
    CASE
        WHEN lower(btrim(c.void)) IN ('true', 'yes', 'y', '1') THEN now()
        ELSE NULL
    END
FROM register_csv AS c
JOIN financial_institutions AS fi ON fi.routing_number = c.routing_number
JOIN accounts AS a
    ON a.fi_id = fi.id
   AND a.external_account_number = c.account_number
WHERE NOT EXISTS (
    SELECT 1
    FROM check_register_items AS i
    WHERE i.fi_id = a.fi_id
      AND i.account_id = a.id
      AND i.check_number = c.check_number
);

DO $$
DECLARE
    n int;
BEGIN
    SELECT count(*) INTO n
    FROM check_register_items
    WHERE batch_id = '55555555-5555-5555-5555-555555555555';
    IF n = 0 THEN
        RAISE EXCEPTION 'beta-operating.csv matched no accounts';
    END IF;
END $$;

UPDATE check_register_batches AS b
SET item_count = (
    SELECT count(*) FROM check_register_items AS i WHERE i.batch_id = b.id
)
WHERE b.id IN (
    '44444444-4444-4444-4444-444444444444',
    '55555555-5555-5555-5555-555555555555'
);
