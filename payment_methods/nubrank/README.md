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
│   ├── chaos/                    # failure-injection middleware (latency, random errors, rate limiting)
│   ├── database/
│   │   ├── migrate.go            # runs embedded migrations against Postgres
│   │   └── migrations/           # numbered up/down SQL migration files
│   ├── json/                     # JSON response helper
│   ├── webhook/                  # outbound webhook delivery (latency, duplicate delivery)
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

curl -X POST http://localhost:8080/payments \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: retry-key-1" \
  -d '{"merchant_id":"...","customer_id":"...","payment_method_id":"...","amount_minor":5000,"currency":"BRL","webhook_url":"https://example.com/hook"}'
# -> 201 with the created payment (status "approved" or "declined"), or 400 on invalid input
# webhook_url is optional; if set, a payment.approved or payment.declined event is POSTed to it asynchronously
# Idempotency-Key is optional; a retried request with the same key (scoped per merchant_id)
# returns the original payment instead of creating a new one, and does not re-send its webhook

curl http://localhost:8080/payments/11111111-1111-1111-1111-111111111111
# -> 200 with the payment, 400 if the id isn't a valid UUID, 404 if no payment has that id
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
| `chaos.LatencyMin` | `CHAOS_LATENCY_MIN_MS` | `0` | Lower bound (ms) of injected per-request latency |
| `chaos.LatencyMax` | `CHAOS_LATENCY_MAX_MS` | `0` | Upper bound (ms) of injected per-request latency. `0` disables latency injection |
| `chaos.ErrorRate` | `CHAOS_ERROR_RATE` | `0` | Probability (`0`-`1`) that a request fails with a `500` before reaching its handler. `0` disables it |
| `chaos.RateLimitRPS` | `CHAOS_RATE_LIMIT_RPS` | `0` | Requests/sec allowed per client IP. `0` disables rate limiting |
| `chaos.RateLimitBurst` | `CHAOS_RATE_LIMIT_BURST` | RPS rounded up | Token bucket burst size per client IP |
| `webhook.LatencyMin` | `CHAOS_WEBHOOK_LATENCY_MIN_MS` | `0` | Lower bound (ms) of delay before each webhook delivery attempt |
| `webhook.LatencyMax` | `CHAOS_WEBHOOK_LATENCY_MAX_MS` | `0` | Upper bound (ms) of delay before each webhook delivery attempt. `0` disables the delay |
| `webhook.DuplicateRate` | `CHAOS_WEBHOOK_DUPLICATE_RATE` | `0` | Probability (`0`-`1`) that a webhook event is delivered a second time with the same id and body. `0` disables it |
| `paymentDecline.Rate` | `PAYMENT_DECLINE_RATE` | `0` | Probability (`0`-`1`) that an otherwise-valid payment is declined instead of approved. `0` disables it — every payment is approved |

## Failure injection

nubrank plays the part of a hostile external payment provider (see the root `README.md`), so it can be configured to misbehave via the `CHAOS_*` env vars above: adding random latency, failing a fraction of requests with `500`s, and rate-limiting per client IP with a `429`. All three are implemented as middleware in `internal/chaos/` and are no-ops unless configured. Rate limiting runs first (so throttled clients don't pay the injected latency), then latency, then the random failure check.

Separately from transport-level chaos, `PAYMENT_DECLINE_RATE` simulates business-level rejection: a fraction of otherwise-valid `POST /payments` requests still return `201`, but with `status: "declined"` and a `decline_reason` (`insufficient_funds`, `card_expired`, `fraud_suspected`, or `issuer_unavailable`, chosen at random) instead of `status: "approved"`. This isn't a chaos-middleware failure — it's nubrank's own decision, exercising the caller's decline-handling path the same way a real gateway would.

## Webhooks

If a `POST /payments` request includes a `webhook_url`, nubrank asynchronously delivers a `payment.approved` or `payment.declined` event to it (matching the created payment's outcome) after the payment is created — the HTTP response isn't delayed waiting on delivery. Delivery lives in `internal/webhook/` and is deliberately unreliable, matching the chaos theme above:

- **Latency** — each delivery attempt is delayed per `CHAOS_WEBHOOK_LATENCY_*`.
- **Duplicate delivery** — per `CHAOS_WEBHOOK_DUPLICATE_RATE`, the identical event (same id, same body) may be sent twice, simulating a provider retry. Consumers are expected to dedupe on the event's `id` (also sent as the `X-Webhook-Event-Id` header).
- **Out-of-order delivery** — not simulated by any special-cased logic; it falls out naturally from independent random latency per delivery across concurrent payments. Each event carries a monotonically increasing `sequence` number so a consumer can detect reordering if it cares to.

Delivery failures (network errors, non-2xx responses) are logged and not retried beyond the duplicate-rate mechanism.

## Idempotency

`POST /payments` accepts an optional `Idempotency-Key` header. A second request with the same key for the same `merchant_id` returns the payment created by the first request instead of creating a duplicate, and does not re-send its webhook — this is what lets a caller safely retry a request that timed out or failed before it saw the response. The key is scoped per merchant, so two merchants may reuse the same key value independently. Uniqueness is enforced at the database with a partial unique index on `(merchant_id, idempotency_key)`; a race between two concurrent requests carrying the same key is resolved there, and the loser looks up and returns the winner's payment rather than erroring.

## Migrations

Migrations live in `internal/database/migrations/` using the `{version}_{name}.{up,down}.sql` naming convention (golang-migrate). They're embedded into the binary via `go:embed` and applied automatically at startup — no separate migration step or CLI needed to run the service.

To add a new migration, create a new `NNNNNN_description.up.sql` / `.down.sql` pair with the next sequential version number.
