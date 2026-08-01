package ledger

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

func (r *postgresRepository) GetOrCreateMerchantAccount(ctx context.Context, merchantID string) (Account, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO ledger_accounts (merchant_id)
		VALUES ($1)
		ON CONFLICT (merchant_id) WHERE merchant_id IS NOT NULL DO NOTHING
		RETURNING uuid, merchant_id, balance_minor, created_at, updated_at
	`, merchantID)

	acc, err := scanAccount(row)
	if err == nil {
		return acc, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Account{}, fmt.Errorf("insert ledger account: %w", err)
	}

	// ON CONFLICT DO NOTHING returned no row: the account already exists.
	row = r.db.QueryRow(ctx, `
		SELECT uuid, merchant_id, balance_minor, created_at, updated_at
		FROM ledger_accounts
		WHERE merchant_id = $1
	`, merchantID)
	return scanAccount(row)
}

func (r *postgresRepository) LockAccount(ctx context.Context, id string) (Account, error) {
	row := r.db.QueryRow(ctx, `
		SELECT uuid, merchant_id, balance_minor, created_at, updated_at
		FROM ledger_accounts
		WHERE uuid = $1
		FOR UPDATE
	`, id)
	return scanAccount(row)
}

func (r *postgresRepository) UpdateBalance(ctx context.Context, id string, newBalanceMinor int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE ledger_accounts SET balance_minor = $2, updated_at = now() WHERE uuid = $1
	`, id, newBalanceMinor)
	if err != nil {
		return fmt.Errorf("update ledger account balance: %w", err)
	}
	return nil
}

func (r *postgresRepository) InsertTransaction(ctx context.Context, referenceType, referenceID, kind string) (Transaction, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO ledger_transactions (reference_type, reference_id, kind)
		VALUES ($1, $2, $3)
		RETURNING uuid, reference_type, reference_id, kind, created_at
	`, referenceType, referenceID, kind)

	var txn Transaction
	if err := row.Scan(&txn.ID, &txn.ReferenceType, &txn.ReferenceID, &txn.Kind, &txn.CreatedAt); err != nil {
		return Transaction{}, fmt.Errorf("insert ledger transaction: %w", err)
	}
	return txn, nil
}

func (r *postgresRepository) InsertEntry(ctx context.Context, transactionID, accountID string, amountMinor int64) (Entry, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO ledger_entries (txn_id, account_id, amount_minor)
		VALUES ($1, $2, $3)
		RETURNING uuid, txn_id, account_id, amount_minor, created_at
	`, transactionID, accountID, amountMinor)

	var e Entry
	if err := row.Scan(&e.ID, &e.TransactionID, &e.AccountID, &e.AmountMinor, &e.CreatedAt); err != nil {
		return Entry{}, fmt.Errorf("insert ledger entry: %w", err)
	}
	return e, nil
}

func scanAccount(row pgx.Row) (Account, error) {
	var a Account
	if err := row.Scan(&a.ID, &a.MerchantID, &a.BalanceMinor, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return Account{}, err
	}
	return a, nil
}
