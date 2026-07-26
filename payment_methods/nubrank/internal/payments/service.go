package payments

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ErrValidation marks errors caused by invalid caller input, as opposed to
// infrastructure failures. Handlers use errors.Is against it to pick the
// right HTTP status.
var ErrValidation = errors.New("validation error")

type CreatePaymentInput struct {
	MerchantID      string
	CustomerID      string
	PaymentMethodID string
	AmountMinor     int64
	Currency        string
}

type Service interface {
	ListPayments(ctx context.Context) ([]Payment, error)
	CreatePayment(ctx context.Context, in CreatePaymentInput) (Payment, error)
}

type svc struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &svc{repo: repo}
}

func (s *svc) ListPayments(ctx context.Context) ([]Payment, error) {
	return s.repo.ListPayments(ctx)
}

func (s *svc) CreatePayment(ctx context.Context, in CreatePaymentInput) (Payment, error) {
	if err := validateCreatePaymentInput(in); err != nil {
		return Payment{}, err
	}

	p := Payment{
		MerchantID:      in.MerchantID,
		CustomerID:      in.CustomerID,
		PaymentMethodID: in.PaymentMethodID,
		AmountMinor:     in.AmountMinor,
		Currency:        in.Currency,
		Status:          StatusApproved,
	}

	return s.repo.CreatePayment(ctx, p)
}

func validateCreatePaymentInput(in CreatePaymentInput) error {
	if _, err := uuid.Parse(in.MerchantID); err != nil {
		return fmt.Errorf("%w: merchant_id must be a valid UUID", ErrValidation)
	}
	if _, err := uuid.Parse(in.CustomerID); err != nil {
		return fmt.Errorf("%w: customer_id must be a valid UUID", ErrValidation)
	}
	if _, err := uuid.Parse(in.PaymentMethodID); err != nil {
		return fmt.Errorf("%w: payment_method_id must be a valid UUID", ErrValidation)
	}
	if in.AmountMinor <= 0 {
		return fmt.Errorf("%w: amount_minor must be greater than zero", ErrValidation)
	}
	if len(in.Currency) != 3 {
		return fmt.Errorf("%w: currency must be a 3-letter ISO 4217 code", ErrValidation)
	}

	return nil
}
