package idempotency

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) Reserve(ctx context.Context, merchantID, key, requestHash string) (Record, bool, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO idempotency_keys (merchant_id, key, request_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (merchant_id, key) DO NOTHING
		RETURNING uuid, merchant_id, key, request_hash, response_status, response_body, created_at
	`, merchantID, key, requestHash)

	var rec Record
	err := row.Scan(&rec.ID, &rec.MerchantID, &rec.Key, &rec.RequestHash, &rec.ResponseStatus, &rec.ResponseBody, &rec.CreatedAt)
	if err == nil {
		return rec, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Record{}, false, fmt.Errorf("reserve idempotency key: %w", err)
	}

	// ON CONFLICT DO NOTHING returned no row: a record already exists.
	existing, getErr := r.getByMerchantAndKey(ctx, merchantID, key)
	if getErr != nil {
		return Record{}, false, getErr
	}
	return existing, false, nil
}

func (r *postgresRepository) getByMerchantAndKey(ctx context.Context, merchantID, key string) (Record, error) {
	row := r.db.QueryRow(ctx, `
		SELECT uuid, merchant_id, key, request_hash, response_status, response_body, created_at
		FROM idempotency_keys
		WHERE merchant_id = $1 AND key = $2
	`, merchantID, key)

	var rec Record
	if err := row.Scan(&rec.ID, &rec.MerchantID, &rec.Key, &rec.RequestHash, &rec.ResponseStatus, &rec.ResponseBody, &rec.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrNotFound
		}
		return Record{}, fmt.Errorf("get idempotency key: %w", err)
	}

	return rec, nil
}

func (r *postgresRepository) Complete(ctx context.Context, id string, responseStatus int, responseBody []byte) error {
	_, err := r.db.Exec(ctx, `
		UPDATE idempotency_keys
		SET response_status = $2, response_body = $3
		WHERE uuid = $1
	`, id, responseStatus, responseBody)
	if err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	return nil
}
