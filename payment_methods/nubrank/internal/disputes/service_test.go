package disputes

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"nubrank/internal/database"
	"nubrank/internal/events"
	"nubrank/internal/payments"

	"github.com/google/uuid"
)

const (
	testMerchantID = "11111111-1111-1111-1111-111111111111"
	testPaymentID  = "22222222-2222-2222-2222-222222222222"
)

type fakeRepository struct {
	mu       sync.Mutex
	disputes []Dispute
}

func (f *fakeRepository) WithQuerier(q database.Querier) Repository { return f }

func (f *fakeRepository) CreateDispute(ctx context.Context, d Dispute) (Dispute, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d.ID = uuid.NewString()
	f.disputes = append(f.disputes, d)
	return d, nil
}

func (f *fakeRepository) UpdateStatus(ctx context.Context, id, status string) (Dispute, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, d := range f.disputes {
		if d.ID == id {
			f.disputes[i].Status = status
			return f.disputes[i], nil
		}
	}
	return Dispute{}, errors.New("not found")
}

func (f *fakeRepository) ListByPayment(ctx context.Context, paymentID string) ([]Dispute, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Dispute
	for _, d := range f.disputes {
		if d.PaymentID == paymentID {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeRepository) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.disputes)
}

type fakePaymentsRepository struct {
	payment payments.Payment
}

