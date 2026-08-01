package ledger

import (
	"context"
	"sync"
	"testing"
	"time"

	"nubrank/internal/database"

	"github.com/google/uuid"
)

type fakeRepository struct {
	mu           sync.Mutex
	accounts     map[string]*Account
	merchantByID map[string]string // merchantID -> accountID
	entries      []Entry
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		accounts:     map[string]*Account{ClearingAccountID: {ID: ClearingAccountID, CreatedAt: time.Now(), UpdatedAt: time.Now()}},
		merchantByID: make(map[string]string),
	}
}

func (f *fakeRepository) WithQuerier(q database.Querier) Repository { return f }

func (f *fakeRepository) GetOrCreateMerchantAccount(ctx context.Context, merchantID string) (Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id, ok := f.merchantByID[merchantID]; ok {
		return *f.accounts[id], nil
	}
	id := uuid.NewString()
	mID := merchantID
	acc := &Account{ID: id, MerchantID: &mID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	f.accounts[id] = acc
	f.merchantByID[merchantID] = id
	return *acc, nil
}

func (f *fakeRepository) LockAccount(ctx context.Context, id string) (Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *f.accounts[id], nil
}

func (f *fakeRepository) UpdateBalance(ctx context.Context, id string, newBalanceMinor int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts[id].BalanceMinor = newBalanceMinor
	return nil
}

func (f *fakeRepository) InsertTransaction(ctx context.Context, referenceType, referenceID, kind string) (Transaction, error) {
	return Transaction{ID: uuid.NewString(), ReferenceType: referenceType, ReferenceID: referenceID, Kind: kind, CreatedAt: time.Now()}, nil
}

func (f *fakeRepository) InsertEntry(ctx context.Context, transactionID, accountID string, amountMinor int64) (Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := Entry{ID: uuid.NewString(), TransactionID: transactionID, AccountID: accountID, AmountMinor: amountMinor, CreatedAt: time.Now()}
	f.entries = append(f.entries, e)
	return e, nil
}

const testMerchantID = "11111111-1111-1111-1111-111111111111"

func TestPost_CreditsMerchantAndDebitsClearing(t *testing.T) {
	repo := newFakeRepository()
	s := NewService(repo)

	if err := s.Post(context.Background(), nil, testMerchantID, "payment", "p1", "charge", 5000); err != nil {
		t.Fatalf("Post returned error: %v", err)
	}

	balance, err := s.Balance(context.Background(), testMerchantID)
	if err != nil {
		t.Fatalf("Balance returned error: %v", err)
	}
	if balance != 5000 {
		t.Fatalf("expected merchant balance 5000, got %d", balance)
	}

	clearing, err := repo.LockAccount(context.Background(), ClearingAccountID)
	if err != nil {
		t.Fatalf("LockAccount returned error: %v", err)
	}
	if clearing.BalanceMinor != -5000 {
		t.Fatalf("expected clearing balance -5000, got %d", clearing.BalanceMinor)
	}
}

func TestPost_EntriesAlwaysSumToZero(t *testing.T) {
	repo := newFakeRepository()
	s := NewService(repo)

	if err := s.Post(context.Background(), nil, testMerchantID, "payment", "p1", "charge", 5000); err != nil {
		t.Fatalf("Post returned error: %v", err)
	}
	if err := s.Post(context.Background(), nil, testMerchantID, "refund", "r1", "refund", -2000); err != nil {
		t.Fatalf("Post returned error: %v", err)
	}

	var sum int64
	for _, e := range repo.entries {
		sum += e.AmountMinor
	}
	if sum != 0 {
		t.Fatalf("expected all ledger entries to sum to zero, got %d", sum)
	}
}

func TestPost_NegativeDeltaDebitsMerchant(t *testing.T) {
	repo := newFakeRepository()
	s := NewService(repo)

	if err := s.Post(context.Background(), nil, testMerchantID, "payment", "p1", "charge", 5000); err != nil {
		t.Fatalf("Post returned error: %v", err)
	}
	if err := s.Post(context.Background(), nil, testMerchantID, "refund", "r1", "refund", -3000); err != nil {
		t.Fatalf("Post returned error: %v", err)
	}

	balance, err := s.Balance(context.Background(), testMerchantID)
	if err != nil {
		t.Fatalf("Balance returned error: %v", err)
	}
	if balance != 2000 {
		t.Fatalf("expected merchant balance 2000 after partial refund, got %d", balance)
	}
}

func TestBalance_UnseenMerchant_StartsAtZero(t *testing.T) {
	s := NewService(newFakeRepository())

	balance, err := s.Balance(context.Background(), "99999999-9999-9999-9999-999999999999")
	if err != nil {
		t.Fatalf("Balance returned error: %v", err)
	}
	if balance != 0 {
		t.Fatalf("expected balance 0 for an unseen merchant, got %d", balance)
	}
}
