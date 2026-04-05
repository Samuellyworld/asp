-- ML analytics schema — tables for ensemble predictions, pattern detection,
-- drift monitoring, autotuner results, and RL agent state.

-- ensemble_predictions — stores ensemble model prediction runs
CREATE TABLE IF NOT EXISTS ensemble_predictions (
    id                  SERIAL PRIMARY KEY,
    symbol              VARCHAR(20) NOT NULL,
    timeframe           VARCHAR(10) NOT NULL,
    direction           VARCHAR(10) NOT NULL CHECK (direction IN ('UP', 'DOWN', 'NEUTRAL')),
    magnitude           DOUBLE PRECISION NOT NULL,
    confidence          DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    predicted_price     DOUBLE PRECISION,
    current_price       DOUBLE PRECISION NOT NULL,
    model_count         INTEGER NOT NULL,
    model_details       JSONB DEFAULT '{}'::jsonb,
    is_ensemble         BOOLEAN DEFAULT TRUE,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_ensemble_predictions_symbol ON ensemble_predictions(symbol, created_at DESC);
CREATE INDEX idx_ensemble_predictions_created_at ON ensemble_predictions(created_at);

-- pattern_detections — detected chart patterns
CREATE TABLE IF NOT EXISTS pattern_detections (
    id                  SERIAL PRIMARY KEY,
    symbol              VARCHAR(20) NOT NULL,
    pattern_name        VARCHAR(50) NOT NULL,
    direction           VARCHAR(10) NOT NULL CHECK (direction IN ('bullish', 'bearish', 'neutral')),
    confidence          DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    signal              VARCHAR(20) NOT NULL,
    signal_strength     DOUBLE PRECISION DEFAULT 0,
    pattern_count       INTEGER DEFAULT 0,
    summary             TEXT,
    all_patterns        JSONB DEFAULT '[]'::jsonb,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_pattern_detections_symbol ON pattern_detections(symbol, created_at DESC);
CREATE INDEX idx_pattern_detections_pattern ON pattern_detections(pattern_name);

-- drift_checks — concept drift monitoring log
CREATE TABLE IF NOT EXISTS drift_checks (
    id                  SERIAL PRIMARY KEY,
    drift_detected      BOOLEAN NOT NULL,
    reason              TEXT NOT NULL,
    recommendation      TEXT NOT NULL,
    checks              JSONB DEFAULT '{}'::jsonb,
    checked_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_drift_checks_detected ON drift_checks(drift_detected, checked_at DESC);
CREATE INDEX idx_drift_checks_at ON drift_checks(checked_at);

-- retrain_runs — model retraining history
CREATE TABLE IF NOT EXISTS retrain_runs (
    id                      SERIAL PRIMARY KEY,
    success                 BOOLEAN NOT NULL,
    message                 TEXT,
    reason                  TEXT,
    promoted                BOOLEAN DEFAULT FALSE,
    new_model_metrics       JSONB,
    current_model_metrics   JSONB,
    training_samples        INTEGER,
    validation_samples      INTEGER,
    epochs                  INTEGER,
    started_at              TIMESTAMPTZ DEFAULT NOW(),
    completed_at            TIMESTAMPTZ
);

CREATE INDEX idx_retrain_runs_promoted ON retrain_runs(promoted) WHERE promoted = TRUE;
CREATE INDEX idx_retrain_runs_at ON retrain_runs(started_at);

-- autotuner_snapshots — per-regime parameter tuning state
CREATE TABLE IF NOT EXISTS autotuner_snapshots (
    id                  SERIAL PRIMARY KEY,
    regime              VARCHAR(30) NOT NULL,
    symbol              VARCHAR(20),
    parameters          JSONB NOT NULL,
    metrics             JSONB NOT NULL,
    trade_count         INTEGER DEFAULT 0,
    win_rate            DOUBLE PRECISION,
    avg_pnl             DOUBLE PRECISION,
    sharpe_ratio        DOUBLE PRECISION,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_autotuner_regime ON autotuner_snapshots(regime, created_at DESC);

-- rl_training_episodes — RL agent training history
CREATE TABLE IF NOT EXISTS rl_training_episodes (
    id                  SERIAL PRIMARY KEY,
    success             BOOLEAN NOT NULL,
    reason              TEXT,
    episodes            INTEGER,
    avg_reward_last_20  DOUBLE PRECISION,
    avg_pnl_last_20     DOUBLE PRECISION,
    best_pnl            DOUBLE PRECISION,
    final_epsilon       DOUBLE PRECISION,
    initial_balance     DOUBLE PRECISION,
    started_at          TIMESTAMPTZ DEFAULT NOW(),
    completed_at        TIMESTAMPTZ
);

CREATE INDEX idx_rl_episodes_at ON rl_training_episodes(started_at);

-- walk_forward_runs — walk-forward validation results
CREATE TABLE IF NOT EXISTS walk_forward_runs (
    id                      SERIAL PRIMARY KEY,
    success                 BOOLEAN NOT NULL,
    reason                  TEXT,
    n_splits                INTEGER,
    folds                   JSONB,
    avg_direction_accuracy  DOUBLE PRECISION,
    avg_magnitude_rmse      DOUBLE PRECISION,
    created_at              TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_walk_forward_at ON walk_forward_runs(created_at);

-- slippage_records — execution quality tracking (expected vs actual fill)
CREATE TABLE IF NOT EXISTS slippage_records (
    id                  SERIAL PRIMARY KEY,
    user_id             INTEGER REFERENCES users(id) ON DELETE SET NULL,
    symbol              VARCHAR(20) NOT NULL,
    side                VARCHAR(10) NOT NULL CHECK (side IN ('BUY', 'SELL')),
    expected_price      DOUBLE PRECISION NOT NULL,
    actual_price        DOUBLE PRECISION NOT NULL,
    slippage_bps        DOUBLE PRECISION NOT NULL,
    quantity            DOUBLE PRECISION NOT NULL,
    is_paper            BOOLEAN DEFAULT TRUE,
    recorded_at         TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_slippage_symbol ON slippage_records(symbol, recorded_at DESC);
CREATE INDEX idx_slippage_user ON slippage_records(user_id) WHERE user_id IS NOT NULL;
