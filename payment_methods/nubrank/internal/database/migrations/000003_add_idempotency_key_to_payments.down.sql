DROP INDEX IF EXISTS idx_payments_merchant_idempotency_key;
ALTER TABLE payments DROP COLUMN idempotency_key;
