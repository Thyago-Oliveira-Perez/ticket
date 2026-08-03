# notification

A fake email/SMS delivery provider written in **Node.js**, part of the ticketing platform's external providers (see the root `README.md`) — it plays the part of a hostile notification service, simulating both the business surface (accounts, messages, delivery lifecycle, signed webhooks, a bounce suppression list) and the unreliability (latency, random failures, rate limiting, duplicate webhooks) of a real one. It's a deliberate stack deviation from the Payment (`nubrank`) and Fraud providers, which are Go.

## Tech stack

- **Node.js** `22`, **TypeScript**
- [Fastify](https://fastify.dev) — HTTP server & hooks
- [Prisma](https://www.prisma.io) — Postgres client & schema migrations
- **`node:test`** — the built-in test runner, no external test framework, mirroring nubrank's use of plain `go test`

## Folder structure

```
notification/
├── src/
│   ├── index.ts                  # entrypoint: loads config, runs migrations, builds the app, starts listening
│   ├── app.ts                    # Fastify instance, error handling, dependency wiring, route registration
│   ├── config.ts                 # env-based config
│   ├── lib/
│   │   ├── prisma.ts             # PrismaClient singleton + the `Db` type (client or transaction client)
│   │   └── errors.ts             # typed errors (ValidationError, NotFoundError, ...) mapped to HTTP status codes
│   ├── plugins/
│   │   ├── auth.ts               # API-key authentication (Fastify `authenticate` decorator)
│   │   ├── chaos.ts              # failure-injection hooks (latency, random errors, rate limiting)
│   │   └── idempotency.ts        # generic Idempotency-Key hook (reserve-then-resolve)
│   └── modules/
│       ├── accounts/             # accounts + API key issuance/verification
│       ├── webhookEndpoints/     # account-registered, signed webhook endpoints
│       ├── events/                # outbox: persists domain events and fans them out to webhook_deliveries
│       ├── webhookDelivery/      # outbound webhook delivery (latency, duplicate delivery, HMAC signing)
│       ├── messages/             # message send + async queued/sent/delivered/bounced/failed lifecycle
│       └── suppressions/         # bounce/complaint suppression list, auto-managed
├── prisma/
│   ├── schema.prisma
│   └── migrations/
├── package.json
├── tsconfig.json
└── README.md
```

Every domain module (`accounts`, `webhookEndpoints`, `messages`, ...) follows the same shape: `repository.ts` (a `Repository` interface + a Prisma-backed implementation), `service.ts` (validation + business logic behind a `Service` interface, throwing typed errors from `lib/errors.ts`), and `routes.ts` (Fastify route registration). Every module also ships a `service.test.ts` (or equivalent) using an in-memory fake repository — no test talks to a real database.

## Getting started

### Prerequisites

- Node.js `22+`
- A running Postgres instance (see `docker-compose.yml` at the repo root, service `notification-db`)

### Run locally

```bash
# from the notification/ directory
npm install
DB_DSN="postgres://notification:secret@localhost:5433/notification?sslmode=disable" npm run dev
```

On boot the service applies any pending Prisma migrations against `DB_DSN`, then starts the HTTP server on **`:3000`** by default (`PORT`).

### Walking through the API

Everything except `POST /accounts` requires an account API key (see [Accounts & authentication](#accounts--authentication) below).

```bash
# 1. Create an account — the only unauthenticated endpoint. The response's
#    api_key is shown once; store it, it's never returned again.
curl -X POST http://localhost:3000/accounts -H "Content-Type: application/json" \
  -d '{"name":"Acme Inc"}'
# -> {"id":"...", "name":"Acme Inc", "status":"active", "api_key":"notif_live_..."}

API_KEY="notif_live_..."  # from the response above
AUTH=(-H "Authorization: Bearer $API_KEY")

# 2. Register a webhook endpoint to receive delivery events
curl -X POST http://localhost:3000/webhook-endpoints "${AUTH[@]}" -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/hook"}'
# -> {"id":"...", "url":"...", "secret":"whsec_...", "active":true} -- secret is shown once

# 3. Send a message (email or sms)
curl -X POST http://localhost:3000/messages "${AUTH[@]}" -H "Content-Type: application/json" \
  -H "Idempotency-Key: retry-key-1" \
  -d '{"channel":"email","to":"buyer@example.com","from":"orders@example.com","subject":"Your ticket","body":"Enjoy the show"}'
# -> 201 with the created message, status "queued" — it transitions to sent, then
#    delivered/bounced/failed in the background; watch the webhook endpoint for events

# 4. Look up a message or list an account's messages
curl "${AUTH[@]}" http://localhost:3000/messages/<message_id>
curl "${AUTH[@]}" http://localhost:3000/messages

# 5. See which addresses have bounced/complained/unsubscribed
curl "${AUTH[@]}" http://localhost:3000/suppressions
```

## API surface

All routes below (except `POST /accounts`) require `Authorization: Bearer <api_key>`.

| Method & path | Description |
| --- | --- |
| `POST /accounts` | Create an account; issues its first API key (shown once) |
| `POST /webhook-endpoints` | Register a URL to receive this account's events; returns a signing secret (shown once) |
| `POST /messages` | Send an email or SMS message |
| `GET /messages` | List the caller's messages |
| `GET /messages/{id}` | Look up a message |
| `GET /suppressions` | List addresses suppressed for the caller (auto-managed, read-only) |

`POST /messages` accepts an optional `Idempotency-Key` header (see [Idempotency](#idempotency)).

## Useful commands

| Command | Description |
| --- | --- |
| `npm run dev` | Run the API locally with hot reload |
| `npm run build` | Type-check and compile to `dist/` |
| `npm start` | Run the compiled build (`dist/index.js`) |
| `npm test` | Run all tests (`node:test`) |
| `npm run prisma:migrate` | Create and apply a new migration in development |
| `npm run prisma:deploy` | Apply pending migrations (what boot does automatically) |
| `npm run prisma:generate` | Regenerate the Prisma client |

## Configuration

Configuration lives in `src/config.ts`, populated from environment variables:

| Setting | Env var | Default | Description |
| --- | --- | --- | --- |
| `addr` | `ADDR` | `0.0.0.0` | Address the HTTP server listens on |
| `port` | `PORT` | `3000` | Port the HTTP server listens on |
| `dbDsn` | `DB_DSN` | `""` | Postgres connection string, e.g. `postgres://user:pass@host:5432/db?sslmode=disable` |
| `chaos.latencyMinMs` | `CHAOS_LATENCY_MIN_MS` | `0` | Lower bound (ms) of injected per-request latency |
| `chaos.latencyMaxMs` | `CHAOS_LATENCY_MAX_MS` | `0` | Upper bound (ms) of injected per-request latency. `0` disables latency injection |
| `chaos.errorRate` | `CHAOS_ERROR_RATE` | `0` | Probability (`0`-`1`) that a request fails with a `500` before reaching its handler. `0` disables it |
| `chaos.rateLimitRps` | `CHAOS_RATE_LIMIT_RPS` | `0` | Requests/sec allowed per client IP. `0` disables rate limiting |
| `chaos.rateLimitBurst` | `CHAOS_RATE_LIMIT_BURST` | RPS rounded up | Token bucket burst size per client IP |
| `webhookChaos.latencyMinMs` | `CHAOS_WEBHOOK_LATENCY_MIN_MS` | `0` | Lower bound (ms) of delay before each webhook delivery attempt |
| `webhookChaos.latencyMaxMs` | `CHAOS_WEBHOOK_LATENCY_MAX_MS` | `0` | Upper bound (ms) of delay before each webhook delivery attempt. `0` disables the delay |
| `webhookChaos.duplicateRate` | `CHAOS_WEBHOOK_DUPLICATE_RATE` | `0` | Probability (`0`-`1`) that a webhook event is delivered a second time with the same id and body. `0` disables it |
| `messageChaos.bounceRate` | `MESSAGE_BOUNCE_RATE` | `0` | Probability (`0`-`1`) that a message ends in a bad outcome (bounced for email, failed for sms) instead of delivered. `0` disables it — every message delivers |
| `messageChaos.sendDelayMinMs`/`MaxMs` | `MESSAGE_SEND_DELAY_MIN_MS` / `MAX_MS` | `0` | Delay (ms) between a message being queued and marked sent |
| `messageChaos.deliverDelayMinMs`/`MaxMs` | `MESSAGE_DELIVER_DELAY_MIN_MS` / `MAX_MS` | `0` | Delay (ms) between a message being sent and its final outcome |

## Failure injection

notification can be configured to misbehave via the `CHAOS_*` env vars above: adding random latency, failing a fraction of requests with `500`s, and rate-limiting per client IP with a `429`. All three are implemented as Fastify `onRequest` hooks in `src/plugins/chaos.ts`, mounted globally, and are no-ops unless configured. Rate limiting runs first (so throttled clients don't pay the injected latency), then latency, then the random failure check — same ordering as nubrank.

Separately from transport-level chaos, `MESSAGE_BOUNCE_RATE` simulates business-level delivery failure: a fraction of messages still get created and queued successfully (`201`), but end up `bounced` (email) or `failed` (sms) instead of `delivered` once their background lifecycle finishes. This isn't a chaos-hook failure — it's the provider's own simulated behavior, exercising the caller's failure-handling path the same way a real ESP would.

## Accounts & authentication

`POST /accounts` is the only endpoint that doesn't require a key — it's how a caller gets its first one. It creates an account and issues an API key (`notif_live_<hex>`), returned **once**, in that response only; only its SHA-256 hash is persisted (`src/modules/accounts/`). Every other endpoint requires `Authorization: Bearer <api_key>`, enforced by the `authenticate` decorator in `src/plugins/auth.ts`: it resolves the account from the key, rejects the request with `401` if it's missing or doesn't match, and makes the account available to handlers via `request.account`. `account_id` is never trusted from a request body — every resource (messages, webhook endpoints, ...) is scoped to the authenticated account, and a lookup for another account's resource returns `404`, not `403` (consistent with not confirming whether it exists at all).

## Idempotency

A `POST /messages` request may carry an `Idempotency-Key` header. It's handled generically by `src/plugins/idempotency.ts`, not by the route itself: the key is scoped per account, and the hook reserves `(account_id, key, request_hash)` in the `idempotency_keys` table (via a unique-constraint insert, so a race between two concurrent requests with the same key is resolved at the database) before running the handler, then a Fastify `onSend` hook stores its response. A retried request with the same key and the same body replays the original response verbatim without re-running the handler; the same key with a *different* body returns `422`; and a key that's still being processed by a concurrent request returns `409`. The request-body hash is computed from a canonicalized (key-sorted) JSON representation, so differing key order in an otherwise-identical body doesn't produce a spurious mismatch.

## Webhooks

`POST /webhook-endpoints` registers a URL to receive an account's events, generating a signing secret (`whsec_<hex>`) returned once. Every domain event (`message.queued`, `message.sent`, `message.delivered`, `message.bounced`, `message.failed`, `message.suppressed`) is:

1. **Persisted as an outbox insert** (`src/modules/events/`) — written to the `events` table in the *same Prisma transaction* as the state change it describes (e.g. the message row's status update), so an event can never be silently lost because the process crashed between the commit and the delivery attempt.
2. **Fanned out** to every active `webhook_endpoints` row for that account, each getting its own `webhook_deliveries` row (`status`/`attempts` tracked independently per endpoint).
3. **Delivered asynchronously** (`src/modules/webhookDelivery/`) after the transaction commits, so the HTTP response isn't delayed waiting on delivery, and signed with the endpoint's secret: `X-Webhook-Signature: t=<unix>,v1=<hex hmac-sha256("<unix>.<body>", secret)>` (Stripe's scheme). Delivery is deliberately unreliable, matching the chaos theme above:
   - **Latency** — each attempt is delayed per `CHAOS_WEBHOOK_LATENCY_*`.
   - **Duplicate delivery** — per `CHAOS_WEBHOOK_DUPLICATE_RATE`, the identical event (same id, same body, freshly signed) may be sent twice, simulating a provider retry. Consumers are expected to dedupe on the event's `id` (also sent as `X-Webhook-Event-Id`).
   - **Out-of-order delivery** — not specially simulated; it falls out naturally from independent per-delivery latency across concurrent messages. Each event carries a monotonically increasing `sequence` number so a consumer can detect reordering.

Delivery failures (network errors, non-2xx responses) are logged and recorded as `failed` on the delivery row, not retried beyond the duplicate-rate mechanism.

## Messages

`POST /messages` (`src/modules/messages/`) creates an email or SMS message and, unless its recipient is suppressed, runs an async lifecycle in the background — mirroring how nubrank's disputes are simulated automatically rather than in response to a caller action:

1. **Suppression check** — if `(channel, to)` is on the account's suppression list, the message is created `suppressed` immediately (`message.suppressed` fires) and the lifecycle stops there.
2. Otherwise the message is created `queued` (`message.queued` fires).
3. After `MESSAGE_SEND_DELAY_*`, it's marked `sent` (`message.sent` fires).
4. After `MESSAGE_DELIVER_DELAY_*`, `MESSAGE_BOUNCE_RATE` decides the outcome: `delivered` (`message.delivered`), or a bad outcome — `bounced` for email (`message.bounced`, and the address is added to the suppression list) or `failed` for sms (`message.failed`, no suppression — a carrier failure doesn't mean the number is bad).

`GET /messages` / `GET /messages/{id}` list or look up messages; an unknown id returns `404`, a malformed id returns `400`.

## Suppressions

`GET /suppressions` (`src/modules/suppressions/`) is read-only — there's no endpoint to manually add or remove a suppression, consistent with how chaos/decline-style behavior works elsewhere in the platform's providers: it happens *to* the account, not in response to a request. Entries are upserted automatically when a message hard-bounces, keyed on `(account_id, channel, address)`; sending to a suppressed address short-circuits future messages straight to `suppressed` without attempting delivery.

## Migrations

Migrations live in `prisma/migrations/`, managed by Prisma Migrate. They're applied automatically at startup via `prisma migrate deploy` (see `src/index.ts`) — no separate migration step needed to run the service. To add a new migration during development, run `npm run prisma:migrate` after editing `prisma/schema.prisma`.
