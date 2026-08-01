package payments

import (
	"context"
	"errors"
	"time"

	"nubrank/internal/database"
)

const (
	StatusApproved = "approved"
	StatusDeclined = "declined"
	StatusRefunded = "refunded"
)

// ErrPaymentNotFound is returned by lookups that find no matching payment.
var ErrPaymentNotFound = errors.New("payment not found")

// ErrRefundNotApplied is returned by RefundPayment when its conditional
// update matches no row, i.e. the payment isn't currently approved (either
// it doesn't exist, is declined, or was already refunded). Callers can't
// tell which from this error alone and should re-fetch the payment by id to
// find out.
var ErrRefundNotApplied = errors.New("refund not applied")

type Payment struct {
	ID              string `json:"id"`
	MerchantID      string `json:"merchant_id"`
	CustomerID      string `json:"customer_id"`
	PaymentMethodID string `json:"payment_method_id"`
	AmountMinor     int64  `json:"amount_minor"`
	Currency        string `json:"currency"`
	Status          string `json:"status"`
	// DeclineReason is set when Status is StatusDeclined and nil otherwise.
	DeclineReason *string `json:"decline_reason,omitempty"`
	// RefundedAt is set when Status is StatusRefunded and nil otherwise.
	RefundedAt *time.Time `json:"refunded_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Repository interface {
	// ListPayments lists payments belonging to merchantID.
	ListPayments(ctx context.Context, merchantID string) ([]Payment, error)
	CreatePayment(ctx context.Context, p Payment) (Payment, error)
	// GetByID looks up a payment by its id, scoped to merchantID so a
	// caller can never look up another merchant's payment. Returns
	// ErrPaymentNotFound if none exists.
	GetByID(ctx context.Context, merchantID, id string) (Payment, error)
	// RefundPayment atomically transitions a payment from StatusApproved to
	// StatusRefunded, scoped to merchantID. Returns ErrRefundNotApplied if
	// the payment isn't currently approved (or belongs to another merchant).
	RefundPayment(ctx context.Context, merchantID, id string) (Payment, error)
	// WithQuerier returns a Repository bound to q instead of the pool, so
	// its calls can participate in a transaction started elsewhere (e.g.
	// atomically with an outbox event insert).
	WithQuerier(q database.Querier) Repository
}
