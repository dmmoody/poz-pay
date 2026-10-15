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
| 6 | decision_audit | reason codes, `decision_audit`, `approvals` |

## Integrity patterns

- Composite `(id, fi_id)` unique on parties/accounts so children can FK both columns and cannot cross FIs.
- Money: `bigint` minor units.
- Lookups + FKs for all stable codes (no free-form decision/exception enums).
- API `decision` / `reason_code` / exception codes are that closed vocabulary; prose fields are not control-plane.
- Check register: unique `(fi_id, account_id, check_number)` where `voided_at IS NULL`.
- Rule sets: at most one enabled FI set per FI; one enabled party set per party.
- `decision_audit` / `approvals`: append-oriented; `ON DELETE RESTRICT` on money-path parents.

## Hot-path indexes

- `check_register_items (fi_id, account_id, check_number)`
- `ach_whitelist_entries (fi_id, party_id, originator_id)` where `status = 'active'`
- `rules (rule_set_id, priority)` where enabled

See also [tenancy.md](tenancy.md), [decision-order.md](decision-order.md).
