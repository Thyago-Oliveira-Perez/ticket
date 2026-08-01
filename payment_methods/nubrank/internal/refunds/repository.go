package refunds

import (
	"context"
	"time"

	"nubrank/internal/database"
)

const StatusSucceeded = "succeeded"

type Refund struct {
	ID          string    `json:"id"`
	PaymentID   string    `json:"payment_id"`
	AmountMinor int64     `json:"amount_minor"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Repository interface {
	CreateRefund(ctx context.Context, r Refund) (Refund, error)
	// SumByPayment returns the total amount already refunded for
	// paymentID, so a new refund request can be checked against the
	// remaining refundable amount.
	SumByPayment(ctx context.Context, paymentID string) (int64, error)
	ListByPayment(ctx context.Context, paymentID string) ([]Refund, error)
	// WithQuerier returns a Repository bound to q instead of the pool, so
	// its calls can participate in a transaction started elsewhere (e.g.
	// atomically with locking the payment row and posting an outbox
	// event).
	WithQuerier(q database.Querier) Repository
}
