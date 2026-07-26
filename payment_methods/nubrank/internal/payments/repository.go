package payments

import (
	"context"
	"time"
)

const (
	StatusApproved = "approved"
)

type Payment struct {
	ID              string
	MerchantID      string
	CustomerID      string
	PaymentMethodID string
	AmountMinor     int64
	Currency        string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Repository interface {
	ListPayments(ctx context.Context) ([]Payment, error)
	CreatePayment(ctx context.Context, p Payment) (Payment, error)
}
