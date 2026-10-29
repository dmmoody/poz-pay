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

Host API against Docker Postgres only:

```bash
make db-up migrate
make run
```

## Make targets

| Target | Purpose |
|--------|---------|
| `make up` | Build/start stack, migrate, and seed |
| `make down` | Stop stack (keep DB volume) |
| `make destroy` | Stop stack and delete DB volume |
| `make db-up` | Postgres only |
| `make migrate` | Apply migrations |
| `make migrate-down` | Roll back one migration |
| `make migrate-version` | Show migration version |
| `make psql` | Shell into `psql` |
| `make seed` | Run `scripts/seed.sql` |
| `make run` | Run API on host |
| `make test` / `make fmt` | Tests / format |

## Layout

- `cmd/api` — HTTP composition root
- `migrations/` — ordered SQL (golang-migrate via Compose profile `tools`)
- `docker-compose.yml` — Postgres 18 + API + migrate tool
