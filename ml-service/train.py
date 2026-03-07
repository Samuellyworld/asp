# training notebook for lstm price prediction model
# downloads historical data, engineers features, trains lstm, evaluates, saves

# %% [markdown]
# # LSTM Price Prediction Training
# trains a pytorch lstm to predict crypto price direction and magnitude

# %% imports
import numpy as np
import json
import os

try:
    import torch
    import torch.nn as nn
    from torch.utils.data import DataLoader, TensorDataset
    TORCH_AVAILABLE = True
except ImportError:
    TORCH_AVAILABLE = False
    print("pytorch not installed - training requires: pip install torch")

# %% generate synthetic training data (replace with real historical data)
def generate_training_data(n_samples=5000, seq_length=30):
    """generates synthetic ohlcv data that mimics crypto price action.
    in production, replace this with actual historical data from binance.
    """
    np.random.seed(42)
    data = []
    price = 40000.0

    for _ in range(n_samples + seq_length):
        # random walk with momentum
        trend = np.random.choice([-1, 1]) * np.random.exponential(0.5)
        volatility = np.random.exponential(1.0) * 0.01
        change = trend * volatility
        price = price * (1 + change / 100)
        price = max(price, 100)  # floor

        high = price * (1 + abs(np.random.normal(0, 0.005)))
        low = price * (1 - abs(np.random.normal(0, 0.005)))
        open_p = price * (1 + np.random.normal(0, 0.002))
        volume = np.random.exponential(1000) * 1000

        data.append({
            "open": open_p,
            "high": high,
            "low": low,
            "close": price,
            "volume": volume,
        })

    return data


# %% feature engineering
def feature_engineer(candles):
    """creates feature matrix from candle data"""
    closes = np.array([c["close"] for c in candles])
    highs = np.array([c["high"] for c in candles])
    lows = np.array([c["low"] for c in candles])
    opens = np.array([c["open"] for c in candles])
    volumes = np.array([c["volume"] for c in candles])

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
    return features, closes


# %% create sequences for lstm
def create_sequences(features, closes, seq_length=30, pred_horizon=1):
    """creates input sequences and target labels for training"""
    X, y = [], []
    for i in range(seq_length, len(features) - pred_horizon):
        X.append(features[i - seq_length:i])
        # target: direction and magnitude of next candle
        future_return = (closes[i + pred_horizon] - closes[i]) / closes[i] * 100
        direction = 1.0 if future_return > 0 else -1.0
        magnitude = abs(future_return)
        y.append([direction, magnitude])

    return np.array(X), np.array(y)


# %% train the model
def train_model(X_train, y_train, X_val, y_val, epochs=50, batch_size=32, lr=0.001):
    """trains the lstm model"""
    if not TORCH_AVAILABLE:
        print("skipping training - pytorch not available")
        return None

    from app.predictor import LSTMModel

    model = LSTMModel(input_size=X_train.shape[2])
    optimizer = torch.optim.Adam(model.parameters(), lr=lr)
    criterion = nn.MSELoss()

    train_dataset = TensorDataset(
        torch.FloatTensor(X_train),
        torch.FloatTensor(y_train),
    )
    train_loader = DataLoader(train_dataset, batch_size=batch_size, shuffle=True)

    X_val_t = torch.FloatTensor(X_val)
    y_val_t = torch.FloatTensor(y_val)

    best_val_loss = float("inf")
    patience = 10
    patience_counter = 0

    for epoch in range(epochs):
        model.train()
        train_loss = 0
        for batch_x, batch_y in train_loader:
            optimizer.zero_grad()
            output = model(batch_x)
            loss = criterion(output, batch_y)
            loss.backward()
            torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
            optimizer.step()
            train_loss += loss.item()

        train_loss /= len(train_loader)

        # validation
        model.eval()
        with torch.no_grad():
            val_output = model(X_val_t)
            val_loss = criterion(val_output, y_val_t).item()

        if (epoch + 1) % 10 == 0:
            print(f"epoch {epoch + 1}/{epochs} - train loss: {train_loss:.6f} - val loss: {val_loss:.6f}")

        # early stopping
        if val_loss < best_val_loss:
            best_val_loss = val_loss
            patience_counter = 0
            best_state = model.state_dict().copy()
        else:
            patience_counter += 1
            if patience_counter >= patience:
                print(f"early stopping at epoch {epoch + 1}")
                break

    model.load_state_dict(best_state)
    return model


# %% main training script
def main():
    print("generating training data...")
    data = generate_training_data(n_samples=5000)

    print("engineering features...")
    features, closes = feature_engineer(data)

    # normalize
    mean = features.mean(axis=0)
    std = features.std(axis=0)
    std[std == 0] = 1
    normalized = (features - mean) / std

    print("creating sequences...")
    X, y = create_sequences(normalized, closes)

    # train/val split (80/20)
    split = int(len(X) * 0.8)
    X_train, X_val = X[:split], X[split:]
    y_train, y_val = y[:split], y[split:]

    print(f"training set: {len(X_train)} samples")
    print(f"validation set: {len(X_val)} samples")

    print("training model...")
    model = train_model(X_train, y_train, X_val, y_val)

    if model is not None:
        # save model and scaler params
        os.makedirs("models", exist_ok=True)
        torch.save(model.state_dict(), "models/lstm_price.pt")

        scaler_params = {
            "mean": mean.tolist(),
            "std": std.tolist(),
        }
        with open("models/scaler_params.json", "w") as f:
            json.dump(scaler_params, f)

        print("model saved to models/lstm_price.pt")
        print("scaler params saved to models/scaler_params.json")

        # evaluate
        model.eval()
        with torch.no_grad():
            val_pred = model(torch.FloatTensor(X_val))
            pred_dirs = (val_pred[:, 0] > 0).float()
            true_dirs = (torch.FloatTensor(y_val[:, 0]) > 0).float()
            accuracy = (pred_dirs == true_dirs).float().mean().item()
            rmse = torch.sqrt(nn.MSELoss()(val_pred[:, 1], torch.FloatTensor(y_val[:, 1]))).item()

        print(f"direction accuracy: {accuracy:.2%}")
        print(f"magnitude rmse: {rmse:.4f}")
    else:
        print("training skipped - no pytorch")


if __name__ == "__main__":
    main()
