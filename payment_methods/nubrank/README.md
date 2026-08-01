# nubrank

A fake payment gateway written in **Go**, part of the `payment_methods` module of the ticketing platform — it plays the part of a hostile external payment provider (see the root `README.md`), simulating both the business surface (merchants, customers, tokenized cards, payments, refunds, payouts, disputes, a double-entry ledger) and the unreliability (latency, random failures, rate limiting, duplicate/signed webhooks) of a real one.

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
│   └── api.go                    # router mount, middlewares, dependency wiring, HTTP server setup
├── internal/
│   ├── auth/                     # API-key authentication middleware
│   ├── chaos/                    # failure-injection middleware (latency, random errors, rate limiting)
│   ├── database/
│   │   ├── migrate.go            # runs embedded migrations against Postgres
│   │   ├── tx.go                 # Querier/TxRunner: lets a repository run against the pool or a shared transaction
│   │   └── migrations/           # numbered up/down SQL migration files
│   ├── json/                     # JSON response helper
│   ├── merchants/                # merchant accounts + API key issuance/verification
│   ├── customers/                # customers (scoped to a merchant)
│   ├── paymentmethods/           # tokenized fake cards (scoped to a customer)
│   ├── idempotency/               # generic Idempotency-Key middleware (reserve-then-resolve)
│   ├── webhookendpoints/         # merchant-registered, signed webhook endpoints
│   ├── events/                   # outbox: persists domain events and fans them out to webhook_deliveries
│   ├── webhook/                  # outbound webhook delivery (latency, duplicate delivery, HMAC signing)
│   ├── ledger/                   # double-entry ledger (accounts, transactions, entries)
│   ├── payments/                 # payment creation, decline simulation, lookups
│   ├── refunds/                  # full/partial refunds against a payment
│   ├── payouts/                  # merchant payouts against the ledger balance
│   └── disputes/                 # simulated chargebacks (auto opened, auto resolved)
├── go.mod
├── go.sum
└── README.md
```

Every domain package (`payments`, `refunds`, `payouts`, `disputes`, `ledger`, `events`, ...) follows the same shape: `repository.go` (model + `Repository` interface), `postgres_repository.go` (Postgres implementation), `service.go` (validation + business logic behind a `Service` interface, dispatched to HTTP status codes via sentinel errors and `errors.Is`), and `handlers.go` (chi HTTP handlers).

## Getting started

### Prerequisites

- Go `1.26+` installed ([download](https://go.dev/dl/))
- A running Postgres instance (see `docker-compose.yml` at the repo root, service `postgresql`)

### Run locally

```bash
# from the nubrank/ directory
DB_DSN="postgres://nubrank:secret@localhost:5432/nubrank?sslmode=disable" go run ./cmd
```

On boot the service applies any pending migrations against `DB_DSN`, then starts the HTTP server on **`:8080`** by default.

### Walking through the API

Everything except `POST /merchants` requires a merchant API key (see [Merchants & authentication](#merchants--authentication) below).

```bash
# 1. Create a merchant — the only unauthenticated endpoint. The response's
#    api_key is shown once; store it, it's never returned again.
curl -X POST http://localhost:8080/merchants -H "Content-Type: application/json" \
  -d '{"name":"Acme Inc"}'
# -> {"id":"...", "name":"Acme Inc", "status":"active", "api_key":"nb_live_..."}

API_KEY="nb_live_..."  # from the response above
AUTH=(-H "Authorization: Bearer $API_KEY")

# 2. Create a customer
curl -X POST http://localhost:8080/customers "${AUTH[@]}" -H "Content-Type: application/json" \
  -d '{"email":"buyer@example.com"}'
# -> {"id":"<customer_id>", "merchant_id":"...", "email":"buyer@example.com", ...}

# 3. Tokenize a fake card for that customer
curl -X POST http://localhost:8080/customers/<customer_id>/payment-methods "${AUTH[@]}" \
  -H "Content-Type: application/json" -d '{"number":"4111111111111111","expire_year":2030}'
