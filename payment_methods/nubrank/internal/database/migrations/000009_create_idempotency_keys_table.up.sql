CREATE TABLE idempotency_keys (
    uuid            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id     UUID NOT NULL REFERENCES merchants (uuid),
    key             VARCHAR(255) NOT NULL,
    request_hash    VARCHAR(64) NOT NULL,
    response_status INT,
    response_body   BYTEA,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_idempotency_keys_merchant_key ON idempotency_keys (merchant_id, key);
