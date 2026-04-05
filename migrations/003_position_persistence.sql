-- position persistence: add missing columns for paper trading recovery on restart

-- spot paper positions need: position_size, close_reason, close_price, platform, internal_id, action
ALTER TABLE positions ADD COLUMN IF NOT EXISTS internal_id VARCHAR(64) UNIQUE;
ALTER TABLE positions ADD COLUMN IF NOT EXISTS action VARCHAR(10) CHECK (action IN ('BUY', 'SELL', 'HOLD'));
ALTER TABLE positions ADD COLUMN IF NOT EXISTS position_size DECIMAL(20, 8);
ALTER TABLE positions ADD COLUMN IF NOT EXISTS close_reason VARCHAR(30);
ALTER TABLE positions ADD COLUMN IF NOT EXISTS close_price DECIMAL(20, 8);
ALTER TABLE positions ADD COLUMN IF NOT EXISTS platform VARCHAR(20);

-- leverage positions need additional columns
ALTER TABLE positions ADD COLUMN IF NOT EXISTS mark_price DECIMAL(20, 8);
ALTER TABLE positions ADD COLUMN IF NOT EXISTS notional_value DECIMAL(20, 8);
ALTER TABLE positions ADD COLUMN IF NOT EXISTS margin_type VARCHAR(20) DEFAULT 'isolated';
ALTER TABLE positions ADD COLUMN IF NOT EXISTS funding_paid DECIMAL(20, 8) DEFAULT 0;

-- index for fast startup recovery: load all open paper positions
CREATE INDEX IF NOT EXISTS idx_positions_paper_open
    ON positions(is_paper, status) WHERE is_paper = TRUE AND status = 'OPEN';

-- index for internal_id lookups during close/adjust operations
CREATE INDEX IF NOT EXISTS idx_positions_internal_id
    ON positions(internal_id) WHERE internal_id IS NOT NULL;
