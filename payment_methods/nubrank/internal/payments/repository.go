package payments

import (
	"context"
	"errors"
	"time"
)

const (
	StatusApproved = "approved"
	StatusDeclined = "declined"
)

// ErrPaymentNotFound is returned by lookups that find no matching payment.
var ErrPaymentNotFound = errors.New("payment not found")

// ErrIdempotencyKeyConflict is returned by CreatePayment when a concurrent
// request already created a payment for the same (merchant_id,
// idempotency_key) pair. Callers should look the existing payment up via
// GetByIdempotencyKey rather than retrying the insert.
var ErrIdempotencyKeyConflict = errors.New("idempotency key conflict")

type Payment struct {
	ID              string
	MerchantID      string
	CustomerID      string
	PaymentMethodID string
	AmountMinor     int64
	Currency        string
	Status          string
	// DeclineReason is set when Status is StatusDeclined and nil otherwise.
	DeclineReason *string
	// IdempotencyKey is set when the caller supplied one on creation, nil
	// otherwise.
	IdempotencyKey *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Repository interface {
	ListPayments(ctx context.Context) ([]Payment, error)
	CreatePayment(ctx context.Context, p Payment) (Payment, error)
	// GetByIdempotencyKey looks up a payment by (merchant_id,
	// idempotency_key). Returns ErrPaymentNotFound if none exists.
	GetByIdempotencyKey(ctx context.Context, merchantID, idempotencyKey string) (Payment, error)
	// GetByID looks up a payment by its id. Returns ErrPaymentNotFound if
	// none exists.
	GetByID(ctx context.Context, id string) (Payment, error)
}
