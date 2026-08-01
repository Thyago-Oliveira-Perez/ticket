CREATE TABLE payouts (
    uuid         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id  UUID NOT NULL REFERENCES merchants (uuid),
    amount_minor BIGINT NOT NULL,
    currency     VARCHAR(3) NOT NULL,
    status       VARCHAR(32) NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payouts_merchant_id ON payouts (merchant_id);
