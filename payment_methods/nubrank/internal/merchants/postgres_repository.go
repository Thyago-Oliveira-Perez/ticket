package merchants

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

func (r *postgresRepository) CreateMerchant(ctx context.Context, m Merchant) (Merchant, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO merchants (name, status)
		VALUES ($1, $2)
		RETURNING uuid, name, status, created_at, updated_at
	`, m.Name, m.Status)

	var created Merchant
	if err := row.Scan(&created.ID, &created.Name, &created.Status, &created.CreatedAt, &created.UpdatedAt); err != nil {
		return Merchant{}, fmt.Errorf("insert merchant: %w", err)
	}

	return created, nil
}

func (r *postgresRepository) CreateAPIKey(ctx context.Context, k APIKey) (APIKey, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO api_keys (merchant_id, key_hash, scope)
		VALUES ($1, $2, $3)
		RETURNING uuid, merchant_id, key_hash, scope, last_used_at, created_at, updated_at
	`, k.MerchantID, k.KeyHash, k.Scope)

	var created APIKey
	if err := row.Scan(
		&created.ID,
		&created.MerchantID,
		&created.KeyHash,
		&created.Scope,
		&created.LastUsedAt,
		&created.CreatedAt,
		&created.UpdatedAt,
	); err != nil {
		return APIKey{}, fmt.Errorf("insert api key: %w", err)
	}

	return created, nil
}

func (r *postgresRepository) GetMerchantByAPIKeyHash(ctx context.Context, keyHash string) (Merchant, error) {
	row := r.db.QueryRow(ctx, `
		SELECT m.uuid, m.name, m.status, m.created_at, m.updated_at
		FROM api_keys ak
		JOIN merchants m ON m.uuid = ak.merchant_id
		WHERE ak.key_hash = $1
	`, keyHash)

	var m Merchant
	if err := row.Scan(&m.ID, &m.Name, &m.Status, &m.CreatedAt, &m.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Merchant{}, ErrInvalidAPIKey
		}
		return Merchant{}, fmt.Errorf("get merchant by api key hash: %w", err)
	}

	return m, nil
}

func (r *postgresRepository) TouchAPIKeyLastUsed(ctx context.Context, keyHash string) error {
	_, err := r.db.Exec(ctx, `UPDATE api_keys SET last_used_at = now() WHERE key_hash = $1`, keyHash)
	if err != nil {
		return fmt.Errorf("touch api key last used: %w", err)
	}
	return nil
}
