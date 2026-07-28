package payments

import (
	"context"
	"sync"
	"testing"
)

type fakeRepository struct {
	mu       sync.Mutex
	payments []Payment
}

func (f *fakeRepository) ListPayments(ctx context.Context) ([]Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.payments, nil
}

func (f *fakeRepository) CreatePayment(ctx context.Context, p Payment) (Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p.ID = "fake-id"
	f.payments = append(f.payments, p)
	return p, nil
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

func validInput() CreatePaymentInput {
	return CreatePaymentInput{
		MerchantID:      "11111111-1111-1111-1111-111111111111",
		CustomerID:      "22222222-2222-2222-2222-222222222222",
		PaymentMethodID: "33333333-3333-3333-3333-333333333333",
		AmountMinor:     5000,
		Currency:        "BRL",
	}
}

func TestCreatePayment_DeclineDisabled_AlwaysApproved(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{})

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
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{Rate: 1})

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
	s := NewService(&fakeRepository{}, webhooks, DeclineConfig{Rate: 1})

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
	s := NewService(&fakeRepository{}, webhooks, DeclineConfig{})

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
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{})

	in := validInput()
	in.AmountMinor = 0
	if _, err := s.CreatePayment(context.Background(), in); err == nil {
		t.Fatal("expected an error for zero amount, got nil")
	}
}
