package payouts

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

func (r *postgresRepository) CreatePayout(ctx context.Context, p Payout) (Payout, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO payouts (merchant_id, amount_minor, currency, status)
		VALUES ($1, $2, $3, $4)
		RETURNING uuid, merchant_id, amount_minor, currency, status, created_at, updated_at
	`, p.MerchantID, p.AmountMinor, p.Currency, p.Status)

	var created Payout
	if err := row.Scan(&created.ID, &created.MerchantID, &created.AmountMinor, &created.Currency, &created.Status, &created.CreatedAt, &created.UpdatedAt); err != nil {
		return Payout{}, fmt.Errorf("insert payout: %w", err)
	}

	return created, nil
}

func (r *postgresRepository) GetByID(ctx context.Context, merchantID, id string) (Payout, error) {
	row := r.db.QueryRow(ctx, `
		SELECT uuid, merchant_id, amount_minor, currency, status, created_at, updated_at
		FROM payouts
		WHERE uuid = $1 AND merchant_id = $2
	`, id, merchantID)

	var p Payout
	if err := row.Scan(&p.ID, &p.MerchantID, &p.AmountMinor, &p.Currency, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Payout{}, ErrPayoutNotFound
		}
		return Payout{}, fmt.Errorf("get payout by id: %w", err)
	}

	return p, nil
}

func (r *postgresRepository) ListByMerchant(ctx context.Context, merchantID string) ([]Payout, error) {
	rows, err := r.db.Query(ctx, `
		SELECT uuid, merchant_id, amount_minor, currency, status, created_at, updated_at
		FROM payouts
		WHERE merchant_id = $1
		ORDER BY created_at DESC
	`, merchantID)
	if err != nil {
		return nil, fmt.Errorf("query payouts: %w", err)
	}
	defer rows.Close()

	var payoutList []Payout
	for rows.Next() {
		var p Payout
		if err := rows.Scan(&p.ID, &p.MerchantID, &p.AmountMinor, &p.Currency, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan payout: %w", err)
		}
		payoutList = append(payoutList, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate payouts: %w", err)
	}

	return payoutList, nil
}
