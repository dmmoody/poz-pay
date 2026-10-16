# ACH positive pay

Controls inbound ACH against whitelist, tiered rules, and approve modes. Learning proposes whitelist entries from human approvals.

## Building blocks

### Approve mode

Stored on FI and optionally overridden on party (`party` wins when set).

| Mode | Undecided after whitelist + rules |
|------|-----------------------------------|
| `approve_all` | `pay` |
| `approve_unknown` | `pay` if the only issue is unknown originator; otherwise `review` |
| `manual_review` | `review` |
| `strict` | `return` |

### Whitelist entry

Active entry is a **full match** only when all present constraints pass:

- `originator_id` equal (required on entry)
- `amount` within `[amount_min, amount_max]` if bounds set
- `sec_code` in `sec_codes` if list non-empty
- `debit_credit` in allowed list if set
- `received_at` inside at least one `time_windows` if any defined
- `account_id` null (all party accounts) or equal to presented account
- `effective_from` / `effective_to` contain decide time

Partial originator hit with failed constraints → accumulate exceptions; no auto-pay.

Statuses: `active` (used on decide), `proposed` (learning), `rejected`.

### Rules

`rule_sets` have `tier` = `fi` | `party`, `fi_id`, optional `party_id`, `enabled`.

Each `rule`:

| Field | Meaning |
|-------|---------|
| `priority` | Lower runs first |
| `predicate` | JSON constraints (all specified must match) |
| `action` | `pay` \| `return` \| `review` \| `continue` |

Predicate keys (all optional):

```json
{
  "originator_ids": ["1234567890"],
  "originator_ids_mode": "any",
  "originator_name_contains": ["PAYROLL"],
  "amount_min": 0,
  "amount_max": 1000000,
  "sec_codes": ["PPD", "CCD"],
  "debit_credit": ["debit"],
  "dow": [1, 2, 3, 4, 5],
  "time_start": "08:00",
  "time_end": "18:00",
  "tz": "America/New_York"
}
```

Empty / omitted predicate matches nothing useful — spike requires at least one constraint or treats as never-match.

Evaluation order: [decision-order.md](decision-order.md).

## Learning (thin)

On `POST …/approvals` with `propose_whitelist: true` and an ACH audit row:

1. Read presented ACH fields from audit snapshot.
2. Insert whitelist row `status=proposed` with:
   - same `originator_id` / name
   - `amount_min` / `amount_max` = amount ± 10% (integer minor units, min ≥ 0)
   - `sec_codes` = `[presented]`
   - `debit_credit` = `[presented]`
   - one time window: same weekday as `received_at`, ±2 hours around received local time in party/FI default tz (spike default `America/New_York`)
3. Human `PATCH` → `active` or `rejected`.

No ML. No auto-activate.

## APIs

See [decision-api.md](decision-api.md) for paths and payloads.
