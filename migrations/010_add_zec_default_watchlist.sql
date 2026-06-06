-- Add ZEC/USDT to the default watchlist for new and existing active users.

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
        (p_user_id, 'POL/USDT', 10),
        (p_user_id, 'ZEC/USDT', 11)
    ON CONFLICT (user_id, symbol) DO NOTHING;
END;
$$ LANGUAGE plpgsql;

INSERT INTO watchlists (user_id, symbol, priority, is_active)
SELECT id, 'ZEC/USDT', 11, TRUE
FROM users
WHERE is_active = TRUE
ON CONFLICT (user_id, symbol) DO UPDATE SET
    is_active = TRUE,
    priority = EXCLUDED.priority;
