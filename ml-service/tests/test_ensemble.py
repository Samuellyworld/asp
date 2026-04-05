# tests for ensemble predictor module

import pytest
import numpy as np
from app.ensemble import EnsemblePredictor


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


class TestEnsemblePredictor:
    def setup_method(self):
        self.ensemble = EnsemblePredictor(predictor=None)

    def test_predict_returns_required_fields(self):
        candles = make_candles(50)
        result = self.ensemble.predict(candles)
        assert "direction" in result
        assert "magnitude" in result
        assert "confidence" in result
        assert "timeframe" in result
        assert "is_ensemble" in result

    def test_predict_with_no_models(self):
        candles = make_candles(50)
        result = self.ensemble.predict(candles)
        assert result["model_count"] == 0
        assert result["confidence"] == 0.0
        assert result["is_ensemble"] is True

    def test_predict_direction_valid(self):
        candles = make_candles(50)
        result = self.ensemble.predict(candles)
        assert result["direction"] in ("up", "down")

    def test_fit_and_predict(self):
        from app.predictor import PricePredictor
        predictor = PricePredictor(model_dir="nonexistent")
        ensemble = EnsemblePredictor(predictor=predictor)

        # generate training data with alternating trend for balanced labels
        np.random.seed(42)
        candles = []
        price = 40000.0
        for i in range(200):
            change = 0.005 if i % 3 != 0 else -0.003
            price = price * (1 + change + np.random.normal(0, 0.001))
            candles.append({
                "open": price * 0.999, "high": price * 1.005,
                "low": price * 0.995, "close": price,
                "volume": 1000000, "timestamp": i,
            })

        closes = np.array([c["close"] for c in candles])
        features = predictor._feature_engineer(candles)
        normalized = predictor._normalize(features)

        seq_len = 30
        X, dirs, mags = [], [], []
        for i in range(seq_len, len(features) - 1):
            X.append(normalized[i - seq_len:i])
            ret = (closes[i + 1] - closes[i]) / closes[i] * 100
            dirs.append(1.0 if ret > 0 else -1.0)
            mags.append(abs(ret))

        X = np.array(X)
        dirs = np.array(dirs)
        mags = np.array(mags)

        # ensure we have both classes
        assert np.sum(dirs > 0) > 0 and np.sum(dirs < 0) > 0

        ensemble.fit(X, dirs, mags)
        result = ensemble.predict(candles[-50:])
        assert result["model_count"] >= 1
        assert result["is_ensemble"] is True

    def test_update_accuracy(self):
        self.ensemble.update_accuracy("lstm", True)
        self.ensemble.update_accuracy("lstm", True)
        assert self.ensemble.rolling_accuracy["lstm"] > 0.5

    def test_update_accuracy_decreases(self):
        initial = self.ensemble.rolling_accuracy["lstm"]
        self.ensemble.update_accuracy("lstm", False)
        assert self.ensemble.rolling_accuracy["lstm"] < initial

    def test_weights_update(self):
        self.ensemble.rolling_accuracy = {"lstm": 0.8, "gradient_boosting": 0.6, "random_forest": 0.4}
        self.ensemble._update_weights()
        assert self.ensemble.weights["lstm"] > self.ensemble.weights["random_forest"]

    def test_weighted_vote_agreement_bonus(self):
        predictions = [
            ("lstm", {"direction": 1.0, "magnitude": 2.0, "confidence": 0.7}),
            ("gradient_boosting", {"direction": 1.0, "magnitude": 1.5, "confidence": 0.6}),
        ]
        result = self.ensemble._weighted_vote(predictions, 40000.0, "4h")
        assert result["direction"] == "up"
        assert result["confidence"] > 0.6  # agreement bonus

    def test_weighted_vote_disagreement(self):
        predictions = [
            ("lstm", {"direction": 1.0, "magnitude": 2.0, "confidence": 0.7}),
            ("gradient_boosting", {"direction": -1.0, "magnitude": 1.5, "confidence": 0.6}),
        ]
        result = self.ensemble._weighted_vote(predictions, 40000.0, "4h")
        assert result["direction"] in ("up", "down")

    def test_fallback_returns_zero_confidence(self):
        result = self.ensemble._fallback(make_candles(5), "4h")
        assert result["confidence"] == 0.0
        assert result["model_count"] == 0

    def test_model_details_in_result(self):
        predictions = [
            ("lstm", {"direction": 1.0, "magnitude": 2.0, "confidence": 0.7}),
        ]
        result = self.ensemble._weighted_vote(predictions, 40000.0, "4h")
        assert "lstm" in result["model_details"]
        assert result["model_details"]["lstm"]["direction"] == "up"

    def test_timeframe_passthrough(self):
        candles = make_candles(50)
        for tf in ["1h", "4h", "1d"]:
            result = self.ensemble.predict(candles, timeframe=tf)
            assert result["timeframe"] == tf
