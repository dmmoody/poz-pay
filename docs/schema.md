# Schema

Postgres 18, shared multi-tenant schema. Migrations under [`migrations/`](../migrations/).

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

## Integrity patterns

- Composite `(id, fi_id)` unique on parties/accounts so children can FK both columns and cannot cross FIs.
- Money: `bigint` minor units.
- Lookups + FKs for all stable codes (no free-form decision/exception enums).
- API `decision`, `match` codes, and exception codes, plus per-code `facts` keys, are that closed vocabulary.
- Check register: unique `(fi_id, account_id, check_number)` where `voided_at IS NULL`.
- Rule sets: at most one enabled FI set per FI; one enabled party set per party.
- `decision_audit` / `approvals`: append-oriented; `ON DELETE RESTRICT` on money-path parents. Check rail rows require `check_number`. One row per presentment or posting (`kind`).
- `check_register_items.paid_at` is the paying system's settlement time. `paid_correlation_id` is set only together with `paid_at`, and only by the first post.

## Hot-path indexes

- `check_register_items (fi_id, account_id, check_number)`
- `decision_audit (fi_id, account_id, check_number, decided_at, id)` where `rail = 'check'`
- `ach_whitelist_entries (fi_id, party_id, originator_id)` where `status = 'active'`
- `rules (rule_set_id, priority)` where enabled

See also [tenancy.md](tenancy.md), [decision-order.md](decision-order.md).
