package payouts

import (
	"context"
	"errors"
	"fmt"

	"nubrank/internal/database"
	"nubrank/internal/events"

	"github.com/google/uuid"
)

// ErrValidation marks errors caused by invalid caller input, as opposed to
// infrastructure failures. Handlers use errors.Is against it to pick the
// right HTTP status.
var ErrValidation = errors.New("validation error")

// ErrInsufficientBalance is returned by CreatePayout when the requested
// amount is more than the merchant's current ledger balance.
var ErrInsufficientBalance = errors.New("insufficient balance for payout")

// LedgerPoster is the subset of ledger.Service CreatePayout needs: checking
// the merchant's available balance and posting the debit.
type LedgerPoster interface {
	LockMerchantBalance(ctx context.Context, q database.Querier, merchantID string) (int64, error)
	Post(ctx context.Context, q database.Querier, merchantID, referenceType, referenceID, kind string, merchantAccountDelta int64) error
}

type CreatePayoutInput struct {
	// AmountMinor, if 0, defaults to the merchant's full available ledger
	// balance.
	AmountMinor int64
	Currency    string
}

type Service interface {
	// CreatePayout pays out (a portion of) merchantID's available ledger
	// balance. Returns an error wrapping ErrValidation if AmountMinor is
	// negative or Currency isn't a 3-letter code, or
	// ErrInsufficientBalance if the amount requested exceeds the
	// merchant's current balance.
	CreatePayout(ctx context.Context, merchantID string, in CreatePayoutInput) (Payout, error)
	GetPayout(ctx context.Context, merchantID, id string) (Payout, error)
	ListPayouts(ctx context.Context, merchantID string) ([]Payout, error)
}

type svc struct {
	repo   Repository
	ledger LedgerPoster
	tx     database.TxRunner
	events events.Publisher
}

func NewService(repo Repository, ledgerPoster LedgerPoster, tx database.TxRunner, eventPublisher events.Publisher) Service {
	return &svc{repo: repo, ledger: ledgerPoster, tx: tx, events: eventPublisher}
}

func (s *svc) CreatePayout(ctx context.Context, merchantID string, in CreatePayoutInput) (Payout, error) {
	if in.AmountMinor < 0 {
		return Payout{}, fmt.Errorf("%w: amount_minor must not be negative", ErrValidation)
	}
	if len(in.Currency) != 3 {
		return Payout{}, fmt.Errorf("%w: currency must be a 3-letter ISO 4217 code", ErrValidation)
	}

	var created Payout
	var deliveries []events.Delivery
	err := s.tx.RunInTx(ctx, func(q database.Querier) error {
		balance, err := s.ledger.LockMerchantBalance(ctx, q, merchantID)
		if err != nil {
			return err
		}

		amount := in.AmountMinor
		if amount == 0 {
			amount = balance
		}
		if amount <= 0 || amount > balance {
			return ErrInsufficientBalance
		}

		created, err = s.repo.WithQuerier(q).CreatePayout(ctx, Payout{
			MerchantID:  merchantID,
			AmountMinor: amount,
			Currency:    in.Currency,
			Status:      StatusPaid,
		})
		if err != nil {
			return err
		}

		if err := s.ledger.Post(ctx, q, merchantID, "payout", created.ID, "payout", -amount); err != nil {
			return err
		}

		deliveries, err = s.events.Publish(ctx, q, merchantID, "payout.paid", created.ID, created)
		return err
	})
	if err != nil {
		return Payout{}, err
	}

	// Detach from the request context's cancellation (the HTTP response has
	// already been decided) but keep any request-scoped values.
	s.events.Dispatch(context.WithoutCancel(ctx), deliveries)

	return created, nil
}

func (s *svc) GetPayout(ctx context.Context, merchantID, id string) (Payout, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Payout{}, fmt.Errorf("%w: id must be a valid UUID", ErrValidation)
	}
	return s.repo.GetByID(ctx, merchantID, id)
}

func (s *svc) ListPayouts(ctx context.Context, merchantID string) ([]Payout, error) {
	return s.repo.ListByMerchant(ctx, merchantID)
}
