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

## Integrity patterns

- Composite `(id, fi_id)` unique on parties/accounts so children can FK both columns and cannot cross FIs.
- Money: `bigint` minor units.
- Lookups + FKs for all stable codes (no free-form decision/exception enums).
- API `decision` + exception codes + per-code `facts` keys are that closed vocabulary.
- Check register: unique `(fi_id, account_id, check_number)` where `voided_at IS NULL`.
- Rule sets: at most one enabled FI set per FI; one enabled party set per party.
- `decision_audit` / `approvals`: append-oriented; `ON DELETE RESTRICT` on money-path parents.

## Hot-path indexes

- `check_register_items (fi_id, account_id, check_number)`
- `ach_whitelist_entries (fi_id, party_id, originator_id)` where `status = 'active'`
- `rules (rule_set_id, priority)` where enabled

See also [tenancy.md](tenancy.md), [decision-order.md](decision-order.md).
