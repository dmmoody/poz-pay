# HTTP API contracts

Base URL: `http://localhost:8080`  
Content-Type: `application/json`  
Auth (spike): none required; optional `X-Api-Key` reserved for later.

## Decide (hot path)

### `POST /v1/decide`

Core / ACH gateway calls this **per transaction**.

#### Request

```json
{
  "rail": "check",
  "fi_id": "11111111-1111-1111-1111-111111111111",
  "account_id": "22222222-2222-2222-2222-222222222222",
  "correlation_id": "core-txn-abc",
  "check": {
    "check_number": "1001",
    "amount": 10050,
    "payee": "ACME SUPPLIES",
    "issue_date": "2026-10-01"
  },
  "ach": null
}
```

ACH example:

```json
{
  "rail": "ach",
  "fi_id": "11111111-1111-1111-1111-111111111111",
  "account_id": "22222222-2222-2222-2222-222222222222",
  "correlation_id": "ach-batch-1-item-9",
  "check": null,
  "ach": {
    "originator_id": "1234567890",
    "originator_name": "PAYROLL CO",
    "amount": 250000,
    "debit_credit": "debit",
    "sec_code": "PPD",
    "effective_date": "2026-10-15",
    "received_at": "2026-10-15T14:30:00Z"
  }
}
```

| Field | Required | Notes |
|-------|----------|-------|
| `rail` | yes | `check` \| `ach` |
| `fi_id` | yes | UUID |
| `account_id` | yes | UUID |
| `correlation_id` | no | Echoed in audit |
| `check` | if rail=check | See check object |
| `ach` | if rail=ach | See ACH object |

`amount` is always minor units (integer).

#### Response `200`

