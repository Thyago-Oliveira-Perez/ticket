package payments

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// uniqueViolation is the Postgres error code for a unique constraint
// violation (23505).
const uniqueViolation = "23505"

type postgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) ListPayments(ctx context.Context) ([]Payment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT uuid, merchant_id, customer_id, payment_method_id, amount_minor, currency, status, decline_reason, idempotency_key, refunded_at, created_at, updated_at
		FROM payments
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query payments: %w", err)
	}
	defer rows.Close()

	var payments []Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(
			&p.ID,
			&p.MerchantID,
			&p.CustomerID,
			&p.PaymentMethodID,
			&p.AmountMinor,
			&p.Currency,
			&p.Status,
			&p.DeclineReason,
			&p.IdempotencyKey,
			&p.RefundedAt,
			&p.CreatedAt,
			&p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan payment: %w", err)
		}
		payments = append(payments, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate payments: %w", err)
	}

	return payments, nil
}

func (r *postgresRepository) CreatePayment(ctx context.Context, p Payment) (Payment, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO payments (merchant_id, customer_id, payment_method_id, amount_minor, currency, status, decline_reason, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING uuid, merchant_id, customer_id, payment_method_id, amount_minor, currency, status, decline_reason, idempotency_key, refunded_at, created_at, updated_at
	`, p.MerchantID, p.CustomerID, p.PaymentMethodID, p.AmountMinor, p.Currency, p.Status, p.DeclineReason, p.IdempotencyKey)

	var created Payment
	if err := row.Scan(
		&created.ID,
		&created.MerchantID,
		&created.CustomerID,
		&created.PaymentMethodID,
		&created.AmountMinor,
		&created.Currency,
		&created.Status,
		&created.DeclineReason,
		&created.IdempotencyKey,
		&created.RefundedAt,
		&created.CreatedAt,
		&created.UpdatedAt,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Payment{}, ErrIdempotencyKeyConflict
		}
		return Payment{}, fmt.Errorf("insert payment: %w", err)
	}

	return created, nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id string) (Payment, error) {
	row := r.db.QueryRow(ctx, `
		SELECT uuid, merchant_id, customer_id, payment_method_id, amount_minor, currency, status, decline_reason, idempotency_key, refunded_at, created_at, updated_at
		FROM payments
		WHERE uuid = $1
	`, id)

	var p Payment
	if err := row.Scan(
		&p.ID,
		&p.MerchantID,
		&p.CustomerID,
		&p.PaymentMethodID,
		&p.AmountMinor,
		&p.Currency,
		&p.Status,
		&p.DeclineReason,
		&p.IdempotencyKey,
		&p.RefundedAt,
		&p.CreatedAt,
		&p.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Payment{}, ErrPaymentNotFound
		}
		return Payment{}, fmt.Errorf("get payment by id: %w", err)
	}

	return p, nil
}

func (r *postgresRepository) RefundPayment(ctx context.Context, id string) (Payment, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE payments
		SET status = $2, refunded_at = now(), updated_at = now()
		WHERE uuid = $1 AND status = $3
		RETURNING uuid, merchant_id, customer_id, payment_method_id, amount_minor, currency, status, decline_reason, idempotency_key, refunded_at, created_at, updated_at
	`, id, StatusRefunded, StatusApproved)

	var p Payment
	if err := row.Scan(
		&p.ID,
		&p.MerchantID,
		&p.CustomerID,
		&p.PaymentMethodID,
		&p.AmountMinor,
		&p.Currency,
		&p.Status,
		&p.DeclineReason,
		&p.IdempotencyKey,
		&p.RefundedAt,
		&p.CreatedAt,
		&p.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Payment{}, ErrRefundNotApplied
		}
		return Payment{}, fmt.Errorf("refund payment: %w", err)
	}

	return p, nil
}

func (r *postgresRepository) GetByIdempotencyKey(ctx context.Context, merchantID, idempotencyKey string) (Payment, error) {
	row := r.db.QueryRow(ctx, `
		SELECT uuid, merchant_id, customer_id, payment_method_id, amount_minor, currency, status, decline_reason, idempotency_key, refunded_at, created_at, updated_at
		FROM payments
		WHERE merchant_id = $1 AND idempotency_key = $2
	`, merchantID, idempotencyKey)

	var p Payment
	if err := row.Scan(
		&p.ID,
		&p.MerchantID,
		&p.CustomerID,
		&p.PaymentMethodID,
		&p.AmountMinor,
		&p.Currency,
		&p.Status,
		&p.DeclineReason,
		&p.IdempotencyKey,
		&p.RefundedAt,
		&p.CreatedAt,
		&p.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Payment{}, ErrPaymentNotFound
		}
		return Payment{}, fmt.Errorf("get payment by idempotency key: %w", err)
	}

	return p, nil
}