func (f *fakePaymentsRepository) WithQuerier(q database.Querier) payments.Repository { return f }
func (f *fakePaymentsRepository) ListPayments(ctx context.Context, merchantID string) ([]payments.Payment, error) {
	return nil, nil
}
func (f *fakePaymentsRepository) CreatePayment(ctx context.Context, p payments.Payment) (payments.Payment, error) {
	return payments.Payment{}, nil
}
func (f *fakePaymentsRepository) GetByID(ctx context.Context, merchantID, id string) (payments.Payment, error) {
	if id == f.payment.ID && merchantID == f.payment.MerchantID {
		return f.payment, nil
	}
	return payments.Payment{}, payments.ErrPaymentNotFound
}
func (f *fakePaymentsRepository) LockForUpdate(ctx context.Context, merchantID, id string) (payments.Payment, error) {
	return f.GetByID(ctx, merchantID, id)
}
func (f *fakePaymentsRepository) UpdateStatus(ctx context.Context, merchantID, id, status string) (payments.Payment, error) {
	return payments.Payment{}, nil
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

type fakeLedger struct {
	mu     sync.Mutex
	posted []int64
}

func (f *fakeLedger) Post(ctx context.Context, q database.Querier, merchantID, referenceType, referenceID, kind string, merchantAccountDelta int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posted = append(f.posted, merchantAccountDelta)
	return nil
}

func newTestSvc(repo Repository, paymentsRepo payments.Repository, ledger LedgerPoster, pub events.Publisher, cfg Config) *svc {
	return &svc{
		repo:     repo,
		payments: paymentsRepo,
		tx:       fakeTxRunner{},
		events:   pub,
		ledger:   ledger,
		cfg:      cfg,
		rollDispute: func() bool {
			return cfg.Rate > 0
		},
		rollWon: func() bool {
			return cfg.WinRate >= 1
		},
	}
}

func TestMaybeDispute_RateZero_NeverDisputes(t *testing.T) {
	repo := &fakeRepository{}
	s := newTestSvc(repo, &fakePaymentsRepository{}, &fakeLedger{}, &fakeEventPublisher{}, Config{Rate: 0})

	s.MaybeDispute(context.Background(), testMerchantID, testPaymentID, 5000)
	time.Sleep(20 * time.Millisecond)

	if repo.count() != 0 {
		t.Fatalf("expected no disputes with Rate=0, got %d", repo.count())
	}
}

func TestOpen_CreatesDisputeAndHoldsFunds(t *testing.T) {
	repo := &fakeRepository{}
	ledger := &fakeLedger{}
	s := newTestSvc(repo, &fakePaymentsRepository{}, ledger, &fakeEventPublisher{}, Config{Rate: 1, WinRate: 1})

	dispute, err := s.open(context.Background(), testMerchantID, testPaymentID, 5000)
	if err != nil {
		t.Fatalf("open returned error: %v", err)
	}
	if dispute.Status != StatusNeedsResponse {
		t.Fatalf("expected status %q, got %q", StatusNeedsResponse, dispute.Status)
	}
	if len(ledger.posted) != 1 || ledger.posted[0] != -5000 {
		t.Fatalf("expected a single -5000 ledger posting (funds held), got %v", ledger.posted)
	}
}

func TestResolve_Won_ReleasesFunds(t *testing.T) {
	repo := &fakeRepository{}
	ledger := &fakeLedger{}
	pub := &fakeEventPublisher{}
	s := newTestSvc(repo, &fakePaymentsRepository{}, ledger, pub, Config{Rate: 1, WinRate: 1})

	dispute, err := s.open(context.Background(), testMerchantID, testPaymentID, 5000)
	if err != nil {
		t.Fatalf("open returned error: %v", err)
	}

	if err := s.resolve(context.Background(), testMerchantID, dispute, 5000); err != nil {
		t.Fatalf("resolve returned error: %v", err)
	}

	disputeList, _ := repo.ListByPayment(context.Background(), testPaymentID)
	if len(disputeList) != 1 || disputeList[0].Status != StatusWon {
		t.Fatalf("expected status %q, got %v", StatusWon, disputeList)
	}
	// One -5000 hold, one +5000 release: net zero.
	var sum int64
	for _, p := range ledger.posted {
		sum += p
	}
	if sum != 0 {
		t.Fatalf("expected hold+release to net to zero, got sum %d from %v", sum, ledger.posted)
	}
	if len(pub.eventType) != 2 || pub.eventType[1] != "dispute.won" {
		t.Fatalf("expected dispute.created then dispute.won, got %v", pub.eventType)
	}
}

func TestResolve_Lost_KeepsFundsHeld(t *testing.T) {
	repo := &fakeRepository{}
	ledger := &fakeLedger{}
	pub := &fakeEventPublisher{}
	s := newTestSvc(repo, &fakePaymentsRepository{}, ledger, pub, Config{Rate: 1, WinRate: 0})

	dispute, err := s.open(context.Background(), testMerchantID, testPaymentID, 5000)
	if err != nil {
		t.Fatalf("open returned error: %v", err)
	}

	if err := s.resolve(context.Background(), testMerchantID, dispute, 5000); err != nil {
		t.Fatalf("resolve returned error: %v", err)
	}

	disputeList, _ := repo.ListByPayment(context.Background(), testPaymentID)
	if len(disputeList) != 1 || disputeList[0].Status != StatusLost {
		t.Fatalf("expected status %q, got %v", StatusLost, disputeList)
	}
	// Only the original -5000 hold; no release posted.
	if len(ledger.posted) != 1 || ledger.posted[0] != -5000 {
		t.Fatalf("expected funds to stay held (no release), got %v", ledger.posted)
	}
	if len(pub.eventType) != 2 || pub.eventType[1] != "dispute.lost" {
		t.Fatalf("expected dispute.created then dispute.lost, got %v", pub.eventType)
	}
}

func TestListDisputes_ReturnsRecorded(t *testing.T) {
	repo := &fakeRepository{}
	paymentsRepo := &fakePaymentsRepository{payment: payments.Payment{ID: testPaymentID, MerchantID: testMerchantID}}
	s := newTestSvc(repo, paymentsRepo, &fakeLedger{}, &fakeEventPublisher{}, Config{Rate: 1, WinRate: 1})

	if _, err := s.open(context.Background(), testMerchantID, testPaymentID, 5000); err != nil {
		t.Fatalf("open returned error: %v", err)
	}

	list, err := s.ListDisputes(context.Background(), testMerchantID, testPaymentID)
	if err != nil {
		t.Fatalf("ListDisputes returned error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly one dispute, got %d", len(list))
	}
}

func TestListDisputes_UnknownPayment_ReturnsNotFound(t *testing.T) {
	s := newTestSvc(&fakeRepository{}, &fakePaymentsRepository{}, &fakeLedger{}, &fakeEventPublisher{}, Config{})

	_, err := s.ListDisputes(context.Background(), testMerchantID, "99999999-9999-9999-9999-999999999999")
	if !errors.Is(err, payments.ErrPaymentNotFound) {
		t.Fatalf("expected payments.ErrPaymentNotFound, got %v", err)
	}
}

func TestListDisputes_InvalidID_ReturnsValidationError(t *testing.T) {
	s := newTestSvc(&fakeRepository{}, &fakePaymentsRepository{}, &fakeLedger{}, &fakeEventPublisher{}, Config{})

	_, err := s.ListDisputes(context.Background(), testMerchantID, "not-a-uuid")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}
