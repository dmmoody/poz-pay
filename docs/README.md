# poz-pay docs

Spike contracts and domain specs. Implement against these; change the docs when the contract changes.

| Doc | Purpose |
|-----|---------|
| [tenancy.md](tenancy.md) | FI → party → account hierarchy |
| [decision-api.md](decision-api.md) | `POST /v1/decide` + admin/setup HTTP contracts |
| [exceptions.md](exceptions.md) | Decision and exception codes |
| [decision-order.md](decision-order.md) | Check and ACH evaluation pipelines |
| [check-positive-pay.md](check-positive-pay.md) | Register upload and check matching |
| [ach-positive-pay.md](ach-positive-pay.md) | Rules, whitelist, approve modes, learning |
| [schema.md](schema.md) | Migration map, integrity patterns, hot-path indexes |

## Conventions

- Money: integer **minor units** (`bigint`), ISO currency on account (default `USD`).
- Time: `timestamptz`, UTC on the wire as RFC 3339.
- IDs: UUID strings unless noted.
- Errors (non-decide): JSON `{"error":{"code":"...","message":"..."}}` with appropriate HTTP status.
- Decide path always returns **200** with a decision body when the request is well-formed and the account is resolvable enough to decide; use **4xx** only for malformed request / unknown FI / validation failure before evaluation.

### Closed vocabulary (normative)

Decision-engine results use **stable opaque codes** from DB lookup tables (`decision_codes`, `exception_codes`, `approve_modes`, …). See [exceptions.md](exceptions.md#closed-vocabulary-contract).

- Clients and cores **MUST** branch only on `decision` and `exceptions[].code`.
- Empty `exceptions` with `decision=pay` means clean allow.
- `exceptions[].detail` is **display/debug only** — never for control flow.
- Servers **MUST NOT** invent ad-hoc codes; new codes require a lookup migration.
- Go (and any client) should use typed constants / generated enums matching those tables — not free-form string compares on prose.
