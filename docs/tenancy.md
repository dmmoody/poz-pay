# Multi-tenancy

Shared Postgres schema, row-scoped by FI. Designed for thousands of FIs without schema-per-tenant.

## Hierarchy

```text
financial_institution (FI)
  └── party          (business | treasury | consumer | …)
        └── account
```

| Entity | Grain | Notes |
|--------|-------|-------|
| `financial_institutions` | One FI | Tenant root; all rows that matter carry `fi_id` |
| `parties` | One customer/org under an FI | Typed via lookup `party_types` |
| `accounts` | One demand/deposit (or similar) account | Belongs to exactly one party; `fi_id` denormalized for hot-path filters |

## Party types (lookup)

| Code | Meaning |
|------|---------|
| `business` | Commercial customer |
| `treasury` | Treasury / cash-management desk |
| `consumer` | Consumer / retail |
| `other` | Extensible catch-all for spike |

New types are rows in `party_types`, not app-only enums.

## Isolation rules

- Every decision and config read is scoped by `fi_id` first.
- A party or account from FI A must never match under FI B (`ACCOUNT_NOT_OURS` / not found).
- Party-tier config (rules, whitelist, approve mode) applies only to that party’s accounts.
- FI-tier config is the default when party config is absent or does not terminate evaluation.

## Identity on the wire

Decide and admin APIs use:

- `fi_id` (UUID)
- `party_id` (UUID) where the resource is party-scoped
- `account_id` (UUID) on decide (engine resolves party via account)

External core account numbers may be stored as `accounts.external_account_number` (unique per FI) for upload mapping; decide prefers `account_id`.
