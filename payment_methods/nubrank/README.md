# nubrank

A payment method service written in **Go**, part of the `payment_methods` module of the ticketing platform.

> ⚠️ **Work in progress.** Boots an HTTP server with a chi router, base middlewares, and a `/payments` route backed by Postgres. Business logic beyond listing payments is still to come.

## Tech stack

- **Go** `1.26`
- [chi/v5](https://github.com/go-chi/chi) — HTTP router & middlewares
- [pgx/v5](https://github.com/jackc/pgx) — Postgres driver & connection pool
- [golang-migrate/v4](https://github.com/golang-migrate/migrate) — schema migrations, embedded into the binary and run automatically on boot

## Folder structure

```
nubrank/
├── cmd/                          # application entrypoint
│   ├── main.go                   # builds config, runs migrations, opens the db pool, starts the server
│   └── api.go                    # router mount, middlewares, HTTP server setup
├── internal/
│   ├── database/
│   │   ├── migrate.go            # runs embedded migrations against Postgres
│   │   └── migrations/           # numbered up/down SQL migration files
│   ├── json/                     # JSON response helper
│   └── payments/
│       ├── handlers.go           # HTTP handlers
│       ├── service.go            # business logic
│       ├── repository.go         # Payment model + Repository interface
│       └── postgres_repository.go # Postgres implementation of Repository
├── go.mod
├── go.sum
└── README.md
```

## Getting started

### Prerequisites

- Go `1.26+` installed ([download](https://go.dev/dl/))
- A running Postgres instance (see `docker-compose.yml` at the repo root, service `postgresql`)

### Run locally

```bash
# from the nubrank/ directory
DB_DSN="postgres://nubrank:secret@localhost:5432/nubrank?sslmode=disable" go run ./cmd
```

On boot the service applies any pending migrations against `DB_DSN`, then starts the HTTP server on **`:8080`** by default:

```bash
curl http://localhost:8080/
# -> hello world

curl http://localhost:8080/payments
# -> [] or a JSON array of payments
```

## Useful commands

| Command | Description |
| --- | --- |
| `go run ./cmd` | Run the API locally |
| `go build -o bin/nubrank ./cmd` | Build a binary into `bin/` |
| `go mod tidy` | Add missing and remove unused modules |
| `go mod download` | Download dependencies into the module cache |
| `go fmt ./...` | Format all Go source files |
| `go vet ./...` | Report suspicious constructs |
| `go test ./...` | Run all tests |
| `go test -race ./...` | Run tests with the race detector |

## Configuration

Configuration currently lives in `cmd/main.go` (`config` / `dbConfig` structs), populated from environment variables:

| Setting | Env var | Default | Description |
| --- | --- | --- | --- |
| `addr` | `ADDR` | `:8080` | Address the HTTP server listens on |
| `db.dsn` | `DB_DSN` | `""` | Postgres connection string, e.g. `postgres://user:pass@host:5432/db?sslmode=disable` |

## Migrations

Migrations live in `internal/database/migrations/` using the `{version}_{name}.{up,down}.sql` naming convention (golang-migrate). They're embedded into the binary via `go:embed` and applied automatically at startup — no separate migration step or CLI needed to run the service.

To add a new migration, create a new `NNNNNN_description.up.sql` / `.down.sql` pair with the next sequential version number.
