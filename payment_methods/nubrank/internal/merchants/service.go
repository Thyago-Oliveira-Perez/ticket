package merchants

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// ErrValidation marks errors caused by invalid caller input, as opposed to
// infrastructure failures. Handlers use errors.Is against it to pick the
// right HTTP status.
var ErrValidation = errors.New("validation error")

// apiKeyScopeFullAccess is the only scope nubrank currently issues; the
// column exists so a caller-facing scoping model can grow into it later.
const apiKeyScopeFullAccess = "full_access"

type Service interface {
	// CreateMerchant creates a merchant and issues its first API key. The
	// plaintext key is returned only here, at creation time, and is never
	// retrievable again — only its hash is persisted.
	CreateMerchant(ctx context.Context, name string) (Merchant, string, error)
	// AuthenticateAPIKey resolves the merchant owning rawKey. Returns
	// ErrInvalidAPIKey if it doesn't match any stored key.
	AuthenticateAPIKey(ctx context.Context, rawKey string) (Merchant, error)
}

type svc struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &svc{repo: repo}
}

func (s *svc) CreateMerchant(ctx context.Context, name string) (Merchant, string, error) {
	if name == "" {
		return Merchant{}, "", fmt.Errorf("%w: name must not be empty", ErrValidation)
	}

	merchant, err := s.repo.CreateMerchant(ctx, Merchant{Name: name, Status: StatusActive})
	if err != nil {
		return Merchant{}, "", err
	}

	rawKey, keyHash, err := generateAPIKey()
	if err != nil {
		return Merchant{}, "", fmt.Errorf("generate api key: %w", err)
	}

	if _, err := s.repo.CreateAPIKey(ctx, APIKey{
		MerchantID: merchant.ID,
		KeyHash:    keyHash,
		Scope:      apiKeyScopeFullAccess,
	}); err != nil {
		return Merchant{}, "", err
	}

	return merchant, rawKey, nil
}

func (s *svc) AuthenticateAPIKey(ctx context.Context, rawKey string) (Merchant, error) {
	if rawKey == "" {
		return Merchant{}, ErrInvalidAPIKey
	}

	keyHash := hashAPIKey(rawKey)

	merchant, err := s.repo.GetMerchantByAPIKeyHash(ctx, keyHash)
	if err != nil {
		return Merchant{}, err
	}

	// Best-effort; a failure to record last-used shouldn't fail the request.
	_ = s.repo.TouchAPIKeyLastUsed(ctx, keyHash)

	return merchant, nil
}

// generateAPIKey returns a new plaintext key (nb_live_<48 hex chars>) and
// the hash that should be persisted for it.
func generateAPIKey() (rawKey, keyHash string, err error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	rawKey = "nb_live_" + hex.EncodeToString(buf)
	return rawKey, hashAPIKey(rawKey), nil
}

func hashAPIKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}
