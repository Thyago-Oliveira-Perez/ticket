ALTER TABLE payments ADD COLUMN idempotency_key VARCHAR(255);

CREATE UNIQUE INDEX idx_payments_merchant_idempotency_key
    ON payments (merchant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
