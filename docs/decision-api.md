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

`decision`, `reason_code`, `exceptions[].code`, and `exceptions[].severity` are **closed codes** (see [exceptions.md](exceptions.md#closed-vocabulary-contract)). Branch on those fields only.

```json
{
  "decision": "return",
  "reason_code": "CHECK_NOT_IN_REGISTER",
  "reason": "Check number not found in register",
  "exceptions": [
    {
      "code": "CHECK_NOT_IN_REGISTER",
      "severity": "hard",
      "detail": "check_number=1001"
    }
  ],
  "correlation_id": "core-txn-abc",
  "latency_ms": 2
}
```

Multi-fail soft example:

```json
{
  "decision": "review",
  "reason_code": "AMOUNT_MISMATCH",
  "reason": "One or more check fields do not match register",
  "exceptions": [
    {"code": "AMOUNT_MISMATCH", "severity": "soft", "detail": "presented=10050 issued=10000"},
    {"code": "PAYEE_MISMATCH", "severity": "soft", "detail": "presented=ACME issued=ACME SUPPLIES LLC"}
  ],
  "correlation_id": "core-txn-abc",
  "latency_ms": 3
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
