package chaos

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestLatency_Disabled(t *testing.T) {
	h := Latency(Config{})(okHandler())

	start := time.Now()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Fatalf("expected no delay, took %s", elapsed)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestLatency_WithinBounds(t *testing.T) {
	cfg := Config{LatencyMin: 20 * time.Millisecond, LatencyMax: 40 * time.Millisecond}
	h := Latency(cfg)(okHandler())

	start := time.Now()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	elapsed := time.Since(start)

	if elapsed < cfg.LatencyMin {
		t.Fatalf("expected delay >= %s, took %s", cfg.LatencyMin, elapsed)
	}
	if elapsed > cfg.LatencyMax+50*time.Millisecond {
		t.Fatalf("expected delay close to <= %s, took %s", cfg.LatencyMax, elapsed)
	}
}

func TestRandomError_Disabled(t *testing.T) {
	h := RandomError(Config{})(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRandomError_AlwaysFails(t *testing.T) {
	h := RandomError(Config{ErrorRate: 1})(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestRateLimit_Disabled(t *testing.T) {
	h := RateLimit(Config{})(okHandler())

	for i := 0; i < 10; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "1.2.3.4:1111"
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}
}

func TestRateLimit_BlocksAfterBurst(t *testing.T) {
	h := RateLimit(Config{RateLimitRPS: 1, RateLimitBurst: 1})(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:1111"

	first := httptest.NewRecorder()
	h.ServeHTTP(first, req)
	if first.Code != http.StatusOK {
		t.Fatalf("expected first request to succeed, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	h.ServeHTTP(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request to be rate limited, got %d", second.Code)
	}
}

func TestRateLimit_SameIPDifferentPortsShareBucket(t *testing.T) {
	// Each incoming TCP connection from the same client gets a distinct
	// ephemeral port; the limiter must key on IP alone or every connection
	// would get its own bucket and the limit would never bind.
	h := RateLimit(Config{RateLimitRPS: 1, RateLimitBurst: 1})(okHandler())

	reqA := httptest.NewRequest(http.MethodGet, "/", nil)
	reqA.RemoteAddr = "1.2.3.4:1111"
	reqB := httptest.NewRequest(http.MethodGet, "/", nil)
	reqB.RemoteAddr = "1.2.3.4:2222"

	first := httptest.NewRecorder()
	h.ServeHTTP(first, reqA)
	if first.Code != http.StatusOK {
		t.Fatalf("expected first request to succeed, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	h.ServeHTTP(second, reqB)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("expected request from same IP on a different port to share the bucket and be limited, got %d", second.Code)
	}
}

func TestRateLimit_PerClientIsolation(t *testing.T) {
	h := RateLimit(Config{RateLimitRPS: 1, RateLimitBurst: 1})(okHandler())

	reqA := httptest.NewRequest(http.MethodGet, "/", nil)
	reqA.RemoteAddr = "1.2.3.4:1111"
	reqB := httptest.NewRequest(http.MethodGet, "/", nil)
	reqB.RemoteAddr = "5.6.7.8:2222"

	recA := httptest.NewRecorder()
	h.ServeHTTP(recA, reqA)
	recB := httptest.NewRecorder()
	h.ServeHTTP(recB, reqB)

	if recA.Code != http.StatusOK || recB.Code != http.StatusOK {
		t.Fatalf("expected distinct clients to have independent buckets, got %d and %d", recA.Code, recB.Code)
	}
}
