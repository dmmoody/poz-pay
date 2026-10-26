# Decisions and exceptions

## Closed vocabulary contract

Codes are a **closed set**, owned by Postgres lookups and mirrored in the API:

| Wire field | Lookup | Notes |
|------------|--------|-------|
| `decision` | `decision_codes` | `pay` \| `return` \| `review` |
| `match` | `match_codes` when non-null | Always present. `null`, or one object when an artifact authorized `pay` |
| `match.facts` | (per-code keys below) | Structured machine fields; omit when empty |
| `exceptions[].code` | `exception_codes` | Machine exception token |
| `exceptions[].facts` | (per-code keys below) | Structured machine fields; omit when empty |

There is **no** primary `reason_code` / `reason`, **no** `severity`, and **no** freeform `detail` scrape bait. The full *why* is `exceptions[]` (empty on clean pay).

**Normative**

1. Control flow uses `decision`, `exceptions[].code`, and `exceptions[].facts` keys — exact match on tokens / keys. `match` is the authorizing artifact for a person or an audit row. Do not branch the rail on it.
2. Fact **keys and JSON types** are closed per code (below). Adding a key is a docs + engine change.
3. Adding a code is a schema + docs change (seed row in the lookup), not a silent server invent.
4. Persist path: `decision_audit.decision` is FK-enforced; `exceptions` JSONB must only contain codes from `exception_codes` (enforced in app on write for the spike).

## Decision values

| `decision` | Meaning |
|------------|---------|
| `pay` | Allow the item |
| `return` | Decline / return |
| `review` | Hold for human (or core) review |

Overall decision follows the rail pipeline (see [decision-order.md](decision-order.md)). Check summary:

1. Gate failure (account / register / expired) → `return` with that code
2. Else any field mismatches → `review`, or `return` when `return_on_mismatch`
3. Else → `pay` with `exceptions: []` and `match` = `REGISTER_MATCH` (`register_item_id`)

## Exception codes and facts

### Shared

| Code | When | Facts |
|------|------|-------|
| `ACCOUNT_NOT_OURS` | Account missing or `fi_id` mismatch | *(none)* |
| `ACCOUNT_INACTIVE` | Account exists but not enabled for positive pay | *(none)* |

### Check rail

| Code | When | Facts |
|------|------|-------|
| `CHECK_NOT_IN_REGISTER` | No active register item | `check_number` (string) |
| `CHECK_VOIDED` | Register item marked void | `check_number` (string) |
| `CHECK_ALREADY_PAID` | Register item already paid/presented | `check_number` (string) |
| `CHECK_EXPIRED` | Past FI/party `max_age_days` (default 180) | `check_number` (string), `issue_date` (YYYY-MM-DD), `age_days` (int), `max_age_days` (int) |
| `AMOUNT_MISMATCH` | Presented amount ≠ issued amount | `presented_amount` (int, minor units), `issued_amount` (int) |
| `PAYEE_MISMATCH` | Payee does not match (normalized) | `presented_payee` (string), `issued_payee` (string) |
| `ISSUE_DATE_MISMATCH` | Presented vs issued issue date | `presented_issue_date` (YYYY-MM-DD), `issued_issue_date` (YYYY-MM-DD) |

### Match codes

Always one JSON field. `null` on `return` and `review`. An object only when something authorized `pay`.

| Code | When | Facts |
|------|------|-------|
| `REGISTER_MATCH` | Clean check pay | `register_item_id` (string, register row id) |

### ACH rail

| Code | When | Facts |
|------|------|-------|
| `ORIGINATOR_DENIED` | Rule action `return` | *(TBD with ACH)* |
| `ORIGINATOR_UNKNOWN` | Not on whitelist | *(TBD)* |
| `AMOUNT_OUT_OF_BOUNDS` | Outside amount band | *(TBD)* |
| `OUTSIDE_TIME_WINDOW` | Outside allowed window | *(TBD)* |
| `SEC_NOT_ALLOWED` | SEC not permitted | *(TBD)* |
| `DEBIT_CREDIT_MISMATCH` | Wrong debit/credit | *(TBD)* |
| `RULE_RETURN` | Explicit rule `return` | *(TBD)* |
| `RULE_REVIEW` | Explicit rule `review` | *(TBD)* |
| `APPROVE_MODE_REVIEW` | Approve mode `manual_review` | *(TBD)* |
| `APPROVE_MODE_STRICT` | Approve mode `strict` | *(TBD)* |

## Response shape (exception object)

```json
{
  "code": "AMOUNT_MISMATCH",
  "facts": {
    "presented_amount": 10050,
    "issued_amount": 10000
  }
}
```

- `code` — stable machine code (lookup-backed in DB)
- `facts` — closed key/value object for that code; omit when empty
