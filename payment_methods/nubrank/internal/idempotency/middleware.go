// Package idempotency provides HTTP middleware that makes a mutating
// endpoint safe to retry: a request carrying an Idempotency-Key header is
// executed at most once per (merchant, key); a retried request with the
// same key and body replays the original response instead of re-running
// the handler, and a retried request with the same key but a different
// body is rejected as a caller error.
package idempotency

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"

	"nubrank/internal/auth"
)

const headerKey = "Idempotency-Key"

// Middleware must be mounted behind auth.Middleware, since it scopes keys
// by the authenticated merchant.
func Middleware(repo Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(headerKey)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			merchantID, _ := auth.MerchantID(r.Context())

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read request body", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			hash := hashRequest(body)

			rec, reserved, err := repo.Reserve(r.Context(), merchantID, key, hash)
			if err != nil {
				log.Println(err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			if !reserved {
				replay(w, rec, hash)
				return
			}

			captured := &responseCapture{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(captured, r)

			if err := repo.Complete(r.Context(), rec.ID, captured.statusCode, captured.body.Bytes()); err != nil {
				log.Println(err)
			}
		})
	}
}

// replay serves the outcome of an earlier request that reserved this key:
// the cached response if the body matches, a conflict if the earlier
// request is still in flight, or a validation error if the key was reused
// with a different body.
func replay(w http.ResponseWriter, rec Record, requestHash string) {
	if rec.RequestHash != requestHash {
		http.Error(w, "Idempotency-Key was already used with a different request body", http.StatusUnprocessableEntity)
		return
	}
	if rec.ResponseStatus == nil {
		http.Error(w, "a request with this Idempotency-Key is already being processed", http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(*rec.ResponseStatus)
	w.Write(rec.ResponseBody)
}

func hashRequest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// responseCapture tees a handler's response so it can be persisted after
// the fact, while still writing it through to the real client immediately.
type responseCapture struct {
	http.ResponseWriter
	statusCode  int
	body        bytes.Buffer
	wroteHeader bool
}

func (c *responseCapture) WriteHeader(code int) {
	c.statusCode = code
	c.wroteHeader = true
	c.ResponseWriter.WriteHeader(code)
}

func (c *responseCapture) Write(b []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	c.body.Write(b)
	return c.ResponseWriter.Write(b)
}
