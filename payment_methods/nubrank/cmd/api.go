package main

import (
	"log"
	"net/http"
	"nubrank/internal/chaos"
	"nubrank/internal/payments"
	"nubrank/internal/webhook"
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

	paymentRepo := payments.NewPostgresRepository(app.db)
	webhookSender := webhook.NewSender(app.config.webhook)
	paymentService := payments.NewService(paymentRepo, webhookSender, app.config.paymentDecline)
	paymentHandler := payments.NewHandler(paymentService)
	r.Get("/payments", paymentHandler.ListPayments)
	r.Post("/payments", paymentHandler.CreatePayment)

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