package paymentmethods

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

func (r *postgresRepository) CreatePaymentMethod(ctx context.Context, pm PaymentMethod) (PaymentMethod, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO payment_methods (customer_id, token, brand, last4, expire_year)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING uuid, customer_id, token, brand, last4, expire_year, created_at, updated_at
	`, pm.CustomerID, pm.Token, pm.Brand, pm.Last4, pm.ExpireYear)

	var created PaymentMethod
	if err := row.Scan(
		&created.ID,
		&created.CustomerID,
		&created.Token,
		&created.Brand,
		&created.Last4,
		&created.ExpireYear,
		&created.CreatedAt,
		&created.UpdatedAt,
	); err != nil {
		return PaymentMethod{}, fmt.Errorf("insert payment method: %w", err)
	}

	return created, nil
}

func (r *postgresRepository) GetByID(ctx context.Context, customerID, id string) (PaymentMethod, error) {
	row := r.db.QueryRow(ctx, `
		SELECT uuid, customer_id, token, brand, last4, expire_year, created_at, updated_at
		FROM payment_methods
		WHERE uuid = $1 AND customer_id = $2
	`, id, customerID)

	var pm PaymentMethod
	if err := row.Scan(
		&pm.ID,
		&pm.CustomerID,
		&pm.Token,
		&pm.Brand,
		&pm.Last4,
		&pm.ExpireYear,
		&pm.CreatedAt,
		&pm.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentMethod{}, ErrPaymentMethodNotFound
		}
		return PaymentMethod{}, fmt.Errorf("get payment method by id: %w", err)
	}

	return pm, nil
}
