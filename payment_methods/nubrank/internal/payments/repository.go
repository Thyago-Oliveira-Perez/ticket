package payments

import (
	"context"
	"errors"
	"time"

	"nubrank/internal/database"
)

const (
	StatusApproved          = "approved"
	StatusDeclined          = "declined"
	StatusRefunded          = "refunded"
	StatusPartiallyRefunded = "partially_refunded"
)

// ErrPaymentNotFound is returned by lookups that find no matching payment.
var ErrPaymentNotFound = errors.New("payment not found")

type Payment struct {
	ID              string `json:"id"`
	MerchantID      string `json:"merchant_id"`
	CustomerID      string `json:"customer_id"`
	PaymentMethodID string `json:"payment_method_id"`
	AmountMinor     int64  `json:"amount_minor"`
	Currency        string `json:"currency"`
	Status          string `json:"status"`
	// DeclineReason is set when Status is StatusDeclined and nil otherwise.
	DeclineReason *string   `json:"decline_reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Repository interface {
	// ListPayments lists payments belonging to merchantID.
	ListPayments(ctx context.Context, merchantID string) ([]Payment, error)
	CreatePayment(ctx context.Context, p Payment) (Payment, error)
	// GetByID looks up a payment by its id, scoped to merchantID so a
	// caller can never look up another merchant's payment. Returns
	// ErrPaymentNotFound if none exists.
	GetByID(ctx context.Context, merchantID, id string) (Payment, error)
	// LockForUpdate is like GetByID but takes a row lock (SELECT ... FOR
	// UPDATE), for callers (namely the refunds package) that need to
	// safely read a payment's current amount/status and then act on it
	// without racing a concurrent refund. Only meaningful when bound to a
	// transaction via WithQuerier.
	LockForUpdate(ctx context.Context, merchantID, id string) (Payment, error)
	// UpdateStatus sets a payment's status directly. Used by the refunds
	// package to transition a payment to StatusRefunded or
	// StatusPartiallyRefunded once a refund has been recorded.
	UpdateStatus(ctx context.Context, merchantID, id, status string) (Payment, error)
	// WithQuerier returns a Repository bound to q instead of the pool, so
	// its calls can participate in a transaction started elsewhere (e.g.
	// atomically with an outbox event insert).
	WithQuerier(q database.Querier) Repository
}
