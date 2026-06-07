import numpy as np
import pytest

import train


def make_candles(n=100):
    candles = []
    for i in range(n):
        close = 100.0 + i
        candles.append({
            "open": close - 0.25,
            "high": close + 0.5,
            "low": close - 0.5,
            "close": close,
            "volume": 1000.0 + i * 10,
        })
    return candles


def test_prepare_dataset_uses_training_features_for_scaler_only():
    candles = make_candles(100)
    seq_length = 10
    pred_horizon = 1
    split = int(len(candles) * 0.8)

    X_train, y_train, X_val, y_val, mean, std = train.prepare_dataset(
        {"BTC/USDT": candles},
        seq_length=seq_length,
        pred_horizon=pred_horizon,
    )

    train_features, _ = train.feature_engineer(candles[:split])
    all_features, _ = train.feature_engineer(candles)

    np.testing.assert_allclose(mean, train_features.mean(axis=0))
    assert not np.allclose(mean, all_features.mean(axis=0))
    assert len(X_train) == split - pred_horizon - seq_length
    assert len(y_train) == len(X_train)
    assert len(X_val) == len(candles) - pred_horizon - split
    assert len(y_val) == len(X_val)
    assert std.shape == mean.shape


def test_prepare_dataset_keeps_validation_targets_chronological():
    candles = make_candles(100)
    split = int(len(candles) * 0.8)

    _, y_train, _, y_val, _, _ = train.prepare_dataset(
        {"BTC/USDT": candles},
        seq_length=10,
        pred_horizon=1,
    )

    last_train_target_index = split - 2
    expected_last_train_return = (
        (candles[last_train_target_index + 1]["close"] - candles[last_train_target_index]["close"])
        / candles[last_train_target_index]["close"]
        * 100
    )
    expected_first_val_return = (
        (candles[split + 1]["close"] - candles[split]["close"])
        / candles[split]["close"]
        * 100
    )

    assert y_train[-1][0] == 1.0
    assert y_val[0][0] == 1.0
    assert y_train[-1][1] == pytest.approx(expected_last_train_return)
    assert y_val[0][1] == pytest.approx(expected_first_val_return)


def test_db_password_must_be_configured(monkeypatch):
    monkeypatch.delenv("DATABASE_PASSWORD", raising=False)
    with pytest.raises(RuntimeError, match="DATABASE_PASSWORD must be set"):
        train.db_connect_kwargs()
