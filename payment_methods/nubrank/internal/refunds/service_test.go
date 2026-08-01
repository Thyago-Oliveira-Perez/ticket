package refunds

import (
	"context"
	"errors"
	"sync"
	"testing"

	"nubrank/internal/database"
	"nubrank/internal/events"
	"nubrank/internal/payments"

	"github.com/google/uuid"
)

const (
	testMerchantID = "11111111-1111-1111-1111-111111111111"
	testPaymentID  = "22222222-2222-2222-2222-222222222222"
)

type fakePaymentsRepository struct {
	mu       sync.Mutex
	payments map[string]payments.Payment
}

func newFakePaymentsRepository(p payments.Payment) *fakePaymentsRepository {
	return &fakePaymentsRepository{payments: map[string]payments.Payment{p.ID: p}}
}

func (f *fakePaymentsRepository) WithQuerier(q database.Querier) payments.Repository { return f }

func (f *fakePaymentsRepository) ListPayments(ctx context.Context, merchantID string) ([]payments.Payment, error) {
	return nil, nil
}

func (f *fakePaymentsRepository) CreatePayment(ctx context.Context, p payments.Payment) (payments.Payment, error) {
	return payments.Payment{}, nil
}

func (f *fakePaymentsRepository) GetByID(ctx context.Context, merchantID, id string) (payments.Payment, error) {
	return f.LockForUpdate(ctx, merchantID, id)
}

func (f *fakePaymentsRepository) LockForUpdate(ctx context.Context, merchantID, id string) (payments.Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.payments[id]
	if !ok || p.MerchantID != merchantID {
		return payments.Payment{}, payments.ErrPaymentNotFound
	}
	return p, nil
}

func (f *fakePaymentsRepository) UpdateStatus(ctx context.Context, merchantID, id, status string) (payments.Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.payments[id]
	if !ok || p.MerchantID != merchantID {
		return payments.Payment{}, payments.ErrPaymentNotFound
	}
	p.Status = status
	f.payments[id] = p
	return p, nil
}

type fakeRefundsRepository struct {
	mu      sync.Mutex
	refunds []Refund
}

func (f *fakeRefundsRepository) WithQuerier(q database.Querier) Repository { return f }

func (f *fakeRefundsRepository) CreateRefund(ctx context.Context, r Refund) (Refund, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r.ID = uuid.NewString()
	f.refunds = append(f.refunds, r)
	return r, nil
}

func (f *fakeRefundsRepository) SumByPayment(ctx context.Context, paymentID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var sum int64
	for _, r := range f.refunds {
		if r.PaymentID == paymentID {
			sum += r.AmountMinor
		}
	}
	return sum, nil
}

func (f *fakeRefundsRepository) ListByPayment(ctx context.Context, paymentID string) ([]Refund, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Refund
	for _, r := range f.refunds {
		if r.PaymentID == paymentID {
			out = append(out, r)
		}
	}
	return out, nil
}

type fakeTxRunner struct{}

func (fakeTxRunner) RunInTx(ctx context.Context, fn func(q database.Querier) error) error {
	return fn(nil)
}

type fakeEventPublisher struct {
	mu        sync.Mutex
	eventType []string
}

func (f *fakeEventPublisher) Publish(ctx context.Context, q database.Querier, merchantID, eventType, resourceID string, payload any) ([]events.Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.eventType = append(f.eventType, eventType)
	return nil, nil
}

func (f *fakeEventPublisher) Dispatch(ctx context.Context, deliveries []events.Delivery) {}

// fakeLedgerPoster is a no-op: refunds tests care about refund/payment
// behavior, not ledger postings (covered separately in internal/ledger).
type fakeLedgerPoster struct{}

func (fakeLedgerPoster) Post(ctx context.Context, q database.Querier, merchantID, referenceType, referenceID, kind string, merchantAccountDelta int64) error {
	return nil
}

func newTestService(refundsRepo Repository, paymentsRepo payments.Repository, pub events.Publisher) Service {
	return NewService(refundsRepo, paymentsRepo, fakeTxRunner{}, pub, fakeLedgerPoster{})
}

func approvedPayment(amount int64) payments.Payment {
	return payments.Payment{ID: testPaymentID, MerchantID: testMerchantID, AmountMinor: amount, Status: payments.StatusApproved}
}

func TestCreateRefund_FullRefund_MarksPaymentRefunded(t *testing.T) {
	paymentsRepo := newFakePaymentsRepository(approvedPayment(5000))
	s := newTestService(&fakeRefundsRepository{}, paymentsRepo, &fakeEventPublisher{})

	refund, err := s.CreateRefund(context.Background(), testMerchantID, testPaymentID, CreateRefundInput{})
	if err != nil {
		t.Fatalf("CreateRefund returned error: %v", err)
	}
	if refund.AmountMinor != 5000 {
		t.Fatalf("expected full refund of 5000, got %d", refund.AmountMinor)
	}

	p, _ := paymentsRepo.GetByID(context.Background(), testMerchantID, testPaymentID)
	if p.Status != payments.StatusRefunded {
		t.Fatalf("expected payment status %q, got %q", payments.StatusRefunded, p.Status)
	}
}

func TestCreateRefund_PartialRefund_MarksPaymentPartiallyRefunded(t *testing.T) {
	paymentsRepo := newFakePaymentsRepository(approvedPayment(5000))
	s := newTestService(&fakeRefundsRepository{}, paymentsRepo, &fakeEventPublisher{})

	refund, err := s.CreateRefund(context.Background(), testMerchantID, testPaymentID, CreateRefundInput{AmountMinor: 2000})
	if err != nil {
		t.Fatalf("CreateRefund returned error: %v", err)
	}
	if refund.AmountMinor != 2000 {
		t.Fatalf("expected refund of 2000, got %d", refund.AmountMinor)
	}

	p, _ := paymentsRepo.GetByID(context.Background(), testMerchantID, testPaymentID)
	if p.Status != payments.StatusPartiallyRefunded {
		t.Fatalf("expected payment status %q, got %q", payments.StatusPartiallyRefunded, p.Status)
	}
}

