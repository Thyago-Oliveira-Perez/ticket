package main

import (
	"log"
	"net/http"
	"nubrank/internal/auth"
	"nubrank/internal/chaos"
	"nubrank/internal/customers"
	"nubrank/internal/database"
	"nubrank/internal/events"
	"nubrank/internal/idempotency"
	"nubrank/internal/ledger"
	"nubrank/internal/merchants"
	"nubrank/internal/paymentmethods"
	"nubrank/internal/payments"
	"nubrank/internal/payouts"
	"nubrank/internal/refunds"
	"nubrank/internal/webhook"
	"nubrank/internal/webhookendpoints"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mount
func (app *application) mount() http.Handler {
	r := chi.NewRouter()

	// middlewares
	r.Use(middleware.RequestID) // important for rate limiting
	r.Use(middleware.RealIP) 		// important for rate limiting, analytics and tracing
	r.Use(middleware.Logger)		// 
	r.Use(middleware.Recoverer) // recover from crashes

	/**
	set a timeout value on the request context (ctx),that will signal
	through ctx.Done() that the request has timed out and further
	processing should be stopped.
	*/
	r.Use(middleware.Timeout(60 * time.Second))

	// chaos: simulate a hostile upstream provider (rate limiting first, so
	// throttled clients don't pay the injected latency, then latency, then
	// a chance of outright failure).
	r.Use(chaos.RateLimit(app.config.chaos))
	r.Use(chaos.Latency(app.config.chaos))
	r.Use(chaos.RandomError(app.config.chaos))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world"))
	})

	merchantRepo := merchants.NewPostgresRepository(app.db)
	merchantService := merchants.NewService(merchantRepo)
	merchantHandler := merchants.NewHandler(merchantService)
	r.Post("/merchants", merchantHandler.CreateMerchant)

	customerRepo := customers.NewPostgresRepository(app.db)
	customerService := customers.NewService(customerRepo)
	customerHandler := customers.NewHandler(customerService)

	paymentMethodRepo := paymentmethods.NewPostgresRepository(app.db)
	paymentMethodService := paymentmethods.NewService(paymentMethodRepo, customerService)
	paymentMethodHandler := paymentmethods.NewHandler(paymentMethodService)

	webhookEndpointRepo := webhookendpoints.NewPostgresRepository(app.db)
	webhookEndpointService := webhookendpoints.NewService(webhookEndpointRepo)
	webhookEndpointHandler := webhookendpoints.NewHandler(webhookEndpointService)

	webhookSender := webhook.NewSender(app.config.webhook)
	eventRepo := events.NewPostgresRepository(app.db)
	eventPublisher := events.NewService(eventRepo, webhookEndpointService, webhookSender)

	txRunner := database.NewTxRunner(app.db)
	ledgerRepo := ledger.NewPostgresRepository(app.db)
	ledgerService := ledger.NewService(ledgerRepo)

	paymentRepo := payments.NewPostgresRepository(app.db)
	paymentService := payments.NewService(paymentRepo, txRunner, eventPublisher, ledgerService, app.config.paymentDecline, customerService, paymentMethodService)
	paymentHandler := payments.NewHandler(paymentService)

	refundRepo := refunds.NewPostgresRepository(app.db)
	refundService := refunds.NewService(refundRepo, paymentRepo, txRunner, eventPublisher, ledgerService)
	refundHandler := refunds.NewHandler(refundService)

	payoutRepo := payouts.NewPostgresRepository(app.db)
	payoutService := payouts.NewService(payoutRepo, ledgerService, txRunner, eventPublisher)
	payoutHandler := payouts.NewHandler(payoutService)

	idempotencyRepo := idempotency.NewPostgresRepository(app.db)
	idempotent := idempotency.Middleware(idempotencyRepo)

	// Every route below requires a valid merchant API key.
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(merchantService))

		r.With(idempotent).Post("/customers", customerHandler.CreateCustomer)
		r.Get("/customers/{id}", customerHandler.GetCustomer)
		r.With(idempotent).Post("/customers/{customerId}/payment-methods", paymentMethodHandler.CreatePaymentMethod)

		r.Post("/webhook-endpoints", webhookEndpointHandler.CreateEndpoint)

		r.Get("/payments", paymentHandler.ListPayments)
		r.With(idempotent).Post("/payments", paymentHandler.CreatePayment)
		r.Get("/payments/{id}", paymentHandler.GetPayment)
		r.With(idempotent).Post("/payments/{id}/refunds", refundHandler.CreateRefund)
		r.Get("/payments/{id}/refunds", refundHandler.ListRefunds)

		r.Get("/payouts", payoutHandler.ListPayouts)
		r.With(idempotent).Post("/payouts", payoutHandler.CreatePayout)
		r.Get("/payouts/{id}", payoutHandler.GetPayout)
	})

	return r
}

// run
func (app *application) run(h http.Handler) error {
	srv := &http.Server {
		Addr: app.config.addr,
		Handler: h,
		WriteTimeout: time.Second * 30,
		ReadTimeout: time.Second * 10,
		IdleTimeout: time.Minute,
	}

	log.Printf("server has started at addr: %s", app.config.addr)

	return srv.ListenAndServe()
}


type application struct {
	config config
	db     *pgxpool.Pool
	// logger
}

type config struct {
	addr string
	db dbConfig
	chaos chaos.Config
	webhook webhook.Config
	paymentDecline payments.DeclineConfig
}

type dbConfig struct {
	dsn string
}