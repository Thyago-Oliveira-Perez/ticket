// Package chaos provides HTTP middleware that simulates real-world
// unreliability — injected latency, random failures, and rate limiting — so
// that consuming services can be built and tested against a hostile
// provider instead of an idealized one.
package chaos

import (
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Config controls which chaos behaviors are active. Each behavior is
// disabled by its own zero value, so a zero Config behaves like plain
// passthrough middleware.
type Config struct {
	// LatencyMin and LatencyMax bound a random delay added before each
	// request reaches its handler. Disabled when LatencyMax <= 0.
	LatencyMin time.Duration
	LatencyMax time.Duration

	// ErrorRate is the probability, in [0, 1], that a request fails with a
	// 500 before reaching its handler. Disabled when ErrorRate <= 0.
	ErrorRate float64

	// RateLimitRPS and RateLimitBurst configure a token bucket per client
	// IP. Disabled when RateLimitRPS <= 0. When enabled with a burst <= 0,
	// the burst defaults to the RPS rounded up (minimum 1).
	RateLimitRPS   float64
	RateLimitBurst int
}

// Latency delays each request by a random duration in [cfg.LatencyMin, cfg.LatencyMax).
func Latency(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if cfg.LatencyMax <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(randomDuration(cfg.LatencyMin, cfg.LatencyMax))
			next.ServeHTTP(w, r)
		})
	}
}

func randomDuration(min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	return min + time.Duration(rand.Int64N(int64(max-min)))
}

// RandomError fails a fraction of requests, set by cfg.ErrorRate, with a 500
// before they reach their handler.
func RandomError(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if cfg.ErrorRate <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if rand.Float64() < cfg.ErrorRate {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit throttles requests per client IP with a token bucket, returning
// 429 once a client's bucket is exhausted. It relies on chi's RealIP
// middleware running first so r.RemoteAddr reflects the real client rather
// than a proxy hop.
func RateLimit(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if cfg.RateLimitRPS <= 0 {
			return next
		}

		burst := cfg.RateLimitBurst
		if burst <= 0 {
			burst = int(math.Ceil(cfg.RateLimitRPS))
			if burst < 1 {
				burst = 1
			}
		}

		var mu sync.Mutex
		limiters := make(map[string]*rate.Limiter)

		limiterFor := func(key string) *rate.Limiter {
			mu.Lock()
			defer mu.Unlock()
			l, ok := limiters[key]
			if !ok {
				l = rate.NewLimiter(rate.Limit(cfg.RateLimitRPS), burst)
				limiters[key] = l
			}
			return l
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiterFor(clientIP(r)).Allow() {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP strips the ephemeral port from r.RemoteAddr so that repeated
// connections from the same client share a bucket instead of each getting
// its own, which would defeat the limiter.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
