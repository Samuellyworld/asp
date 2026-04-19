"""
ml service - lstm price predictions + sentiment analysis + ensemble + patterns + drift + RL
"""

from fastapi import FastAPI, HTTPException

from app.schemas import (
    PricePredictionRequest,
    PricePredictionResponse,
    SentimentRequest,
    SentimentResponse,
    EnsemblePredictionRequest,
    EnsemblePredictionResponse,
    PatternDetectRequest,
    PatternDetectResponse,
    DriftCheckRequest,
    DriftCheckResponse,
    RetrainRequest,
    RetrainResponse,
    WalkForwardRequest,
    WalkForwardResponse,
    RLTrainRequest,
    RLTrainResponse,
    RLActionRequest,
    RLActionResponse,
)
from app.predictor import PricePredictor
from app.sentiment import SentimentAnalyzer
from app.ensemble import EnsemblePredictor
from app.patterns import PatternDetector
from app.drift import DriftDetector
from app.retrain import RetrainingPipeline
from app.rl_agent import TradingRLAgent

import logging
import json
import sys
import numpy as np


class JSONFormatter(logging.Formatter):
    """Structured JSON log formatter for production."""
    def format(self, record):
        log_entry = {
            "timestamp": self.formatTime(record),
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
        }
        if record.exc_info and record.exc_info[0]:
            log_entry["exception"] = self.formatException(record.exc_info)
        return json.dumps(log_entry)


# configure structured logging
handler = logging.StreamHandler(sys.stdout)
handler.setFormatter(JSONFormatter())
logging.basicConfig(level=logging.INFO, handlers=[handler])
logger = logging.getLogger("ml-service")

app = FastAPI(
    title="Trading Bot ML Service",
    description="LSTM price predictions, sentiment analysis, ensemble models, chart patterns, drift detection, and RL agent",
    version="0.2.0",
)

# initialize models on startup
predictor = PricePredictor()
sentiment_analyzer = SentimentAnalyzer()
ensemble = EnsemblePredictor(predictor=predictor)
pattern_detector = PatternDetector()
drift_detector = DriftDetector()
retrain_pipeline = RetrainingPipeline()
rl_agent = TradingRLAgent()


@app.get("/")
async def root():
    return {
        "service": "ml-service",
        "version": "0.2.0",
        "status": "ready",
        "endpoints": {
            "predict_price": "/predict/price",
            "analyze_sentiment": "/analyze/sentiment",
            "ensemble_predict": "/ensemble/predict",
            "detect_patterns": "/patterns/detect",
            "check_drift": "/drift/check",
            "retrain": "/retrain",
            "walk_forward": "/walk-forward",
            "rl_train": "/rl/train",
            "rl_action": "/rl/action",
        },
    }


@app.get("/health")
async def health():
    return {"status": "healthy"}


@app.post("/predict/price", response_model=PricePredictionResponse)
async def predict_price(request: PricePredictionRequest):
    """predicts price direction and magnitude using lstm or statistical fallback"""
    try:
        candles = [c.model_dump() for c in request.candles]
        result = predictor.predict(candles, request.timeframe)
        return PricePredictionResponse(**result)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"prediction failed: {str(e)}")


@app.post("/analyze/sentiment", response_model=SentimentResponse)
async def analyze_sentiment(request: SentimentRequest):
    """analyzes text sentiment for crypto trading context"""
    try:
        result = sentiment_analyzer.analyze(request.text)
        return SentimentResponse(**result)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"sentiment analysis failed: {str(e)}")


@app.post("/ensemble/predict", response_model=EnsemblePredictionResponse)
async def ensemble_predict(request: EnsemblePredictionRequest):
    """predicts using weighted ensemble of LSTM + gradient boosting + random forest"""
    try:
        candles = [c.model_dump() for c in request.candles]
        result = ensemble.predict(candles, request.timeframe)
        return EnsemblePredictionResponse(**result)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"ensemble prediction failed: {str(e)}")


@app.post("/patterns/detect", response_model=PatternDetectResponse)
async def detect_patterns(request: PatternDetectRequest):
    """detects chart patterns in candle data"""
    try:
        candles = [c.model_dump() for c in request.candles]
        result = pattern_detector.detect(candles)
        return PatternDetectResponse(**result)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"pattern detection failed: {str(e)}")


@app.post("/drift/check", response_model=DriftCheckResponse)
async def check_drift(request: DriftCheckRequest):
    """checks for concept drift in model inputs"""
    try:
        candles = [c.model_dump() for c in request.candles]
        features = predictor._feature_engineer(candles)
        normalized = predictor._normalize(features)

        # use training scaler params as reference if available
        if drift_detector.reference_features is None and predictor.scaler_params:
            # generate synthetic reference from scaler params
            mean = np.array(predictor.scaler_params["mean"])
            std = np.array(predictor.scaler_params["std"])
            ref = np.random.normal(0, 1, (200, len(mean)))  # normalized reference
            drift_detector.set_reference(ref)

        recent_accuracy = drift_detector.get_recent_accuracy()
        result = drift_detector.check_drift(normalized, recent_accuracy)
        return DriftCheckResponse(**result)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"drift check failed: {str(e)}")


@app.post("/retrain", response_model=RetrainResponse)
async def retrain(request: RetrainRequest):
    """triggers model retraining with new data"""
    try:
        candles = [c.model_dump() for c in request.candles]
        features = predictor._feature_engineer(candles)
        closes = np.array([c["close"] for c in candles])

        mean = features.mean(axis=0)
        std = features.std(axis=0)
        std[std == 0] = 1
        normalized = (features - mean) / std

        result = retrain_pipeline.retrain(normalized, closes, epochs=request.epochs)

        if result.get("promoted"):
            predictor._load_model()

        return RetrainResponse(**result)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"retraining failed: {str(e)}")


@app.post("/walk-forward", response_model=WalkForwardResponse)
async def walk_forward(request: WalkForwardRequest):
    """runs walk-forward cross-validation"""
    try:
        candles = [c.model_dump() for c in request.candles]
        features = predictor._feature_engineer(candles)
        closes = np.array([c["close"] for c in candles])

        mean = features.mean(axis=0)
        std = features.std(axis=0)
        std[std == 0] = 1
        normalized = (features - mean) / std

        result = retrain_pipeline.walk_forward_validate(
            normalized, closes, n_splits=request.n_splits,
        )
        return WalkForwardResponse(**result)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"walk-forward failed: {str(e)}")


@app.post("/rl/train", response_model=RLTrainResponse)
async def rl_train(request: RLTrainRequest):
    """trains the RL agent from historical candle data"""
    try:
        candles = [c.model_dump() for c in request.candles]
        result = rl_agent.train_from_backtest(
            candles, initial_balance=request.initial_balance, episodes=request.episodes,
        )
        return RLTrainResponse(**result)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"RL training failed: {str(e)}")


@app.post("/rl/action", response_model=RLActionResponse)
async def rl_action(request: RLActionRequest):
    """gets action suggestion from RL agent"""
    try:
        candles = [c.model_dump() for c in request.candles]
        state = rl_agent._extract_state(
            candles, len(candles) - 1, request.balance,
            request.position, request.entry_price, request.balance,
        )
        result = rl_agent.get_action(state, explore=False)
        return RLActionResponse(**result)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"RL action failed: {str(e)}")
