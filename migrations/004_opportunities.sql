-- opportunities table for persisting trade opportunities through the approval flow

CREATE TABLE IF NOT EXISTS opportunities (
    id              VARCHAR(64) PRIMARY KEY,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    symbol          VARCHAR(20) NOT NULL,
    action          VARCHAR(10) NOT NULL CHECK (action IN ('BUY', 'SELL', 'HOLD')),
    status          VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'modified')),
    confidence      DECIMAL(5, 2),
    entry_price     DECIMAL(20, 8),
    stop_loss       DECIMAL(20, 8),
    take_profit     DECIMAL(20, 8),
    position_size   DECIMAL(20, 8),
    risk_reward     DECIMAL(10, 4),
    reasoning       TEXT,
    modified_entry  DECIMAL(20, 8),
    modified_sl     DECIMAL(20, 8),
    modified_tp     DECIMAL(20, 8),
    modified_size   DECIMAL(20, 8),
    use_leverage    BOOLEAN DEFAULT FALSE,
    leverage        INTEGER DEFAULT 1,
    position_side   VARCHAR(10) CHECK (position_side IN ('LONG', 'SHORT', '')),
    platform        VARCHAR(20) CHECK (platform IN ('telegram', 'discord', 'whatsapp', 'cli')),
    message_id      INTEGER,
    channel_id      VARCHAR(100),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    resolved_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_opportunities_user_id ON opportunities(user_id);
CREATE INDEX IF NOT EXISTS idx_opportunities_status ON opportunities(status) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_opportunities_user_pending ON opportunities(user_id, status) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_opportunities_created_at ON opportunities(created_at);

-- slippage records for adaptive slippage model

CREATE TABLE IF NOT EXISTS slippage_records (
    id              SERIAL PRIMARY KEY,
    symbol          VARCHAR(20) NOT NULL,
    side            VARCHAR(10) NOT NULL CHECK (side IN ('BUY', 'SELL')),
    expected_price  DECIMAL(20, 8) NOT NULL,
    actual_price    DECIMAL(20, 8) NOT NULL,
    slippage_bps    DECIMAL(10, 4) NOT NULL,
    quantity        DECIMAL(20, 8) NOT NULL,
    is_paper        BOOLEAN DEFAULT FALSE,
    recorded_at     TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_slippage_records_symbol ON slippage_records(symbol);
CREATE INDEX IF NOT EXISTS idx_slippage_records_symbol_recent ON slippage_records(symbol, recorded_at DESC) WHERE is_paper = FALSE;
