# nubrank

A payment method service written in **Go**, part of the `payment_methods` module of the ticketing platform.

> ⚠️ **Work in progress.** The API is being scaffolded — currently it boots an HTTP server with a chi router, base middlewares and a single health/`hello world` route. Database, handlers and business logic are still to come.

## Tech stack

- **Go** `1.26`
- [chi/v5](https://github.com/go-chi/chi) — HTTP router & middlewares
- [gin-gonic/gin](https://github.com/gin-gonic/gin) — (declared dependency)
- [mongo-driver/v2](https://go.mongodb.org/mongo-driver) — MongoDB driver (planned persistence)

## Folder structure

```
nubrank/
├── cmd/                # application entrypoint
│   ├── main.go         # builds config + application, starts the server
│   └── api.go          # router mount, middlewares, HTTP server setup
├── internal/           # private application code (handlers, store, services) — WIP
├── go.mod
├── go.sum
└── README.md
```

## Getting started

### Prerequisites

- Go `1.26+` installed ([download](https://go.dev/dl/))

### Run locally

```bash
# from the nubrank/ directory
go run ./cmd
```

The server starts on **`:8080`** by default:

```bash
curl http://localhost:8080/
# -> hello world
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

Configuration currently lives in `cmd/main.go` (`config` / `dbConfig` structs):

| Setting | Default | Description |
| --- | --- | --- |
| `addr` | `:8080` | Address the HTTP server listens on |
| `db.dsn` | `""` | Database connection string (not wired up yet) |

> Environment-variable based config is planned.