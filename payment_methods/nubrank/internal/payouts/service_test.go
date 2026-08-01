package payouts

import (
	"context"
	"errors"
	"sync"
	"testing"

	"nubrank/internal/database"
	"nubrank/internal/events"

	"github.com/google/uuid"
)

const testMerchantID = "11111111-1111-1111-1111-111111111111"

type fakeRepository struct {
	mu      sync.Mutex
	payouts []Payout
}

func (f *fakeRepository) WithQuerier(q database.Querier) Repository { return f }

func (f *fakeRepository) CreatePayout(ctx context.Context, p Payout) (Payout, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p.ID = uuid.NewString()
	f.payouts = append(f.payouts, p)
	return p, nil
}

func (f *fakeRepository) GetByID(ctx context.Context, merchantID, id string) (Payout, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.payouts {
		if p.ID == id && p.MerchantID == merchantID {
			return p, nil
		}
	}
	return Payout{}, ErrPayoutNotFound
}

func (f *fakeRepository) ListByMerchant(ctx context.Context, merchantID string) ([]Payout, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Payout
	for _, p := range f.payouts {
		if p.MerchantID == merchantID {
			out = append(out, p)
		}
	}
	return out, nil
}

// fakeLedger tracks a single merchant's balance directly, mirroring what a
// real ledger.Service would enforce (sufficiency check + debit) without
// needing a real double-entry backend.
type fakeLedger struct {
	mu      sync.Mutex
	balance int64
	posted  []int64
}

func (f *fakeLedger) LockMerchantBalance(ctx context.Context, q database.Querier, merchantID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.balance, nil
}

func (f *fakeLedger) Post(ctx context.Context, q database.Querier, merchantID, referenceType, referenceID, kind string, merchantAccountDelta int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.balance += merchantAccountDelta
	f.posted = append(f.posted, merchantAccountDelta)
	return nil
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

func TestCreatePayout_DefaultAmount_PaysOutFullBalance(t *testing.T) {
	ledger := &fakeLedger{balance: 5000}
	s := NewService(&fakeRepository{}, ledger, fakeTxRunner{}, &fakeEventPublisher{})

	payout, err := s.CreatePayout(context.Background(), testMerchantID, CreatePayoutInput{Currency: "BRL"})
	if err != nil {
		t.Fatalf("CreatePayout returned error: %v", err)
	}
	if payout.AmountMinor != 5000 {
		t.Fatalf("expected payout of full balance 5000, got %d", payout.AmountMinor)
	}
	if ledger.balance != 0 {
		t.Fatalf("expected ledger balance to be debited to 0, got %d", ledger.balance)
	}
}

func TestCreatePayout_PartialAmount_DebitsOnlyThatAmount(t *testing.T) {
	ledger := &fakeLedger{balance: 5000}
	s := NewService(&fakeRepository{}, ledger, fakeTxRunner{}, &fakeEventPublisher{})

	payout, err := s.CreatePayout(context.Background(), testMerchantID, CreatePayoutInput{AmountMinor: 2000, Currency: "BRL"})
	if err != nil {
		t.Fatalf("CreatePayout returned error: %v", err)
	}
	if payout.AmountMinor != 2000 {
		t.Fatalf("expected payout of 2000, got %d", payout.AmountMinor)
	}
	if ledger.balance != 3000 {
		t.Fatalf("expected remaining ledger balance 3000, got %d", ledger.balance)
	}
}

func TestCreatePayout_ExceedsBalance_ReturnsInsufficientBalance(t *testing.T) {
	ledger := &fakeLedger{balance: 1000}
	s := NewService(&fakeRepository{}, ledger, fakeTxRunner{}, &fakeEventPublisher{})

	_, err := s.CreatePayout(context.Background(), testMerchantID, CreatePayoutInput{AmountMinor: 2000, Currency: "BRL"})
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}
}

func TestCreatePayout_ZeroBalance_ReturnsInsufficientBalance(t *testing.T) {
	ledger := &fakeLedger{balance: 0}
	s := NewService(&fakeRepository{}, ledger, fakeTxRunner{}, &fakeEventPublisher{})

	_, err := s.CreatePayout(context.Background(), testMerchantID, CreatePayoutInput{Currency: "BRL"})
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}
}

func TestCreatePayout_InvalidCurrency_ReturnsValidationError(t *testing.T) {
	s := NewService(&fakeRepository{}, &fakeLedger{balance: 5000}, fakeTxRunner{}, &fakeEventPublisher{})

	_, err := s.CreatePayout(context.Background(), testMerchantID, CreatePayoutInput{Currency: "TOOLONG"})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestCreatePayout_NegativeAmount_ReturnsValidationError(t *testing.T) {
	s := NewService(&fakeRepository{}, &fakeLedger{balance: 5000}, fakeTxRunner{}, &fakeEventPublisher{})

	_, err := s.CreatePayout(context.Background(), testMerchantID, CreatePayoutInput{AmountMinor: -1, Currency: "BRL"})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestCreatePayout_PublishesPayoutPaidEvent(t *testing.T) {
	pub := &fakeEventPublisher{}
	s := NewService(&fakeRepository{}, &fakeLedger{balance: 5000}, fakeTxRunner{}, pub)

	if _, err := s.CreatePayout(context.Background(), testMerchantID, CreatePayoutInput{Currency: "BRL"}); err != nil {
		t.Fatalf("CreatePayout returned error: %v", err)
	}

	if len(pub.eventType) != 1 || pub.eventType[0] != "payout.paid" {
		t.Fatalf("expected exactly one payout.paid event, got %v", pub.eventType)
	}
}

func TestListPayouts_ReturnsCreatedPayouts(t *testing.T) {
	s := NewService(&fakeRepository{}, &fakeLedger{balance: 5000}, fakeTxRunner{}, &fakeEventPublisher{})

	if _, err := s.CreatePayout(context.Background(), testMerchantID, CreatePayoutInput{AmountMinor: 1000, Currency: "BRL"}); err != nil {
		t.Fatalf("CreatePayout returned error: %v", err)
	}

	list, err := s.ListPayouts(context.Background(), testMerchantID)
	if err != nil {
		t.Fatalf("ListPayouts returned error: %v", err)
	}
	if len(list) != 1 || list[0].AmountMinor != 1000 {
		t.Fatalf("expected exactly one 1000 payout, got %v", list)
	}
}