# -> {"id":"<payment_method_id>", "brand":"visa", "last4":"1111", "token":"pm_tok_...", ...}

# 4. Create a payment
curl -X POST http://localhost:8080/payments "${AUTH[@]}" -H "Content-Type: application/json" \
  -H "Idempotency-Key: retry-key-1" \
  -d '{"customer_id":"<customer_id>","payment_method_id":"<payment_method_id>","amount_minor":5000,"currency":"BRL"}'
# -> 201 with the created payment (status "approved" or "declined"), or 400/404 on invalid input

# 5. Refund it (fully or partially)
curl -X POST http://localhost:8080/payments/<payment_id>/refunds "${AUTH[@]}" \
  -H "Content-Type: application/json" -d '{"amount_minor":2000}'
# -> 201 with the refund; amount_minor is optional and defaults to the full remaining amount

# 6. Pay out the merchant's available ledger balance
curl -X POST http://localhost:8080/payouts "${AUTH[@]}" -H "Content-Type: application/json" \
  -d '{"currency":"BRL"}'
# -> 201 with the payout, or 409 if the amount requested exceeds the available balance

# 7. Register a webhook endpoint to receive events
curl -X POST http://localhost:8080/webhook-endpoints "${AUTH[@]}" -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/hook"}'
# -> {"id":"...", "url":"...", "secret":"whsec_...", "active":true}  -- secret is shown once
```

## API surface

All routes below (except `POST /merchants`) require `Authorization: Bearer <api_key>`.

| Method & path | Description |
| --- | --- |
| `POST /merchants` | Create a merchant; issues its first API key (shown once) |
| `POST /customers` | Create a customer scoped to the caller's merchant |
| `GET /customers/{id}` | Look up a customer |
| `POST /customers/{id}/payment-methods` | Tokenize a fake card for a customer |
| `POST /webhook-endpoints` | Register a URL to receive this merchant's events; returns a signing secret (shown once) |
| `GET /payments` | List the caller's payments |
| `POST /payments` | Create a payment (approved or declined) |
| `GET /payments/{id}` | Look up a payment |
| `POST /payments/{id}/refunds` | Refund a payment, fully or partially |
| `GET /payments/{id}/refunds` | List refunds recorded against a payment |
| `GET /payments/{id}/disputes` | List simulated disputes against a payment (read-only — see [Disputes](#disputes)) |
| `POST /payouts` | Pay out (some or all of) the merchant's ledger balance |
| `GET /payouts` | List the caller's payouts |
| `GET /payouts/{id}` | Look up a payout |

`POST /payments`, `POST /customers`, `POST /customers/{id}/payment-methods`, and `POST /payouts` all accept an optional `Idempotency-Key` header (see [Idempotency](#idempotency)).

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

Configuration lives in `cmd/main.go` (`config` / `dbConfig` structs), populated from environment variables:

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
| `paymentDispute.Rate` | `PAYMENT_DISPUTE_RATE` | `0` | Probability (`0`-`1`) that an approved payment is disputed at some point after creation. `0` disables it |
| `paymentDispute.WinRate` | `PAYMENT_DISPUTE_WIN_RATE` | `0.5` | Probability (`0`-`1`) that a dispute resolves in the merchant's favor (held funds released) |
| `paymentDispute.OpenDelayMin/Max` | `CHAOS_DISPUTE_OPEN_DELAY_MIN_MS` / `MAX_MS` | `0` | Delay (ms) between payment creation and a dispute opening |
| `paymentDispute.ResolveDelayMin/Max` | `CHAOS_DISPUTE_RESOLVE_DELAY_MIN_MS` / `MAX_MS` | `0` | Delay (ms) between a dispute opening and it auto-resolving |

## Failure injection

nubrank can be configured to misbehave via the `CHAOS_*` env vars above: adding random latency, failing a fraction of requests with `500`s, and rate-limiting per client IP with a `429`. All three are implemented as middleware in `internal/chaos/`, mounted globally, and are no-ops unless configured. Rate limiting runs first (so throttled clients don't pay the injected latency), then latency, then the random failure check.

Separately from transport-level chaos, `PAYMENT_DECLINE_RATE` simulates business-level rejection: a fraction of otherwise-valid `POST /payments` requests still return `201`, but with `status: "declined"` and a `decline_reason` (`insufficient_funds`, `card_expired`, `fraud_suspected`, or `issuer_unavailable`, chosen at random) instead of `status: "approved"`. This isn't a chaos-middleware failure — it's nubrank's own decision, exercising the caller's decline-handling path the same way a real gateway would.

## Merchants & authentication

`POST /merchants` is the only endpoint that doesn't require a key — it's how a caller gets its first one. It creates a merchant and issues an API key (`nb_live_<hex>`), returned **once**, in that response only; only its SHA-256 hash is persisted (`internal/merchants/`). Every other endpoint requires `Authorization: Bearer <api_key>`, enforced by `internal/auth/` middleware: it resolves the merchant from the key, rejects the request with `401` if it's missing or doesn't match, and makes the merchant id available to handlers via context. `merchant_id` is never trusted from a request body — every resource (customers, payments, payouts, ...) is scoped to the authenticated merchant, and a lookup for another merchant's resource returns `404`, not `403` (consistent with not confirming whether it exists at all).

## Customers & payment methods

`POST /customers` and `POST /customers/{id}/payment-methods` (`internal/customers/`, `internal/paymentmethods/`) model the tokenization step of a real gateway: a customer holds zero or more tokenized cards, and `POST /payments` requires `customer_id` and `payment_method_id` to already exist and belong to each other (and to the authenticated merchant) — an unknown or mismatched id is a `404`. Tokenizing a card doesn't trust the caller's claimed brand: `brand` is derived from the card number's leading digit (visa/mastercard/amex/unknown) the way a real vault would derive it from the BIN, and only `last4` plus an opaque `token` (`pm_tok_<hex>`) are stored/returned — never the full number.

## Idempotency

Any `POST /payments`, `POST /customers`, `POST /customers/{id}/payment-methods`, or `POST /payouts` request may carry an `Idempotency-Key` header. It's handled generically by middleware in `internal/idempotency/`, not by each endpoint: the key is scoped per merchant, and the middleware reserves `(merchant_id, key, request_hash)` in the `idempotency_keys` table before running the handler, then stores its response. A retried request with the same key and the same body replays the original response verbatim without re-running the handler; the same key with a *different* body returns `422`; and a key that's still being processed by a concurrent request returns `409`. Reservation is done via `INSERT ... ON CONFLICT DO NOTHING`, so a race between two concurrent requests with the same key is resolved at the database, not in application code.

## Webhooks

`POST /webhook-endpoints` registers a URL to receive a merchant's events, generating a signing secret (`whsec_<hex>`) returned once. Every domain event (`payment.approved`, `payment.declined`, `payment.refunded`, `payout.paid`, `dispute.created`, `dispute.won`, `dispute.lost`, ...) is:

1. **Persisted as an outbox insert** (`internal/events/`) — written to the `events` table in the *same database transaction* as the state change it describes (e.g. the payment row's insert), so an event can never be silently lost because the process crashed between the commit and the delivery attempt. This mirrors the outbox pattern the root README describes for the platform's own .NET/RabbitMQ side.
2. **Fanned out** to every active `webhooks_endpoints` row for that merchant, each getting its own `webhook_deliveries` row (`status`/`attempts` tracked independently per endpoint).
3. **Delivered asynchronously** (`internal/webhook/`) after the transaction commits, so the HTTP response isn't delayed waiting on delivery, and signed with the endpoint's secret: `X-Webhook-Signature: t=<unix>,v1=<hex hmac-sha256("<unix>.<body>", secret)>` (Stripe's scheme). Delivery is deliberately unreliable, matching the chaos theme above:
   - **Latency** — each attempt is delayed per `CHAOS_WEBHOOK_LATENCY_*`.
   - **Duplicate delivery** — per `CHAOS_WEBHOOK_DUPLICATE_RATE`, the identical event (same id, same body, freshly signed) may be sent twice, simulating a provider retry. Consumers are expected to dedupe on the event's `id` (also sent as `X-Webhook-Event-Id`).
   - **Out-of-order delivery** — not specially simulated; it falls out naturally from independent per-delivery latency across concurrent payments. Each event carries a monotonically increasing `sequence` number so a consumer can detect reordering.

Delivery failures (network errors, non-2xx responses) are logged and recorded as `failed` on the delivery row, not retried beyond the duplicate-rate mechanism.

## Refunds

`POST /payments/{id}/refunds` (`internal/refunds/`) records a refund — full or partial — against an `approved` or `partially_refunded` payment. `amount_minor` is optional and defaults to the payment's full remaining refundable amount (its original amount minus whatever's already been refunded). A payment's status becomes `partially_refunded` until the sum of its refunds reaches the original amount, at which point it becomes `refunded`.

- Requesting more than the remaining refundable amount returns `409`.
- Refunding a `declined` payment, or one already fully `refunded`, returns `409`.
- An unknown payment id returns `404`.

The check-then-insert is race-safe: the payment row is locked (`SELECT ... FOR UPDATE`) inside the same transaction as the refund insert and the payment's status update, so two concurrent refund requests against the same payment can't together exceed its amount. `GET /payments/{id}/refunds` lists everything recorded against a payment.

## Ledger

Every approved payment and every refund posts a balanced double-entry transaction to `internal/ledger/`: a merchant account (one per merchant, created lazily) and a single platform "clearing" account (a seeded singleton, id `00000000-0000-0000-0000-000000000000`) that serves as the universal counterparty. A charge credits the merchant and debits clearing; a refund reverses it; a dispute hold debits the merchant and credits clearing until it resolves. Every `ledger_entries` row pair sums to zero, so the ledger is auditable, not just a running counter — `ledger_transactions` records what caused each posting (`reference_type`/`reference_id`/`kind`), and both account legs are row-locked (clearing always first, to make lock ordering deadlock-free) within the same transaction as the state change that triggered them.

## Payouts

`POST /payouts` pays out some or all of a merchant's available ledger balance (`internal/payouts/`). `amount_minor` is optional and defaults to the full balance. The merchant's ledger account is locked and its balance checked before the payout is recorded and the ledger is debited, all in one transaction — requesting more than the available balance returns `409`. `GET /payouts` / `GET /payouts/{id}` list or look up payouts.

## Disputes

Disputes are fully simulated (`internal/disputes/`), not caller-triggered — consistent with how decline/latency/duplicate-webhooks already work, chaos happens *to* the merchant here, not in response to a request. After an approved payment is created, `PAYMENT_DISPUTE_RATE` decides (independently, per payment) whether it'll be disputed; if so, a background process waits `CHAOS_DISPUTE_OPEN_DELAY_*`, then opens the dispute — holding the disputed amount (a ledger debit) and publishing `dispute.created` — waits `CHAOS_DISPUTE_RESOLVE_DELAY_*` more, then resolves it won (`PAYMENT_DISPUTE_WIN_RATE`, releasing the held funds back and publishing `dispute.won`) or lost (funds stay held, `dispute.lost`). `GET /payments/{id}/disputes` is read-only — there's no endpoint to manually open or resolve one.

## Migrations

Migrations live in `internal/database/migrations/` using the `{version}_{name}.{up,down}.sql` naming convention (golang-migrate). They're embedded into the binary via `go:embed` and applied automatically at startup — no separate migration step or CLI needed to run the service.

To add a new migration, create a new `NNNNNN_description.up.sql` / `.down.sql` pair with the next sequential version number.
