package paymentmethods

import (
	"context"
	"errors"
	"time"
)

// ErrPaymentMethodNotFound is returned by lookups that find no matching
// payment method (including one that exists but belongs to a different
// customer).
var ErrPaymentMethodNotFound = errors.New("payment method not found")

type PaymentMethod struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	Token      string    `json:"token"`
	Brand      string    `json:"brand"`
	Last4      string    `json:"last4"`
	ExpireYear int       `json:"expire_year"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Repository interface {
	CreatePaymentMethod(ctx context.Context, pm PaymentMethod) (PaymentMethod, error)
	// GetByID looks up a payment method by id, scoped to customerID. Returns
	// ErrPaymentMethodNotFound if none exists.
	GetByID(ctx context.Context, customerID, id string) (PaymentMethod, error)
}
