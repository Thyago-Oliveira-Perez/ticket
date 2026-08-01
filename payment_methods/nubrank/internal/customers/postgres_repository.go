package customers

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

func (r *postgresRepository) CreateCustomer(ctx context.Context, c Customer) (Customer, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO customers (merchant_id, email)
		VALUES ($1, $2)
		RETURNING uuid, merchant_id, email, created_at, updated_at
	`, c.MerchantID, c.Email)

	var created Customer
	if err := row.Scan(&created.ID, &created.MerchantID, &created.Email, &created.CreatedAt, &created.UpdatedAt); err != nil {
		return Customer{}, fmt.Errorf("insert customer: %w", err)
	}

	return created, nil
}

func (r *postgresRepository) GetByID(ctx context.Context, merchantID, id string) (Customer, error) {
	row := r.db.QueryRow(ctx, `
		SELECT uuid, merchant_id, email, created_at, updated_at
		FROM customers
		WHERE uuid = $1 AND merchant_id = $2
	`, id, merchantID)

	var c Customer
	if err := row.Scan(&c.ID, &c.MerchantID, &c.Email, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Customer{}, ErrCustomerNotFound
		}
		return Customer{}, fmt.Errorf("get customer by id: %w", err)
	}

	return c, nil
}
