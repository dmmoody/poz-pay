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

Records that the paying system posted the item. Stores that system's `paid_at` once.

```json
{
  "check_number": "1005",
  "paid_at": "2026-10-26T17:44:00Z",
  "correlation_id": "core-txn-abc"
}
```

| Field | Required | Notes |
|-------|----------|-------|
| `check_number` | yes | Register key |
| `paid_at` | yes | RFC3339 timestamp from the paying system. A repeat does not replace it |
| `correlation_id` | no | Stored on the first post only |

#### Response `201` (first post) / `200` (already posted)

The body is the posting either way. A repeat returns the same `paid_at` and `correlation_id`. Status `201` means this call wrote them. Status `200` means they were already stored.

```json
{
  "register_item_id": "1a1a1a1a-1a1a-1a1a-1a1a-1a1a1a1a1a1a",
  "check_number": "1005",
  "paid_at": "2026-10-26T17:44:00Z",
  "correlation_id": "core-txn-abc"
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

### `GET /v1/fis/{fi_id}/accounts/{account_id}/checks/{check_number}/audit`

One stream for the check, oldest recorded first. `kind` is `presentment` or `posted`. A presentment carries `presentment`. The posting carries `posted.paid_at` from the paying system. `recorded_at` is when this service stored the event. A repeated post does not add another `posted` row.

```json
{
  "check_number": "1005",
  "register_item_id": "1a1a1a1a-1a1a-1a1a-1a1a-1a1a1a1a1a1a",
  "events": [
    {
      "id": "44444444-4444-4444-4444-444444444444",
      "kind": "presentment",
      "recorded_at": "2026-10-26T17:40:00Z",
      "correlation_id": "core-txn-abc",
      "register_item_id": "1a1a1a1a-1a1a-1a1a-1a1a-1a1a1a1a1a1a",
      "request": {},
      "presentment": {
        "decision": "pay",
        "match": {
          "code": "REGISTER_MATCH",
          "facts": {"register_item_id": "1a1a1a1a-1a1a-1a1a-1a1a-1a1a1a1a1a1a"}
        },
        "exceptions": [],
        "latency_ms": 2
      }
    },
    {
      "id": "55555555-5555-5555-5555-555555555555",
      "kind": "posted",
      "recorded_at": "2026-10-26T17:44:01Z",
      "correlation_id": "core-txn-abc",
      "register_item_id": "1a1a1a1a-1a1a-1a1a-1a1a-1a1a1a1a1a1a",
      "request": {},
      "posted": {"paid_at": "2026-10-26T17:44:00Z"}
    }
  ]
}
```

---

## Check register upload

JSON upload writes the batch in the request. CSV upload stores the file and returns immediately; a worker ingests rows after a broker message. Column mapping is not available yet: a CSV must already use the canonical header. A later map should rewrite a customer's columns onto that header before the same pipeline.

### `POST /v1/fis/{fi_id}/parties/{party_id}/check-registers`

`Content-Type: application/json`

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

`format`: `json`. `amount` is minor units.

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

### `POST /v1/fis/{fi_id}/parties/{party_id}/check-registers` (CSV)

`Content-Type: multipart/form-data` with a single file field named `file`.

The customer is the party. The party must belong to the institution. The institution must be active and have its routing number stored. Every data row's `routing_number` must equal that stored number. `account_number` is `accounts.external_account_number` for an account on that customer, not an internal account id.

Canonical header (names required, order may vary; extra columns are rejected until column mapping exists):

```text
routing_number,account_number,check_number,amount,payee,issue_date,void
```

| Column | Required cell | Notes |
|--------|---------------|-------|
| `routing_number` | yes | 9 digits. Must equal the institution's stored routing number |
| `account_number` | yes | External account number on this customer. At most 34 characters |
| `check_number` | yes | Leading zeros preserved. At most 32 characters |
| `amount` | yes | Major units, at most 2 fractional digits (`100`, `100.50`, `$100.50`). Stored as minor units. No thousands separators |
| `payee` | yes | At most 140 characters |
| `issue_date` | no | `YYYY-MM-DD` when present |
| `void` | no | Blank, `true`/`false`, `yes`/`no`, `y`/`n`, `1`/`0` |

The request stores the bytes and an outbox row, then returns. It does not parse every data row. Default max size is 64 MiB (`CHECK_REGISTER_MAX_UPLOAD_BYTES`). A broken file stops ingest at the last committed batch; rows already accepted stay on the register. Upload the corrected file again. Duplicate active check numbers are rejected per row, so the rows that already landed are skipped and the rest can insert.

[`samples/check-registers/acme-operating.csv`](../samples/check-registers/acme-operating.csv) is the file to upload. Seed already loads `acme-treasury.csv` and `beta-operating.csv`.

#### Response `202`

```json
{
  "upload_id": "44444444-4444-4444-4444-444444444444",
  "fi_id": "11111111-1111-1111-1111-111111111111",
  "party_id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
  "routing_number": "011000015",
  "filename": "acme-operating.csv",
  "byte_size": 24811,
  "sha256": "hex",
  "status": "accepted",
  "received_at": "2026-10-29T14:00:00Z"
}
```

`status` moves `accepted` → `ingesting` → `completed` or `failed`. `accepted` means the file is stored and the outbox row exists, including while the broker has not delivered yet.

| Status | When |
|--------|------|
| `400` `VALIDATION_ERROR` | Missing file, or the header is not canonical |
| `404` `FI_NOT_FOUND` / `PARTY_NOT_FOUND` | Unknown institution, or the customer is not an active party of that institution |
| `409` `FI_INACTIVE` | Institution is inactive |
| `413` `PAYLOAD_TOO_LARGE` | File exceeds the limit |
| `422` `ROUTING_NUMBER_REQUIRED` | Institution has no valid ABA routing number |

### `GET /v1/fis/{fi_id}/parties/{party_id}/check-register-uploads/{upload_id}`

Poll one CSV upload. `row` is 1-based among data records (the header is not a row). At most the first 100 rejections are stored; `rejections_truncated` is true when more rows were rejected than were kept.

```json
{
  "upload_id": "44444444-4444-4444-4444-444444444444",
  "fi_id": "11111111-1111-1111-1111-111111111111",
  "party_id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
  "routing_number": "011000015",
  "filename": "acme-operating.csv",
  "byte_size": 24811,
  "sha256": "hex",
  "status": "completed",
  "batch_id": "55555555-5555-5555-5555-555555555555",
  "accepted": 298,
  "rejected": 2,
  "rows_processed": 300,
  "rejections_truncated": false,
  "rejections": [
    {
      "row": 14,
      "code": "unknown_account",
      "message": "account number is not on this customer"
    }
  ],
  "received_at": "2026-10-29T14:00:00Z"
}
```

Rejection `code` values: `invalid_row`, `invalid_routing`, `invalid_account`, `invalid_check_number`, `invalid_amount`, `invalid_payee`, `invalid_date`, `invalid_void`, `routing_mismatch`, `unknown_account`, `duplicate_check`.

`failed` means the worker could not keep splitting the file (for example a broken quote). `failure` explains that stop. `accepted` on that upload is the rows already written.

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
