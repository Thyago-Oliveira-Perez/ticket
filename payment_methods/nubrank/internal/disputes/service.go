// Package disputes simulates chargebacks the same way the rest of nubrank
// simulates other real-provider unpleasantness: automatically, not in
// response to any caller action. A random fraction of approved payments get
// disputed some time after creation (funds held immediately), and each
// dispute auto-resolves won or lost after another delay — callers only get
// read access to see what happened.
package disputes

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"time"

	"nubrank/internal/database"
	"nubrank/internal/events"
	"nubrank/internal/payments"

	"github.com/google/uuid"
)

// ErrValidation marks errors caused by invalid caller input, as opposed to
// infrastructure failures. Handlers use errors.Is against it to pick the
// right HTTP status.
var ErrValidation = errors.New("validation error")

// Config controls simulated dispute behavior. Zero value never opens a
// dispute.
type Config struct {
	// Rate is the probability, in [0, 1], that an approved payment is
	// disputed at some point after creation. Disabled when Rate <= 0.
	Rate float64
	// WinRate is the probability, in [0, 1], that a dispute resolves in
	// the merchant's favor (held funds are released back).
	WinRate float64
	// OpenDelayMin/Max bound how long after payment creation a disputed
	// payment's dispute is opened.
	OpenDelayMin, OpenDelayMax time.Duration
	// ResolveDelayMin/Max bound how long after opening a dispute
	// auto-resolves.
	ResolveDelayMin, ResolveDelayMax time.Duration
}

// LedgerPoster is the subset of ledger.Service the dispute lifecycle needs:
// holding funds when a dispute opens, releasing them back if it's won.
type LedgerPoster interface {
	Post(ctx context.Context, q database.Querier, merchantID, referenceType, referenceID, kind string, merchantAccountDelta int64) error
}

// Simulator rolls the dice for a newly-approved payment. Implemented by
// Service; split out so payments.Service can depend on just this.
type Simulator interface {
	// MaybeDispute returns immediately. If the roll hits, the full
	// open-then-resolve lifecycle runs in the background.
	MaybeDispute(ctx context.Context, merchantID, paymentID string, amountMinor int64)
}

type Service interface {
	Simulator
	// ListDisputes lists disputes recorded against paymentID, scoped to
	// merchantID.
	ListDisputes(ctx context.Context, merchantID, paymentID string) ([]Dispute, error)
}

type svc struct {
	repo     Repository
	payments payments.Repository
	tx       database.TxRunner
	events   events.Publisher
	ledger   LedgerPoster
	cfg      Config
	// rollDispute and rollWon are overridden in tests for determinism.
	rollDispute func() bool
	rollWon     func() bool
}

func NewService(repo Repository, paymentsRepo payments.Repository, tx database.TxRunner, eventPublisher events.Publisher, ledgerPoster LedgerPoster, cfg Config) Service {
	return &svc{
		repo:     repo,
		payments: paymentsRepo,
		tx:       tx,
		events:   eventPublisher,
		ledger:   ledgerPoster,
		cfg:      cfg,
		rollDispute: func() bool {
			return cfg.Rate > 0 && rand.Float64() < cfg.Rate
		},
		rollWon: func() bool {
			return rand.Float64() < cfg.WinRate
		},
	}
}

func (s *svc) MaybeDispute(ctx context.Context, merchantID, paymentID string, amountMinor int64) {
	if !s.rollDispute() {
		return
	}
	go s.runLifecycle(ctx, merchantID, paymentID, amountMinor)
}

func (s *svc) runLifecycle(ctx context.Context, merchantID, paymentID string, amountMinor int64) {
	sleep(s.cfg.OpenDelayMin, s.cfg.OpenDelayMax)

	dispute, err := s.open(ctx, merchantID, paymentID, amountMinor)
	if err != nil {
		log.Printf("disputes: failed to open dispute for payment %s: %v", paymentID, err)
		return
	}

	sleep(s.cfg.ResolveDelayMin, s.cfg.ResolveDelayMax)

	if err := s.resolve(ctx, merchantID, dispute, amountMinor); err != nil {
		log.Printf("disputes: failed to resolve dispute %s: %v", dispute.ID, err)
	}
}

func (s *svc) open(ctx context.Context, merchantID, paymentID string, amountMinor int64) (Dispute, error) {
	var dispute Dispute
	var deliveries []events.Delivery
	err := s.tx.RunInTx(ctx, func(q database.Querier) error {
		var err error
		dispute, err = s.repo.WithQuerier(q).CreateDispute(ctx, Dispute{
			PaymentID:   paymentID,
			AmountMinor: amountMinor,
			Status:      StatusNeedsResponse,
		})
		if err != nil {
			return err
		}

		// Funds are held pending resolution, mirroring how a real
		// processor immediately debits a merchant when a chargeback lands.
		if err := s.ledger.Post(ctx, q, merchantID, "dispute", dispute.ID, "dispute_hold", -amountMinor); err != nil {
			return err
		}

		deliveries, err = s.events.Publish(ctx, q, merchantID, "dispute.created", dispute.ID, dispute)
		return err
	})
	if err != nil {
		return Dispute{}, err
	}

	s.events.Dispatch(ctx, deliveries)
	return dispute, nil
}

func (s *svc) resolve(ctx context.Context, merchantID string, dispute Dispute, amountMinor int64) error {
	won := s.rollWon()
	newStatus := StatusLost
	eventType := "dispute.lost"
	if won {
		newStatus = StatusWon
		eventType = "dispute.won"
	}

	var resolved Dispute
	var deliveries []events.Delivery
	err := s.tx.RunInTx(ctx, func(q database.Querier) error {
		var err error
		resolved, err = s.repo.WithQuerier(q).UpdateStatus(ctx, dispute.ID, newStatus)
		if err != nil {
			return err
		}

		if won {
			// The merchant wins: release the held funds back.
			if err := s.ledger.Post(ctx, q, merchantID, "dispute", dispute.ID, "dispute_release", amountMinor); err != nil {
				return err
			}
		}

		deliveries, err = s.events.Publish(ctx, q, merchantID, eventType, resolved.ID, resolved)
		return err
	})
	if err != nil {
		return err
	}

	s.events.Dispatch(ctx, deliveries)
	return nil
}

func (s *svc) ListDisputes(ctx context.Context, merchantID, paymentID string) ([]Dispute, error) {
	if _, err := uuid.Parse(paymentID); err != nil {
		return nil, fmt.Errorf("%w: payment id must be a valid UUID", ErrValidation)
	}
	// Confirms the payment exists and belongs to merchantID before
	// exposing any disputes against it.
	if _, err := s.payments.GetByID(ctx, merchantID, paymentID); err != nil {
		return nil, err
	}
	return s.repo.ListByPayment(ctx, paymentID)
}

func sleep(min, max time.Duration) {
	if max <= 0 {
		return
	}
	d := min
	if max > min {
		d += time.Duration(rand.Int64N(int64(max - min)))
	}
	time.Sleep(d)
}
