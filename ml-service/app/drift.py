# concept drift detection for ML model monitoring
# monitors feature distribution shifts using PSI, KS test, and accuracy decay

import logging
import numpy as np
from typing import Optional
from datetime import datetime

logger = logging.getLogger(__name__)

try:
    from scipy import stats as scipy_stats
    SCIPY_AVAILABLE = True
except ImportError:
    SCIPY_AVAILABLE = False


class DriftDetector:
    """monitors for concept drift in model inputs and outputs.
    uses Population Stability Index (PSI), Kolmogorov-Smirnov test,
    and accuracy decay tracking.
    """

    def __init__(self, psi_threshold: float = 0.2, ks_threshold: float = 0.05,
                 accuracy_decay_threshold: float = 0.1, window_size: int = 100):
        self.psi_threshold = psi_threshold
        self.ks_threshold = ks_threshold
        self.accuracy_decay_threshold = accuracy_decay_threshold
        self.window_size = window_size
        self.reference_features: Optional[np.ndarray] = None
        self.reference_accuracy: Optional[float] = None
        self.prediction_log: list[dict] = []

    def set_reference(self, features: np.ndarray, accuracy: Optional[float] = None):
        """sets the reference distribution from training data"""
        self.reference_features = features
        self.reference_accuracy = accuracy

    def check_drift(self, current_features: np.ndarray,
                    recent_accuracy: Optional[float] = None) -> dict:
        """runs all drift checks and returns a comprehensive report"""
        if self.reference_features is None:
            return {
                "drift_detected": False,
                "reason": "no reference distribution set",
                "checks": {},
                "recommendation": "set_reference",
            }

        checks = {}

        # 1. PSI check per feature
        psi_results = self._check_psi(current_features)
        checks["psi"] = psi_results

        # 2. KS test per feature
        ks_results = self._check_ks(current_features)
        checks["ks_test"] = ks_results

        # 3. Accuracy decay
        accuracy_result = self._check_accuracy_decay(recent_accuracy)
        checks["accuracy_decay"] = accuracy_result

        # overall drift decision
        drift_detected = bool(
            psi_results.get("drifted", False) or
            ks_results.get("drifted", False) or
            accuracy_result.get("drifted", False)
        )

        reasons = []
        if psi_results.get("drifted"):
            reasons.append(f"PSI={psi_results['max_psi']:.3f} exceeds {self.psi_threshold}")
        if ks_results.get("drifted"):
            reasons.append(f"KS test detected distribution shift (p={ks_results['min_p_value']:.4f})")
        if accuracy_result.get("drifted"):
            reasons.append(f"accuracy decay={accuracy_result['decay']:.3f}")

        recommendation = "retrain" if drift_detected else "no_action"
        if drift_detected and len(reasons) >= 2:
            recommendation = "urgent_retrain"

        return {
            "drift_detected": drift_detected,
            "reason": "; ".join(reasons) if reasons else "no drift detected",
            "checks": checks,
            "recommendation": recommendation,
            "timestamp": datetime.utcnow().isoformat(),
        }

    def _check_psi(self, current: np.ndarray) -> dict:
        """calculates Population Stability Index for each feature"""
        ref = self.reference_features
        if ref.ndim == 3:
            ref = ref.reshape(-1, ref.shape[-1])
        if current.ndim == 3:
            current = current.reshape(-1, current.shape[-1])

        n_features = min(ref.shape[1], current.shape[1])
        psi_values = []

        for i in range(n_features):
            psi = self._calculate_psi(ref[:, i], current[:, i])
            psi_values.append(psi)

        max_psi = max(psi_values) if psi_values else 0.0
        drifted = bool(max_psi > self.psi_threshold)

        return {
            "psi_per_feature": [round(p, 4) for p in psi_values],
            "max_psi": round(max_psi, 4),
            "threshold": self.psi_threshold,
            "drifted": drifted,
        }

    def _calculate_psi(self, reference: np.ndarray, current: np.ndarray,
                       n_bins: int = 10) -> float:
        """calculates PSI between two distributions"""
        eps = 1e-6

        # use reference distribution to define bins
        min_val = min(reference.min(), current.min())
        max_val = max(reference.max(), current.max())

        if min_val == max_val:
            return 0.0

        bins = np.linspace(min_val, max_val, n_bins + 1)

        ref_counts = np.histogram(reference, bins=bins)[0].astype(float)
        cur_counts = np.histogram(current, bins=bins)[0].astype(float)

        ref_pct = ref_counts / max(ref_counts.sum(), 1) + eps
        cur_pct = cur_counts / max(cur_counts.sum(), 1) + eps

        psi = np.sum((cur_pct - ref_pct) * np.log(cur_pct / ref_pct))
        return float(psi)

    def _check_ks(self, current: np.ndarray) -> dict:
        """runs Kolmogorov-Smirnov test for each feature"""
        if not SCIPY_AVAILABLE:
            return {"drifted": False, "reason": "scipy not available"}

        ref = self.reference_features
        if ref.ndim == 3:
            ref = ref.reshape(-1, ref.shape[-1])
        if current.ndim == 3:
            current = current.reshape(-1, current.shape[-1])

        n_features = min(ref.shape[1], current.shape[1])
        p_values = []
        statistics = []

        for i in range(n_features):
            stat, p_val = scipy_stats.ks_2samp(ref[:, i], current[:, i])
            p_values.append(p_val)
            statistics.append(stat)

        min_p = min(p_values) if p_values else 1.0
        drifted = bool(min_p < self.ks_threshold)

        return {
            "p_values": [round(p, 6) for p in p_values],
            "statistics": [round(s, 6) for s in statistics],
            "min_p_value": round(min_p, 6),
            "threshold": self.ks_threshold,
            "drifted": drifted,
        }

    def _check_accuracy_decay(self, recent_accuracy: Optional[float]) -> dict:
        """checks if model accuracy has decayed beyond threshold"""
        if self.reference_accuracy is None or recent_accuracy is None:
            return {"drifted": False, "reason": "no accuracy data"}

        decay = self.reference_accuracy - recent_accuracy
        drifted = bool(decay > self.accuracy_decay_threshold)

        return {
            "reference_accuracy": round(self.reference_accuracy, 4),
            "recent_accuracy": round(recent_accuracy, 4),
            "decay": round(decay, 4),
            "threshold": self.accuracy_decay_threshold,
            "drifted": drifted,
        }

    def log_prediction(self, features: np.ndarray, predicted_direction: str,
                       actual_direction: Optional[str] = None):
        """logs a prediction for tracking accuracy over time"""
        entry = {
            "timestamp": datetime.utcnow().isoformat(),
            "predicted": predicted_direction,
            "actual": actual_direction,
        }
        self.prediction_log.append(entry)

        # keep only recent window
        if len(self.prediction_log) > self.window_size * 2:
            self.prediction_log = self.prediction_log[-self.window_size:]

    def get_recent_accuracy(self) -> Optional[float]:
        """calculates accuracy from recent logged predictions"""
        resolved = [
            p for p in self.prediction_log[-self.window_size:]
            if p["actual"] is not None
        ]
        if len(resolved) < 10:
            return None

        correct = sum(1 for p in resolved if p["predicted"] == p["actual"])
        return correct / len(resolved)
