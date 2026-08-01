package customers

import (
	"context"
	"errors"
	"time"
)

// ErrCustomerNotFound is returned by lookups that find no matching customer
// (including one that exists but belongs to a different merchant).
var ErrCustomerNotFound = errors.New("customer not found")

type Customer struct {
	ID         string    `json:"id"`
	MerchantID string    `json:"merchant_id"`
	Email      string    `json:"email"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Repository interface {
	CreateCustomer(ctx context.Context, c Customer) (Customer, error)
	// GetByID looks up a customer by id, scoped to merchantID. Returns
	// ErrCustomerNotFound if none exists.
	GetByID(ctx context.Context, merchantID, id string) (Customer, error)
}
