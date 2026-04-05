# request/response models for the ml service api

from pydantic import BaseModel, Field
from typing import Optional


class Candle(BaseModel):
    """ohlcv candle data"""
    open: float
    high: float
    low: float
    close: float
    volume: float
    timestamp: int


class PricePredictionRequest(BaseModel):
    """input for price prediction endpoint"""
    symbol: str = Field(..., description="trading pair e.g. BTC/USDT")
    candles: list[Candle] = Field(..., min_length=30, description="historical candle data")
    timeframe: str = Field(default="4h", description="prediction timeframe: 1h, 4h, 1d")


class PricePredictionResponse(BaseModel):
    """output from price prediction"""
    direction: str = Field(..., description="up or down")
    magnitude: float = Field(..., description="predicted price change percentage")
    confidence: float = Field(..., ge=0, le=1, description="model confidence 0-1")
    timeframe: str = Field(..., description="prediction timeframe")
    predicted_price: Optional[float] = Field(None, description="predicted price target")
    current_price: float = Field(..., description="current price used as baseline")


class SentimentRequest(BaseModel):
    """input for sentiment analysis endpoint"""
    text: str = Field(..., min_length=1, description="text to analyze")


class SentimentResponse(BaseModel):
    """output from sentiment analysis"""
    score: float = Field(..., ge=-1, le=1, description="sentiment score -1 to 1")
    label: str = Field(..., description="BULLISH, BEARISH, or NEUTRAL")
    confidence: float = Field(..., ge=0, le=1, description="classification confidence")


# --- ensemble ---

class EnsemblePredictionRequest(BaseModel):
    """input for ensemble prediction endpoint"""
    symbol: str = Field(..., description="trading pair e.g. BTC/USDT")
    candles: list[Candle] = Field(..., min_length=30, description="historical candle data")
    timeframe: str = Field(default="4h", description="prediction timeframe: 1h, 4h, 1d")


class ModelDetail(BaseModel):
    direction: str
    magnitude: float
    confidence: float
    weight: float


class EnsemblePredictionResponse(BaseModel):
    """output from ensemble prediction"""
    direction: str
    magnitude: float
    confidence: float = Field(..., ge=0, le=1)
    timeframe: str
    predicted_price: Optional[float] = None
    current_price: float
    model_count: int
    model_details: dict = Field(default_factory=dict)
    is_ensemble: bool = True


# --- patterns ---

class PatternDetectRequest(BaseModel):
    """input for chart pattern detection"""
    symbol: str = Field(..., description="trading pair")
    candles: list[Candle] = Field(..., min_length=20, description="historical candle data")


class PatternInfo(BaseModel):
    name: str
    direction: str
    confidence: float


class PatternDetectResponse(BaseModel):
    """output from pattern detection"""
    patterns: list[dict] = Field(default_factory=list)
    pattern_count: int = 0
    signal: str = "neutral"
    signal_strength: float = 0.0
    summary: str = ""


# --- drift ---

class DriftCheckRequest(BaseModel):
    """input for drift detection"""
    candles: list[Candle] = Field(..., min_length=30, description="recent candle data")


class DriftCheckResponse(BaseModel):
    """output from drift check"""
    drift_detected: bool
    reason: str
    recommendation: str
    checks: dict = Field(default_factory=dict)
    timestamp: Optional[str] = None


# --- retrain ---

class RetrainRequest(BaseModel):
    """input for retraining pipeline"""
    candles: list[Candle] = Field(..., min_length=60, description="training candle data")
    epochs: int = Field(default=50, ge=1, le=500)


class RetrainResponse(BaseModel):
    """output from retraining pipeline"""
    success: bool
    message: Optional[str] = None
    reason: Optional[str] = None
    promoted: Optional[bool] = None
    new_model_metrics: Optional[dict] = None
    current_model_metrics: Optional[dict] = None
    training_samples: Optional[int] = None
    validation_samples: Optional[int] = None


# --- walk-forward ---

class WalkForwardRequest(BaseModel):
    """input for walk-forward validation"""
    candles: list[Candle] = Field(..., min_length=60, description="historical candle data")
    n_splits: int = Field(default=5, ge=2, le=20)


class WalkForwardResponse(BaseModel):
    """output from walk-forward validation"""
    success: bool
    reason: Optional[str] = None
    n_splits: Optional[int] = None
    folds: Optional[list[dict]] = None
    avg_direction_accuracy: Optional[float] = None
    avg_magnitude_rmse: Optional[float] = None


# --- RL agent ---

class RLTrainRequest(BaseModel):
    """input for RL agent training"""
    candles: list[Candle] = Field(..., min_length=50, description="historical candle data")
    episodes: int = Field(default=100, ge=1, le=1000)
    initial_balance: float = Field(default=10000, gt=0)


class RLTrainResponse(BaseModel):
    """output from RL training"""
    success: bool
    reason: Optional[str] = None
    episodes: Optional[int] = None
    avg_reward_last_20: Optional[float] = None
    avg_pnl_last_20: Optional[float] = None
    best_pnl: Optional[float] = None
    final_epsilon: Optional[float] = None


class RLActionRequest(BaseModel):
    """input for RL action suggestion"""
    candles: list[Candle] = Field(..., min_length=20, description="recent candle data")
    balance: float = Field(..., gt=0)
    position: float = Field(default=0, ge=0)
    entry_price: float = Field(default=0, ge=0)


class RLActionResponse(BaseModel):
    """output from RL action suggestion"""
    action: str
    action_idx: int
    q_values: list[float] = Field(default_factory=list)
    exploring: bool = False
