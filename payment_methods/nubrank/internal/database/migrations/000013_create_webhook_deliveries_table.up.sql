CREATE TABLE webhook_deliveries (
    uuid        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint_id UUID NOT NULL REFERENCES webhooks_endpoints (uuid),
    event_id    UUID NOT NULL REFERENCES events (uuid),
    attempts    INT NOT NULL DEFAULT 0,
    status      VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_webhook_deliveries_endpoint_id ON webhook_deliveries (endpoint_id);
CREATE INDEX idx_webhook_deliveries_event_id ON webhook_deliveries (event_id);
