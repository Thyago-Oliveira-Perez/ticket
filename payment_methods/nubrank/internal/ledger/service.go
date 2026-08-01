package ledger

import (
	"context"
	"fmt"

	"nubrank/internal/database"
)

type Service interface {
	// Post records a balanced ledger transaction for merchantID:
	// merchantAccountDelta is applied to the merchant's account balance,
	// and its negation is applied to the platform clearing account, so
	// the two entries always sum to zero. Must be called with q bound to
	// the same transaction as the state change (payment/refund/payout)
	// this posting represents, so they commit or roll back together.
	Post(ctx context.Context, q database.Querier, merchantID, referenceType, referenceID, kind string, merchantAccountDelta int64) error
	// Balance returns merchantID's current ledger account balance
	// (creating the account with a zero balance if it doesn't exist yet).
	Balance(ctx context.Context, merchantID string) (int64, error)
}

type svc struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &svc{repo: repo}
}

func (s *svc) Post(ctx context.Context, q database.Querier, merchantID, referenceType, referenceID, kind string, merchantAccountDelta int64) error {
	repo := s.repo.WithQuerier(q)

	merchantAccount, err := repo.GetOrCreateMerchantAccount(ctx, merchantID)
	if err != nil {
		return fmt.Errorf("get or create merchant ledger account: %w", err)
	}

	// Lock the clearing account first, always, then the merchant account.
	// Since every posting involves the same clearing account, taking its
	// lock first establishes one global order across all concurrent
	// postings, which rules out lock-ordering deadlocks entirely.
	clearing, err := repo.LockAccount(ctx, ClearingAccountID)
	if err != nil {
		return fmt.Errorf("lock clearing account: %w", err)
	}
	merchant, err := repo.LockAccount(ctx, merchantAccount.ID)
	if err != nil {
		return fmt.Errorf("lock merchant ledger account: %w", err)
	}

	txn, err := repo.InsertTransaction(ctx, referenceType, referenceID, kind)
	if err != nil {
		return err
	}

	if _, err := repo.InsertEntry(ctx, txn.ID, merchant.ID, merchantAccountDelta); err != nil {
		return err
	}
	if _, err := repo.InsertEntry(ctx, txn.ID, clearing.ID, -merchantAccountDelta); err != nil {
		return err
	}

	if err := repo.UpdateBalance(ctx, merchant.ID, merchant.BalanceMinor+merchantAccountDelta); err != nil {
		return err
	}
	if err := repo.UpdateBalance(ctx, clearing.ID, clearing.BalanceMinor-merchantAccountDelta); err != nil {
		return err
	}

	return nil
}

func (s *svc) Balance(ctx context.Context, merchantID string) (int64, error) {
	acc, err := s.repo.GetOrCreateMerchantAccount(ctx, merchantID)
	if err != nil {
		return 0, err
	}
	return acc.BalanceMinor, nil
}