func TestCreateRefund_SequentialPartialRefunds_LastOneFullyRefunds(t *testing.T) {
	paymentsRepo := newFakePaymentsRepository(approvedPayment(5000))
	s := newTestService(&fakeRefundsRepository{}, paymentsRepo, &fakeEventPublisher{})

	if _, err := s.CreateRefund(context.Background(), testMerchantID, testPaymentID, CreateRefundInput{AmountMinor: 2000}); err != nil {
		t.Fatalf("first CreateRefund returned error: %v", err)
	}
	// Second refund omits amount_minor, so it should default to the
	// remaining 3000 rather than erroring or refunding 0.
	refund, err := s.CreateRefund(context.Background(), testMerchantID, testPaymentID, CreateRefundInput{})
	if err != nil {
		t.Fatalf("second CreateRefund returned error: %v", err)
	}
	if refund.AmountMinor != 3000 {
		t.Fatalf("expected remaining refund of 3000, got %d", refund.AmountMinor)
	}

	p, _ := paymentsRepo.GetByID(context.Background(), testMerchantID, testPaymentID)
	if p.Status != payments.StatusRefunded {
		t.Fatalf("expected payment status %q, got %q", payments.StatusRefunded, p.Status)
	}
}

func TestCreateRefund_ExceedsRemaining_ReturnsError(t *testing.T) {
	paymentsRepo := newFakePaymentsRepository(approvedPayment(5000))
	s := newTestService(&fakeRefundsRepository{}, paymentsRepo, &fakeEventPublisher{})

	_, err := s.CreateRefund(context.Background(), testMerchantID, testPaymentID, CreateRefundInput{AmountMinor: 6000})
	if !errors.Is(err, ErrRefundExceedsRemaining) {
		t.Fatalf("expected ErrRefundExceedsRemaining, got %v", err)
	}
}

func TestCreateRefund_DeclinedPayment_ReturnsNotRefundable(t *testing.T) {
	p := approvedPayment(5000)
	p.Status = payments.StatusDeclined
	paymentsRepo := newFakePaymentsRepository(p)
	s := newTestService(&fakeRefundsRepository{}, paymentsRepo, &fakeEventPublisher{})

	_, err := s.CreateRefund(context.Background(), testMerchantID, testPaymentID, CreateRefundInput{})
	if !errors.Is(err, ErrPaymentNotRefundable) {
		t.Fatalf("expected ErrPaymentNotRefundable, got %v", err)
	}
}

func TestCreateRefund_UnknownPayment_ReturnsNotFound(t *testing.T) {
	paymentsRepo := newFakePaymentsRepository(approvedPayment(5000))
	s := newTestService(&fakeRefundsRepository{}, paymentsRepo, &fakeEventPublisher{})

	_, err := s.CreateRefund(context.Background(), testMerchantID, "99999999-9999-9999-9999-999999999999", CreateRefundInput{})
	if !errors.Is(err, payments.ErrPaymentNotFound) {
		t.Fatalf("expected payments.ErrPaymentNotFound, got %v", err)
	}
}

func TestCreateRefund_InvalidID_ReturnsValidationError(t *testing.T) {
	paymentsRepo := newFakePaymentsRepository(approvedPayment(5000))
	s := newTestService(&fakeRefundsRepository{}, paymentsRepo, &fakeEventPublisher{})

	_, err := s.CreateRefund(context.Background(), testMerchantID, "not-a-uuid", CreateRefundInput{})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestCreateRefund_NegativeAmount_ReturnsValidationError(t *testing.T) {
	paymentsRepo := newFakePaymentsRepository(approvedPayment(5000))
	s := newTestService(&fakeRefundsRepository{}, paymentsRepo, &fakeEventPublisher{})

	_, err := s.CreateRefund(context.Background(), testMerchantID, testPaymentID, CreateRefundInput{AmountMinor: -1})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestCreateRefund_PublishesRefundedEvent(t *testing.T) {
	paymentsRepo := newFakePaymentsRepository(approvedPayment(5000))
	pub := &fakeEventPublisher{}
	s := newTestService(&fakeRefundsRepository{}, paymentsRepo, pub)

	if _, err := s.CreateRefund(context.Background(), testMerchantID, testPaymentID, CreateRefundInput{}); err != nil {
		t.Fatalf("CreateRefund returned error: %v", err)
	}

	if len(pub.eventType) != 1 || pub.eventType[0] != "payment.refunded" {
		t.Fatalf("expected exactly one payment.refunded event, got %v", pub.eventType)
	}
}

func TestListRefunds_ReturnsRecordedRefunds(t *testing.T) {
	paymentsRepo := newFakePaymentsRepository(approvedPayment(5000))
	s := newTestService(&fakeRefundsRepository{}, paymentsRepo, &fakeEventPublisher{})

	if _, err := s.CreateRefund(context.Background(), testMerchantID, testPaymentID, CreateRefundInput{AmountMinor: 1000}); err != nil {
		t.Fatalf("CreateRefund returned error: %v", err)
	}

	list, err := s.ListRefunds(context.Background(), testMerchantID, testPaymentID)
	if err != nil {
		t.Fatalf("ListRefunds returned error: %v", err)
	}
	if len(list) != 1 || list[0].AmountMinor != 1000 {
		t.Fatalf("expected exactly one 1000 refund, got %v", list)
	}
}
