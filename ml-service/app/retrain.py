# automated retraining pipeline
# triggers retraining when drift is detected, validates new model before promoting

import logging
import numpy as np
import os
import json
from typing import Optional
from datetime import datetime

logger = logging.getLogger(__name__)

try:
    import torch
    import torch.nn as nn
    from torch.utils.data import DataLoader, TensorDataset
    TORCH_AVAILABLE = True
except ImportError:
    TORCH_AVAILABLE = False

try:
    from sklearn.model_selection import TimeSeriesSplit
    SKLEARN_AVAILABLE = True
except ImportError:
    SKLEARN_AVAILABLE = False


class RetrainingPipeline:
    """automated retraining with validation and model promotion"""

    def __init__(self, model_dir: str = "models", min_improvement: float = 0.01):
        self.model_dir = model_dir
        self.min_improvement = min_improvement
        self.retrain_history: list[dict] = []

    def retrain(self, features: np.ndarray, closes: np.ndarray,
                seq_length: int = 30, epochs: int = 50) -> dict:
        """runs the full retraining pipeline:
        1. creates sequences from new data
        2. trains new model with walk-forward validation
        3. compares against current model
        4. promotes if improved
        """
        if not TORCH_AVAILABLE:
            return {"success": False, "reason": "pytorch not available"}

        if len(features) < seq_length + 10:
            return {"success": False, "reason": "insufficient data for retraining"}

        # create sequences
        X, y = self._create_sequences(features, closes, seq_length)

        # walk-forward split
        split = int(len(X) * 0.8)
        X_train, X_val = X[:split], X[split:]
        y_train, y_val = y[:split], y[split:]

        if len(X_train) < 10 or len(X_val) < 5:
            return {"success": False, "reason": "insufficient data after split"}

        # train new model
        new_model, train_metrics = self._train(X_train, y_train, X_val, y_val, epochs)
        if new_model is None:
            return {"success": False, "reason": "training failed"}

        # evaluate new model
        new_metrics = self._evaluate(new_model, X_val, y_val)

        # compare with current model
        current_metrics = self._evaluate_current(X_val, y_val)

        should_promote = self._should_promote(new_metrics, current_metrics)

        result = {
            "success": True,
            "new_model_metrics": new_metrics,
            "current_model_metrics": current_metrics,
            "promoted": should_promote,
            "timestamp": datetime.utcnow().isoformat(),
            "training_samples": len(X_train),
            "validation_samples": len(X_val),
        }

        if should_promote:
            self._promote_model(new_model, features)
            result["message"] = "new model promoted — improved performance"
        else:
            result["message"] = "new model not promoted — no significant improvement"

        self.retrain_history.append(result)
        return result

    def walk_forward_validate(self, features: np.ndarray, closes: np.ndarray,
                              seq_length: int = 30, n_splits: int = 5) -> dict:
        """performs walk-forward cross-validation on the data"""
        if not TORCH_AVAILABLE or not SKLEARN_AVAILABLE:
            return {"success": False, "reason": "missing dependencies (torch, sklearn)"}

        X, y = self._create_sequences(features, closes, seq_length)

        if len(X) < n_splits * 10:
            return {"success": False, "reason": "insufficient data for walk-forward"}

        tscv = TimeSeriesSplit(n_splits=n_splits)
        fold_results = []

        for fold, (train_idx, val_idx) in enumerate(tscv.split(X)):
            X_train, X_val = X[train_idx], X[val_idx]
            y_train, y_val = y[train_idx], y[val_idx]

            model, _ = self._train(X_train, y_train, X_val, y_val, epochs=30)
            if model is None:
                continue

            metrics = self._evaluate(model, X_val, y_val)
            metrics["fold"] = fold + 1
            metrics["train_size"] = len(train_idx)
            metrics["val_size"] = len(val_idx)
            fold_results.append(metrics)

        if not fold_results:
            return {"success": False, "reason": "all folds failed"}

        avg_accuracy = np.mean([f["direction_accuracy"] for f in fold_results])
        avg_rmse = np.mean([f["magnitude_rmse"] for f in fold_results])

        return {
            "success": True,
            "n_splits": n_splits,
            "folds": fold_results,
            "avg_direction_accuracy": round(float(avg_accuracy), 4),
            "avg_magnitude_rmse": round(float(avg_rmse), 4),
            "timestamp": datetime.utcnow().isoformat(),
        }

    def _create_sequences(self, features: np.ndarray, closes: np.ndarray,
                          seq_length: int) -> tuple:
        """creates sequences for LSTM training"""
        X, y = [], []
        for i in range(seq_length, len(features) - 1):
            X.append(features[i - seq_length:i])
            future_return = (closes[i + 1] - closes[i]) / closes[i] * 100
            direction = 1.0 if future_return > 0 else -1.0
            magnitude = abs(future_return)
            y.append([direction, magnitude])
        return np.array(X), np.array(y)

    def _train(self, X_train: np.ndarray, y_train: np.ndarray,
               X_val: np.ndarray, y_val: np.ndarray,
               epochs: int = 50) -> tuple:
        """trains an LSTM model"""
        try:
            from app.predictor import LSTMModel

            model = LSTMModel(input_size=X_train.shape[2])
            optimizer = torch.optim.Adam(model.parameters(), lr=0.001)
            criterion = nn.MSELoss()

            dataset = TensorDataset(
                torch.FloatTensor(X_train),
                torch.FloatTensor(y_train),
            )
            loader = DataLoader(dataset, batch_size=32, shuffle=True)

            X_val_t = torch.FloatTensor(X_val)
            y_val_t = torch.FloatTensor(y_val)

            best_val_loss = float("inf")
            best_state = None
            patience = 10
            patience_counter = 0

            for epoch in range(epochs):
                model.train()
                for batch_x, batch_y in loader:
                    optimizer.zero_grad()
                    output = model(batch_x)
                    loss = criterion(output, batch_y)
                    loss.backward()
                    torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
                    optimizer.step()

                model.eval()
                with torch.no_grad():
                    val_output = model(X_val_t)
                    val_loss = criterion(val_output, y_val_t).item()

                if val_loss < best_val_loss:
                    best_val_loss = val_loss
                    patience_counter = 0
                    best_state = model.state_dict().copy()
                else:
                    patience_counter += 1
                    if patience_counter >= patience:
                        break

            if best_state:
                model.load_state_dict(best_state)

            metrics = {"final_val_loss": best_val_loss, "epochs_trained": epoch + 1}
            return model, metrics

        except Exception as e:
            logger.error(f"training failed: {e}")
            return None, {}

    def _evaluate(self, model, X_val: np.ndarray, y_val: np.ndarray) -> dict:
        """evaluates a model on validation data"""
        model.eval()
        with torch.no_grad():
            pred = model(torch.FloatTensor(X_val))
            pred_dirs = (pred[:, 0] > 0).float()
            true_dirs = (torch.FloatTensor(y_val[:, 0]) > 0).float()
            accuracy = (pred_dirs == true_dirs).float().mean().item()
            rmse = torch.sqrt(nn.MSELoss()(pred[:, 1], torch.FloatTensor(y_val[:, 1]))).item()

        return {
            "direction_accuracy": round(accuracy, 4),
            "magnitude_rmse": round(rmse, 4),
        }

    def _evaluate_current(self, X_val: np.ndarray, y_val: np.ndarray) -> dict:
        """evaluates the currently deployed model"""
        model_path = os.path.join(self.model_dir, "lstm_price.pt")
        if not os.path.exists(model_path) or not TORCH_AVAILABLE:
            return {"direction_accuracy": 0.0, "magnitude_rmse": float("inf")}

        try:
            from app.predictor import LSTMModel
            model = LSTMModel(input_size=X_val.shape[2])
            model.load_state_dict(torch.load(model_path, map_location="cpu"))
            return self._evaluate(model, X_val, y_val)
        except Exception:
            return {"direction_accuracy": 0.0, "magnitude_rmse": float("inf")}

    def _should_promote(self, new_metrics: dict, current_metrics: dict) -> bool:
        """decides if the new model is better than the current one"""
        new_acc = new_metrics.get("direction_accuracy", 0)
        cur_acc = current_metrics.get("direction_accuracy", 0)
        return new_acc > cur_acc + self.min_improvement

    def _promote_model(self, model, features: np.ndarray):
        """saves the new model and scaler params, backing up the old one"""
        os.makedirs(self.model_dir, exist_ok=True)

        model_path = os.path.join(self.model_dir, "lstm_price.pt")
        backup_path = os.path.join(self.model_dir, "lstm_price_backup.pt")

        # backup current model
        if os.path.exists(model_path):
            import shutil
            shutil.copy2(model_path, backup_path)

        torch.save(model.state_dict(), model_path)

        # update scaler params
        if features.ndim == 3:
            flat = features.reshape(-1, features.shape[-1])
        else:
            flat = features
        mean = flat.mean(axis=0)
        std = flat.std(axis=0)
        std[std == 0] = 1

        scaler_path = os.path.join(self.model_dir, "scaler_params.json")
        with open(scaler_path, "w") as f:
            json.dump({"mean": mean.tolist(), "std": std.tolist()}, f)

        logger.info("new model promoted and saved")
