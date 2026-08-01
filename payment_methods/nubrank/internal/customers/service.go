package customers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ErrValidation marks errors caused by invalid caller input, as opposed to
// infrastructure failures. Handlers use errors.Is against it to pick the
// right HTTP status.
var ErrValidation = errors.New("validation error")

type Service interface {
	CreateCustomer(ctx context.Context, merchantID, email string) (Customer, error)
	// GetCustomer looks up a customer by id, scoped to merchantID. Returns
	// an error wrapping ErrValidation if id isn't a valid UUID, or
	// ErrCustomerNotFound if no customer with that id exists for that
	// merchant.
	GetCustomer(ctx context.Context, merchantID, id string) (Customer, error)
	// VerifyOwnership returns nil if customerID exists and belongs to
	// merchantID, or an error wrapping ErrValidation (bad UUID) or
	// ErrCustomerNotFound.
	VerifyOwnership(ctx context.Context, merchantID, customerID string) error
}

type svc struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &svc{repo: repo}
}

func (s *svc) CreateCustomer(ctx context.Context, merchantID, email string) (Customer, error) {
	if !strings.Contains(email, "@") {
		return Customer{}, fmt.Errorf("%w: email must be a valid email address", ErrValidation)
	}

	return s.repo.CreateCustomer(ctx, Customer{MerchantID: merchantID, Email: email})
}

func (s *svc) GetCustomer(ctx context.Context, merchantID, id string) (Customer, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Customer{}, fmt.Errorf("%w: id must be a valid UUID", ErrValidation)
	}
	return s.repo.GetByID(ctx, merchantID, id)
}

func (s *svc) VerifyOwnership(ctx context.Context, merchantID, customerID string) error {
	if _, err := uuid.Parse(customerID); err != nil {
		return fmt.Errorf("%w: customer_id must be a valid UUID", ErrValidation)
	}
	_, err := s.repo.GetByID(ctx, merchantID, customerID)
	return err
}
