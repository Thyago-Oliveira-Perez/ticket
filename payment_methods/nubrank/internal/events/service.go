package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"nubrank/internal/database"
	"nubrank/internal/webhookendpoints"
)

// EndpointLister is the subset of webhookendpoints.Service Publish needs:
// finding which endpoints should receive a merchant's events.
type EndpointLister interface {
	ListActiveByMerchant(ctx context.Context, merchantID string) ([]webhookendpoints.Endpoint, error)
}

// Sender delivers a signed webhook event over HTTP. webhook.Sender
// satisfies this structurally.
type Sender interface {
	Send(ctx context.Context, url, secret, eventType string, data any) error
}

type Publisher interface {
	// Publish records that eventType happened to resourceID (writing the
	// event and one pending delivery row per active endpoint via q, so
	// they commit atomically with whatever state change q is also part
	// of), and returns the deliveries so the caller can Dispatch them once
	// that transaction has committed.
	Publish(ctx context.Context, q database.Querier, merchantID, eventType, resourceID string, payload any) ([]Delivery, error)
	// Dispatch attempts HTTP delivery for each delivery, updating its
	// stored status afterward. Safe to call only after the transaction
	// that produced deliveries has committed. Non-blocking: each delivery
	// is attempted in its own goroutine.
	Dispatch(ctx context.Context, deliveries []Delivery)
}

type svc struct {
	repo      Repository
	endpoints EndpointLister
	sender    Sender
}

func NewService(repo Repository, endpoints EndpointLister, sender Sender) Publisher {
	return &svc{repo: repo, endpoints: endpoints, sender: sender}
}

func (s *svc) Publish(ctx context.Context, q database.Querier, merchantID, eventType, resourceID string, payload any) ([]Delivery, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal event payload: %w", err)
	}

	event, err := s.repo.InsertEvent(ctx, q, Event{
		MerchantID: merchantID,
		Type:       eventType,
		ResourceID: resourceID,
		Payload:    payloadJSON,
	})
	if err != nil {
		return nil, err
	}

	endpoints, err := s.endpoints.ListActiveByMerchant(ctx, merchantID)
	if err != nil {
		return nil, err
	}

	deliveries := make([]Delivery, 0, len(endpoints))
	for _, ep := range endpoints {
		d, err := s.repo.InsertDelivery(ctx, q, ep.ID, event.ID)
		if err != nil {
			return nil, err
		}
		d.EndpointURL = ep.URL
		d.EndpointSecret = ep.Secret
		d.EventType = eventType
		d.Payload = payload
		deliveries = append(deliveries, d)
	}

	return deliveries, nil
}

func (s *svc) Dispatch(ctx context.Context, deliveries []Delivery) {
	for _, d := range deliveries {
		go func(d Delivery) {
			status := DeliveryStatusSucceeded
			if err := s.sender.Send(ctx, d.EndpointURL, d.EndpointSecret, d.EventType, d.Payload); err != nil {
				status = DeliveryStatusFailed
				log.Printf("events: delivery %s to %s failed: %v", d.ID, d.EndpointURL, err)
			}
			if err := s.repo.UpdateDeliveryStatus(ctx, d.ID, status, d.Attempts+1); err != nil {
				log.Println(err)
			}
		}(d)
	}
}
