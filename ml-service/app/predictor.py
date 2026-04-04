# lstm price prediction model
# predicts price direction and magnitude for crypto assets

import logging
import numpy as np
import os
import json
from typing import Optional

logger = logging.getLogger(__name__)

try:
    import torch
    import torch.nn as nn
    TORCH_AVAILABLE = True
except ImportError:
    TORCH_AVAILABLE = False
    # stub so the class definition below doesn't fail
    class _ModuleStub:
        pass

    class _NNStub:
        Module = _ModuleStub
        LSTM = None
        Linear = None
        ReLU = None
        Dropout = None
        Sequential = None

    class _TorchStub:
        nn = _NNStub()

    nn = _NNStub()


class LSTMModel(nn.Module):
    """lstm network for price prediction"""

    def __init__(self, input_size: int = 5, hidden_size: int = 64,
                 num_layers: int = 2, dropout: float = 0.2):
        super().__init__()
        self.hidden_size = hidden_size
        self.num_layers = num_layers

        self.lstm = nn.LSTM(
            input_size=input_size,
            hidden_size=hidden_size,
            num_layers=num_layers,
            batch_first=True,
            dropout=dropout if num_layers > 1 else 0,
        )
        self.fc = nn.Sequential(
            nn.Linear(hidden_size, 32),
            nn.ReLU(),
            nn.Dropout(dropout),
            nn.Linear(32, 2),  # direction confidence, magnitude
        )

    def forward(self, x: "torch.Tensor") -> "torch.Tensor":
        lstm_out, _ = self.lstm(x)
        last_hidden = lstm_out[:, -1, :]
        return self.fc(last_hidden)


