CREATE TABLE webhooks_endpoints (
    uuid        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL REFERENCES merchants (uuid),
    url         TEXT NOT NULL,
    secret      VARCHAR(64) NOT NULL,
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_webhooks_endpoints_merchant_id ON webhooks_endpoints (merchant_id);
