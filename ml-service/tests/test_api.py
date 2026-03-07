# tests for fastapi endpoints

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


@pytest.mark.asyncio
async def test_root():
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.get("/")
    assert resp.status_code == 200
    data = resp.json()
    assert data["service"] == "ml-service"
    assert data["status"] == "ready"


@pytest.mark.asyncio
async def test_health():
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.get("/health")
    assert resp.status_code == 200
    assert resp.json()["status"] == "healthy"


@pytest.mark.asyncio
async def test_predict_price(candles):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/predict/price", json={
            "symbol": "BTC/USDT",
            "candles": candles,
            "timeframe": "4h",
        })
    assert resp.status_code == 200
    data = resp.json()
    assert data["direction"] in ("up", "down")
    assert 0 <= data["confidence"] <= 1
    assert data["magnitude"] >= 0
    assert data["timeframe"] == "4h"
    assert data["current_price"] > 0


@pytest.mark.asyncio
async def test_predict_price_1h(candles):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/predict/price", json={
            "symbol": "ETH/USDT",
            "candles": candles,
            "timeframe": "1h",
        })
    assert resp.status_code == 200
    assert resp.json()["timeframe"] == "1h"


@pytest.mark.asyncio
async def test_predict_price_1d(candles):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/predict/price", json={
            "symbol": "BTC/USDT",
            "candles": candles,
            "timeframe": "1d",
        })
    assert resp.status_code == 200
    assert resp.json()["timeframe"] == "1d"


@pytest.mark.asyncio
async def test_predict_price_insufficient_candles():
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/predict/price", json={
            "symbol": "BTC/USDT",
            "candles": [{"open": 100, "high": 105, "low": 95,
                         "close": 102, "volume": 1000, "timestamp": i}
                        for i in range(5)],
            "timeframe": "4h",
        })
    # should fail validation (min_length=30)
    assert resp.status_code == 422


@pytest.mark.asyncio
async def test_analyze_sentiment_bullish():
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/analyze/sentiment", json={
            "text": "BTC breaks resistance! Massive bullish breakout!",
        })
    assert resp.status_code == 200
    data = resp.json()
    assert data["label"] == "BULLISH"
    assert data["score"] > 0
    assert 0 <= data["confidence"] <= 1


@pytest.mark.asyncio
async def test_analyze_sentiment_bearish():
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/analyze/sentiment", json={
            "text": "crash incoming, sell everything, bearish dump!",
        })
    assert resp.status_code == 200
    data = resp.json()
    assert data["label"] == "BEARISH"
    assert data["score"] < 0


@pytest.mark.asyncio
async def test_analyze_sentiment_neutral():
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/analyze/sentiment", json={
            "text": "the weather is nice today",
        })
    assert resp.status_code == 200
    assert resp.json()["label"] == "NEUTRAL"


@pytest.mark.asyncio
async def test_analyze_sentiment_empty():
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/analyze/sentiment", json={
            "text": "",
        })
    # empty text should fail validation (min_length=1)
    assert resp.status_code == 422


@pytest.mark.asyncio
async def test_predict_price_response_schema(candles):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/predict/price", json={
            "symbol": "BTC/USDT",
            "candles": candles,
        })
    assert resp.status_code == 200
    data = resp.json()
    # verify all expected fields present
    expected = {"direction", "magnitude", "confidence", "timeframe",
                "predicted_price", "current_price"}
    assert expected.issubset(set(data.keys()))


@pytest.mark.asyncio
async def test_sentiment_response_schema():
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/analyze/sentiment", json={
            "text": "bullish momentum",
        })
    assert resp.status_code == 200
    data = resp.json()
    expected = {"score", "label", "confidence"}
    assert expected.issubset(set(data.keys()))
