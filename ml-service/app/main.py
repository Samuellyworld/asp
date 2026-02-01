"""
ml service - lstm price predictions + sentiment analysis
"""

from fastapi import FastAPI

app = FastAPI(
    title="Trading Bot ML Service",
    description="LSTM price predictions and sentiment analysis",
    version="0.1.0"
)


@app.get("/")
async def root():
    return {
        "service": "ml-service",
        "version": "0.1.0",
        "status": "ready",
        "endpoints": {
            "predict_price": "/predict/price",
            "analyze_sentiment": "/analyze/sentiment"
        }
    }


@app.get("/health")
async def health():
    return {"status": "healthy"}
