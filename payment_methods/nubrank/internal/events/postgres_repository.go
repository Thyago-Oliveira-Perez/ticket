package events

import (
	"context"
	"fmt"

	"nubrank/internal/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	// pool is used only for post-commit status updates, which happen after
	// the transaction that created the delivery row has already finished.
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) InsertEvent(ctx context.Context, q database.Querier, e Event) (Event, error) {
	row := q.QueryRow(ctx, `
		INSERT INTO events (merchant_id, type, resource_id, payload)
		VALUES ($1, $2, $3, $4)
		RETURNING uuid, merchant_id, type, resource_id, payload, created_at
	`, e.MerchantID, e.Type, e.ResourceID, e.Payload)

	var created Event
	if err := row.Scan(&created.ID, &created.MerchantID, &created.Type, &created.ResourceID, &created.Payload, &created.CreatedAt); err != nil {
		return Event{}, fmt.Errorf("insert event: %w", err)
	}

	return created, nil
}

func (r *postgresRepository) InsertDelivery(ctx context.Context, q database.Querier, endpointID, eventID string) (Delivery, error) {
	row := q.QueryRow(ctx, `
		INSERT INTO webhook_deliveries (endpoint_id, event_id, status)
		VALUES ($1, $2, $3)
		RETURNING uuid, endpoint_id, event_id, attempts, status
	`, endpointID, eventID, DeliveryStatusPending)

	var d Delivery
	if err := row.Scan(&d.ID, &d.EndpointID, &d.EventID, &d.Attempts, &d.Status); err != nil {
		return Delivery{}, fmt.Errorf("insert webhook delivery: %w", err)
	}

	return d, nil
}

func (r *postgresRepository) UpdateDeliveryStatus(ctx context.Context, id, status string, attempts int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET status = $2, attempts = $3, updated_at = now()
		WHERE uuid = $1
	`, id, status, attempts)
	if err != nil {
		return fmt.Errorf("update webhook delivery status: %w", err)
	}
	return nil
}
