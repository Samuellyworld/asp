-- trading bot database schema

-- enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

--  users - core user table
CREATE TABLE IF NOT EXISTS users (
    id                  SERIAL PRIMARY KEY,
    uuid                UUID DEFAULT uuid_generate_v4() UNIQUE NOT NULL,
    telegram_id         BIGINT UNIQUE,
    discord_id          BIGINT UNIQUE,
    whatsapp_id         VARCHAR(50) UNIQUE,
    username            VARCHAR(100),
    is_activated        BOOLEAN DEFAULT FALSE,
    is_banned           BOOLEAN DEFAULT FALSE,
    trading_mode        VARCHAR(20) DEFAULT 'paper' CHECK (trading_mode IN ('paper', 'live')),
    leverage_enabled    BOOLEAN DEFAULT FALSE,
    last_active_channel VARCHAR(20) CHECK (last_active_channel IN ('telegram', 'discord', 'whatsapp', 'cli')),
    last_active_at      TIMESTAMPTZ,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_users_telegram_id ON users(telegram_id) WHERE telegram_id IS NOT NULL;
CREATE INDEX idx_users_discord_id ON users(discord_id) WHERE discord_id IS NOT NULL;
CREATE INDEX idx_users_is_activated ON users(is_activated) WHERE is_activated = TRUE;

-- user_api_credentials - encrypted exchange api keys
CREATE TABLE IF NOT EXISTS user_api_credentials (
    id                      SERIAL PRIMARY KEY,
    user_id                 INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    exchange                VARCHAR(50) NOT NULL DEFAULT 'binance',
    api_key_encrypted       BYTEA NOT NULL,
    api_secret_encrypted    BYTEA NOT NULL,
    salt                    BYTEA NOT NULL,
    permissions             JSONB DEFAULT '{"spot": false, "futures": false, "withdraw": false}'::jsonb,
    is_testnet              BOOLEAN DEFAULT TRUE,
    is_valid                BOOLEAN DEFAULT TRUE,
    last_validated_at       TIMESTAMPTZ,
    created_at              TIMESTAMPTZ DEFAULT NOW(),
    updated_at              TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, exchange)
);

CREATE INDEX idx_user_api_credentials_user_id ON user_api_credentials(user_id);

