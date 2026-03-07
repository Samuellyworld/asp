# tests for sentiment analyzer module

import pytest
from app.sentiment import SentimentAnalyzer


class TestSentimentAnalyzer:
    def setup_method(self):
        # use keyword fallback (no model download in tests)
        self.analyzer = SentimentAnalyzer.__new__(SentimentAnalyzer)
        self.analyzer._pipeline = None

    def test_bullish_text(self):
        result = self.analyzer.analyze("BTC is bullish, breakout imminent!")
        assert result["label"] == "BULLISH"
        assert result["score"] > 0

    def test_bearish_text(self):
        result = self.analyzer.analyze("crash incoming, sell everything, bearish!")
        assert result["label"] == "BEARISH"
        assert result["score"] < 0

    def test_neutral_text(self):
        result = self.analyzer.analyze("the weather is nice today")
        assert result["label"] == "NEUTRAL"

    def test_empty_text(self):
        result = self.analyzer.analyze("")
        assert result["label"] == "NEUTRAL"
        assert result["confidence"] == 0.0

    def test_whitespace_text(self):
        result = self.analyzer.analyze("   ")
        assert result["label"] == "NEUTRAL"

    def test_score_range(self):
        texts = [
            "massive bullish rally incoming!",
            "bearish crash dump sell!",
            "neutral market today",
        ]
        for text in texts:
            result = self.analyzer.analyze(text)
            assert -1 <= result["score"] <= 1

    def test_confidence_range(self):
        result = self.analyzer.analyze("buy buy buy bullish moon!")
        assert 0 <= result["confidence"] <= 1

    def test_returns_required_fields(self):
        result = self.analyzer.analyze("test text")
        assert "score" in result
        assert "label" in result
        assert "confidence" in result

    def test_label_values(self):
        result = self.analyzer.analyze("bullish breakout!")
        assert result["label"] in ("BULLISH", "BEARISH", "NEUTRAL")

    def test_strong_bullish_keywords(self):
        result = self.analyzer.analyze(
            "breakout confirmed, support holds, golden cross forming, bullish!"
        )
        assert result["label"] == "BULLISH"
        assert result["confidence"] > 0.5

    def test_strong_bearish_keywords(self):
        result = self.analyzer.analyze(
            "death cross, breakdown, rug pull, bearish crash dump!"
        )
        assert result["label"] == "BEARISH"
        assert result["confidence"] > 0.5

    def test_mixed_signals(self):
        result = self.analyzer.analyze("bullish indicators but bearish volume")
        # mixed should be closer to neutral
        assert abs(result["score"]) < 0.8

    def test_exclamation_marks_boost_intensity(self):
        calm = self.analyzer.analyze("btc is bullish")
        excited = self.analyzer.analyze("btc is bullish!!!")
        assert excited["confidence"] >= calm["confidence"]

    def test_crypto_keyword_boost(self):
        boost = self.analyzer._crypto_keyword_boost("golden cross forming on the daily chart")
        assert boost > 0
        boost = self.analyzer._crypto_keyword_boost("death cross confirmed, liquidation cascade")
        assert boost < 0
        boost = self.analyzer._crypto_keyword_boost("nice weather today")
        assert boost == 0

    def test_case_insensitive(self):
        lower = self.analyzer.analyze("bullish breakout moon")
        upper = self.analyzer.analyze("BULLISH BREAKOUT MOON")
        assert lower["label"] == upper["label"]

    def test_long_text(self):
        text = "bullish " * 100 + "breakout " * 50
        result = self.analyzer.analyze(text)
        assert result["label"] == "BULLISH"

    def test_numbers_only(self):
        result = self.analyzer.analyze("42000 38500 45000")
        assert result["label"] == "NEUTRAL"
