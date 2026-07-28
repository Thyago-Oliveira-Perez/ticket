package payments

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
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
	// WebhookURL, if set, receives a payment.approved or payment.declined
	// event once the payment is created. Optional.
	WebhookURL string
	// IdempotencyKey, if set, makes payment creation safe to retry: a
	// second CreatePayment call for the same (merchant_id,
	// idempotency_key) pair returns the original payment instead of
	// creating a new one, and does not re-send its webhook. Optional.
	IdempotencyKey string
}

type Service interface {
	ListPayments(ctx context.Context) ([]Payment, error)
	CreatePayment(ctx context.Context, in CreatePaymentInput) (Payment, error)
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
	repo     Repository
	webhooks WebhookSender
	decline  DeclineConfig
	// rollDecline and pickReason are overridden in tests for determinism;
	// they default to real randomness via NewService.
	rollDecline func() bool
	pickReason  func() string
}

func NewService(repo Repository, webhooks WebhookSender, decline DeclineConfig) Service {
	return &svc{
		repo:     repo,
		webhooks: webhooks,
		decline:  decline,
		rollDecline: func() bool {
			return decline.Rate > 0 && rand.Float64() < decline.Rate
		},
		pickReason: func() string {
			return declineReasons[rand.IntN(len(declineReasons))]
		},
	}
}

func (s *svc) ListPayments(ctx context.Context) ([]Payment, error) {
	return s.repo.ListPayments(ctx)
}

func (s *svc) CreatePayment(ctx context.Context, in CreatePaymentInput) (Payment, error) {
	if err := validateCreatePaymentInput(in); err != nil {
		return Payment{}, err
	}

	if in.IdempotencyKey != "" {
		existing, err := s.repo.GetByIdempotencyKey(ctx, in.MerchantID, in.IdempotencyKey)
		if err == nil {
			// Replay of a prior request: return the original payment
			// as-is, without creating a new one or re-sending its webhook.
			return existing, nil
		}
		if !errors.Is(err, ErrPaymentNotFound) {
			return Payment{}, err
		}
	}

	p := Payment{
		MerchantID:      in.MerchantID,
		CustomerID:      in.CustomerID,
		PaymentMethodID: in.PaymentMethodID,
		AmountMinor:     in.AmountMinor,
		Currency:        in.Currency,
		Status:          StatusApproved,
	}
	if in.IdempotencyKey != "" {
		p.IdempotencyKey = &in.IdempotencyKey
	}
	if s.rollDecline() {
		reason := s.pickReason()
		p.Status = StatusDeclined
		p.DeclineReason = &reason
	}

	created, err := s.repo.CreatePayment(ctx, p)
	if err != nil {
		if errors.Is(err, ErrIdempotencyKeyConflict) {
			// Lost the race to a concurrent request with the same key;
			// return whatever it created instead of erroring out.
			return s.repo.GetByIdempotencyKey(ctx, in.MerchantID, in.IdempotencyKey)
		}
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
	if in.WebhookURL != "" {
		u, err := url.ParseRequestURI(in.WebhookURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("%w: webhook_url must be a valid http(s) URL", ErrValidation)
		}
	}

	return nil
}
