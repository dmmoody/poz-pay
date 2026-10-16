-- Demo seed aligned with docs/decision-api.md example UUIDs.
-- Idempotent-ish: fixed UUIDs; re-run after migrate-down/up or on fresh volume.

BEGIN;

INSERT INTO financial_institutions (id, name, routing_number)
VALUES
    ('11111111-1111-1111-1111-111111111111', 'First Demo Bank', '011000015'),
    ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', 'Second Demo CU', '211274450')
ON CONFLICT (id) DO NOTHING;

INSERT INTO parties (id, fi_id, party_type, name)
VALUES
    ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', '11111111-1111-1111-1111-111111111111', 'business', 'Acme Corp'),
    ('cccccccc-cccc-cccc-cccc-cccccccccccc', '11111111-1111-1111-1111-111111111111', 'treasury', 'Acme Treasury'),
    ('dddddddd-dddd-dddd-dddd-dddddddddddd', 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', 'business', 'Beta LLC')
ON CONFLICT (id) DO NOTHING;

INSERT INTO accounts (id, fi_id, party_id, external_account_number, name, active, positive_pay_enabled)
VALUES
    ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111',
     'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', '100200300', 'Acme Operating', true, true),
    ('eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee', '11111111-1111-1111-1111-111111111111',
     'cccccccc-cccc-cccc-cccc-cccccccccccc', '100200301', 'Acme Treasury Sweep', true, true),
    ('ffffffff-ffff-ffff-ffff-ffffffffffff', 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
     'dddddddd-dddd-dddd-dddd-dddddddddddd', '900100', 'Beta Operating', true, true),
    -- Inactive / PP disabled for ACCOUNT_INACTIVE demos
    ('99999999-9999-9999-9999-999999999999', '11111111-1111-1111-1111-111111111111',
     'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', '100200399', 'Acme Closed', false, false)
ON CONFLICT (id) DO NOTHING;

INSERT INTO fi_ach_settings (fi_id, approve_mode)
VALUES
    ('11111111-1111-1111-1111-111111111111', 'manual_review'),
    ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', 'strict')
ON CONFLICT (fi_id) DO NOTHING;

INSERT INTO party_ach_settings (party_id, fi_id, approve_mode)
VALUES
    ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', '11111111-1111-1111-1111-111111111111', 'approve_unknown')
ON CONFLICT (party_id) DO NOTHING;

-- Acme: wash/mismatch → return (strict demo). FI default remains review when no party row.
INSERT INTO fi_check_settings (fi_id, return_on_mismatch)
VALUES
    ('11111111-1111-1111-1111-111111111111', false),
    ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', false)
ON CONFLICT (fi_id) DO NOTHING;

INSERT INTO party_check_settings (party_id, fi_id, return_on_mismatch)
VALUES
    ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', '11111111-1111-1111-1111-111111111111', true)
ON CONFLICT (party_id) DO NOTHING;

INSERT INTO rule_sets (id, fi_id, party_id, tier, name, enabled)
VALUES
    ('12121212-1212-1212-1212-121212121212', '11111111-1111-1111-1111-111111111111',
     NULL, 'fi', 'FI default', true),
    ('13131313-1313-1313-1313-131313131313', '11111111-1111-1111-1111-111111111111',
     'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', 'party', 'Acme party rules', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO rules (id, rule_set_id, priority, name, predicate, action)
VALUES
    ('14141414-1414-1414-1414-141414141414', '12121212-1212-1212-1212-121212121212',
     10, 'deny known bad originator',
     '{"originator_ids":["0000000000"],"originator_ids_mode":"any"}'::jsonb,
     'return'),
    ('15151515-1515-1515-1515-151515151515', '13131313-1313-1313-1313-131313131313',
     10, 'large debit review',
     '{"debit_credit":["debit"],"amount_min":1000000}'::jsonb,
     'review')
ON CONFLICT (id) DO NOTHING;

INSERT INTO ach_whitelist_entries (
    id, fi_id, party_id, originator_id, originator_name,
    amount_min, amount_max, sec_codes, debit_credit, status
)
VALUES
    ('16161616-1616-1616-1616-161616161616',
     '11111111-1111-1111-1111-111111111111',
     'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
     '1234567890', 'PAYROLL CO',
     0, 500000, ARRAY['PPD'], ARRAY['debit'], 'active')
ON CONFLICT (id) DO NOTHING;

INSERT INTO check_register_batches (id, fi_id, party_id, item_count, source)
VALUES
    ('33333333-3333-3333-3333-333333333333',
     '11111111-1111-1111-1111-111111111111',
     'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
     4, 'seed')
ON CONFLICT (id) DO NOTHING;

INSERT INTO check_register_items (
    id, batch_id, fi_id, party_id, account_id,
    check_number, amount, payee, payee_normalized, issue_date, voided_at, paid_at
)
VALUES
    -- Happy-path / wash target
    ('17171717-1717-1717-1717-171717171717',
     '33333333-3333-3333-3333-333333333333',
     '11111111-1111-1111-1111-111111111111',
     'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
     '22222222-2222-2222-2222-222222222222',
     '1001', 10000, 'ACME SUPPLIES LLC', 'ACME SUPPLIES LLC', DATE '2026-10-01', NULL, NULL),
    -- Stop-pay / stolen → CHECK_VOIDED
    ('18181818-1818-1818-1818-181818181818',
     '33333333-3333-3333-3333-333333333333',
     '11111111-1111-1111-1111-111111111111',
     'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
     '22222222-2222-2222-2222-222222222222',
     '1003', 25000, 'VOIDED PAYEE INC', 'VOIDED PAYEE INC', DATE '2026-09-15',
     TIMESTAMPTZ '2026-09-20 12:00:00+00', NULL),
    -- Already cleared → CHECK_ALREADY_PAID
    ('19191919-1919-1919-1919-191919191919',
     '33333333-3333-3333-3333-333333333333',
     '11111111-1111-1111-1111-111111111111',
     'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
     '22222222-2222-2222-2222-222222222222',
     '1004', 7500, 'OFFICE DEPOT', 'OFFICE DEPOT', DATE '2026-09-01',
     NULL, TIMESTAMPTZ '2026-09-05 15:30:00+00'),
    -- Second unpaid item for double-presentment demo (pay once, then already-paid)
    ('1a1a1a1a-1a1a-1a1a-1a1a-1a1a1a1a1a1a',
     '33333333-3333-3333-3333-333333333333',
     '11111111-1111-1111-1111-111111111111',
     'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
     '22222222-2222-2222-2222-222222222222',
     '1005', 12000, 'STAPLES INC', 'STAPLES INC', DATE '2026-10-05', NULL, NULL)
ON CONFLICT (id) DO NOTHING;

COMMIT;
