-- merchant_id isn't in the original ERD's events table, but fanning out to
-- the right merchant's webhook endpoints requires knowing whose event this
-- is without an indirect join through whatever resource_id points at.
CREATE TABLE events (
    uuid        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL REFERENCES merchants (uuid),
    type        VARCHAR(64) NOT NULL,
    resource_id UUID NOT NULL,
    payload     JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_events_merchant_id ON events (merchant_id);
CREATE INDEX idx_events_resource_id ON events (resource_id);
