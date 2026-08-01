CREATE TABLE ledger_accounts (
    uuid          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id   UUID REFERENCES merchants (uuid),
    balance_minor BIGINT NOT NULL DEFAULT 0,
    status        VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- merchant_id is NULL only for the singleton platform clearing account
-- (see the seed row below); every merchant gets at most one account.
CREATE UNIQUE INDEX idx_ledger_accounts_merchant_id ON ledger_accounts (merchant_id) WHERE merchant_id IS NOT NULL;

-- The clearing account is the universal counterparty for every ledger
-- transaction, so a merchant charge/refund/payout is always double-entry
-- (a merchant-account leg and an offsetting clearing-account leg) even
-- though there's only one ledger_accounts row per real merchant.
INSERT INTO ledger_accounts (uuid, merchant_id, balance_minor) VALUES ('00000000-0000-0000-0000-000000000000', NULL, 0);
