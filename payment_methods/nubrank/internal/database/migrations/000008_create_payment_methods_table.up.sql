CREATE TABLE payment_methods (
    uuid            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id     UUID NOT NULL REFERENCES customers (uuid),
    token           VARCHAR(64) NOT NULL UNIQUE,
    brand           VARCHAR(32) NOT NULL,
    last4           VARCHAR(4) NOT NULL,
    expire_year     INT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payment_methods_customer_id ON payment_methods (customer_id);
