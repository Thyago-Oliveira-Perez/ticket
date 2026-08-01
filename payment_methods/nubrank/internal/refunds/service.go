package refunds

import (
	"context"
	"errors"
	"fmt"

	"nubrank/internal/database"
	"nubrank/internal/events"
	"nubrank/internal/payments"

	"github.com/google/uuid"
)

// ErrValidation marks errors caused by invalid caller input, as opposed to
// infrastructure failures. Handlers use errors.Is against it to pick the
// right HTTP status.
var ErrValidation = errors.New("validation error")

// ErrPaymentNotRefundable is returned by CreateRefund when the payment
// isn't in a refundable state (it was declined, or has already been fully
// refunded).
var ErrPaymentNotRefundable = errors.New("payment is not refundable")

// ErrRefundExceedsRemaining is returned by CreateRefund when the requested
// amount is more than what's left to refund on the payment.
var ErrRefundExceedsRemaining = errors.New("refund amount exceeds remaining refundable amount")

type CreateRefundInput struct {
	// AmountMinor, if 0, defaults to the full remaining refundable amount
	// (a full refund, or the rest of a partially-refunded payment).
	AmountMinor int64
}

type Service interface {
	// CreateRefund records a (possibly partial) refund of paymentID,
	// scoped to merchantID. Returns an error wrapping ErrValidation if
	// paymentID isn't a valid UUID or AmountMinor is negative,
	// payments.ErrPaymentNotFound if no such payment exists for that
	// merchant, ErrPaymentNotRefundable if the payment is declined or
	// already fully refunded, or ErrRefundExceedsRemaining if the amount
	// requested is more than what's left to refund.
	CreateRefund(ctx context.Context, merchantID, paymentID string, in CreateRefundInput) (Refund, error)
	// ListRefunds lists refunds recorded against paymentID, scoped to
	// merchantID.
	ListRefunds(ctx context.Context, merchantID, paymentID string) ([]Refund, error)
}

type svc struct {
	repo     Repository
	payments payments.Repository
	tx       database.TxRunner
	events   events.Publisher
}

func NewService(repo Repository, paymentsRepo payments.Repository, tx database.TxRunner, eventPublisher events.Publisher) Service {
	return &svc{repo: repo, payments: paymentsRepo, tx: tx, events: eventPublisher}
}

func (s *svc) CreateRefund(ctx context.Context, merchantID, paymentID string, in CreateRefundInput) (Refund, error) {
	if _, err := uuid.Parse(paymentID); err != nil {
		return Refund{}, fmt.Errorf("%w: payment id must be a valid UUID", ErrValidation)
	}
	if in.AmountMinor < 0 {
		return Refund{}, fmt.Errorf("%w: amount_minor must not be negative", ErrValidation)
	}

	var created Refund
	var deliveries []events.Delivery
	err := s.tx.RunInTx(ctx, func(q database.Querier) error {
		paymentsRepo := s.payments.WithQuerier(q)
		refundsRepo := s.repo.WithQuerier(q)

		payment, err := paymentsRepo.LockForUpdate(ctx, merchantID, paymentID)
		if err != nil {
			return err
		}
		if payment.Status != payments.StatusApproved && payment.Status != payments.StatusPartiallyRefunded {
			return ErrPaymentNotRefundable
		}

		refundedSoFar, err := refundsRepo.SumByPayment(ctx, paymentID)
		if err != nil {
			return err
		}

		remaining := payment.AmountMinor - refundedSoFar
		amount := in.AmountMinor
		if amount == 0 {
			amount = remaining
		}
		if amount <= 0 || amount > remaining {
			return ErrRefundExceedsRemaining
		}

		created, err = refundsRepo.CreateRefund(ctx, Refund{PaymentID: paymentID, AmountMinor: amount, Status: StatusSucceeded})
		if err != nil {
			return err
		}

		newStatus := payments.StatusPartiallyRefunded
		if refundedSoFar+amount == payment.AmountMinor {
			newStatus = payments.StatusRefunded
		}
		if _, err := paymentsRepo.UpdateStatus(ctx, merchantID, paymentID, newStatus); err != nil {
			return err
		}

		deliveries, err = s.events.Publish(ctx, q, merchantID, "payment.refunded", created.ID, created)
		return err
	})
	if err != nil {
		return Refund{}, err
	}

	// Detach from the request context's cancellation (the HTTP response has
	// already been decided) but keep any request-scoped values.
	s.events.Dispatch(context.WithoutCancel(ctx), deliveries)

	return created, nil
}

func (s *svc) ListRefunds(ctx context.Context, merchantID, paymentID string) ([]Refund, error) {
	if _, err := uuid.Parse(paymentID); err != nil {
		return nil, fmt.Errorf("%w: payment id must be a valid UUID", ErrValidation)
	}
	// Confirms the payment exists and belongs to merchantID before
	// exposing any refunds against it.
	if _, err := s.payments.GetByID(ctx, merchantID, paymentID); err != nil {
		return nil, err
	}
	return s.repo.ListByPayment(ctx, paymentID)
}
