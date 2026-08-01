-- reference_type/reference_id (rather than a single payment_id FK) lets a
-- ledger transaction originate from a payment, refund, payout, or dispute.
CREATE TABLE ledger_transactions (
    uuid           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reference_type VARCHAR(32) NOT NULL,
    reference_id   UUID NOT NULL,
    kind           VARCHAR(32) NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ledger_transactions_reference ON ledger_transactions (reference_type, reference_id);
