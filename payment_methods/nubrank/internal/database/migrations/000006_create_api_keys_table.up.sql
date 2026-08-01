CREATE TABLE api_keys (
    uuid            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id     UUID NOT NULL REFERENCES merchants (uuid),
    key_hash        VARCHAR(64) NOT NULL UNIQUE,
    scope           VARCHAR(32) NOT NULL DEFAULT 'full_access',
    last_used_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_api_keys_merchant_id ON api_keys (merchant_id);
