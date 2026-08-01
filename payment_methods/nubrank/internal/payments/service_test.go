package payments

import (
	"context"
	"errors"
	"sync"
	"testing"

	"nubrank/internal/customers"
	"nubrank/internal/database"
	"nubrank/internal/events"
	"nubrank/internal/paymentmethods"

	"github.com/google/uuid"
)

type fakeRepository struct {
	mu       sync.Mutex
	payments []Payment
}

// WithQuerier is a no-op for the fake: it isn't backed by a real database,
// so there's no transaction to bind to.
func (f *fakeRepository) WithQuerier(q database.Querier) Repository { return f }

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

func (f *fakeRepository) LockForUpdate(ctx context.Context, merchantID, id string) (Payment, error) {
	return f.GetByID(ctx, merchantID, id)
}

func (f *fakeRepository) UpdateStatus(ctx context.Context, merchantID, id, status string) (Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, p := range f.payments {
		if p.ID == id && p.MerchantID == merchantID {
			f.payments[i].Status = status
			return f.payments[i], nil
		}
	}
	return Payment{}, ErrPaymentNotFound
}

// fakeTxRunner runs fn directly with a nil Querier: none of the fakes in
// this file are backed by a real database, so there's no transaction to
// actually start.
type fakeTxRunner struct{}

func (fakeTxRunner) RunInTx(ctx context.Context, fn func(q database.Querier) error) error {
	return fn(nil)
}

type publishedEvent struct {
	merchantID string
	eventType  string
	resourceID string
	payload    any
}

type fakeEventPublisher struct {
	mu        sync.Mutex
	published []publishedEvent
}

func newFakeEventPublisher() *fakeEventPublisher {
	return &fakeEventPublisher{}
}

func (f *fakeEventPublisher) Publish(ctx context.Context, q database.Querier, merchantID, eventType, resourceID string, payload any) ([]events.Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, publishedEvent{merchantID, eventType, resourceID, payload})
	return nil, nil
}

func (f *fakeEventPublisher) Dispatch(ctx context.Context, deliveries []events.Delivery) {}

func (f *fakeEventPublisher) eventTypes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var types []string
	for _, e := range f.published {
		types = append(types, e.eventType)
	}
	return types
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

// fakeLedgerPoster is a no-op: payments tests care about payment/event
// behavior, not ledger postings (covered separately in internal/ledger).
type fakeLedgerPoster struct{}

func (fakeLedgerPoster) Post(ctx context.Context, q database.Querier, merchantID, referenceType, referenceID, kind string, merchantAccountDelta int64) error {
	return nil
}

// fakeDisputeSimulator is a no-op: payments tests care about payment/event
// behavior, not dispute simulation (covered separately in internal/disputes).
type fakeDisputeSimulator struct{}

func (fakeDisputeSimulator) MaybeDispute(ctx context.Context, merchantID, paymentID string, amountMinor int64) {
}

func newTestService(repo Repository, pub events.Publisher, decline DeclineConfig, customerVerifier CustomerVerifier, paymentMethodVerifier PaymentMethodVerifier) Service {
	return NewService(repo, fakeTxRunner{}, pub, fakeLedgerPoster{}, fakeDisputeSimulator{}, decline, customerVerifier, paymentMethodVerifier)
}

func TestCreatePayment_DeclineDisabled_AlwaysApproved(t *testing.T) {
	s := newTestService(&fakeRepository{}, newFakeEventPublisher(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

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
	s := newTestService(&fakeRepository{}, newFakeEventPublisher(), DeclineConfig{Rate: 1}, alwaysOKVerifier{}, alwaysOKVerifier{})

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

func TestCreatePayment_DeclinedPayment_PublishesDeclinedEvent(t *testing.T) {
	pub := newFakeEventPublisher()
	s := newTestService(&fakeRepository{}, pub, DeclineConfig{Rate: 1}, alwaysOKVerifier{}, alwaysOKVerifier{})

	if _, err := s.CreatePayment(context.Background(), validInput()); err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	types := pub.eventTypes()
	if len(types) != 1 || types[0] != "payment.declined" {
		t.Fatalf("expected exactly one payment.declined event, got %v", types)
	}
}

func TestCreatePayment_ApprovedPayment_PublishesApprovedEvent(t *testing.T) {
	pub := newFakeEventPublisher()
	s := newTestService(&fakeRepository{}, pub, DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	if _, err := s.CreatePayment(context.Background(), validInput()); err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	types := pub.eventTypes()
	if len(types) != 1 || types[0] != "payment.approved" {
		t.Fatalf("expected exactly one payment.approved event, got %v", types)
	}
}

func TestCreatePayment_InvalidInput_ReturnsValidationError(t *testing.T) {
	s := newTestService(&fakeRepository{}, newFakeEventPublisher(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	in := validInput()
	in.AmountMinor = 0
	if _, err := s.CreatePayment(context.Background(), in); err == nil {
		t.Fatal("expected an error for zero amount, got nil")
	}
}

func TestCreatePayment_UnknownCustomer_ReturnsCustomerNotFound(t *testing.T) {
	s := newTestService(&fakeRepository{}, newFakeEventPublisher(), DeclineConfig{}, notFoundVerifier{customers.ErrCustomerNotFound}, alwaysOKVerifier{})

	_, err := s.CreatePayment(context.Background(), validInput())
	if !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("expected ErrCustomerNotFound, got %v", err)
	}
}

func TestCreatePayment_UnknownPaymentMethod_ReturnsPaymentMethodNotFound(t *testing.T) {
	s := newTestService(&fakeRepository{}, newFakeEventPublisher(), DeclineConfig{}, alwaysOKVerifier{}, notFoundVerifier{paymentmethods.ErrPaymentMethodNotFound})

	_, err := s.CreatePayment(context.Background(), validInput())
	if !errors.Is(err, ErrPaymentMethodNotFound) {
		t.Fatalf("expected ErrPaymentMethodNotFound, got %v", err)
	}
}

func TestGetPayment_ExistingID_ReturnsPayment(t *testing.T) {
	s := newTestService(&fakeRepository{}, newFakeEventPublisher(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

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
	s := newTestService(&fakeRepository{}, newFakeEventPublisher(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	_, err := s.GetPayment(context.Background(), defaultMerchantID, "11111111-1111-1111-1111-111111111111")
	if !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("expected ErrPaymentNotFound, got %v", err)
	}
}

func TestGetPayment_InvalidID_ReturnsValidationError(t *testing.T) {
	s := newTestService(&fakeRepository{}, newFakeEventPublisher(), DeclineConfig{}, alwaysOKVerifier{}, alwaysOKVerifier{})

	_, err := s.GetPayment(context.Background(), defaultMerchantID, "not-a-uuid")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}
