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
| Unknown check # | — | `CHECK_NOT_IN_REGISTER` hard → `return` |
| Known, voided | void | `CHECK_VOIDED` hard → `return` |
| Amount differs | amount | `AMOUNT_MISMATCH` soft |
| Payee differs | payee | `PAYEE_MISMATCH` soft |
| All match | — | `pay` |

Default for any soft mismatch: `decision=review`. Optional future FI setting `return_on_check_mismatch` may force `return`.

## Upload API

See [decision-api.md](decision-api.md)#check-register-upload.

## Audit

Every decide writes `decision_audit` (append-only) with rail, ids, request snapshot (JSONB), decision, exceptions, `latency_ms`, `correlation_id`.
