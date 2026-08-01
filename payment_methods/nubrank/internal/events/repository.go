package events

import (
	"context"
	"time"

	"nubrank/internal/database"
)

const (
	DeliveryStatusPending   = "pending"
	DeliveryStatusSucceeded = "succeeded"
	DeliveryStatusFailed    = "failed"
)

type Event struct {
	ID         string
	MerchantID string
	Type       string
	ResourceID string
	Payload    []byte // JSON
	CreatedAt  time.Time
}

// Delivery is one endpoint's copy of an event, tracked separately so each
// endpoint's delivery outcome (and retry count, if nubrank ever adds
// retries) is visible independently of the others.
type Delivery struct {
	ID             string
	EndpointID     string
	EndpointURL    string
	EndpointSecret string
	EventID        string
	EventType      string
	Payload        any
	Attempts       int
	Status         string
}

type Repository interface {
	// InsertEvent writes e via q, so it can participate in the same
	// transaction as the state change it describes (the outbox insert).
	InsertEvent(ctx context.Context, q database.Querier, e Event) (Event, error)
	// InsertDelivery writes one pending delivery row via q, alongside the
	// event insert.
	InsertDelivery(ctx context.Context, q database.Querier, endpointID, eventID string) (Delivery, error)
	// UpdateDeliveryStatus records the outcome of a delivery attempt, made
	// after the transaction that created the row has already committed.
	UpdateDeliveryStatus(ctx context.Context, id, status string, attempts int) error
}
