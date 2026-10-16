# Decisions and exceptions

## Closed vocabulary contract

Codes are a **closed set**, owned by Postgres lookups and mirrored in the API:

| Wire field | Lookup | Notes |
|------------|--------|-------|
| `decision` | `decision_codes` | `pay` \| `return` \| `review` |
| `exceptions[].code` | `exception_codes` | Machine exception token |

There is **no** primary `reason_code` / `reason`, and **no** `severity` on exceptions. The full *why* is `exceptions[]` (empty on clean pay). Pipeline gates vs field matching are engine internals — clients branch on `decision` + codes only.

**Normative**

1. Control flow uses `decision` plus `exceptions[].code` — exact match on the token.
2. `exceptions[].detail` is non-normative human/debug text; wording may change without a version bump.
3. Adding a code is a schema + docs change (seed row in the lookup), not a silent server invent.
4. Persist path: `decision_audit.decision` is FK-enforced; `exceptions` JSONB must only contain codes from `exception_codes` (enforced in app on write for the spike).

Ints / numeric aliases are **out of scope** unless a core integration requires a parallel mapped field later.

## Decision values

| `decision` | Meaning |
|------------|---------|
| `pay` | Allow the item |
| `return` | Decline / return |
| `review` | Hold for human (or core) review |

Overall decision follows the rail pipeline (see [decision-order.md](decision-order.md)). Check summary:

1. Gate failure (account / register / expired) → `return` with that code
2. Else any field mismatches → `review`, or `return` when `return_on_mismatch`
3. Else → `pay` with `exceptions: []`

## Exception codes

### Shared

| Code | When |
|------|------|
| `ACCOUNT_NOT_OURS` | Account missing or `fi_id` mismatch |
| `ACCOUNT_INACTIVE` | Account exists but not enabled for positive pay |

### Check rail

| Code | When |
|------|------|
| `CHECK_NOT_IN_REGISTER` | No active register item for `(fi_id, account_id, check_number)` |
| `CHECK_VOIDED` | Register item marked void |
| `CHECK_ALREADY_PAID` | Register item already marked paid/presented |
| `CHECK_EXPIRED` | Register `issue_date` older than FI/party `max_age_days` (default 180 ≈ UCC 6 months) |
| `AMOUNT_MISMATCH` | Presented amount ≠ issued amount |
| `PAYEE_MISMATCH` | Payee does not match (normalized compare) |
| `ISSUE_DATE_MISMATCH` | Issue/effective date outside allowed tolerance (if configured) |

### ACH rail

| Code | When |
|------|------|
| `ORIGINATOR_DENIED` | Rule action `return` for this originator / predicate |
| `ORIGINATOR_UNKNOWN` | Not on whitelist and no terminating allow rule |
| `AMOUNT_OUT_OF_BOUNDS` | Outside whitelist or rule amount band |
| `OUTSIDE_TIME_WINDOW` | Outside allowed window |
| `SEC_NOT_ALLOWED` | SEC code not permitted by matching rule/whitelist |
| `DEBIT_CREDIT_MISMATCH` | Wrong debit/credit vs allowed |
| `RULE_RETURN` | Explicit rule action `return` |
| `RULE_REVIEW` | Explicit rule action `review` |
| `APPROVE_MODE_REVIEW` | Fell through to approve mode `manual_review` |
| `APPROVE_MODE_STRICT` | Fell through to approve mode `strict` |

## Response shape (exception object)

```json
{
  "code": "AMOUNT_MISMATCH",
  "detail": "presented=10050 issued=10000"
}
```

- `code` — stable machine code (lookup-backed in DB)
- `detail` — short human/debug string; no secrets; safe to log
