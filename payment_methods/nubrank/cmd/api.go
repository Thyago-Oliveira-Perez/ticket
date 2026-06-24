package main

import (
	"net/http"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"log"
	"time"
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

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world"))
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
	// logger
	// db driver
}

type config struct {
	addr string
	db dbConfig
}

type dbConfig struct {
	dsn string
}