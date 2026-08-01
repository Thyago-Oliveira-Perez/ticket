package paymentmethods

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// ErrValidation marks errors caused by invalid caller input, as opposed to
// infrastructure failures. Handlers use errors.Is against it to pick the
// right HTTP status.
var ErrValidation = errors.New("validation error")

var cardNumberPattern = regexp.MustCompile(`^[0-9]{12,19}$`)

// CustomerVerifier is the subset of customers.Service this package needs:
// confirming a customer exists and belongs to a given merchant, without
// importing the customers package's concrete service (keeps this package
// wireable against a fake in tests).
type CustomerVerifier interface {
	// VerifyOwnership returns nil if customerID exists and belongs to
	// merchantID, or the error the concrete implementation uses to signal
	// otherwise (checked by callers via errors.Is against that package's
	// own sentinel).
	VerifyOwnership(ctx context.Context, merchantID, customerID string) error
}

// CreatePaymentMethodInput is a fake card to tokenize. Nothing here is a
// real card network detail — brand is derived from the number's leading
// digit purely for simulation, matching how a real vault derives it from
// the BIN rather than trusting the caller.
type CreatePaymentMethodInput struct {
	Number     string
	ExpireYear int
}

type Service interface {
	CreatePaymentMethod(ctx context.Context, merchantID, customerID string, in CreatePaymentMethodInput) (PaymentMethod, error)
	// VerifyOwnership returns nil if paymentMethodID exists and belongs to
	// customerID, or an error wrapping ErrValidation (bad UUID) or
	// ErrPaymentMethodNotFound.
	VerifyOwnership(ctx context.Context, customerID, paymentMethodID string) error
}

type svc struct {
	repo      Repository
	customers CustomerVerifier
}

func NewService(repo Repository, customers CustomerVerifier) Service {
	return &svc{repo: repo, customers: customers}
}

func (s *svc) CreatePaymentMethod(ctx context.Context, merchantID, customerID string, in CreatePaymentMethodInput) (PaymentMethod, error) {
	if _, err := uuid.Parse(customerID); err != nil {
		return PaymentMethod{}, fmt.Errorf("%w: customer id must be a valid UUID", ErrValidation)
	}
	if err := s.customers.VerifyOwnership(ctx, merchantID, customerID); err != nil {
		return PaymentMethod{}, err
	}
	if err := validateCreateInput(in); err != nil {
		return PaymentMethod{}, err
	}

	token, err := generateToken()
	if err != nil {
		return PaymentMethod{}, fmt.Errorf("generate payment method token: %w", err)
	}

	pm := PaymentMethod{
		CustomerID: customerID,
		Token:      token,
		Brand:      brandFor(in.Number),
		Last4:      in.Number[len(in.Number)-4:],
		ExpireYear: in.ExpireYear,
	}

	return s.repo.CreatePaymentMethod(ctx, pm)
}

func (s *svc) VerifyOwnership(ctx context.Context, customerID, paymentMethodID string) error {
	if _, err := uuid.Parse(paymentMethodID); err != nil {
		return fmt.Errorf("%w: payment_method_id must be a valid UUID", ErrValidation)
	}
	_, err := s.repo.GetByID(ctx, customerID, paymentMethodID)
	return err
}

func validateCreateInput(in CreatePaymentMethodInput) error {
	if !cardNumberPattern.MatchString(in.Number) {
		return fmt.Errorf("%w: number must be 12-19 digits", ErrValidation)
	}
	if in.ExpireYear < time.Now().Year() {
		return fmt.Errorf("%w: expire_year must not be in the past", ErrValidation)
	}
	return nil
}

// brandFor simulates deriving a card brand from its BIN (the leading
// digits), the same way a real vault would — not an exhaustive real BIN
// table, just enough to make tokenized cards look plausible.
func brandFor(number string) string {
	switch number[0] {
	case '4':
		return "visa"
	case '5':
		return "mastercard"
	case '3':
		return "amex"
	default:
		return "unknown"
	}
}

func generateToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "pm_tok_" + hex.EncodeToString(buf), nil
}
