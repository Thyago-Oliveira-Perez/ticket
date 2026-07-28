package payments

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) ListPayments(ctx context.Context) ([]Payment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT uuid, merchant_id, customer_id, payment_method_id, amount_minor, currency, status, decline_reason, created_at, updated_at
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
