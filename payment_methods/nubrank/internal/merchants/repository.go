package merchants

import (
	"context"
	"errors"
	"time"
)

const (
	StatusActive = "active"
)

// ErrInvalidAPIKey is returned when an API key doesn't match any stored
// key hash.
var ErrInvalidAPIKey = errors.New("invalid api key")

type Merchant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// APIKey is never serialized to JSON; only the plaintext key generated at
// creation time is ever shown to a caller, and only once.
type APIKey struct {
	ID         string
	MerchantID string
	KeyHash    string
	Scope      string
	LastUsedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Repository interface {
	CreateMerchant(ctx context.Context, m Merchant) (Merchant, error)
	CreateAPIKey(ctx context.Context, k APIKey) (APIKey, error)
	// GetMerchantByAPIKeyHash looks up the merchant owning the API key with
	// the given hash. Returns ErrInvalidAPIKey if no key matches.
	GetMerchantByAPIKeyHash(ctx context.Context, keyHash string) (Merchant, error)
	// TouchAPIKeyLastUsed best-effort records that a key was just used.
	TouchAPIKeyLastUsed(ctx context.Context, keyHash string) error
}
