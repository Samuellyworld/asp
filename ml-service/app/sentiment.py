# sentiment analysis for crypto market text
# classifies text as bullish, bearish, or neutral

import re
from typing import Optional

try:
    from transformers import pipeline as hf_pipeline
    TRANSFORMERS_AVAILABLE = True
except ImportError:
    TRANSFORMERS_AVAILABLE = False


class SentimentAnalyzer:
    """analyzes text sentiment for crypto trading context.
    uses a huggingface transformer model when available,
    falls back to keyword-based analysis otherwise.
    """

    def __init__(self, model_name: str = "distilbert-base-uncased-finetuned-sst-2-english"):
        self.model_name = model_name
        self._pipeline = None
        self._load_model()

    def _load_model(self):
        """loads the sentiment transformer model"""
        if not TRANSFORMERS_AVAILABLE:
            return
        try:
            self._pipeline = hf_pipeline(
                "sentiment-analysis",
                model=self.model_name,
                truncation=True,
                max_length=512,
            )
        except Exception:
            # model download may fail offline, use fallback
            self._pipeline = None

    def analyze(self, text: str) -> dict:
        """analyzes sentiment of the given text.
        returns dict with score (-1 to 1), label, and confidence.
        """
        if not text or not text.strip():
            return {"score": 0.0, "label": "NEUTRAL", "confidence": 0.0}

        if self._pipeline is not None:
            return self._model_analyze(text)

        return self._keyword_analyze(text)

    def _model_analyze(self, text: str) -> dict:
        """runs transformer model for sentiment classification"""
        result = self._pipeline(text)[0]
        label = result["label"]
        confidence = result["score"]

        # map model output to trading labels
        if label == "POSITIVE":
            score = confidence
            trading_label = "BULLISH" if confidence > 0.6 else "NEUTRAL"
        elif label == "NEGATIVE":
            score = -confidence
            trading_label = "BEARISH" if confidence > 0.6 else "NEUTRAL"
        else:
            score = 0.0
            trading_label = "NEUTRAL"

        # apply crypto-specific adjustments
        crypto_adj = self._crypto_keyword_boost(text)
        score = max(-1, min(1, score + crypto_adj * 0.2))

        if abs(score) > 0.5:
            trading_label = "BULLISH" if score > 0 else "BEARISH"
        elif abs(score) < 0.2:
            trading_label = "NEUTRAL"

        return {
            "score": round(score, 4),
            "label": trading_label,
            "confidence": round(confidence, 4),
        }

    def _keyword_analyze(self, text: str) -> dict:
        """fallback keyword-based sentiment analysis for crypto text"""
        text_lower = text.lower()

        bullish_words = [
            "bullish", "buy", "long", "moon", "pump", "breakout",
            "support", "accumulate", "rally", "surge", "recovery",
            "uptrend", "higher", "gains", "growth", "strong",
            "breaks resistance", "all-time high", "ath", "golden cross",
            "ascending", "reversal up", "oversold", "bounce",
        ]
        bearish_words = [
            "bearish", "sell", "short", "dump", "crash", "breakdown",
            "resistance", "distribute", "plunge", "decline", "correction",
            "downtrend", "lower", "losses", "weak", "death cross",
            "breaks support", "capitulation", "overbought", "falling",
            "descending", "reversal down", "liquidation", "rug pull",
        ]
        intensifiers = [
            "very", "extremely", "massive", "huge", "incredible",
            "strong", "significant", "major", "critical",
        ]

        bull_count = sum(1 for w in bullish_words if re.search(r'\b' + re.escape(w) + r'\b', text_lower))
        bear_count = sum(1 for w in bearish_words if re.search(r'\b' + re.escape(w) + r'\b', text_lower))
        intensity = sum(1 for w in intensifiers if re.search(r'\b' + re.escape(w) + r'\b', text_lower))

        # exclamation marks and caps increase intensity
        caps_ratio = sum(1 for c in text if c.isupper()) / max(len(text), 1)
        exclaim_count = text.count("!")
        intensity += min(caps_ratio * 3, 1) + min(exclaim_count * 0.3, 1)

        total = bull_count + bear_count
        if total == 0:
            return {"score": 0.0, "label": "NEUTRAL", "confidence": 0.3}

        raw_score = (bull_count - bear_count) / total
        # apply intensity boost
        intensity_mult = 1 + min(intensity * 0.15, 0.5)
        score = max(-1, min(1, raw_score * intensity_mult))

        confidence = min(0.4 + total * 0.08 + intensity * 0.05, 0.95)

        if score > 0.2:
            label = "BULLISH"
        elif score < -0.2:
            label = "BEARISH"
        else:
            label = "NEUTRAL"

        return {
            "score": round(score, 4),
            "label": label,
            "confidence": round(confidence, 4),
        }

    def _crypto_keyword_boost(self, text: str) -> float:
        """returns a sentiment adjustment based on crypto-specific keywords"""
        text_lower = text.lower()
        boost = 0.0

        bullish_crypto = ["breakout", "support holds", "accumulation", "bullish divergence", "golden cross"]
        bearish_crypto = ["breakdown", "death cross", "liquidation", "rug pull", "bearish divergence"]

        for word in bullish_crypto:
            if word in text_lower:
                boost += 0.3
        for word in bearish_crypto:
            if word in text_lower:
                boost -= 0.3

        return max(-1, min(1, boost))
