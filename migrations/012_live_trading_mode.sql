-- persist live trading mode across application restarts.

ALTER TABLE users ADD COLUMN IF NOT EXISTS live_enabled BOOLEAN NOT NULL DEFAULT FALSE;

