# poz-pay docs

Spike contracts and domain specs. Implement against these; change the docs when the contract changes.

| Doc | Purpose |
|-----|---------|
| [tenancy.md](tenancy.md) | FI → party → account hierarchy |
| [decision-api.md](decision-api.md) | `POST /v1/decide` + admin/setup HTTP contracts |
| [exceptions.md](exceptions.md) | Decision/reason/exception codes and severity |
| [decision-order.md](decision-order.md) | Check and ACH evaluation pipelines |
| [check-positive-pay.md](check-positive-pay.md) | Register upload and check matching |
| [ach-positive-pay.md](ach-positive-pay.md) | Rules, whitelist, approve modes, learning |

## Conventions

- Money: integer **minor units** (`bigint`), ISO currency on account (default `USD`).
- Time: `timestamptz`, UTC on the wire as RFC 3339.
- IDs: UUID strings unless noted.
- Errors (non-decide): JSON `{"error":{"code":"...","message":"..."}}` with appropriate HTTP status.
- Decide path always returns **200** with a decision body when the request is well-formed and the account is resolvable enough to decide; use **4xx** only for malformed request / unknown FI / validation failure before evaluation.
