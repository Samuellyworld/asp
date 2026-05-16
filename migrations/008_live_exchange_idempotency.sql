-- live exchange routing and idempotency metadata.

ALTER TABLE trades ADD COLUMN IF NOT EXISTS exchange VARCHAR(30);

CREATE UNIQUE INDEX IF NOT EXISTS idx_trades_live_exchange_order_unique
    ON trades(exchange, exchange_order_id)
    WHERE exchange IS NOT NULL AND exchange_order_id IS NOT NULL AND is_paper = FALSE;

ALTER TABLE positions ADD COLUMN IF NOT EXISTS exchange VARCHAR(30);
ALTER TABLE positions ADD COLUMN IF NOT EXISTS main_order_id BIGINT;
ALTER TABLE positions ADD COLUMN IF NOT EXISTS sl_order_id BIGINT;
ALTER TABLE positions ADD COLUMN IF NOT EXISTS tp_order_id BIGINT;

ALTER TABLE opportunities
    DROP CONSTRAINT IF EXISTS opportunities_status_check;

ALTER TABLE opportunities
    ADD CONSTRAINT opportunities_status_check
    CHECK (status IN ('pending', 'executing', 'approved', 'rejected', 'expired', 'modified'));
