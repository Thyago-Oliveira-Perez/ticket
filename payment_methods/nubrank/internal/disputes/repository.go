package disputes

import (
	"context"
	"time"

	"nubrank/internal/database"
)

const (
	StatusNeedsResponse = "needs_response"
	StatusWon           = "won"
	StatusLost          = "lost"
)

type Dispute struct {
	ID          string    `json:"id"`
	PaymentID   string    `json:"payment_id"`
	AmountMinor int64     `json:"amount_minor"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Repository interface {
	CreateDispute(ctx context.Context, d Dispute) (Dispute, error)
	UpdateStatus(ctx context.Context, id, status string) (Dispute, error)
	ListByPayment(ctx context.Context, paymentID string) ([]Dispute, error)
	// WithQuerier returns a Repository bound to q instead of the pool, so
	// its calls can participate in a transaction started elsewhere (e.g.
	// atomically with the ledger hold/release it represents).
	WithQuerier(q database.Querier) Repository
}
