-- Refund history now lives in the refunds table (which supports partial
-- and repeated refunds); payments.status carries "refunded" /
-- "partially_refunded" same as before.
ALTER TABLE payments DROP COLUMN refunded_at;
