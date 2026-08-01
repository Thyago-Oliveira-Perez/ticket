package payments

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"nubrank/internal/customers"
	"nubrank/internal/paymentmethods"

	"github.com/google/uuid"
)

type fakeRepository struct {
	mu       sync.Mutex
	payments []Payment
}

func (f *fakeRepository) ListPayments(ctx context.Context, merchantID string) ([]Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Payment
	for _, p := range f.payments {
		if p.MerchantID == merchantID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeRepository) CreatePayment(ctx context.Context, p Payment) (Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p.ID = uuid.NewString()
	f.payments = append(f.payments, p)
	return p, nil
}

func (f *fakeRepository) GetByID(ctx context.Context, merchantID, id string) (Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.payments {
		if p.ID == id && p.MerchantID == merchantID {
			return p, nil
		}
	}
	return Payment{}, ErrPaymentNotFound
}

func (f *fakeRepository) RefundPayment(ctx context.Context, merchantID, id string) (Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, p := range f.payments {
		if p.ID != id || p.MerchantID != merchantID {
			continue
		}
		if p.Status != StatusApproved {
			return Payment{}, ErrRefundNotApplied
		}
		now := time.Now()
		f.payments[i].Status = StatusRefunded
		f.payments[i].RefundedAt = &now
		return f.payments[i], nil
	}
	return Payment{}, ErrRefundNotApplied
}

type capturedWebhook struct {
	url       string
	eventType string
	data      any
}

type fakeWebhookSender struct {
	mu   sync.Mutex
	sent []capturedWebhook
	done chan struct{}
}

func newFakeWebhookSender() *fakeWebhookSender {
	return &fakeWebhookSender{done: make(chan struct{}, 1)}
}

func (f *fakeWebhookSender) Send(ctx context.Context, url, eventType string, data any) error {
	f.mu.Lock()
	f.sent = append(f.sent, capturedWebhook{url: url, eventType: eventType, data: data})
	f.mu.Unlock()
	f.done <- struct{}{}
	return nil
}

const defaultMerchantID = "11111111-1111-1111-1111-111111111111"

// alwaysOKVerifier satisfies both CustomerVerifier and PaymentMethodVerifier,
// used by tests that don't exercise ownership-validation failures.
type alwaysOKVerifier struct{}

func (alwaysOKVerifier) VerifyOwnership(ctx context.Context, a, b string) error { return nil }

// notFoundVerifier always fails ownership, returning whatever error it's
// constructed with (customers.ErrCustomerNotFound or
// paymentmethods.ErrPaymentMethodNotFound in practice).
type notFoundVerifier struct{ err error }

func (v notFoundVerifier) VerifyOwnership(ctx context.Context, a, b string) error { return v.err }

func validInput() CreatePaymentInput {
	return CreatePaymentInput{
		MerchantID:      defaultMerchantID,
		CustomerID:      "22222222-2222-2222-2222-222222222222",
		PaymentMethodID: "33333333-3333-3333-3333-333333333333",
		AmountMinor:     5000,
		Currency:        "BRL",
	}
}

func TestCreatePayment_DeclineDisabled_AlwaysApproved(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	p, err := s.CreatePayment(context.Background(), validInput())
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	if p.Status != StatusApproved {
		t.Fatalf("expected status %q, got %q", StatusApproved, p.Status)
	}
	if p.DeclineReason != nil {
		t.Fatalf("expected no decline reason, got %v", *p.DeclineReason)
	}
}

func TestCreatePayment_DeclineRateOne_AlwaysDeclined(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{Rate: 1}, alwaysOKVerifier{}, alwaysOKVerifier{})

	p, err := s.CreatePayment(context.Background(), validInput())
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	if p.Status != StatusDeclined {
		t.Fatalf("expected status %q, got %q", StatusDeclined, p.Status)
	}
	if p.DeclineReason == nil || *p.DeclineReason == "" {
		t.Fatalf("expected a decline reason, got %v", p.DeclineReason)
	}
}

func TestCreatePayment_DeclinedPayment_SendsDeclinedWebhook(t *testing.T) {
	webhooks := newFakeWebhookSender()
	s := NewService(&fakeRepository{}, webhooks, DeclineConfig{Rate: 1}, alwaysOKVerifier{}, alwaysOKVerifier{})

	in := validInput()
	in.WebhookURL = "https://example.com/hook"
	if _, err := s.CreatePayment(context.Background(), in); err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	<-webhooks.done
	webhooks.mu.Lock()
	defer webhooks.mu.Unlock()
	if len(webhooks.sent) != 1 {
		t.Fatalf("expected exactly 1 webhook delivery, got %d", len(webhooks.sent))
	}
	if webhooks.sent[0].eventType != "payment.declined" {
		t.Fatalf("expected event type payment.declined, got %s", webhooks.sent[0].eventType)
	}
}

