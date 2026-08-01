package disputes

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

func (r *postgresRepository) CreateDispute(ctx context.Context, d Dispute) (Dispute, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO disputes (payment_id, amount_minor, status)
		VALUES ($1, $2, $3)
		RETURNING uuid, payment_id, amount_minor, status, created_at, updated_at
	`, d.PaymentID, d.AmountMinor, d.Status)

	var created Dispute
	if err := row.Scan(&created.ID, &created.PaymentID, &created.AmountMinor, &created.Status, &created.CreatedAt, &created.UpdatedAt); err != nil {
		return Dispute{}, fmt.Errorf("insert dispute: %w", err)
	}

	return created, nil
}

func (r *postgresRepository) UpdateStatus(ctx context.Context, id, status string) (Dispute, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE disputes SET status = $2, updated_at = now() WHERE uuid = $1
		RETURNING uuid, payment_id, amount_minor, status, created_at, updated_at
	`, id, status)

	var d Dispute
	if err := row.Scan(&d.ID, &d.PaymentID, &d.AmountMinor, &d.Status, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return Dispute{}, fmt.Errorf("update dispute status: %w", err)
	}

	return d, nil
}

func (r *postgresRepository) ListByPayment(ctx context.Context, paymentID string) ([]Dispute, error) {
	rows, err := r.db.Query(ctx, `
		SELECT uuid, payment_id, amount_minor, status, created_at, updated_at
		FROM disputes
		WHERE payment_id = $1
		ORDER BY created_at DESC
	`, paymentID)
	if err != nil {
		return nil, fmt.Errorf("query disputes: %w", err)
	}
	defer rows.Close()

	var disputeList []Dispute
	for rows.Next() {
		var d Dispute
		if err := rows.Scan(&d.ID, &d.PaymentID, &d.AmountMinor, &d.Status, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan dispute: %w", err)
		}
		disputeList = append(disputeList, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate disputes: %w", err)
	}

	return disputeList, nil
}
