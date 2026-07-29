package payments

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
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
	if p.IdempotencyKey != nil {
		for _, existing := range f.payments {
			if existing.MerchantID == p.MerchantID && existing.IdempotencyKey != nil && *existing.IdempotencyKey == *p.IdempotencyKey {
				return Payment{}, ErrIdempotencyKeyConflict
			}
		}
	}
	p.ID = uuid.NewString()
	f.payments = append(f.payments, p)
	return p, nil
}

func (f *fakeRepository) GetByIdempotencyKey(ctx context.Context, merchantID, idempotencyKey string) (Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.payments {
		if p.MerchantID == merchantID && p.IdempotencyKey != nil && *p.IdempotencyKey == idempotencyKey {
			return p, nil
		}
	}
	return Payment{}, ErrPaymentNotFound
}

func (f *fakeRepository) GetByID(ctx context.Context, id string) (Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.payments {
		if p.ID == id {
			return p, nil
		}
	}
	return Payment{}, ErrPaymentNotFound
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

func TestCreatePayment_RepeatedIdempotencyKey_ReturnsOriginalPayment(t *testing.T) {
	repo := &fakeRepository{}
	s := NewService(repo, newFakeWebhookSender(), DeclineConfig{})

	in := validInput()
	in.IdempotencyKey = "retry-key-1"

	first, err := s.CreatePayment(context.Background(), in)
	if err != nil {
		t.Fatalf("first CreatePayment returned error: %v", err)
	}

	second, err := s.CreatePayment(context.Background(), in)
	if err != nil {
		t.Fatalf("second CreatePayment returned error: %v", err)
	}

	if second.ID != first.ID {
		t.Fatalf("expected replay to return the same payment id %q, got %q", first.ID, second.ID)
	}
	if len(repo.payments) != 1 {
		t.Fatalf("expected exactly 1 payment to be created, got %d", len(repo.payments))
	}
}

func TestCreatePayment_RepeatedIdempotencyKey_DoesNotResendWebhook(t *testing.T) {
	webhooks := newFakeWebhookSender()
	s := NewService(&fakeRepository{}, webhooks, DeclineConfig{})

	in := validInput()
	in.IdempotencyKey = "retry-key-2"
	in.WebhookURL = "https://example.com/hook"

	if _, err := s.CreatePayment(context.Background(), in); err != nil {
		t.Fatalf("first CreatePayment returned error: %v", err)
	}
	<-webhooks.done

	if _, err := s.CreatePayment(context.Background(), in); err != nil {
		t.Fatalf("second CreatePayment returned error: %v", err)
	}

	webhooks.mu.Lock()
	defer webhooks.mu.Unlock()
	if len(webhooks.sent) != 1 {
		t.Fatalf("expected exactly 1 webhook delivery across both requests, got %d", len(webhooks.sent))
	}
}

func TestCreatePayment_DifferentMerchantsSameIdempotencyKey_BothCreated(t *testing.T) {
	repo := &fakeRepository{}
	s := NewService(repo, newFakeWebhookSender(), DeclineConfig{})

	in1 := validInput()
	in1.IdempotencyKey = "shared-key"

	in2 := validInput()
	in2.IdempotencyKey = "shared-key"
	in2.MerchantID = "44444444-4444-4444-4444-444444444444"

	if _, err := s.CreatePayment(context.Background(), in1); err != nil {
		t.Fatalf("CreatePayment for merchant 1 returned error: %v", err)
	}
	if _, err := s.CreatePayment(context.Background(), in2); err != nil {
		t.Fatalf("CreatePayment for merchant 2 returned error: %v", err)
	}

	if len(repo.payments) != 2 {
		t.Fatalf("expected 2 payments (idempotency key is scoped per merchant), got %d", len(repo.payments))
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

func TestGetPayment_ExistingID_ReturnsPayment(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{})

	created, err := s.CreatePayment(context.Background(), validInput())
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	got, err := s.GetPayment(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetPayment returned error: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected payment id %q, got %q", created.ID, got.ID)
	}
}

func TestGetPayment_UnknownID_ReturnsNotFound(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{})

	_, err := s.GetPayment(context.Background(), "11111111-1111-1111-1111-111111111111")
	if !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("expected ErrPaymentNotFound, got %v", err)
	}
}

func TestGetPayment_InvalidID_ReturnsValidationError(t *testing.T) {
	s := NewService(&fakeRepository{}, newFakeWebhookSender(), DeclineConfig{})

	_, err := s.GetPayment(context.Background(), "not-a-uuid")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}
