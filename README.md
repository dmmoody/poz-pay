# poz-pay

Positive pay spike: multi-tenant check + ACH decision engine (HTTP JSON).

**Contracts:** see [`docs/`](docs/README.md) (decision API, exceptions, tenancy, check/ACH pipelines).

## Prerequisites

- Docker + Docker Compose (Rancher Desktop / Docker Desktop)
- Go 1.27+
- Make

If `docker` cannot reach the daemon on Rancher Desktop:

```bash
export DOCKER_HOST=unix://$HOME/.rd/docker.sock
# or: docker context use rancher-desktop
```

## Quick start

```bash
cp .env.example .env   # optional
make up                # Postgres + API, migrate, seed
curl -s localhost:8080/healthz
```

Postgres 18 mounts the volume at `/var/lib/postgresql` (not `.../data`).

Host API against Docker Postgres and RabbitMQ:

```bash
make infra migrate
make run
```

## Make targets

| Target | Purpose |
|--------|---------|
| `make up` | Build/start stack (Postgres, RabbitMQ, API), migrate, and seed |
| `make down` | Stop stack (keep DB volume) |
| `make destroy` | Stop stack and delete DB volume |
| `make infra` | Postgres and RabbitMQ for a host API |
| `make db-up` | Postgres only |
| `make migrate` | Apply migrations |
| `make migrate-down` | Roll back one migration |
| `make migrate-version` | Show migration version |
| `make psql` | Shell into `psql` |
| `make seed` | Load demo institutions and the registers that ship already on the books |
| `make run` | Run API on host (needs Postgres and RabbitMQ) |
| `make test` / `make fmt` | Tests / format |

## Layout

- `cmd/api` — HTTP composition root (API, outbox relay, ingest consumer)
- `internal/store/postgres/migrations/` — ordered SQL (golang-migrate via Compose profile `tools`)
- `samples/check-registers/` — CSV files. Seed loads treasury and Beta. Acme Operating is left for upload
- `docker-compose.yml` — Postgres 18 + RabbitMQ + API + migrate tool
