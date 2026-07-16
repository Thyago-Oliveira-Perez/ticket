package payments

import (
	"context"
	"time"
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
}
