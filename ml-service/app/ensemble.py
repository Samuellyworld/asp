# ensemble model combining LSTM, gradient boosting, and random forest
# weighted voting for more robust predictions

import logging
import numpy as np
from typing import Optional

logger = logging.getLogger(__name__)

try:
    from sklearn.ensemble import GradientBoostingClassifier, GradientBoostingRegressor
    from sklearn.ensemble import RandomForestClassifier, RandomForestRegressor
    SKLEARN_AVAILABLE = True
except ImportError:
    SKLEARN_AVAILABLE = False

try:
    import torch
    TORCH_AVAILABLE = True
except ImportError:
    TORCH_AVAILABLE = False


class EnsemblePredictor:
    """combines LSTM, gradient boosting, and random forest for robust predictions.
    uses weighted voting where weights are based on recent rolling accuracy.
    """

    def __init__(self, predictor=None):
        self.predictor = predictor  # PricePredictor instance (LSTM)
        self.gb_clf: Optional[GradientBoostingClassifier] = None
        self.gb_reg: Optional[GradientBoostingRegressor] = None
        self.rf_clf: Optional[RandomForestClassifier] = None
        self.rf_reg: Optional[RandomForestRegressor] = None
        self.weights = {"lstm": 0.5, "gradient_boosting": 0.3, "random_forest": 0.2}
        self.rolling_accuracy = {"lstm": 0.5, "gradient_boosting": 0.5, "random_forest": 0.5}
        self._fitted = False

    def fit(self, features: np.ndarray, directions: np.ndarray, magnitudes: np.ndarray):
        """trains gradient boosting and random forest on flattened feature sequences"""
        if not SKLEARN_AVAILABLE:
            logger.warning("scikit-learn not available, ensemble training skipped")
            return

        # flatten sequences for tree models: (n_samples, seq_len, n_features) -> (n_samples, seq_len * n_features)
        if features.ndim == 3:
            flat = features.reshape(features.shape[0], -1)
        else:
            flat = features

        dir_labels = (directions > 0).astype(int)

        self.gb_clf = GradientBoostingClassifier(
            n_estimators=100, max_depth=4, learning_rate=0.1, random_state=42,
        )
        self.gb_reg = GradientBoostingRegressor(
            n_estimators=100, max_depth=4, learning_rate=0.1, random_state=42,
        )
        self.rf_clf = RandomForestClassifier(
            n_estimators=100, max_depth=6, random_state=42,
        )
        self.rf_reg = RandomForestRegressor(
            n_estimators=100, max_depth=6, random_state=42,
        )

        self.gb_clf.fit(flat, dir_labels)
        self.gb_reg.fit(flat, magnitudes)
        self.rf_clf.fit(flat, dir_labels)
        self.rf_reg.fit(flat, magnitudes)
        self._fitted = True
        logger.info("ensemble models fitted: GB + RF classifiers and regressors")

    def predict(self, candles: list[dict], timeframe: str = "4h") -> dict:
        """ensemble prediction combining all available models"""
        predictions = []

        # 1. LSTM prediction
        lstm_pred = self._lstm_predict(candles, timeframe)
        if lstm_pred:
            predictions.append(("lstm", lstm_pred))

        # 2. tree model predictions (need feature engineering)
        if self._fitted and len(candles) >= 30:
            tree_features = self._extract_tree_features(candles)
            gb_pred = self._tree_predict("gradient_boosting", tree_features, candles, timeframe)
            if gb_pred:
                predictions.append(("gradient_boosting", gb_pred))
            rf_pred = self._tree_predict("random_forest", tree_features, candles, timeframe)
            if rf_pred:
                predictions.append(("random_forest", rf_pred))

        if not predictions:
            return self._fallback(candles, timeframe)

        return self._weighted_vote(predictions, candles[-1]["close"], timeframe)

    def _lstm_predict(self, candles: list[dict], timeframe: str) -> Optional[dict]:
        """gets LSTM prediction from the wrapped predictor"""
        if self.predictor is None:
            return None
        try:
            result = self.predictor.predict(candles, timeframe)
            return {
                "direction": 1.0 if result["direction"] == "up" else -1.0,
                "magnitude": result["magnitude"],
                "confidence": result["confidence"],
            }
        except Exception as e:
            logger.warning(f"LSTM prediction failed: {e}")
            return None

    def _extract_tree_features(self, candles: list[dict]) -> np.ndarray:
        """extracts flattened features for tree models from recent candles"""
        window = candles[-30:]
        closes = np.array([c["close"] for c in window])
        highs = np.array([c["high"] for c in window])
        lows = np.array([c["low"] for c in window])
        opens = np.array([c["open"] for c in window])
        volumes = np.array([c["volume"] for c in window])

        returns = np.zeros(len(closes))
        returns[1:] = (closes[1:] - closes[:-1]) / closes[:-1] * 100

        vol_avg = np.convolve(volumes, np.ones(10) / 10, mode="same")
        vol_avg[vol_avg == 0] = 1
        volume_ratio = volumes / vol_avg

        hl_range = (highs - lows) / closes * 100
        co_range = (closes - opens) / opens * 100

        sma = np.convolve(closes, np.ones(20) / 20, mode="same")
        sma[sma == 0] = 1
        sma_ratio = closes / sma

        features = np.column_stack([returns, volume_ratio, hl_range, co_range, sma_ratio])
        return features.flatten().reshape(1, -1)

    def _tree_predict(self, model_name: str, features: np.ndarray,
                      candles: list[dict], timeframe: str) -> Optional[dict]:
        """runs prediction through a tree model"""
        try:
            if model_name == "gradient_boosting":
                clf, reg = self.gb_clf, self.gb_reg
            else:
                clf, reg = self.rf_clf, self.rf_reg

            dir_prob = clf.predict_proba(features)[0]
            direction = 1.0 if dir_prob[1] > 0.5 else -1.0
            confidence = max(dir_prob)
            magnitude = abs(reg.predict(features)[0])

            scale = {"1h": 0.5, "4h": 1.0, "1d": 2.5}.get(timeframe, 1.0)
            magnitude = min(magnitude * scale, 15.0)

            return {
                "direction": direction,
                "magnitude": magnitude,
                "confidence": confidence,
            }
        except Exception as e:
            logger.warning(f"{model_name} prediction failed: {e}")
            return None

    def _weighted_vote(self, predictions: list[tuple], current_price: float,
                       timeframe: str) -> dict:
        """combines predictions using dynamic weights based on rolling accuracy"""
        self._update_weights()

        total_weight = 0
        weighted_dir = 0.0
        weighted_mag = 0.0
        weighted_conf = 0.0
        model_details = {}

        for name, pred in predictions:
            w = self.weights.get(name, 0.1)
            total_weight += w
            weighted_dir += pred["direction"] * w
            weighted_mag += pred["magnitude"] * w
            weighted_conf += pred["confidence"] * w
            model_details[name] = {
                "direction": "up" if pred["direction"] > 0 else "down",
                "magnitude": round(pred["magnitude"], 2),
                "confidence": round(pred["confidence"], 4),
                "weight": round(w, 3),
            }

        if total_weight == 0:
            total_weight = 1

        direction = "up" if weighted_dir > 0 else "down"
        magnitude = round(weighted_mag / total_weight, 2)
        confidence = round(weighted_conf / total_weight, 4)

        # agreement bonus: if all models agree, boost confidence
        dirs = [p["direction"] for _, p in predictions]
        if len(dirs) > 1 and all(d == dirs[0] for d in dirs):
            confidence = min(confidence * 1.15, 0.99)

        predicted_price = current_price * (1 + (magnitude / 100) * (1 if direction == "up" else -1))

        return {
            "direction": direction,
            "magnitude": magnitude,
            "confidence": round(confidence, 4),
            "timeframe": timeframe,
            "predicted_price": round(predicted_price, 2),
            "current_price": current_price,
            "model_count": len(predictions),
            "model_details": model_details,
            "is_ensemble": True,
        }

    def _update_weights(self):
        """recalculates weights based on rolling accuracy scores"""
        total = sum(self.rolling_accuracy.values())
        if total == 0:
            return
        for name in self.weights:
            self.weights[name] = self.rolling_accuracy[name] / total

    def update_accuracy(self, model_name: str, correct: bool):
        """updates rolling accuracy for a model after a trade outcome"""
        if model_name not in self.rolling_accuracy:
            return
        alpha = 0.1  # exponential moving average decay
        val = 1.0 if correct else 0.0
        self.rolling_accuracy[model_name] = (
            (1 - alpha) * self.rolling_accuracy[model_name] + alpha * val
        )

    def _fallback(self, candles: list[dict], timeframe: str) -> dict:
        """fallback when no models are available"""
        current_price = candles[-1]["close"] if candles else 0
        return {
            "direction": "up",
            "magnitude": 0.0,
            "confidence": 0.0,
            "timeframe": timeframe,
            "predicted_price": current_price,
            "current_price": current_price,
            "model_count": 0,
            "model_details": {},
            "is_ensemble": True,
        }
