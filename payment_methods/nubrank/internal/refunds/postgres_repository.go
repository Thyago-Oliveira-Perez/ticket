package refunds

import (
	"context"
	"fmt"

	"nubrank/internal/database"
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

func (r *postgresRepository) CreateRefund(ctx context.Context, ref Refund) (Refund, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO refunds (payment_id, amount_minor, status)
		VALUES ($1, $2, $3)
		RETURNING uuid, payment_id, amount_minor, status, created_at, updated_at
	`, ref.PaymentID, ref.AmountMinor, ref.Status)

	var created Refund
	if err := row.Scan(&created.ID, &created.PaymentID, &created.AmountMinor, &created.Status, &created.CreatedAt, &created.UpdatedAt); err != nil {
		return Refund{}, fmt.Errorf("insert refund: %w", err)
	}

	return created, nil
}

func (r *postgresRepository) SumByPayment(ctx context.Context, paymentID string) (int64, error) {
	row := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_minor), 0) FROM refunds WHERE payment_id = $1
	`, paymentID)

	var sum int64
	if err := row.Scan(&sum); err != nil {
		return 0, fmt.Errorf("sum refunds by payment: %w", err)
	}

	return sum, nil
}

func (r *postgresRepository) ListByPayment(ctx context.Context, paymentID string) ([]Refund, error) {
	rows, err := r.db.Query(ctx, `
		SELECT uuid, payment_id, amount_minor, status, created_at, updated_at
		FROM refunds
		WHERE payment_id = $1
		ORDER BY created_at DESC
	`, paymentID)
	if err != nil {
		return nil, fmt.Errorf("query refunds: %w", err)
	}
	defer rows.Close()

	var refundList []Refund
	for rows.Next() {
		var ref Refund
		if err := rows.Scan(&ref.ID, &ref.PaymentID, &ref.AmountMinor, &ref.Status, &ref.CreatedAt, &ref.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan refund: %w", err)
		}
		refundList = append(refundList, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate refunds: %w", err)
	}

	return refundList, nil
}
