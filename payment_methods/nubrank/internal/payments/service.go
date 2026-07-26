package payments

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"

	"github.com/google/uuid"
)

// ErrValidation marks errors caused by invalid caller input, as opposed to
// infrastructure failures. Handlers use errors.Is against it to pick the
// right HTTP status.
var ErrValidation = errors.New("validation error")

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
	// WebhookURL, if set, receives a payment.approved event once the
	// payment is created. Optional.
	WebhookURL string
}

type Service interface {
	ListPayments(ctx context.Context) ([]Payment, error)
	CreatePayment(ctx context.Context, in CreatePaymentInput) (Payment, error)
}

type svc struct {
	repo     Repository
	webhooks WebhookSender
}

func NewService(repo Repository, webhooks WebhookSender) Service {
	return &svc{repo: repo, webhooks: webhooks}
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

	created, err := s.repo.CreatePayment(ctx, p)
	if err != nil {
		return Payment{}, err
	}

	if in.WebhookURL != "" {
		// Detach from the request context's cancellation (the HTTP response
		// has already been decided) but keep any request-scoped values.
		webhookCtx := context.WithoutCancel(ctx)
		go func() {
			if err := s.webhooks.Send(webhookCtx, in.WebhookURL, "payment.approved", created); err != nil {
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
	if in.WebhookURL != "" {
		u, err := url.ParseRequestURI(in.WebhookURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("%w: webhook_url must be a valid http(s) URL", ErrValidation)
		}
	}

	return nil
}
