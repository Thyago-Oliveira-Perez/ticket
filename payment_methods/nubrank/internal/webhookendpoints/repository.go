package webhookendpoints

import (
	"context"
	"time"
)

type Endpoint struct {
	ID         string    `json:"id"`
	MerchantID string    `json:"merchant_id"`
	URL        string    `json:"url"`
	Secret     string    `json:"-"` // shown only in the creation response
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Repository interface {
	CreateEndpoint(ctx context.Context, e Endpoint) (Endpoint, error)
	// ListActiveByMerchant lists active endpoints for merchantID, for
	// fanning out an event to every endpoint that should receive it.
	ListActiveByMerchant(ctx context.Context, merchantID string) ([]Endpoint, error)
}
