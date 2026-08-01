package webhookendpoints

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
)

// ErrValidation marks errors caused by invalid caller input, as opposed to
// infrastructure failures. Handlers use errors.Is against it to pick the
// right HTTP status.
var ErrValidation = errors.New("validation error")

type Service interface {
	// CreateEndpoint registers url to receive this merchant's webhook
	// events. The returned Endpoint's Secret is populated only here, at
	// creation time — it's never retrievable again.
	CreateEndpoint(ctx context.Context, merchantID, endpointURL string) (Endpoint, error)
	ListActiveByMerchant(ctx context.Context, merchantID string) ([]Endpoint, error)
}

type svc struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &svc{repo: repo}
}

func (s *svc) CreateEndpoint(ctx context.Context, merchantID, endpointURL string) (Endpoint, error) {
	if err := validateURL(endpointURL); err != nil {
		return Endpoint{}, err
	}

	secret, err := generateSecret()
	if err != nil {
		return Endpoint{}, fmt.Errorf("generate webhook secret: %w", err)
	}

	return s.repo.CreateEndpoint(ctx, Endpoint{MerchantID: merchantID, URL: endpointURL, Secret: secret, Active: true})
}

func (s *svc) ListActiveByMerchant(ctx context.Context, merchantID string) ([]Endpoint, error) {
	return s.repo.ListActiveByMerchant(ctx, merchantID)
}

func validateURL(endpointURL string) error {
	u, err := url.ParseRequestURI(endpointURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%w: url must be a valid http(s) URL", ErrValidation)
	}
	return nil
}

func generateSecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "whsec_" + hex.EncodeToString(buf), nil
}
