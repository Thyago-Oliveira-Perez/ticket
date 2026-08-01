package payments

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/url"

	"nubrank/internal/customers"
	"nubrank/internal/paymentmethods"

	"github.com/google/uuid"
)

// ErrValidation marks errors caused by invalid caller input, as opposed to
// infrastructure failures. Handlers use errors.Is against it to pick the
// right HTTP status.
var ErrValidation = errors.New("validation error")

// ErrPaymentAlreadyRefunded is returned by RefundPayment when the payment
// has already been refunded.
var ErrPaymentAlreadyRefunded = errors.New("payment already refunded")

// ErrPaymentNotApproved is returned by RefundPayment when the payment isn't
// in StatusApproved (e.g. it was declined) and so can't be refunded.
var ErrPaymentNotApproved = errors.New("payment is not approved and cannot be refunded")

// ErrCustomerNotFound is returned by CreatePayment when customer_id doesn't
// exist (or doesn't belong to the authenticated merchant).
var ErrCustomerNotFound = errors.New("customer not found")

// ErrPaymentMethodNotFound is returned by CreatePayment when
// payment_method_id doesn't exist (or doesn't belong to customer_id).
var ErrPaymentMethodNotFound = errors.New("payment method not found")

// CustomerVerifier is the subset of customers.Service CreatePayment needs:
// confirming customer_id belongs to the authenticated merchant.
type CustomerVerifier interface {
	VerifyOwnership(ctx context.Context, merchantID, customerID string) error
}

// PaymentMethodVerifier is the subset of paymentmethods.Service
// CreatePayment needs: confirming payment_method_id belongs to customer_id.
type PaymentMethodVerifier interface {
	VerifyOwnership(ctx context.Context, customerID, paymentMethodID string) error
}

// WebhookSender delivers a domain event to a caller-supplied URL. Defined
// here (rather than depending on the webhook package's concrete type) so
// this package doesn't need to import it — the webhook package's Sender
// satisfies this structurally.
type WebhookSender interface {
	Send(ctx context.Context, url, eventType string, data any) error
}

type CreatePaymentInput struct {
	MerchantID      string
	CustomerID      string
	PaymentMethodID string
	AmountMinor     int64
	Currency        string
	// WebhookURL, if set, receives a payment.approved or payment.declined
	// event once the payment is created. Optional.
	WebhookURL string
}

type RefundInput struct {
	// WebhookURL, if set, receives a payment.refunded event once the
	// payment is refunded. Optional.
	WebhookURL string
}

type Service interface {
	// ListPayments lists payments belonging to merchantID.
	ListPayments(ctx context.Context, merchantID string) ([]Payment, error)
	CreatePayment(ctx context.Context, in CreatePaymentInput) (Payment, error)
	// GetPayment looks up a payment by id, scoped to merchantID. Returns an
	// error wrapping ErrValidation if id isn't a valid UUID, or
	// ErrPaymentNotFound if no payment with that id exists for that
	// merchant.
	GetPayment(ctx context.Context, merchantID, id string) (Payment, error)
	// RefundPayment transitions an approved payment to StatusRefunded,
	// scoped to merchantID. Returns an error wrapping ErrValidation if id
	// isn't a valid UUID, ErrPaymentNotFound if no payment with that id
	// exists for that merchant, ErrPaymentAlreadyRefunded if it was already
	// refunded, or ErrPaymentNotApproved if it isn't currently approved
	// (e.g. declined).
	RefundPayment(ctx context.Context, merchantID, id string, in RefundInput) (Payment, error)
}

// DeclineConfig controls the probability that an otherwise-valid payment is
// declined, simulating a real gateway's business-level rejections (as
// opposed to the chaos package's transport-level failures).
type DeclineConfig struct {
	// Rate is the probability, in [0, 1], that a payment is declined.
	// Disabled (payments are always approved) when Rate <= 0.
	Rate float64
}

// declineReasons lists the reasons a simulated decline may carry. Picked
// uniformly at random; not meant to reflect real-world decline frequency.
var declineReasons = []string{
	"insufficient_funds",
	"card_expired",
	"fraud_suspected",
	"issuer_unavailable",
}

type svc struct {
	repo           Repository
	webhooks       WebhookSender
	decline        DeclineConfig
	customers      CustomerVerifier
	paymentMethods PaymentMethodVerifier
	// rollDecline and pickReason are overridden in tests for determinism;
	// they default to real randomness via NewService.
	rollDecline func() bool
	pickReason  func() string
}

