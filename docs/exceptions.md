# Decisions, reasons, and exceptions

## Decision values

| `decision` | Meaning |
|------------|---------|
| `pay` | Allow the item |
| `return` | Hard decline / return |
| `review` | Hold for human (or core) review |

Overall decision is derived from exceptions + rail policy (see [decision-order.md](decision-order.md)). Rough rule:

1. Any **hard** exception that maps to return → `return`
2. Else any **soft** exception under review policy → `review`
3. Else → `pay`

`reason_code` / `reason` describe the **primary** outcome (first hard, else highest-severity soft, else `OK`).

## Exception severity

| `severity` | Behavior |
|------------|----------|
| `hard` | First-fail: stop further matching for that rail stage; still return this exception in `exceptions[]` |
| `soft` | Accumulate: continue evaluating; all soft failures are returned |

## Exception codes

### Shared

| Code | Severity | When |
|------|----------|------|
| `ACCOUNT_NOT_OURS` | hard | Account missing or `fi_id` mismatch |
| `ACCOUNT_INACTIVE` | hard | Account exists but not enabled for positive pay |
| `OK` | — | Reason only; not used as an exception entry |

### Check rail

| Code | Severity | When |
|------|----------|------|
| `CHECK_NOT_IN_REGISTER` | hard | No active register item for `(fi_id, account_id, check_number)` |
| `CHECK_VOIDED` | hard | Register item marked void |
| `CHECK_ALREADY_PAID` | hard | Register item already marked paid/presented (spike: optional flag) |
| `AMOUNT_MISMATCH` | soft | Presented amount ≠ issued amount |
| `PAYEE_MISMATCH` | soft | Payee does not match (normalized compare) |
| `ISSUE_DATE_MISMATCH` | soft | Issue/effective date outside allowed tolerance (if configured) |

### ACH rail

| Code | Severity | When |
|------|----------|------|
| `ORIGINATOR_DENIED` | hard | Rule action `return` for this originator / predicate |
| `ORIGINATOR_UNKNOWN` | soft | Not on whitelist and no terminating allow rule |
| `AMOUNT_OUT_OF_BOUNDS` | soft | Outside whitelist or rule amount band |
| `OUTSIDE_TIME_WINDOW` | soft | Outside allowed window |
| `SEC_NOT_ALLOWED` | soft | SEC code not permitted by matching rule/whitelist |
| `DEBIT_CREDIT_MISMATCH` | soft | Wrong debit/credit vs allowed |
| `RULE_RETURN` | hard | Explicit rule action `return` |
| `RULE_REVIEW` | soft | Explicit rule action `review` |
| `APPROVE_MODE_REVIEW` | soft | Fell through to approve mode `manual_review` |
| `APPROVE_MODE_STRICT` | hard | Fell through to approve mode `strict` |

## Response shape (exception object)

```json
{
  "code": "AMOUNT_MISMATCH",
  "severity": "soft",
  "detail": "presented=10050 issued=10000"
}
```

- `code` — stable machine code (lookup-backed in DB)
- `severity` — `hard` | `soft`
- `detail` — short human/debug string; no secrets; safe to log
