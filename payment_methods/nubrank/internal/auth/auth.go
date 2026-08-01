// Package auth authenticates requests against merchant API keys and makes
// the resolved merchant id available to downstream handlers via context.
package auth

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"nubrank/internal/merchants"
)

type contextKey int

const merchantIDKey contextKey = iota

// MerchantID returns the authenticated merchant id from ctx. Only valid
// inside a handler mounted behind Middleware.
func MerchantID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(merchantIDKey).(string)
	return id, ok
}

// Middleware resolves the merchant from an "Authorization: Bearer <key>"
// header via service, and rejects the request with 401 if it's missing or
// doesn't match a known API key.
func Middleware(service merchants.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawKey, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
				return
			}

			merchant, err := service.AuthenticateAPIKey(r.Context(), rawKey)
			if err != nil {
				if errors.Is(err, merchants.ErrInvalidAPIKey) {
					http.Error(w, "invalid api key", http.StatusUnauthorized)
					return
				}
				log.Println(err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			ctx := context.WithValue(r.Context(), merchantIDKey, merchant.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}
