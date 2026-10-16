# Check positive pay

Issue a register of checks; presented items are matched on decide.

## Register model

- **Batch** (`check_register_batches`): one upload event (who, when, party, FI).
- **Item** (`check_register_items`): one issued check.

Natural uniqueness (active items): `(fi_id, account_id, check_number)` where `voided_at IS NULL`.

| Field | Required | Notes |
|-------|----------|-------|
| `account_id` | yes | Must belong to the party path |
| `check_number` | yes | String; leading zeros preserved |
| `amount` | yes | Minor units |
| `payee` | yes | Stored as uploaded; compare uses normalization |
| `issue_date` | no | `date` |
| `void` | no | If true, item is voided at insert |

## Payee normalization (spike)

1. Trim  
2. Uppercase  
3. Collapse internal whitespace  
4. Exact equality after normalize  

No fuzzy matching in spike.

## Matching rules

See [decision-order.md](decision-order.md) check pipeline and [exceptions.md](exceptions.md).

| Presented | Register | Result |
|-----------|----------|--------|
| Unknown check # | — | `CHECK_NOT_IN_REGISTER` → `return` |
| Known, voided | void | `CHECK_VOIDED` → `return` |
| Past max age | `issue_date` | `CHECK_EXPIRED` → `return` |
| Amount differs | amount | `AMOUNT_MISMATCH` |
| Payee differs | payee | `PAYEE_MISMATCH` |
| All match | — | `pay` |

Default for field mismatch: `decision=review`. Party/FI **`return_on_mismatch`** (see `party_check_settings` / `fi_check_settings`) forces `return` instead — useful for wash demos (Acme party seed is `true`).

### Expiry / stale dated

Age is calendar days from register **`issue_date`** to decide time. FI/party **`max_age_days`** (default **180**, ≈ UCC six months) — institutions commonly set **60** or **90**. No register `issue_date` → expiry check skipped. Exceeded → **`CHECK_EXPIRED`** / `return`.

### Paid presentment

On a successful **`pay`** decide, the engine sets `check_register_items.paid_at`. A second presentment of the same check number returns **`CHECK_ALREADY_PAID`**.

## Demo seed (Acme operating)

| Check # | Scenario |
|---------|----------|
| `1001` | Match → pay; wrong payee/amount → return (party `return_on_mismatch`) |
| `1003` | Voided → `CHECK_VOIDED` |
| `1004` | Already paid → `CHECK_ALREADY_PAID` |
| `1005` | Unpaid; pay once, then already-paid on replay |
| `1006` | Stale (`issue_date` 2025-01-01) → `CHECK_EXPIRED` under default 180 |
| account `99999999-…` | Inactive → `ACCOUNT_INACTIVE` |

## Upload API

See [decision-api.md](decision-api.md)#check-register-upload.

## Audit

Every decide writes `decision_audit` (append-only) with rail, ids, request snapshot (JSONB), decision, exceptions, `latency_ms`, `correlation_id`.