-- api_key_access_log - audit trail for key decryption
CREATE TABLE IF NOT EXISTS api_key_access_log (
    id                  SERIAL PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id       INTEGER NOT NULL REFERENCES user_api_credentials(id) ON DELETE CASCADE,
    action              VARCHAR(50) NOT NULL,
    ip_address          INET,
    user_agent          VARCHAR(500),
    success             BOOLEAN NOT NULL,
    error_message       TEXT,
    accessed_at         TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_api_key_access_log_user_id ON api_key_access_log(user_id);
CREATE INDEX idx_api_key_access_log_accessed_at ON api_key_access_log(accessed_at);

--  trades - individual trade records
CREATE TABLE IF NOT EXISTS trades (
    id                  SERIAL PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    position_id         INTEGER,
    exchange_order_id   VARCHAR(100),
    symbol              VARCHAR(20) NOT NULL,
    side                VARCHAR(10) NOT NULL CHECK (side IN ('BUY', 'SELL')),
    trade_type          VARCHAR(20) NOT NULL CHECK (trade_type IN ('SPOT', 'FUTURES_LONG', 'FUTURES_SHORT')),
    quantity            DECIMAL(20, 8) NOT NULL,
    price               DECIMAL(20, 8) NOT NULL,
    fee                 DECIMAL(20, 8) DEFAULT 0,
    fee_currency        VARCHAR(10),
    is_paper            BOOLEAN DEFAULT TRUE,
    executed_at         TIMESTAMPTZ DEFAULT NOW(),
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_trades_user_id ON trades(user_id);
CREATE INDEX idx_trades_symbol ON trades(symbol);
CREATE INDEX idx_trades_executed_at ON trades(executed_at);
CREATE INDEX idx_trades_position_id ON trades(position_id) WHERE position_id IS NOT NULL;

-- positions - open and closed positions
CREATE TABLE IF NOT EXISTS positions (
    id                      SERIAL PRIMARY KEY,
    user_id                 INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    symbol                  VARCHAR(20) NOT NULL,
    side                    VARCHAR(10) NOT NULL CHECK (side IN ('LONG', 'SHORT')),
    position_type           VARCHAR(20) NOT NULL CHECK (position_type IN ('SPOT', 'FUTURES')),
    status                  VARCHAR(20) DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'CLOSED', 'LIQUIDATED')),
    entry_price             DECIMAL(20, 8) NOT NULL,
    current_price           DECIMAL(20, 8),
    quantity                DECIMAL(20, 8) NOT NULL,
    margin                  DECIMAL(20, 8),
    leverage                INTEGER DEFAULT 1,
    stop_loss               DECIMAL(20, 8),
    take_profit             DECIMAL(20, 8),
    trailing_stop_pct       DECIMAL(5, 2),
    liquidation_price       DECIMAL(20, 8),
    unrealized_pnl          DECIMAL(20, 8) DEFAULT 0,
    realized_pnl            DECIMAL(20, 8) DEFAULT 0,
    is_paper                BOOLEAN DEFAULT TRUE,
    ai_decision_id          INTEGER,
    opened_at               TIMESTAMPTZ DEFAULT NOW(),
    closed_at               TIMESTAMPTZ,
    last_updated_at         TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_positions_user_id ON positions(user_id);
CREATE INDEX idx_positions_status ON positions(status) WHERE status = 'OPEN';
CREATE INDEX idx_positions_symbol ON positions(symbol);

-- watchlists - user's tracked symbols
CREATE TABLE IF NOT EXISTS watchlists (
    id              SERIAL PRIMARY KEY,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    symbol          VARCHAR(20) NOT NULL,
    is_active       BOOLEAN DEFAULT TRUE,
    priority        INTEGER DEFAULT 0,
    added_at        TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_watchlists_user_symbol ON watchlists(user_id, symbol);
CREATE INDEX idx_watchlists_user_id ON watchlists(user_id);

-- alerts - price and indicator alerts
CREATE TABLE IF NOT EXISTS alerts (
    id                  SERIAL PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    symbol              VARCHAR(20) NOT NULL,
    alert_type          VARCHAR(50) NOT NULL CHECK (alert_type IN ('PRICE_ABOVE', 'PRICE_BELOW', 'RSI_OVERBOUGHT', 'RSI_OVERSOLD', 'MACD_CROSS', 'VOLUME_SPIKE', 'CUSTOM')),
    condition_value     DECIMAL(20, 8),
    condition_params    JSONB,
    is_active           BOOLEAN DEFAULT TRUE,
    is_triggered        BOOLEAN DEFAULT FALSE,
    triggered_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_alerts_user_id ON alerts(user_id);
CREATE INDEX idx_alerts_active ON alerts(is_active, is_triggered) WHERE is_active = TRUE AND is_triggered = FALSE;

-- scanning_preferences - per-user scanner settings
CREATE TABLE IF NOT EXISTS scanning_preferences (
    id                      SERIAL PRIMARY KEY,
    user_id                 INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    min_confidence          INTEGER DEFAULT 80 CHECK (min_confidence >= 0 AND min_confidence <= 100),
    scan_interval_minutes   INTEGER DEFAULT 5,
    enabled_timeframes      JSONB DEFAULT '["1h", "4h", "1d"]'::jsonb,
    enabled_indicators      JSONB DEFAULT '["RSI", "MACD", "BOLLINGER", "EMA", "VOLUME"]'::jsonb,
    use_ml_predictions      BOOLEAN DEFAULT TRUE,
    use_sentiment_analysis  BOOLEAN DEFAULT TRUE,
    is_scanning_enabled     BOOLEAN DEFAULT TRUE,
    created_at              TIMESTAMPTZ DEFAULT NOW(),
    updated_at              TIMESTAMPTZ DEFAULT NOW()
);

-- notification_preferences - how/when to notify user
CREATE TABLE IF NOT EXISTS notification_preferences (
    id                          SERIAL PRIMARY KEY,
    user_id                     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    max_daily_notifications     INTEGER DEFAULT 10,
    opportunity_notifications   BOOLEAN DEFAULT TRUE,
    trade_executed_notifications BOOLEAN DEFAULT TRUE,
    milestone_notifications     BOOLEAN DEFAULT TRUE,
    periodic_update_minutes     INTEGER DEFAULT 30,
    daily_summary_enabled       BOOLEAN DEFAULT TRUE,
    daily_summary_hour          INTEGER DEFAULT 20 CHECK (daily_summary_hour >= 0 AND daily_summary_hour <= 23),
    timezone                    VARCHAR(50) DEFAULT 'UTC',
    created_at                  TIMESTAMPTZ DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ DEFAULT NOW()
);

-- position_notifications - notifications sent for positions
CREATE TABLE IF NOT EXISTS position_notifications (
    id                  SERIAL PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    position_id         INTEGER NOT NULL REFERENCES positions(id) ON DELETE CASCADE,
    notification_type   VARCHAR(50) NOT NULL,
    message             TEXT,
    channel             VARCHAR(20) CHECK (channel IN ('telegram', 'discord', 'whatsapp')),
    sent_at             TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_position_notifications_position_id ON position_notifications(position_id);
CREATE INDEX idx_position_notifications_sent_at ON position_notifications(sent_at);

-- daily_notification_log - track daily notification count
CREATE TABLE IF NOT EXISTS daily_notification_log (
    id                  SERIAL PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date                DATE NOT NULL DEFAULT CURRENT_DATE,
    notification_count  INTEGER DEFAULT 0,
    last_notification_at TIMESTAMPTZ,
    UNIQUE(user_id, date)
);

CREATE INDEX idx_daily_notification_log_user_date ON daily_notification_log(user_id, date);

-- ai_decisions - claude ai decision logs
CREATE TABLE IF NOT EXISTS ai_decisions (
    id                  SERIAL PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    symbol              VARCHAR(20) NOT NULL,
    timeframe           VARCHAR(10),
    decision            VARCHAR(20) NOT NULL CHECK (decision IN ('BUY', 'SELL', 'HOLD', 'CLOSE')),
    confidence          INTEGER NOT NULL CHECK (confidence >= 0 AND confidence <= 100),
    entry_price         DECIMAL(20, 8),
    stop_loss           DECIMAL(20, 8),
    take_profit         DECIMAL(20, 8),
    position_size_usd   DECIMAL(20, 2),
    risk_reward_ratio   DECIMAL(5, 2),
    reasoning           TEXT,
    indicators_data     JSONB,
    ml_prediction       JSONB,
    sentiment_data      JSONB,
    prompt_tokens       INTEGER,
    completion_tokens   INTEGER,
    latency_ms          INTEGER,
    was_approved        BOOLEAN,
    was_executed        BOOLEAN DEFAULT FALSE,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_ai_decisions_user_id ON ai_decisions(user_id);
CREATE INDEX idx_ai_decisions_symbol ON ai_decisions(symbol);
CREATE INDEX idx_ai_decisions_created_at ON ai_decisions(created_at);

-- daily_stats - daily trading statistics per user
CREATE TABLE IF NOT EXISTS daily_stats (
    id                  SERIAL PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date                DATE NOT NULL DEFAULT CURRENT_DATE,
    total_trades        INTEGER DEFAULT 0,
    winning_trades      INTEGER DEFAULT 0,
    losing_trades       INTEGER DEFAULT 0,
    realized_pnl        DECIMAL(20, 2) DEFAULT 0,
    unrealized_pnl      DECIMAL(20, 2) DEFAULT 0,
    fees_paid           DECIMAL(20, 8) DEFAULT 0,
    funding_paid        DECIMAL(20, 8) DEFAULT 0,
    ai_decisions_made   INTEGER DEFAULT 0,
    ai_decisions_approved INTEGER DEFAULT 0,
    notifications_sent  INTEGER DEFAULT 0,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, date)
);

CREATE INDEX idx_daily_stats_user_date ON daily_stats(user_id, date);

-- trading_preferences - user's trading settings
CREATE TABLE IF NOT EXISTS trading_preferences (
    id                      SERIAL PRIMARY KEY,
    user_id                 INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    default_position_size   DECIMAL(20, 2) DEFAULT 100,
    max_position_size       DECIMAL(20, 2) DEFAULT 1000,
    max_open_positions      INTEGER DEFAULT 5,
    daily_loss_limit        DECIMAL(20, 2) DEFAULT 100,
    default_stop_loss_pct   DECIMAL(5, 2) DEFAULT 2.0,
    default_take_profit_pct DECIMAL(5, 2) DEFAULT 4.0,
    max_leverage            INTEGER DEFAULT 3,
    margin_mode             VARCHAR(20) DEFAULT 'ISOLATED' CHECK (margin_mode IN ('ISOLATED', 'CROSS')),
    auto_compound           BOOLEAN DEFAULT FALSE,
    risk_per_trade_pct      DECIMAL(5, 2) DEFAULT 2.0,
    created_at              TIMESTAMPTZ DEFAULT NOW(),
    updated_at              TIMESTAMPTZ DEFAULT NOW()
);

-- opportunity_notifications - trading opportunity alerts sent
CREATE TABLE IF NOT EXISTS opportunity_notifications (
    id                  SERIAL PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ai_decision_id      INTEGER REFERENCES ai_decisions(id) ON DELETE SET NULL,
    symbol              VARCHAR(20) NOT NULL,
    direction           VARCHAR(10) NOT NULL CHECK (direction IN ('BUY', 'SELL')),
    confidence          INTEGER NOT NULL,
    channel             VARCHAR(20) NOT NULL CHECK (channel IN ('telegram', 'discord', 'whatsapp')),
    message_id          VARCHAR(100),
    status              VARCHAR(20) DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'MODIFIED', 'EXPIRED')),
    user_response       JSONB,
    expires_at          TIMESTAMPTZ,
    responded_at        TIMESTAMPTZ,
    sent_at             TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_opportunity_notifications_user_id ON opportunity_notifications(user_id);
CREATE INDEX idx_opportunity_notifications_status ON opportunity_notifications(status) WHERE status = 'PENDING';
CREATE INDEX idx_opportunity_notifications_sent_at ON opportunity_notifications(sent_at);

-- add foreign key for positions -> ai_decisions (after both tables exist)
ALTER TABLE positions ADD CONSTRAINT fk_positions_ai_decision
    FOREIGN KEY (ai_decision_id) REFERENCES ai_decisions(id) ON DELETE SET NULL;

ALTER TABLE trades ADD CONSTRAINT fk_trades_position
    FOREIGN KEY (position_id) REFERENCES positions(id) ON DELETE SET NULL;

-- create updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- apply updated_at triggers
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_user_api_credentials_updated_at BEFORE UPDATE ON user_api_credentials
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_scanning_preferences_updated_at BEFORE UPDATE ON scanning_preferences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_notification_preferences_updated_at BEFORE UPDATE ON notification_preferences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_daily_stats_updated_at BEFORE UPDATE ON daily_stats
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_trading_preferences_updated_at BEFORE UPDATE ON trading_preferences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- insert default top 10 watchlist symbols (reusable function)
CREATE OR REPLACE FUNCTION populate_default_watchlist(p_user_id INTEGER)
RETURNS void AS $$
BEGIN
    INSERT INTO watchlists (user_id, symbol, priority) VALUES
        (p_user_id, 'BTC/USDT', 1),
        (p_user_id, 'ETH/USDT', 2),
        (p_user_id, 'BNB/USDT', 3),
        (p_user_id, 'SOL/USDT', 4),
        (p_user_id, 'XRP/USDT', 5),
        (p_user_id, 'ADA/USDT', 6),
        (p_user_id, 'DOGE/USDT', 7),
        (p_user_id, 'AVAX/USDT', 8),
        (p_user_id, 'DOT/USDT', 9),
        (p_user_id, 'MATIC/USDT', 10)
    ON CONFLICT (user_id, symbol) DO NOTHING;
END;
$$ LANGUAGE plpgsql;

-- function to create default preferences for new user
CREATE OR REPLACE FUNCTION create_default_user_preferences(p_user_id INTEGER)
RETURNS void AS $$
BEGIN
    INSERT INTO scanning_preferences (user_id) VALUES (p_user_id)
    ON CONFLICT (user_id) DO NOTHING;
    
    INSERT INTO notification_preferences (user_id) VALUES (p_user_id)
    ON CONFLICT (user_id) DO NOTHING;
    
    INSERT INTO trading_preferences (user_id) VALUES (p_user_id)
    ON CONFLICT (user_id) DO NOTHING;
    
    PERFORM populate_default_watchlist(p_user_id);
END;
$$ LANGUAGE plpgsql;
