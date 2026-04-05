# tests for new api endpoints: ensemble, patterns, drift, retrain, walk-forward, rl

import pytest
from httpx import AsyncClient, ASGITransport
from app.main import app


@pytest.fixture
def candles():
    """generates 50 test candles"""
    price = 40000.0
    result = []
    for i in range(50):
        price = price * 1.005
        result.append({
            "open": price * 0.999,
            "high": price * 1.005,
            "low": price * 0.995,
            "close": price,
            "volume": 1000000,
            "timestamp": i,
        })
    return result


@pytest.fixture
def large_candles():
    """generates 100 test candles for retraining"""
    price = 40000.0
    result = []
    for i in range(100):
        price = price * (1.002 + (i % 5) * 0.001)
        result.append({
            "open": price * 0.999,
            "high": price * 1.005,
            "low": price * 0.995,
            "close": price,
            "volume": 1000000,
            "timestamp": i,
        })
    return result


@pytest.mark.asyncio
async def test_root_includes_new_endpoints():
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.get("/")
    assert resp.status_code == 200
    data = resp.json()
    assert data["version"] == "0.2.0"
    endpoints = data["endpoints"]
    assert "ensemble_predict" in endpoints
    assert "detect_patterns" in endpoints
    assert "check_drift" in endpoints
    assert "retrain" in endpoints
    assert "rl_train" in endpoints


@pytest.mark.asyncio
async def test_ensemble_predict(candles):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/ensemble/predict", json={
            "symbol": "BTC/USDT",
            "candles": candles,
            "timeframe": "4h",
        })
    assert resp.status_code == 200
    data = resp.json()
    assert data["direction"] in ("up", "down")
    assert data["is_ensemble"] is True
    assert "model_count" in data


@pytest.mark.asyncio
async def test_ensemble_predict_different_timeframes(candles):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        for tf in ["1h", "4h", "1d"]:
            resp = await client.post("/ensemble/predict", json={
                "symbol": "BTC/USDT",
                "candles": candles,
                "timeframe": tf,
            })
            assert resp.status_code == 200
            assert resp.json()["timeframe"] == tf


@pytest.mark.asyncio
async def test_patterns_detect(candles):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/patterns/detect", json={
            "symbol": "BTC/USDT",
            "candles": candles,
        })
    assert resp.status_code == 200
    data = resp.json()
    assert "patterns" in data
    assert "pattern_count" in data
    assert "signal" in data
    assert "summary" in data


@pytest.mark.asyncio
async def test_patterns_detect_min_candles():
    """should require at least 20 candles"""
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/patterns/detect", json={
            "symbol": "BTC/USDT",
            "candles": [{"open": 100, "high": 105, "low": 95,
                         "close": 102, "volume": 1000, "timestamp": i}
                        for i in range(10)],
        })
    assert resp.status_code == 422


@pytest.mark.asyncio
async def test_drift_check(candles):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/drift/check", json={
            "candles": candles,
        })
    assert resp.status_code == 200
    data = resp.json()
    assert "drift_detected" in data
    assert "recommendation" in data


@pytest.mark.asyncio
async def test_retrain(large_candles):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/retrain", json={
            "candles": large_candles,
            "epochs": 2,
        })
    assert resp.status_code == 200
    data = resp.json()
    assert "success" in data


@pytest.mark.asyncio
async def test_retrain_insufficient_candles():
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/retrain", json={
            "candles": [{"open": 100, "high": 105, "low": 95,
                         "close": 102, "volume": 1000, "timestamp": i}
                        for i in range(20)],
        })
    assert resp.status_code == 422  # min 60 candles


@pytest.mark.asyncio
async def test_walk_forward(large_candles):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/walk-forward", json={
            "candles": large_candles,
            "n_splits": 3,
        })
    assert resp.status_code == 200
    data = resp.json()
    assert "success" in data


@pytest.mark.asyncio
async def test_rl_train(candles):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/rl/train", json={
            "candles": candles,
            "episodes": 2,
            "initial_balance": 10000,
        })
    assert resp.status_code == 200
    data = resp.json()
    assert "success" in data


@pytest.mark.asyncio
async def test_rl_action(candles):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/rl/action", json={
            "candles": candles,
            "balance": 10000,
            "position": 0,
            "entry_price": 0,
        })
    assert resp.status_code == 200
    data = resp.json()
    assert data["action"] in ["hold", "buy_small", "buy_large", "sell_small", "sell_all"]


@pytest.mark.asyncio
async def test_rl_action_with_position(candles):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/rl/action", json={
            "candles": candles,
            "balance": 5000,
            "position": 0.1,
            "entry_price": 40000,
        })
    assert resp.status_code == 200
    assert resp.json()["action"] in ["hold", "buy_small", "buy_large", "sell_small", "sell_all"]
