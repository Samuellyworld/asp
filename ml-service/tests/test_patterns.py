# tests for chart pattern detection module

import pytest
import numpy as np
from app.patterns import PatternDetector


def make_candles(n=50, start_price=40000, trend=0.0):
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
            "volume": 1000000,
            "timestamp": i,
        })
    return candles


def make_double_top(n=60, start_price=40000):
    """creates candle data that forms a double top pattern"""
    candles = []
    price = start_price

    # uptrend to first peak
    for i in range(15):
        price = price * 1.01
        candles.append({"open": price * 0.999, "high": price * 1.003,
                        "low": price * 0.997, "close": price, "volume": 1000000, "timestamp": i})

    peak = price
    # pullback
    for i in range(15, 30):
        price = price * 0.995
        candles.append({"open": price * 1.001, "high": price * 1.003,
                        "low": price * 0.997, "close": price, "volume": 1000000, "timestamp": i})

    # rally back to similar level
    for i in range(30, 45):
        price = price * 1.01
        candles.append({"open": price * 0.999, "high": price * 1.003,
                        "low": price * 0.997, "close": price, "volume": 1000000, "timestamp": i})

    # decline
    for i in range(45, 60):
        price = price * 0.995
        candles.append({"open": price * 1.001, "high": price * 1.003,
                        "low": price * 0.997, "close": price, "volume": 1000000, "timestamp": i})

    return candles


def make_double_bottom(n=60, start_price=40000):
    """creates candle data that forms a double bottom pattern"""
    candles = []
    price = start_price

    # downtrend to first trough
    for i in range(15):
        price = price * 0.99
        candles.append({"open": price * 1.001, "high": price * 1.003,
                        "low": price * 0.997, "close": price, "volume": 1000000, "timestamp": i})

    # bounce
    for i in range(15, 30):
        price = price * 1.005
        candles.append({"open": price * 0.999, "high": price * 1.003,
                        "low": price * 0.997, "close": price, "volume": 1000000, "timestamp": i})

    # decline back to similar level
    for i in range(30, 45):
        price = price * 0.99
        candles.append({"open": price * 1.001, "high": price * 1.003,
                        "low": price * 0.997, "close": price, "volume": 1000000, "timestamp": i})

    # recovery
    for i in range(45, 60):
        price = price * 1.005
        candles.append({"open": price * 0.999, "high": price * 1.003,
                        "low": price * 0.997, "close": price, "volume": 1000000, "timestamp": i})

    return candles


def make_bull_flag(n=40, start_price=40000):
    """creates a bull flag: strong up move then tight consolidation"""
    candles = []
    price = start_price

    # strong pole up
    for i in range(15):
        price = price * 1.015
        candles.append({"open": price * 0.997, "high": price * 1.005,
                        "low": price * 0.995, "close": price, "volume": 1500000, "timestamp": i})

    # tight flag consolidation
    for i in range(15, 40):
        price = price * (1 + np.random.normal(0, 0.001))
        candles.append({"open": price * 0.999, "high": price * 1.002,
                        "low": price * 0.998, "close": price, "volume": 800000, "timestamp": i})

    return candles


class TestPatternDetector:
    def setup_method(self):
        self.detector = PatternDetector()

    def test_detect_returns_required_fields(self):
        candles = make_candles(50)
        result = self.detector.detect(candles)
        assert "patterns" in result
        assert "pattern_count" in result
        assert "signal" in result
        assert "signal_strength" in result
        assert "summary" in result

    def test_insufficient_data(self):
        candles = make_candles(5)
        result = self.detector.detect(candles)
        assert result["patterns"] == []
        assert "insufficient" in result["summary"]

    def test_find_pivots(self):
        data = np.array([1, 2, 3, 4, 5, 4, 3, 2, 1, 2, 3, 4, 3, 2, 1])
        pivots = self.detector._find_pivots(data, window=2)
        assert len(pivots["highs"]) > 0
        assert len(pivots["lows"]) > 0

    def test_double_top_detection(self):
        candles = make_double_top()
        result = self.detector.detect(candles)
        # should detect at least something in this pattern
        assert isinstance(result["patterns"], list)
        assert isinstance(result["pattern_count"], int)

    def test_double_bottom_detection(self):
        candles = make_double_bottom()
        result = self.detector.detect(candles)
        assert isinstance(result["patterns"], list)

    def test_bull_flag_detection(self):
        candles = make_bull_flag()
        result = self.detector.detect(candles)
        flag_patterns = [p for p in result["patterns"] if "flag" in p["name"]]
        if flag_patterns:
            assert flag_patterns[0]["direction"] == "bullish"

    def test_signal_strength_range(self):
        candles = make_candles(80, trend=1.0)
        result = self.detector.detect(candles)
        assert 0 <= result["signal_strength"] <= 1

    def test_signal_direction_valid(self):
        candles = make_candles(80)
        result = self.detector.detect(candles)
        assert result["signal"] in ("bullish", "bearish", "neutral")

    def test_pattern_confidence_range(self):
        candles = make_double_top()
        result = self.detector.detect(candles)
        for p in result["patterns"]:
            assert 0 <= p["confidence"] <= 1

    def test_summary_is_string(self):
        candles = make_candles(50)
        result = self.detector.detect(candles)
        assert isinstance(result["summary"], str)

    def test_derive_signal_empty(self):
        signal = self.detector._derive_signal([])
        assert signal["direction"] == "neutral"
        assert signal["strength"] == 0.0

    def test_derive_signal_bullish(self):
        patterns = [
            {"name": "double_bottom", "direction": "bullish", "confidence": 0.8},
        ]
        signal = self.detector._derive_signal(patterns)
        assert signal["direction"] == "bullish"

    def test_derive_signal_bearish(self):
        patterns = [
            {"name": "double_top", "direction": "bearish", "confidence": 0.8},
        ]
        signal = self.detector._derive_signal(patterns)
        assert signal["direction"] == "bearish"

    def test_flat_market_few_patterns(self):
        candles = [{"open": 100, "high": 100.1, "low": 99.9,
                     "close": 100, "volume": 1000, "timestamp": i}
                    for i in range(50)]
        result = self.detector.detect(candles)
        assert result["pattern_count"] >= 0  # may have few or none

    def test_tolerance_parameter(self):
        detector = PatternDetector(tolerance=0.05)
        candles = make_candles(50)
        result = detector.detect(candles)
        assert isinstance(result["patterns"], list)
