# training notebook for lstm price prediction model
# downloads historical data, engineers features, trains lstm, evaluates, saves

# %% [markdown]
# # LSTM Price Prediction Training
# trains a pytorch lstm to predict crypto price direction and magnitude

# %% imports
import argparse
from collections import defaultdict
import csv
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

try:
    import psycopg
    PSYCOPG_AVAILABLE = True
except ImportError:
    PSYCOPG_AVAILABLE = False

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


def db_connect_kwargs():
    """returns postgres connection settings from compose-compatible env vars"""
    password = os.getenv("DATABASE_PASSWORD")
    if not password:
        raise RuntimeError("DATABASE_PASSWORD must be set to load candles from the database")

    return {
        "host": os.getenv("DATABASE_HOST", "postgres"),
        "port": int(os.getenv("DATABASE_PORT", "5432")),
        "user": os.getenv("DATABASE_USER", "trading_bot"),
        "password": password,
        "dbname": os.getenv("DATABASE_NAME", "trading_bot"),
    }


def load_candles_from_db(interval="4h", symbols=None, min_candles=120, limit_per_symbol=5000):
    """loads real OHLCV candles from the bot's candles table, grouped by symbol"""
    if not PSYCOPG_AVAILABLE:
        raise RuntimeError("psycopg is not installed; rebuild ml-service after updating requirements.txt")

    params = [interval, limit_per_symbol]
    symbol_filter = ""
    if symbols:
        symbol_filter = "AND symbol = ANY(%s)"
        params.insert(1, symbols)

    query = f"""
        WITH ranked AS (
            SELECT
                time, symbol, open, high, low, close, volume,
                row_number() OVER (PARTITION BY symbol ORDER BY time DESC) AS rn
            FROM candles
            WHERE interval = %s
              {symbol_filter}
        )
        SELECT symbol, open, high, low, close, volume
        FROM ranked
        WHERE rn <= %s
        ORDER BY symbol, time ASC
    """

    grouped = defaultdict(list)
    with psycopg.connect(**db_connect_kwargs()) as conn:
        with conn.cursor() as cur:
            cur.execute(query, params)
            for symbol, open_p, high, low, close, volume in cur.fetchall():
                grouped[symbol].append({
                    "open": float(open_p),
                    "high": float(high),
                    "low": float(low),
                    "close": float(close),
                    "volume": float(volume),
                })

    return {symbol: candles for symbol, candles in grouped.items() if len(candles) >= min_candles}


def load_candles_from_csv(path, symbols=None, min_candles=120):
    """loads OHLCV candles from a CSV dataset, grouped by symbol.

    Required columns: open, high, low, close, volume.
    Optional columns: symbol, timestamp.
    If symbol is omitted, all rows are grouped under DATASET/USDT.
    """
    wanted = set(symbols or [])
    grouped = defaultdict(list)
    default_symbol = "DATASET/USDT"

    with open(path, newline="") as f:
        reader = csv.DictReader(f)
        required = {"open", "high", "low", "close", "volume"}
        missing = required - set(reader.fieldnames or [])
        if missing:
            raise ValueError(f"dataset missing required columns: {', '.join(sorted(missing))}")

        for row in reader:
            symbol = (row.get("symbol") or default_symbol).strip()
            if wanted and symbol not in wanted:
                continue
            grouped[symbol].append({
                "timestamp": row.get("timestamp") or "",
                "open": float(row["open"]),
                "high": float(row["high"]),
                "low": float(row["low"]),
                "close": float(row["close"]),
                "volume": float(row["volume"]),
            })

    for candles in grouped.values():
        candles.sort(key=lambda c: c["timestamp"])

    return {symbol: candles for symbol, candles in grouped.items() if len(candles) >= min_candles}


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
def create_sequences(features, closes, seq_length=30, pred_horizon=1, min_target_index=None, max_target_index=None):
    """creates input sequences and target labels for training"""
    X, y = [], []
    for i in range(seq_length, len(features) - pred_horizon):
        if min_target_index is not None and i < min_target_index:
            continue
        if max_target_index is not None and i >= max_target_index:
            continue

        X.append(features[i - seq_length:i])
        # target: direction and magnitude of next candle
        future_return = (closes[i + pred_horizon] - closes[i]) / closes[i] * 100
        direction = 1.0 if future_return > 0 else -1.0
        magnitude = abs(future_return)
        y.append([direction, magnitude])

    return np.array(X), np.array(y)


