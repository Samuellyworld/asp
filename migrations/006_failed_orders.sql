-- dead-letter queue for failed order placements.
-- tracks orders that failed so they can be reviewed and manually recovered.

CREATE TABLE IF NOT EXISTS failed_orders (
    id              SERIAL PRIMARY KEY,
    user_id         INTEGER NOT NULL,
    position_id     TEXT,               -- internal position ID if applicable
    symbol          TEXT NOT NULL,
    side            TEXT NOT NULL,       -- 'BUY' or 'SELL'
    order_type      TEXT NOT NULL,       -- 'MARKET', 'STOP_LOSS_LIMIT', etc.
    quantity        DOUBLE PRECISION NOT NULL DEFAULT 0,
    price           DOUBLE PRECISION NOT NULL DEFAULT 0,
    stop_price      DOUBLE PRECISION NOT NULL DEFAULT 0,
    trade_type      TEXT NOT NULL DEFAULT 'SPOT',  -- 'SPOT', 'FUTURES'
    error_message   TEXT NOT NULL,
    resolved        BOOLEAN NOT NULL DEFAULT FALSE,
    resolved_at     TIMESTAMPTZ,
    resolve_note    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_failed_orders_user ON failed_orders(user_id);
CREATE INDEX IF NOT EXISTS idx_failed_orders_unresolved ON failed_orders(resolved) WHERE resolved = FALSE;