class PricePredictor:
    """wraps the lstm model with feature engineering and inference"""

    def __init__(self, model_dir: str = "models"):
        self.model_dir = model_dir
        self.model: Optional[LSTMModel] = None
        self.scaler_params: Optional[dict] = None
        self.sequence_length = 30
        self._load_model()

    def _load_model(self):
        """loads a trained model from disk if available"""
        if not TORCH_AVAILABLE:
            return

        model_path = os.path.join(self.model_dir, "lstm_price.pt")
        scaler_path = os.path.join(self.model_dir, "scaler_params.json")

        if os.path.exists(model_path):
            self.model = LSTMModel()
            self.model.load_state_dict(torch.load(model_path, map_location="cpu"))
            self.model.eval()

        if os.path.exists(scaler_path):
            with open(scaler_path) as f:
                self.scaler_params = json.load(f)

    def _feature_engineer(self, candles: list[dict]) -> np.ndarray:
        """creates feature matrix from raw candle data.
        features: returns, volume_ratio, high_low_range, close_open_range, sma_ratio
        """
        closes = np.array([c["close"] for c in candles])
        highs = np.array([c["high"] for c in candles])
        lows = np.array([c["low"] for c in candles])
        opens = np.array([c["open"] for c in candles])
        volumes = np.array([c["volume"] for c in candles])

        # price returns (percentage change)
        returns = np.zeros(len(closes))
        returns[1:] = (closes[1:] - closes[:-1]) / closes[:-1] * 100

        # volume ratio (current / rolling average)
        vol_avg = np.convolve(volumes, np.ones(10) / 10, mode="same")
        vol_avg[vol_avg == 0] = 1
        volume_ratio = volumes / vol_avg

        # high-low range as percentage
        hl_range = (highs - lows) / closes * 100

        # close-open range as percentage
        co_range = (closes - opens) / opens * 100

        # simple moving average ratio (close / sma20)
        sma = np.convolve(closes, np.ones(min(20, len(closes))) / min(20, len(closes)), mode="same")
        sma[sma == 0] = 1
        sma_ratio = closes / sma

        features = np.column_stack([returns, volume_ratio, hl_range, co_range, sma_ratio])
        return features

    def _normalize(self, features: np.ndarray) -> np.ndarray:
        """normalizes features using saved parameters or computed stats"""
        if self.scaler_params:
            mean = np.array(self.scaler_params["mean"])
            std = np.array(self.scaler_params["std"])
            std[std == 0] = 1
            return (features - mean) / std

        # fallback: normalize with current data stats
        mean = features.mean(axis=0)
        std = features.std(axis=0)
        std[std == 0] = 1
        return (features - mean) / std

    def predict(self, candles: list[dict], timeframe: str = "4h") -> dict:
        """predicts price direction and magnitude.
        returns dict with direction, magnitude, confidence.
        falls back to statistical prediction if no model is loaded.
        """
        if len(candles) < self.sequence_length:
            return self._default_prediction(candles, timeframe)

        features = self._feature_engineer(candles)
        normalized = self._normalize(features)

        # use last sequence_length candles
        sequence = normalized[-self.sequence_length:]
        current_price = candles[-1]["close"]

        if self.model is not None and TORCH_AVAILABLE:
            return self._model_predict(sequence, current_price, timeframe)

        return self._statistical_predict(candles, timeframe)

    def _model_predict(self, sequence: np.ndarray, current_price: float,
                       timeframe: str) -> dict:
        """runs inference through the lstm model"""
        x = torch.FloatTensor(sequence).unsqueeze(0)

        with torch.no_grad():
            output = self.model(x)

        direction_score = torch.tanh(output[0, 0]).item()
        magnitude = abs(output[0, 1].item())

        # timeframe scaling: longer timeframes allow bigger moves
        scale = {"1h": 0.5, "4h": 1.0, "1d": 2.5}.get(timeframe, 1.0)
        magnitude = magnitude * scale

        direction = "up" if direction_score > 0 else "down"
        confidence = min(abs(direction_score), 0.99)
        magnitude = min(magnitude, 15.0)  # cap at 15%

        predicted_price = current_price * (1 + (magnitude / 100) * (1 if direction == "up" else -1))

        return {
            "direction": direction,
            "magnitude": round(magnitude, 2),
            "confidence": round(confidence, 4),
            "timeframe": timeframe,
            "predicted_price": round(predicted_price, 2),
            "current_price": current_price,
            "is_fallback": False,
        }

    def _statistical_predict(self, candles: list[dict], timeframe: str) -> dict:
        """fallback prediction using momentum and trend analysis
        when no trained model is available"""
        logger.warning(
            "LSTM model not available — using statistical fallback. "
            "Predictions will have lower accuracy (confidence capped at 0.75). "
            "Train a model with train.py to enable full predictions."
        )
        closes = np.array([c["close"] for c in candles])
        current_price = closes[-1]

        # compute momentum indicators
        returns = np.diff(closes) / closes[:-1] * 100
        recent_returns = returns[-10:]
        momentum = np.mean(recent_returns)

        # exponential weighted momentum (recent data matters more)
        weights = np.exp(np.linspace(-1, 0, len(recent_returns)))
        weights /= weights.sum()
        weighted_momentum = np.sum(recent_returns * weights)

        # trend strength via linear regression slope
        x = np.arange(len(closes[-20:]))
        y = closes[-20:]
        if len(x) > 1:
            slope = np.polyfit(x, y, 1)[0]
            trend = slope / current_price * 100
        else:
            trend = 0

        # combine signals
        combined = (weighted_momentum * 0.5 + trend * 0.3 + momentum * 0.2)

        direction = "up" if combined > 0 else "down"
        magnitude = abs(combined)

        # timeframe scaling
        scale = {"1h": 0.5, "4h": 1.0, "1d": 2.5}.get(timeframe, 1.0)
        magnitude = magnitude * scale
        magnitude = min(magnitude, 15.0)

        # confidence based on signal consistency
        signals = [1 if s > 0 else -1 for s in [weighted_momentum, trend, momentum]]
        agreement = abs(sum(signals)) / 3
        confidence = 0.4 + agreement * 0.35  # range 0.4-0.75

        predicted_price = current_price * (1 + (magnitude / 100) * (1 if direction == "up" else -1))

        return {
            "direction": direction,
            "magnitude": round(magnitude, 2),
            "confidence": round(confidence, 4),
            "timeframe": timeframe,
            "predicted_price": round(predicted_price, 2),
            "current_price": current_price,
            "is_fallback": True,
        }

    def _default_prediction(self, candles: list[dict], timeframe: str) -> dict:
        """minimal prediction when insufficient data is provided"""
        current_price = candles[-1]["close"] if candles else 0
        return {
            "direction": "up",
            "magnitude": 0.0,
            "confidence": 0.0,
            "timeframe": timeframe,
            "predicted_price": current_price,
            "current_price": current_price,
            "is_fallback": True,
        }