func NewService(repo Repository, webhooks WebhookSender, decline DeclineConfig, customerVerifier CustomerVerifier, paymentMethodVerifier PaymentMethodVerifier) Service {
	return &svc{
		repo:           repo,
		webhooks:       webhooks,
		decline:        decline,
		customers:      customerVerifier,
		paymentMethods: paymentMethodVerifier,
		rollDecline: func() bool {
			return decline.Rate > 0 && rand.Float64() < decline.Rate
		},
		pickReason: func() string {
			return declineReasons[rand.IntN(len(declineReasons))]
		},
	}
}

func (s *svc) ListPayments(ctx context.Context, merchantID string) ([]Payment, error) {
	return s.repo.ListPayments(ctx, merchantID)
}

func (s *svc) GetPayment(ctx context.Context, merchantID, id string) (Payment, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Payment{}, fmt.Errorf("%w: id must be a valid UUID", ErrValidation)
	}
	return s.repo.GetByID(ctx, merchantID, id)
}

func (s *svc) RefundPayment(ctx context.Context, merchantID, id string, in RefundInput) (Payment, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Payment{}, fmt.Errorf("%w: id must be a valid UUID", ErrValidation)
	}
	if err := validateWebhookURL(in.WebhookURL); err != nil {
		return Payment{}, err
	}

	refunded, err := s.repo.RefundPayment(ctx, merchantID, id)
	if err != nil {
		if !errors.Is(err, ErrRefundNotApplied) {
			return Payment{}, err
		}
		// The conditional update matched no row; re-fetch to tell a missing
		// payment apart from one that just isn't refundable right now.
		existing, getErr := s.repo.GetByID(ctx, merchantID, id)
		if getErr != nil {
			return Payment{}, getErr
		}
		if existing.Status == StatusRefunded {
			return Payment{}, ErrPaymentAlreadyRefunded
		}
		return Payment{}, ErrPaymentNotApproved
	}

	if in.WebhookURL != "" {
		// Detach from the request context's cancellation (the HTTP response
		// has already been decided) but keep any request-scoped values.
		webhookCtx := context.WithoutCancel(ctx)
		go func() {
			if err := s.webhooks.Send(webhookCtx, in.WebhookURL, "payment.refunded", refunded); err != nil {
				log.Printf("webhook: send failed for payment %s: %v", refunded.ID, err)
			}
		}()
	}

	return refunded, nil
}

func (s *svc) CreatePayment(ctx context.Context, in CreatePaymentInput) (Payment, error) {
	if err := validateCreatePaymentInput(in); err != nil {
		return Payment{}, err
	}
	if err := s.customers.VerifyOwnership(ctx, in.MerchantID, in.CustomerID); err != nil {
		if errors.Is(err, customers.ErrCustomerNotFound) {
			return Payment{}, ErrCustomerNotFound
		}
		return Payment{}, err
	}
	if err := s.paymentMethods.VerifyOwnership(ctx, in.CustomerID, in.PaymentMethodID); err != nil {
		if errors.Is(err, paymentmethods.ErrPaymentMethodNotFound) {
			return Payment{}, ErrPaymentMethodNotFound
		}
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
	if s.rollDecline() {
		reason := s.pickReason()
		p.Status = StatusDeclined
		p.DeclineReason = &reason
	}

	created, err := s.repo.CreatePayment(ctx, p)
	if err != nil {
		return Payment{}, err
	}

	if in.WebhookURL != "" {
		eventType := "payment.approved"
		if created.Status == StatusDeclined {
			eventType = "payment.declined"
		}
		// Detach from the request context's cancellation (the HTTP response
		// has already been decided) but keep any request-scoped values.
		webhookCtx := context.WithoutCancel(ctx)
		go func() {
			if err := s.webhooks.Send(webhookCtx, in.WebhookURL, eventType, created); err != nil {
				log.Printf("webhook: send failed for payment %s: %v", created.ID, err)
			}
		}()
	}

	return created, nil
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

	return validateWebhookURL(in.WebhookURL)
}

func validateWebhookURL(webhookURL string) error {
	if webhookURL == "" {
		return nil
	}
	u, err := url.ParseRequestURI(webhookURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%w: webhook_url must be a valid http(s) URL", ErrValidation)
	}
	return nil
}
