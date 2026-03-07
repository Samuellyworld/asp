"""
ml service - lstm price predictions + sentiment analysis
"""

from fastapi import FastAPI, HTTPException

from app.schemas import (
    PricePredictionRequest,
    PricePredictionResponse,
    SentimentRequest,
    SentimentResponse,
)
from app.predictor import PricePredictor
from app.sentiment import SentimentAnalyzer

app = FastAPI(
    title="Trading Bot ML Service",
    description="LSTM price predictions and sentiment analysis",
    version="0.1.0",
)

# initialize models on startup
predictor = PricePredictor()
sentiment_analyzer = SentimentAnalyzer()


@app.get("/")
async def root():
    return {
        "service": "ml-service",
        "version": "0.1.0",
        "status": "ready",
        "endpoints": {
            "predict_price": "/predict/price",
            "analyze_sentiment": "/analyze/sentiment",
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
