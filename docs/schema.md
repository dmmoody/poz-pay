# Schema

Postgres 18, shared multi-tenant schema. Migrations under [`internal/store/postgres/migrations/`](../internal/store/postgres/migrations/).

## Migration order

| Version | Name | Contents |
|---------|------|----------|
| 1 | extensions | `pgcrypto` |
| 2 | lookups | party/rail/decision/exception/approve/rule/whitelist codes |
| 3 | tenants | `financial_institutions`, `parties`, `accounts` |
| 4 | check_register | batches + items (partial unique on active check #) |
| 5 | ach_config | FI/party settings, rule_sets/rules, whitelist |
| 6 | decision_audit | `decision_audit`, `approvals` (initially included reason columns) |
| 7 | check_settings | `fi_check_settings` / `party_check_settings` (`return_on_mismatch`) |
| 8 | drop_reason_codes | Remove `reason` / `reason_code` / `reason_codes` (exceptions-only why) |
| 9 | drop_exception_severity | Remove `exception_severities` and `exception_codes.severity` |
| 10 | check_max_age | `max_age_days` (default 180) + `CHECK_EXPIRED` |
| 11 | decision_match | `match_codes` + nullable `decision_audit.match` |
| 12 | check_presentment_history | `decision_audit.check_number` + `register_item_id`; `paid_correlation_id` on register items |
| 13 | check_audit_kind | `check_audit_kinds`; `decision_audit.kind` + `paid_at` so presentment and posted share one stream |
| 14 | check_register_csv | CSV intake blobs, rejection codes, 9-digit routing number, transactional outbox |

## Integrity patterns

- Composite `(id, fi_id)` unique on parties/accounts so children can FK both columns and cannot cross FIs.
- Money: `bigint` minor units.
- Lookups + FKs for all stable codes (no free-form decision/exception enums).
- API `decision`, `match` codes, and exception codes, plus per-code `facts` keys, are that closed vocabulary.
- Check register: unique `(fi_id, account_id, check_number)` where `voided_at IS NULL`.
- `financial_institutions.routing_number`, when set, is 9 digits and unique. CSV intake copies that stored value and rejects a row that does not match it.
- CSV upload: one `check_register_uploads` row per file. Bytes live in `check_register_upload_blobs` (ordered chunks) so a large file is not one value. `message_outbox` is the wake-up for the broker; the upload row remains the source of truth if the broker is down. Ingest commits in batches and resumes from `rows_processed`. A failed file keeps the rows already accepted.
- Rule sets: at most one enabled FI set per FI; one enabled party set per party.
- `decision_audit` / `approvals`: append-oriented; `ON DELETE RESTRICT` on money-path parents. Check rail rows require `check_number`. One row per presentment or posting (`kind`).
- `check_register_items.paid_at` is the paying system's settlement time. `paid_correlation_id` is set only together with `paid_at`, and only by the first post.

## Hot-path indexes

- `check_register_items (fi_id, account_id, check_number)`
- `check_register_uploads (fi_id, party_id, received_at DESC)`
- `message_outbox (created_at)` where `published_at IS NULL`
- `decision_audit (fi_id, account_id, check_number, decided_at, id)` where `rail = 'check'`
- `ach_whitelist_entries (fi_id, party_id, originator_id)` where `status = 'active'`
- `rules (rule_set_id, priority)` where enabled

See also [tenancy.md](tenancy.md), [decision-order.md](decision-order.md).
