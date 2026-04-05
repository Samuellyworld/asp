# tests for drift detection and retraining pipeline

import pytest
import numpy as np
from app.drift import DriftDetector
from app.retrain import RetrainingPipeline


class TestDriftDetector:
    def setup_method(self):
        self.detector = DriftDetector()

    def test_no_reference_set(self):
        current = np.random.randn(50, 5)
        result = self.detector.check_drift(current)
        assert result["drift_detected"] is False
        assert result["recommendation"] == "set_reference"

    def test_no_drift_same_distribution(self):
        np.random.seed(42)
        ref = np.random.randn(1000, 5)
        current = np.random.randn(1000, 5)  # same dist, large sample
        self.detector.set_reference(ref)
        result = self.detector.check_drift(current)
        # PSI should be low for same distribution; KS may flag at large N
        psi_ok = not result["checks"].get("psi", {}).get("drifted", True)
        assert psi_ok

    def test_drift_with_shifted_distribution(self):
        np.random.seed(42)
        ref = np.random.randn(200, 5)
        current = np.random.randn(100, 5) + 5  # shifted by 5
        self.detector.set_reference(ref)
        result = self.detector.check_drift(current)
        assert result["drift_detected"] is True

    def test_psi_calculation(self):
        np.random.seed(42)
        ref = np.random.randn(500)
        same = np.random.randn(500)
        psi = self.detector._calculate_psi(ref, same)
        assert psi < 0.2  # same distribution, low PSI

    def test_psi_high_for_different_distributions(self):
        np.random.seed(42)
        ref = np.random.randn(500)
        different = np.random.randn(500) + 10
        psi = self.detector._calculate_psi(ref, different)
        assert psi > 0.2

    def test_psi_constant_data(self):
        ref = np.ones(100)
        current = np.ones(100)
        psi = self.detector._calculate_psi(ref, current)
        assert psi == 0.0

    def test_accuracy_decay_no_data(self):
        result = self.detector._check_accuracy_decay(None)
        assert result["drifted"] is False

    def test_accuracy_decay_detected(self):
        self.detector.set_reference(np.random.randn(100, 5), accuracy=0.75)
        result = self.detector._check_accuracy_decay(0.55)
        assert result["drifted"] is True
        assert result["decay"] > 0.1

    def test_accuracy_decay_not_detected(self):
        self.detector.set_reference(np.random.randn(100, 5), accuracy=0.75)
        result = self.detector._check_accuracy_decay(0.72)
        assert result["drifted"] is False

    def test_log_prediction(self):
        self.detector.log_prediction(np.zeros(5), "up", "up")
        self.detector.log_prediction(np.zeros(5), "down", "up")
        assert len(self.detector.prediction_log) == 2

    def test_get_recent_accuracy_insufficient(self):
        for _ in range(5):
            self.detector.log_prediction(np.zeros(5), "up", "up")
        assert self.detector.get_recent_accuracy() is None

    def test_get_recent_accuracy_sufficient(self):
        for i in range(20):
            actual = "up" if i % 2 == 0 else "down"
            self.detector.log_prediction(np.zeros(5), "up", actual)
        acc = self.detector.get_recent_accuracy()
        assert acc is not None
        assert 0 <= acc <= 1

    def test_check_drift_returns_timestamp(self):
        ref = np.random.randn(100, 5)
        current = np.random.randn(50, 5)
        self.detector.set_reference(ref)
        result = self.detector.check_drift(current)
        assert "timestamp" in result

    def test_3d_features_handled(self):
        ref = np.random.randn(100, 30, 5)  # sequence data
        current = np.random.randn(50, 30, 5)
        self.detector.set_reference(ref)
        result = self.detector.check_drift(current)
        assert "checks" in result

    def test_recommendation_values(self):
        ref = np.random.randn(200, 5)
        self.detector.set_reference(ref)
        result = self.detector.check_drift(np.random.randn(100, 5))
        assert result["recommendation"] in ("no_action", "retrain", "urgent_retrain")


class TestRetrainingPipeline:
    def setup_method(self):
        self.pipeline = RetrainingPipeline(model_dir="test_models_tmp")

    def test_insufficient_data(self):
        features = np.random.randn(20, 5)
        closes = np.random.randn(20) * 100 + 40000
        result = self.pipeline.retrain(features, closes)
        assert result["success"] is False

    def test_create_sequences(self):
        features = np.random.randn(100, 5)
        closes = np.cumsum(np.random.randn(100)) + 40000
        X, y = self.pipeline._create_sequences(features, closes, seq_length=30)
        assert X.shape[0] == y.shape[0]
        assert X.shape[1] == 30
        assert X.shape[2] == 5
        assert y.shape[1] == 2

    def test_should_promote_improvement(self):
        new = {"direction_accuracy": 0.7}
        current = {"direction_accuracy": 0.5}
        assert self.pipeline._should_promote(new, current) is True

    def test_should_not_promote_no_improvement(self):
        new = {"direction_accuracy": 0.5}
        current = {"direction_accuracy": 0.5}
        assert self.pipeline._should_promote(new, current) is False

    def test_should_not_promote_marginal(self):
        new = {"direction_accuracy": 0.505}
        current = {"direction_accuracy": 0.5}
        assert self.pipeline._should_promote(new, current) is False

    def test_walk_forward_insufficient_data(self):
        features = np.random.randn(20, 5)
        closes = np.random.randn(20) * 100 + 40000
        result = self.pipeline.walk_forward_validate(features, closes)
        assert result["success"] is False
