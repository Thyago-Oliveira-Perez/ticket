package webhookendpoints

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

func (r *postgresRepository) CreateEndpoint(ctx context.Context, e Endpoint) (Endpoint, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO webhooks_endpoints (merchant_id, url, secret, active)
		VALUES ($1, $2, $3, $4)
		RETURNING uuid, merchant_id, url, secret, active, created_at, updated_at
	`, e.MerchantID, e.URL, e.Secret, e.Active)

	var created Endpoint
	if err := row.Scan(
		&created.ID,
		&created.MerchantID,
		&created.URL,
		&created.Secret,
		&created.Active,
		&created.CreatedAt,
		&created.UpdatedAt,
	); err != nil {
		return Endpoint{}, fmt.Errorf("insert webhook endpoint: %w", err)
	}

	return created, nil
}

func (r *postgresRepository) ListActiveByMerchant(ctx context.Context, merchantID string) ([]Endpoint, error) {
	rows, err := r.db.Query(ctx, `
		SELECT uuid, merchant_id, url, secret, active, created_at, updated_at
		FROM webhooks_endpoints
		WHERE merchant_id = $1 AND active = true
	`, merchantID)
	if err != nil {
		return nil, fmt.Errorf("query webhook endpoints: %w", err)
	}
	defer rows.Close()

	var endpoints []Endpoint
	for rows.Next() {
		var e Endpoint
		if err := rows.Scan(&e.ID, &e.MerchantID, &e.URL, &e.Secret, &e.Active, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan webhook endpoint: %w", err)
		}
		endpoints = append(endpoints, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate webhook endpoints: %w", err)
	}

	return endpoints, nil
}
