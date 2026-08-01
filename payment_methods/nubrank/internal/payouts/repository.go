package payouts

import (
	"context"
	"errors"
	"time"

	"nubrank/internal/database"
)

const StatusPaid = "paid"

// ErrPayoutNotFound is returned by lookups that find no matching payout
// (including one that exists but belongs to a different merchant).
var ErrPayoutNotFound = errors.New("payout not found")

type Payout struct {
	ID          string    `json:"id"`
	MerchantID  string    `json:"merchant_id"`
	AmountMinor int64     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Repository interface {
	CreatePayout(ctx context.Context, p Payout) (Payout, error)
	GetByID(ctx context.Context, merchantID, id string) (Payout, error)
	ListByMerchant(ctx context.Context, merchantID string) ([]Payout, error)
	// WithQuerier returns a Repository bound to q instead of the pool, so
	// its calls can participate in a transaction started elsewhere (e.g.
	// atomically with the ledger debit it represents).
	WithQuerier(q database.Querier) Repository
}