def prepare_dataset(candles_by_symbol, seq_length=30, pred_horizon=1, train_ratio=0.8):
    """engineers, normalizes, and sequences candle groups without crossing symbols.

    The chronological train/validation split is done per symbol before computing
    scaler stats so validation candles do not leak into training normalization.
    """
    engineered = {}
    train_features = []
    split_by_symbol = {}

    for symbol, candles in candles_by_symbol.items():
        split = int(len(candles) * train_ratio)
        split = min(max(split, seq_length + pred_horizon), len(candles) - pred_horizon)
        if split <= seq_length or split >= len(candles):
            continue

        features, closes = feature_engineer(candles)
        engineered[symbol] = (features, closes)

        scaler_features, _ = feature_engineer(candles[:split])
        split_by_symbol[symbol] = split
        train_features.append(scaler_features)

    if not train_features:
        raise ValueError("no candle groups available for training")

    stacked = np.vstack(train_features)
    mean = stacked.mean(axis=0)
    std = stacked.std(axis=0)
    std[std == 0] = 1

    X_train_parts, y_train_parts, X_val_parts, y_val_parts = [], [], [], []
    for symbol, (features, closes) in engineered.items():
        split = split_by_symbol.get(symbol)
        if split is None:
            continue

        normalized = (features - mean) / std
        X_train_sym, y_train_sym = create_sequences(
            normalized,
            closes,
            seq_length,
            pred_horizon,
            max_target_index=split - pred_horizon,
        )
        X_val_sym, y_val_sym = create_sequences(
            normalized,
            closes,
            seq_length,
            pred_horizon,
            min_target_index=split,
        )
        if len(X_train_sym) > 0:
            X_train_parts.append(X_train_sym)
            y_train_parts.append(y_train_sym)
        if len(X_val_sym) > 0:
            X_val_parts.append(X_val_sym)
            y_val_parts.append(y_val_sym)

    if not X_train_parts or not X_val_parts:
        raise ValueError("not enough candles to create training and validation sequences")

    X_train = np.concatenate(X_train_parts, axis=0)
    y_train = np.concatenate(y_train_parts, axis=0)
    X_val = np.concatenate(X_val_parts, axis=0)
    y_val = np.concatenate(y_val_parts, axis=0)
    return X_train, y_train, X_val, y_val, mean, std


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
    best_state = None
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
            best_state = {k: v.detach().clone() for k, v in model.state_dict().items()}
        else:
            patience_counter += 1
            if patience_counter >= patience:
                print(f"early stopping at epoch {epoch + 1}")
                break

    if best_state is not None:
        model.load_state_dict(best_state)
    return model


# %% main training script
def main():
    parser = argparse.ArgumentParser(description="Train the LSTM price model")
    parser.add_argument("--source", choices=["db", "csv", "synthetic"], default=os.getenv("TRAIN_DATA_SOURCE", "db"))
    parser.add_argument("--dataset", default=os.getenv("TRAIN_DATASET", ""))
    parser.add_argument("--interval", default=os.getenv("TRAIN_INTERVAL", "4h"))
    parser.add_argument("--symbols", default=os.getenv("TRAIN_SYMBOLS", ""))
    parser.add_argument("--min-candles", type=int, default=int(os.getenv("TRAIN_MIN_CANDLES", "120")))
    parser.add_argument("--limit-per-symbol", type=int, default=int(os.getenv("TRAIN_LIMIT_PER_SYMBOL", "5000")))
    parser.add_argument("--epochs", type=int, default=int(os.getenv("TRAIN_EPOCHS", "50")))
    parser.add_argument("--model-dir", default=os.getenv("MODEL_PATH", "models"))
    args = parser.parse_args()

    symbols = [s.strip() for s in args.symbols.split(",") if s.strip()]

    if args.source == "db":
        print(f"loading real candles from database interval={args.interval} symbols={symbols or 'all'}...")
        candles_by_symbol = load_candles_from_db(
            interval=args.interval,
            symbols=symbols or None,
            min_candles=args.min_candles,
            limit_per_symbol=args.limit_per_symbol,
        )
        if not candles_by_symbol:
            raise RuntimeError(
                f"no candle groups with at least {args.min_candles} candles for interval {args.interval}; "
                "wait for ingestion or run with --source synthetic"
            )
        total_candles = sum(len(c) for c in candles_by_symbol.values())
        print(f"loaded {total_candles} real candles across {len(candles_by_symbol)} symbols")
    elif args.source == "csv":
        if not args.dataset:
            raise RuntimeError("--dataset is required when --source csv")
        print(f"loading candles from csv dataset={args.dataset} symbols={symbols or 'all'}...")
        candles_by_symbol = load_candles_from_csv(
            args.dataset,
            symbols=symbols or None,
            min_candles=args.min_candles,
        )
        if not candles_by_symbol:
            raise RuntimeError(
                f"no candle groups with at least {args.min_candles} candles in {args.dataset}"
            )
        total_candles = sum(len(c) for c in candles_by_symbol.values())
        print(f"loaded {total_candles} csv candles across {len(candles_by_symbol)} symbols")
    else:
        print("generating synthetic training data...")
        candles_by_symbol = {"SYNTH/USDT": generate_training_data(n_samples=5000)}

    print("engineering features and creating sequences...")
    X_train, y_train, X_val, y_val, mean, std = prepare_dataset(candles_by_symbol)

    print(f"training set: {len(X_train)} samples")
    print(f"validation set: {len(X_val)} samples")

    print("training model...")
    model = train_model(X_train, y_train, X_val, y_val, epochs=args.epochs)

    if model is not None:
        # save model and scaler params
        os.makedirs(args.model_dir, exist_ok=True)
        model_path = os.path.join(args.model_dir, "lstm_price.pt")
        scaler_path = os.path.join(args.model_dir, "scaler_params.json")
        torch.save(model.state_dict(), model_path)

        scaler_params = {
            "mean": mean.tolist(),
            "std": std.tolist(),
        }
        with open(scaler_path, "w") as f:
            json.dump(scaler_params, f)

        print(f"model saved to {model_path}")
        print(f"scaler params saved to {scaler_path}")

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
