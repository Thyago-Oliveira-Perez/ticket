package payments

import (
	"context"
	"errors"
	"fmt"

	"nubrank/internal/database"

	"github.com/jackc/pgx/v5"
)

type postgresRepository struct {
	db database.Querier
}

func NewPostgresRepository(db database.Querier) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) WithQuerier(q database.Querier) Repository {
	return &postgresRepository{db: q}
}

func (r *postgresRepository) ListPayments(ctx context.Context, merchantID string) ([]Payment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT uuid, merchant_id, customer_id, payment_method_id, amount_minor, currency, status, decline_reason, created_at, updated_at
		FROM payments
		WHERE merchant_id = $1
		ORDER BY created_at DESC
	`, merchantID)
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
		INSERT INTO payments (merchant_id, customer_id, payment_method_id, amount_minor, currency, status, decline_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING uuid, merchant_id, customer_id, payment_method_id, amount_minor, currency, status, decline_reason, created_at, updated_at
	`, p.MerchantID, p.CustomerID, p.PaymentMethodID, p.AmountMinor, p.Currency, p.Status, p.DeclineReason)

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
		&created.CreatedAt,
		&created.UpdatedAt,
	); err != nil {
		return Payment{}, fmt.Errorf("insert payment: %w", err)
	}

	return created, nil
}

func (r *postgresRepository) GetByID(ctx context.Context, merchantID, id string) (Payment, error) {
	row := r.db.QueryRow(ctx, `
		SELECT uuid, merchant_id, customer_id, payment_method_id, amount_minor, currency, status, decline_reason, created_at, updated_at
		FROM payments
		WHERE uuid = $1 AND merchant_id = $2
	`, id, merchantID)

	return scanPayment(row)
}

func (r *postgresRepository) LockForUpdate(ctx context.Context, merchantID, id string) (Payment, error) {
	row := r.db.QueryRow(ctx, `
		SELECT uuid, merchant_id, customer_id, payment_method_id, amount_minor, currency, status, decline_reason, created_at, updated_at
		FROM payments
		WHERE uuid = $1 AND merchant_id = $2
		FOR UPDATE
	`, id, merchantID)

	return scanPayment(row)
}

func (r *postgresRepository) UpdateStatus(ctx context.Context, merchantID, id, status string) (Payment, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE payments
		SET status = $3, updated_at = now()
		WHERE uuid = $1 AND merchant_id = $2
		RETURNING uuid, merchant_id, customer_id, payment_method_id, amount_minor, currency, status, decline_reason, created_at, updated_at
	`, id, merchantID, status)

	return scanPayment(row)
}

func scanPayment(row pgx.Row) (Payment, error) {
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
		&p.CreatedAt,
		&p.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Payment{}, ErrPaymentNotFound
		}
		return Payment{}, fmt.Errorf("scan payment: %w", err)
	}
	return p, nil
}
