CREATE TABLE ledger_entries (
    uuid         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    txn_id       UUID NOT NULL REFERENCES ledger_transactions (uuid),
    account_id   UUID NOT NULL REFERENCES ledger_accounts (uuid),
    amount_minor BIGINT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ledger_entries_txn_id ON ledger_entries (txn_id);
CREATE INDEX idx_ledger_entries_account_id ON ledger_entries (account_id);
