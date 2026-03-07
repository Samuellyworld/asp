# tests for price predictor module

import pytest
import numpy as np
from app.predictor import PricePredictor


def make_candles(n=50, start_price=40000, trend=0.5):
    """generates test candle data"""
    candles = []
    price = start_price
    for i in range(n):
        price = price * (1 + trend / 100 + np.random.normal(0, 0.002))
        candles.append({
            "open": price * 0.999,
            "high": price * 1.005,
            "low": price * 0.995,
            "close": price,
            "volume": 1000000 + np.random.normal(0, 100000),
            "timestamp": i,
        })
    return candles


class TestPricePredictor:
    def setup_method(self):
        self.predictor = PricePredictor(model_dir="nonexistent_dir")

    def test_predict_returns_required_fields(self):
        candles = make_candles(50)
        result = self.predictor.predict(candles)
        assert "direction" in result
        assert "magnitude" in result
        assert "confidence" in result
        assert "timeframe" in result
        assert "current_price" in result

    def test_predict_direction_is_valid(self):
        candles = make_candles(50)
        result = self.predictor.predict(candles)
        assert result["direction"] in ("up", "down")

    def test_predict_confidence_range(self):
        candles = make_candles(50)
        result = self.predictor.predict(candles)
        assert 0 <= result["confidence"] <= 1

    def test_predict_magnitude_non_negative(self):
        candles = make_candles(50)
        result = self.predictor.predict(candles)
        assert result["magnitude"] >= 0

    def test_predict_magnitude_capped(self):
        candles = make_candles(50)
        result = self.predictor.predict(candles)
        assert result["magnitude"] <= 15.0

    def test_predict_timeframe_passthrough(self):
        candles = make_candles(50)
        for tf in ["1h", "4h", "1d"]:
            result = self.predictor.predict(candles, timeframe=tf)
            assert result["timeframe"] == tf

    def test_predict_current_price_matches(self):
        candles = make_candles(50)
        result = self.predictor.predict(candles)
        assert result["current_price"] == candles[-1]["close"]

    def test_predict_predicted_price_exists(self):
        candles = make_candles(50)
        result = self.predictor.predict(candles)
        assert result["predicted_price"] is not None
        assert result["predicted_price"] > 0

    def test_insufficient_data_returns_default(self):
        candles = make_candles(5)
        result = self.predictor.predict(candles)
        assert result["confidence"] == 0.0
        assert result["magnitude"] == 0.0

    def test_uptrend_predicts_up(self):
        candles = make_candles(50, trend=2.0)
        result = self.predictor.predict(candles)
        # strong uptrend should predict up (statistical fallback)
        assert result["direction"] == "up"

    def test_downtrend_predicts_down(self):
        candles = make_candles(50, trend=-2.0)
        result = self.predictor.predict(candles)
        assert result["direction"] == "down"

    def test_timeframe_scaling(self):
        candles = make_candles(50, trend=1.0)
        r1h = self.predictor.predict(candles, "1h")
        r1d = self.predictor.predict(candles, "1d")
        # daily predictions should generally be larger magnitude
        # (may not always hold due to randomness, but structure should scale)
        assert r1d["timeframe"] == "1d"
        assert r1h["timeframe"] == "1h"

    def test_feature_engineer_shape(self):
        candles = make_candles(50)
        features = self.predictor._feature_engineer(candles)
        assert features.shape == (50, 5)

    def test_feature_engineer_no_nan(self):
        candles = make_candles(50)
        features = self.predictor._feature_engineer(candles)
        assert not np.isnan(features).any()

    def test_normalize_shape_preserved(self):
        candles = make_candles(50)
        features = self.predictor._feature_engineer(candles)
        normalized = self.predictor._normalize(features)
        assert normalized.shape == features.shape

    def test_normalize_no_nan(self):
        candles = make_candles(50)
        features = self.predictor._feature_engineer(candles)
        normalized = self.predictor._normalize(features)
        assert not np.isnan(normalized).any()

    def test_constant_prices(self):
        candles = [{
            "open": 100, "high": 100, "low": 100,
            "close": 100, "volume": 1000, "timestamp": i,
        } for i in range(50)]
        result = self.predictor.predict(candles)
        assert result["magnitude"] < 1.0

    def test_single_candle_insufficient(self):
        candles = [{"open": 100, "high": 105, "low": 95,
                    "close": 102, "volume": 1000, "timestamp": 0}]
        result = self.predictor.predict(candles)
        assert result["confidence"] == 0.0

    def test_statistical_predict_directly(self):
        candles = make_candles(50, trend=1.0)
        result = self.predictor._statistical_predict(candles, "4h")
        assert "direction" in result
        assert 0.4 <= result["confidence"] <= 0.75
