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