`decision`, `match` (`null` or `match.code` / `match.facts`), `exceptions[].code`, and `exceptions[].facts` keys are **closed** (see [exceptions.md](exceptions.md#closed-vocabulary-contract)). Branch the rail on `decision` and `exceptions` only. `match` is always present: the one artifact that authorized a `pay`, or `null`. There is no `reason_code`, `severity`, confidence score, or freeform `detail`.

```json
{
  "decision": "return",
  "match": null,
  "exceptions": [
    {
      "code": "CHECK_NOT_IN_REGISTER",
      "facts": {"check_number": "1001"}
    }
  ],
  "correlation_id": "core-txn-abc",
  "latency_ms": 2
}
```

Multi-mismatch example:

```json
{
  "decision": "return",
  "match": null,
  "exceptions": [
    {
      "code": "AMOUNT_MISMATCH",
      "facts": {"presented_amount": 10050, "issued_amount": 10000}
    },
    {
      "code": "PAYEE_MISMATCH",
      "facts": {"presented_payee": "ACME", "issued_payee": "ACME SUPPLIES LLC"}
    }
  ],
  "correlation_id": "core-txn-abc",
  "latency_ms": 3
}
```

Clean pay names the register row. Field matches are not listed.

```json
{
  "decision": "pay",
  "match": {
    "code": "REGISTER_MATCH",
    "facts": {"register_item_id": "17171717-1717-1717-1717-171717171717"}
  },
  "exceptions": [],
  "correlation_id": "core-txn-abc",
  "latency_ms": 2
}
```

#### Error `400` / `404`

Malformed body, unknown `rail`, missing rail payload, or unknown `fi_id`:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "check payload required when rail=check"
  }
}
```

Unknown account under a known FI is a **decide outcome** (`ACCOUNT_NOT_OURS`), not 404.

Decide does not post the check. The same request may be sent again; each call appends decision history and, until a posting, returns the same `pay` for a clean match.

### `POST /v1/fis/{fi_id}/accounts/{account_id}/check-postings`

Records that the core posted the item. Sets `paid_at` once.

```json
{
  "check_number": "1005",
  "correlation_id": "core-txn-abc"
}
```

| Field | Required | Notes |
|-------|----------|-------|
| `check_number` | yes | Register key |
| `correlation_id` | no | Stored on the first post only |

#### Response `201` (first post) / `200` (already posted)

`200` returns the original `paid_at` and `correlation_id`. `already_paid` is true. Example below is the first post.

```json
{
  "register_item_id": "1a1a1a1a-1a1a-1a1a-1a1a-1a1a1a1a1a1a",
  "check_number": "1005",
  "paid_at": "2026-10-26T17:44:00Z",
  "correlation_id": "core-txn-abc",
  "already_paid": false
}
```

| Status | When |
|--------|------|
| `404` `FI_NOT_FOUND` | Unknown `fi_id` |
| `404` `ACCOUNT_NOT_FOUND` | Account not under the FI |
| `404` `CHECK_NOT_IN_REGISTER` | No register row |
| `409` `CHECK_VOIDED` | Only a voided row |

### `GET /v1/fis/{fi_id}/accounts/{account_id}/checks/{check_number}/decisions`

Every decide for that check, oldest first, plus the posting when `paid_at` is set. Empty `decisions` is `200` when the account exists and the check has never been presented.

```json
{
  "check_number": "1005",
  "register_item_id": "1a1a1a1a-1a1a-1a1a-1a1a-1a1a1a1a1a1a",
  "paid_at": null,
  "decisions": [
    {
      "id": "44444444-4444-4444-4444-444444444444",
      "decision": "pay",
      "match": {
        "code": "REGISTER_MATCH",
        "facts": {"register_item_id": "1a1a1a1a-1a1a-1a1a-1a1a-1a1a1a1a1a1a"}
      },
      "exceptions": [],
      "correlation_id": "core-txn-abc",
      "register_item_id": "1a1a1a1a-1a1a-1a1a-1a1a-1a1a1a1a1a1a",
      "request": {},
      "decided_at": "2026-10-26T17:40:00Z",
      "latency_ms": 2
    }
  ]
}
```

`request` is the stored decide body for that attempt. The same `correlation_id` can appear on more than one row.

---

## Check register upload

### `POST /v1/fis/{fi_id}/parties/{party_id}/check-registers`

```json
{
  "format": "json",
  "items": [
    {
      "account_id": "22222222-2222-2222-2222-222222222222",
      "check_number": "1001",
      "amount": 10000,
      "payee": "ACME SUPPLIES LLC",
      "issue_date": "2026-10-01",
      "void": false
    }
  ]
}
```

`format`: `json` (spike). CSV may be added later as `multipart/form-data`.

#### Response `201`

```json
{
  "batch_id": "33333333-3333-3333-3333-333333333333",
  "accepted": 1,
  "rejected": 0,
  "errors": []
}
```

Per-item reject (unknown account, duplicate active check number) appears in `errors[]` without failing the whole batch unless `strict: true` (optional request flag; default false).

---

## ACH admin (spike)

### Settings

`GET|PUT /v1/fis/{fi_id}/ach-settings`  
`GET|PUT /v1/fis/{fi_id}/parties/{party_id}/ach-settings`

```json
{
  "approve_mode": "manual_review"
}
```

`approve_mode`: `approve_all` | `approve_unknown` | `manual_review` | `strict`

### Rule sets

`GET /v1/fis/{fi_id}/rule-sets`  
`POST /v1/fis/{fi_id}/rule-sets` — FI tier (`tier: "fi"`)  
`POST /v1/fis/{fi_id}/parties/{party_id}/rule-sets` — party tier  

```json
{
  "name": "default",
  "enabled": true,
  "rules": [
    {
      "priority": 10,
      "name": "deny bad originator",
      "predicate": {
        "originator_ids": ["0000000000"],
        "originator_ids_mode": "any"
      },
      "action": "return"
    },
    {
      "priority": 20,
      "name": "large debit review",
      "predicate": {
        "debit_credit": ["debit"],
        "amount_min": 1000000
      },
      "action": "review"
    }
  ]
}
```

`action`: `pay` | `return` | `review` | `continue`  
Predicate fields are optional; missing field = no constraint. See [ach-positive-pay.md](ach-positive-pay.md).

### Whitelist

`GET|POST /v1/fis/{fi_id}/parties/{party_id}/ach-whitelist`  
`PATCH /v1/fis/{fi_id}/parties/{party_id}/ach-whitelist/{entry_id}`

```json
{
  "originator_id": "1234567890",
  "originator_name": "PAYROLL CO",
  "amount_min": 0,
  "amount_max": 500000,
  "sec_codes": ["PPD"],
  "debit_credit": ["debit"],
  "time_windows": [
    {"dow": [1, 2, 3, 4, 5], "start_time": "08:00", "end_time": "18:00", "tz": "America/New_York"}
  ],
  "account_id": null,
  "effective_from": "2026-01-01T00:00:00Z",
  "effective_to": null,
  "status": "active"
}
```

`status`: `active` | `proposed` | `rejected`

### Approvals + learning

`POST /v1/fis/{fi_id}/parties/{party_id}/approvals`

```json
{
  "decision_audit_id": "44444444-4444-4444-4444-444444444444",
  "action": "approve",
  "propose_whitelist": true
}
```

When `propose_whitelist` is true, engine inserts a `proposed` whitelist entry derived from the audited ACH request (originator, amount ± band, SEC, received time window heuristic).

`PATCH …/ach-whitelist/{id}` with `{"status":"active"}` or `"rejected"` accepts/rejects the proposal.

---

## Tenant bootstrap (spike)

Minimal create endpoints for seeds/demos:

- `POST /v1/fis`
- `POST /v1/fis/{fi_id}/parties`
- `POST /v1/fis/{fi_id}/parties/{party_id}/accounts`

Bodies: `name` (+ `party_type` / `external_account_number` as needed). Full CRUD beyond create is out of spike unless needed for tests.