func TestCreatePayment_ApprovedPayment_SendsApprovedWebhook(t *testing.T) {
	webhooks := newFakeWebhookSender()
	s := NewService(&fakeRepository{}, webhooks, DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	in := validInput()
	in.WebhookURL = "https://example.com/hook"
	if _, err := s.CreatePayment(context.Background(), in); err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	<-webhooks.done
	webhooks.mu.Lock()
	defer webhooks.mu.Unlock()
	if len(webhooks.sent) != 1 {
		t.Fatalf("expected exactly 1 webhook delivery, got %d", len(webhooks.sent))
	}
	if webhooks.sent[0].eventType != "payment.approved" {
		t.Fatalf("expected event type payment.approved, got %s", webhooks.sent[0].eventType)
	}
}

func TestCreatePayment_InvalidInput_ReturnsValidationError(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	in := validInput()
	in.AmountMinor = 0
	if _, err := s.CreatePayment(context.Background(), in); err == nil {
		t.Fatal("expected an error for zero amount, got nil")
	}
}

func TestCreatePayment_UnknownCustomer_ReturnsCustomerNotFound(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{}, notFoundVerifier{customers.ErrCustomerNotFound}, alwaysOKVerifier{})

	_, err := s.CreatePayment(context.Background(), validInput())
	if !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("expected ErrCustomerNotFound, got %v", err)
	}
}

func TestCreatePayment_UnknownPaymentMethod_ReturnsPaymentMethodNotFound(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{}, alwaysOKVerifier{}, notFoundVerifier{paymentmethods.ErrPaymentMethodNotFound})

	_, err := s.CreatePayment(context.Background(), validInput())
	if !errors.Is(err, ErrPaymentMethodNotFound) {
		t.Fatalf("expected ErrPaymentMethodNotFound, got %v", err)
	}
}

func TestGetPayment_ExistingID_ReturnsPayment(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	created, err := s.CreatePayment(context.Background(), validInput())
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	got, err := s.GetPayment(context.Background(), defaultMerchantID, created.ID)
	if err != nil {
		t.Fatalf("GetPayment returned error: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected payment id %q, got %q", created.ID, got.ID)
	}
}

func TestGetPayment_UnknownID_ReturnsNotFound(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	_, err := s.GetPayment(context.Background(), defaultMerchantID, "11111111-1111-1111-1111-111111111111")
	if !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("expected ErrPaymentNotFound, got %v", err)
	}
}

func TestGetPayment_InvalidID_ReturnsValidationError(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	_, err := s.GetPayment(context.Background(), defaultMerchantID, "not-a-uuid")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestRefundPayment_ApprovedPayment_Refunds(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	created, err := s.CreatePayment(context.Background(), validInput())
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	refunded, err := s.RefundPayment(context.Background(), defaultMerchantID, created.ID, RefundInput{})
	if err != nil {
		t.Fatalf("RefundPayment returned error: %v", err)
	}
	if refunded.Status != StatusRefunded {
		t.Fatalf("expected status %q, got %q", StatusRefunded, refunded.Status)
	}
	if refunded.RefundedAt == nil {
		t.Fatal("expected RefundedAt to be set")
	}
}

func TestRefundPayment_AlreadyRefunded_ReturnsAlreadyRefundedError(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	created, err := s.CreatePayment(context.Background(), validInput())
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	if _, err := s.RefundPayment(context.Background(), defaultMerchantID, created.ID, RefundInput{}); err != nil {
		t.Fatalf("first RefundPayment returned error: %v", err)
	}

	_, err = s.RefundPayment(context.Background(), defaultMerchantID, created.ID, RefundInput{})
	if !errors.Is(err, ErrPaymentAlreadyRefunded) {
		t.Fatalf("expected ErrPaymentAlreadyRefunded, got %v", err)
	}
}

func TestRefundPayment_DeclinedPayment_ReturnsNotApprovedError(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{Rate: 1}, alwaysOKVerifier{}, alwaysOKVerifier{})

	created, err := s.CreatePayment(context.Background(), validInput())
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	if created.Status != StatusDeclined {
		t.Fatalf("expected test setup to produce a declined payment, got %q", created.Status)
	}

	_, err = s.RefundPayment(context.Background(), defaultMerchantID, created.ID, RefundInput{})
	if !errors.Is(err, ErrPaymentNotApproved) {
		t.Fatalf("expected ErrPaymentNotApproved, got %v", err)
	}
}

func TestRefundPayment_UnknownID_ReturnsNotFound(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	_, err := s.RefundPayment(context.Background(), defaultMerchantID, "11111111-1111-1111-1111-111111111111", RefundInput{})
	if !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("expected ErrPaymentNotFound, got %v", err)
	}
}

func TestRefundPayment_InvalidID_ReturnsValidationError(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	_, err := s.RefundPayment(context.Background(), defaultMerchantID, "not-a-uuid", RefundInput{})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestRefundPayment_SendsRefundedWebhook(t *testing.T) {
	webhooks := newFakeWebhookSender()
	s := NewService(&fakeRepository{}, webhooks, DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	created, err := s.CreatePayment(context.Background(), validInput())
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	if _, err := s.RefundPayment(context.Background(), defaultMerchantID, created.ID, RefundInput{WebhookURL: "https://example.com/hook"}); err != nil {
		t.Fatalf("RefundPayment returned error: %v", err)
	}

	<-webhooks.done
	webhooks.mu.Lock()
	defer webhooks.mu.Unlock()
	if len(webhooks.sent) != 1 {
		t.Fatalf("expected exactly 1 webhook delivery, got %d", len(webhooks.sent))
	}
	if webhooks.sent[0].eventType != "payment.refunded" {
		t.Fatalf("expected event type payment.refunded, got %s", webhooks.sent[0].eventType)
	}
}
