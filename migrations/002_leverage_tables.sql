-- leverage trading tables

CREATE TABLE IF NOT EXISTS funding_fee_log (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    position_id VARCHAR(64) NOT NULL,
    symbol VARCHAR(32) NOT NULL,
    funding_rate DECIMAL(20, 10) NOT NULL,
    amount DECIMAL(20, 8) NOT NULL,
    notional DECIMAL(20, 8) NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_funding_fee_user ON funding_fee_log(user_id);
CREATE INDEX IF NOT EXISTS idx_funding_fee_position ON funding_fee_log(position_id);

CREATE TABLE IF NOT EXISTS leverage_confirmations (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL UNIQUE REFERENCES users(id),
    confirmed BOOLEAN NOT NULL DEFAULT FALSE,
    confirmed_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS liquidation_alerts (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    position_id VARCHAR(64) NOT NULL,
    symbol VARCHAR(32) NOT NULL,
    alert_level VARCHAR(16) NOT NULL,
    distance_pct DECIMAL(10, 4) NOT NULL,
    mark_price DECIMAL(20, 8) NOT NULL,
    liquidation_price DECIMAL(20, 8) NOT NULL,
    alerted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_liq_alerts_user ON liquidation_alerts(user_id);
CREATE INDEX IF NOT EXISTS idx_liq_alerts_position ON liquidation_alerts(position_id);
