package ledger

import (
	"context"
	"time"

	"nubrank/internal/database"
)

// ClearingAccountID is the fixed id of the singleton platform account that
// serves as the universal counterparty for every merchant transaction, so
// entries always sum to zero across a real (merchant leg + clearing leg)
// double-entry pair. Seeded by migration 000016.
const ClearingAccountID = "00000000-0000-0000-0000-000000000000"

type Account struct {
	ID           string
	MerchantID   *string
	BalanceMinor int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Transaction struct {
	ID            string
	ReferenceType string
	ReferenceID   string
	Kind          string
	CreatedAt     time.Time
}

type Entry struct {
	ID            string
	TransactionID string
	AccountID     string
	AmountMinor   int64
	CreatedAt     time.Time
}

type Repository interface {
	// GetOrCreateMerchantAccount returns merchantID's ledger account,
	// creating it with a zero balance on first use.
	GetOrCreateMerchantAccount(ctx context.Context, merchantID string) (Account, error)
	// LockAccount row-locks an account (SELECT ... FOR UPDATE) so its
	// balance can be safely read and updated within a transaction.
	LockAccount(ctx context.Context, id string) (Account, error)
	UpdateBalance(ctx context.Context, id string, newBalanceMinor int64) error
	InsertTransaction(ctx context.Context, referenceType, referenceID, kind string) (Transaction, error)
	InsertEntry(ctx context.Context, transactionID, accountID string, amountMinor int64) (Entry, error)
	// WithQuerier returns a Repository bound to q instead of the pool, so
	// its calls can participate in a transaction started elsewhere (e.g.
	// atomically with the payment/refund/payout state change that caused
	// this posting).
	WithQuerier(q database.Querier) Repository
}
