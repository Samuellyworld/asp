# chart pattern detection using price action analysis
# detects: head & shoulders, double top/bottom, triangles, flags, wedges

import logging
import numpy as np
from typing import Optional

logger = logging.getLogger(__name__)


class PatternDetector:
    """detects classical chart patterns from OHLCV candle data"""

    def __init__(self, min_pattern_bars: int = 10, tolerance: float = 0.02):
        self.min_pattern_bars = min_pattern_bars
        self.tolerance = tolerance  # price tolerance for pattern matching (2%)

    def detect(self, candles: list[dict]) -> dict:
        """runs all pattern detectors and returns findings"""
        if len(candles) < self.min_pattern_bars:
            return {"patterns": [], "summary": "insufficient data"}

        closes = np.array([c["close"] for c in candles])
        highs = np.array([c["high"] for c in candles])
        lows = np.array([c["low"] for c in candles])

        pivots = self._find_pivots(closes, window=5)
        patterns = []

        # run all detectors
        hs = self._detect_head_shoulders(pivots, closes, is_inverse=False)
        if hs:
            patterns.append(hs)

        ihs = self._detect_head_shoulders(pivots, closes, is_inverse=True)
        if ihs:
            patterns.append(ihs)

        dt = self._detect_double_top(pivots, closes, highs)
        if dt:
            patterns.append(dt)

        db = self._detect_double_bottom(pivots, closes, lows)
        if db:
            patterns.append(db)

        tri = self._detect_triangle(pivots, closes, highs, lows)
        if tri:
            patterns.append(tri)

        flag = self._detect_flag(closes, highs, lows)
        if flag:
            patterns.append(flag)

        wedge = self._detect_wedge(pivots, closes, highs, lows)
        if wedge:
            patterns.append(wedge)

        # sort by confidence
        patterns.sort(key=lambda p: p["confidence"], reverse=True)

        # derive overall signal
        signal = self._derive_signal(patterns)

        return {
            "patterns": patterns,
            "pattern_count": len(patterns),
            "signal": signal["direction"],
            "signal_strength": signal["strength"],
            "summary": self._summarize(patterns),
        }

    def _find_pivots(self, data: np.ndarray, window: int = 5) -> dict:
        """finds local highs and lows in price data"""
        highs = []
        lows = []

        for i in range(window, len(data) - window):
            if all(data[i] >= data[i - j] for j in range(1, window + 1)) and \
               all(data[i] >= data[i + j] for j in range(1, window + 1)):
                highs.append((i, data[i]))

            if all(data[i] <= data[i - j] for j in range(1, window + 1)) and \
               all(data[i] <= data[i + j] for j in range(1, window + 1)):
                lows.append((i, data[i]))

        return {"highs": highs, "lows": lows}

    def _detect_head_shoulders(self, pivots: dict, closes: np.ndarray,
                                is_inverse: bool = False) -> Optional[dict]:
        """detects head and shoulders (or inverse) pattern"""
        peaks = pivots["lows"] if is_inverse else pivots["highs"]
        if len(peaks) < 3:
            return None

        # check last 3 peaks for H&S formation
        for i in range(len(peaks) - 2):
            left_idx, left_val = peaks[i]
            head_idx, head_val = peaks[i + 1]
            right_idx, right_val = peaks[i + 2]

            if is_inverse:
                # inverse: head is lowest, shoulders higher
                if head_val < left_val and head_val < right_val:
                    shoulder_diff = abs(left_val - right_val) / left_val
                    if shoulder_diff < self.tolerance:
                        depth = (left_val - head_val) / left_val
                        if depth > 0.01:  # at least 1% depth
                            neckline = max(left_val, right_val)
                            target = neckline + (neckline - head_val)
                            confidence = min(0.5 + depth * 5 - shoulder_diff * 10, 0.95)
                            return {
                                "name": "inverse_head_shoulders",
                                "direction": "bullish",
                                "confidence": round(max(confidence, 0.3), 4),
                                "neckline": round(neckline, 2),
                                "target": round(target, 2),
                                "start_bar": left_idx,
                                "end_bar": right_idx,
                            }
            else:
                # regular: head is highest, shoulders lower
                if head_val > left_val and head_val > right_val:
                    shoulder_diff = abs(left_val - right_val) / left_val
                    if shoulder_diff < self.tolerance:
                        height = (head_val - left_val) / left_val
                        if height > 0.01:
                            neckline = min(left_val, right_val)
                            target = neckline - (head_val - neckline)
                            confidence = min(0.5 + height * 5 - shoulder_diff * 10, 0.95)
                            return {
                                "name": "head_shoulders",
                                "direction": "bearish",
                                "confidence": round(max(confidence, 0.3), 4),
                                "neckline": round(neckline, 2),
                                "target": round(target, 2),
                                "start_bar": left_idx,
                                "end_bar": right_idx,
                            }

        return None

    def _detect_double_top(self, pivots: dict, closes: np.ndarray,
                           highs: np.ndarray) -> Optional[dict]:
        """detects double top pattern (bearish reversal)"""
        peaks = pivots["highs"]
        if len(peaks) < 2:
            return None

        for i in range(len(peaks) - 1):
            idx1, val1 = peaks[i]
            idx2, val2 = peaks[i + 1]

            price_diff = abs(val1 - val2) / val1
            bar_gap = idx2 - idx1

            if price_diff < self.tolerance and bar_gap >= self.min_pattern_bars:
                # find the valley between the two peaks
                valley = min(closes[idx1:idx2 + 1])
                neckline = valley
                height = ((val1 + val2) / 2) - neckline
                target = neckline - height

                confidence = min(0.5 + (1 - price_diff / self.tolerance) * 0.3, 0.9)
                return {
                    "name": "double_top",
                    "direction": "bearish",
                    "confidence": round(confidence, 4),
                    "neckline": round(neckline, 2),
                    "target": round(target, 2),
                    "start_bar": idx1,
                    "end_bar": idx2,
                }

        return None

    def _detect_double_bottom(self, pivots: dict, closes: np.ndarray,
                              lows: np.ndarray) -> Optional[dict]:
        """detects double bottom pattern (bullish reversal)"""
        troughs = pivots["lows"]
        if len(troughs) < 2:
            return None

        for i in range(len(troughs) - 1):
            idx1, val1 = troughs[i]
            idx2, val2 = troughs[i + 1]

            price_diff = abs(val1 - val2) / val1
            bar_gap = idx2 - idx1

            if price_diff < self.tolerance and bar_gap >= self.min_pattern_bars:
                peak = max(closes[idx1:idx2 + 1])
                neckline = peak
                height = neckline - ((val1 + val2) / 2)
                target = neckline + height

                confidence = min(0.5 + (1 - price_diff / self.tolerance) * 0.3, 0.9)
                return {
                    "name": "double_bottom",
                    "direction": "bullish",
                    "confidence": round(confidence, 4),
                    "neckline": round(neckline, 2),
                    "target": round(target, 2),
                    "start_bar": idx1,
                    "end_bar": idx2,
                }

        return None

    def _detect_triangle(self, pivots: dict, closes: np.ndarray,
                         highs: np.ndarray, lows: np.ndarray) -> Optional[dict]:
        """detects ascending, descending, or symmetrical triangle"""
        peaks = pivots["highs"]
        troughs = pivots["lows"]

        if len(peaks) < 2 or len(troughs) < 2:
            return None

        # use last few pivots
        recent_highs = peaks[-3:] if len(peaks) >= 3 else peaks[-2:]
        recent_lows = troughs[-3:] if len(troughs) >= 3 else troughs[-2:]

        high_vals = [v for _, v in recent_highs]
        low_vals = [v for _, v in recent_lows]

        if len(high_vals) < 2 or len(low_vals) < 2:
            return None

        # calculate slopes
        high_slope = (high_vals[-1] - high_vals[0]) / max(len(high_vals) - 1, 1)
        low_slope = (low_vals[-1] - low_vals[0]) / max(len(low_vals) - 1, 1)

        avg_price = np.mean(closes[-20:])
        high_slope_pct = high_slope / avg_price * 100
        low_slope_pct = low_slope / avg_price * 100

        # classify triangle type
        if abs(high_slope_pct) < 0.5 and low_slope_pct > 0.3:
            tri_type = "ascending_triangle"
            direction = "bullish"
            confidence = 0.55
        elif high_slope_pct < -0.3 and abs(low_slope_pct) < 0.5:
            tri_type = "descending_triangle"
            direction = "bearish"
            confidence = 0.55
        elif high_slope_pct < -0.2 and low_slope_pct > 0.2:
            tri_type = "symmetrical_triangle"
            direction = "neutral"
            confidence = 0.4
        else:
            return None

        # converging range = stronger signal
        range_ratio = abs(high_vals[-1] - low_vals[-1]) / abs(high_vals[0] - low_vals[0]) if abs(high_vals[0] - low_vals[0]) > 0 else 1
        if range_ratio < 0.7:
            confidence += 0.15

        return {
            "name": tri_type,
            "direction": direction,
            "confidence": round(min(confidence, 0.9), 4),
            "start_bar": min(recent_highs[0][0], recent_lows[0][0]),
            "end_bar": max(recent_highs[-1][0], recent_lows[-1][0]),
        }

    def _detect_flag(self, closes: np.ndarray, highs: np.ndarray,
                     lows: np.ndarray) -> Optional[dict]:
        """detects bull/bear flag pattern (continuation)"""
        if len(closes) < 20:
            return None

        # look for a strong move (pole) followed by consolidation (flag)
        pole_len = min(10, len(closes) // 3)
        flag_len = min(10, len(closes) // 3)

        pole_section = closes[-(pole_len + flag_len):-flag_len]
        flag_section = closes[-flag_len:]

        if len(pole_section) < 3 or len(flag_section) < 3:
            return None

        pole_return = (pole_section[-1] - pole_section[0]) / pole_section[0] * 100
        flag_range = (max(flag_section) - min(flag_section)) / np.mean(flag_section) * 100
        flag_return = (flag_section[-1] - flag_section[0]) / flag_section[0] * 100

        # bull flag: strong up pole, slight down/sideways consolidation
        if pole_return > 3.0 and flag_range < 3.0 and flag_return < 1.0:
            confidence = min(0.4 + abs(pole_return) * 0.03, 0.85)
            return {
                "name": "bull_flag",
                "direction": "bullish",
                "confidence": round(confidence, 4),
                "pole_return": round(pole_return, 2),
                "start_bar": len(closes) - pole_len - flag_len,
                "end_bar": len(closes) - 1,
            }

        # bear flag: strong down pole, slight up/sideways consolidation
        if pole_return < -3.0 and flag_range < 3.0 and flag_return > -1.0:
            confidence = min(0.4 + abs(pole_return) * 0.03, 0.85)
            return {
                "name": "bear_flag",
                "direction": "bearish",
                "confidence": round(confidence, 4),
                "pole_return": round(pole_return, 2),
                "start_bar": len(closes) - pole_len - flag_len,
                "end_bar": len(closes) - 1,
            }

        return None

    def _detect_wedge(self, pivots: dict, closes: np.ndarray,
                      highs: np.ndarray, lows: np.ndarray) -> Optional[dict]:
        """detects rising or falling wedge pattern"""
        peaks = pivots["highs"]
        troughs = pivots["lows"]

        if len(peaks) < 2 or len(troughs) < 2:
            return None

        recent_highs = peaks[-3:] if len(peaks) >= 3 else peaks[-2:]
        recent_lows = troughs[-3:] if len(troughs) >= 3 else troughs[-2:]

        high_vals = [v for _, v in recent_highs]
        low_vals = [v for _, v in recent_lows]

        if len(high_vals) < 2 or len(low_vals) < 2:
            return None

        high_slope = (high_vals[-1] - high_vals[0]) / max(len(high_vals) - 1, 1)
        low_slope = (low_vals[-1] - low_vals[0]) / max(len(low_vals) - 1, 1)

        avg_price = np.mean(closes[-20:])

        # rising wedge: both slopes up, but lower slope steeper (converging up)
        if high_slope > 0 and low_slope > 0 and low_slope > high_slope * 0.5:
            slope_diff = (low_slope - high_slope) / avg_price * 100
            if slope_diff > 0.1:
                return {
                    "name": "rising_wedge",
                    "direction": "bearish",
                    "confidence": round(min(0.45 + slope_diff * 0.5, 0.85), 4),
                    "start_bar": min(recent_highs[0][0], recent_lows[0][0]),
                    "end_bar": max(recent_highs[-1][0], recent_lows[-1][0]),
                }

        # falling wedge: both slopes down, but upper slope steeper (converging down)
        if high_slope < 0 and low_slope < 0 and abs(high_slope) > abs(low_slope) * 0.5:
            slope_diff = (abs(high_slope) - abs(low_slope)) / avg_price * 100
            if slope_diff > 0.1:
                return {
                    "name": "falling_wedge",
                    "direction": "bullish",
                    "confidence": round(min(0.45 + slope_diff * 0.5, 0.85), 4),
                    "start_bar": min(recent_highs[0][0], recent_lows[0][0]),
                    "end_bar": max(recent_highs[-1][0], recent_lows[-1][0]),
                }

        return None

    def _derive_signal(self, patterns: list[dict]) -> dict:
        """derives overall trading signal from detected patterns"""
        if not patterns:
            return {"direction": "neutral", "strength": 0.0}

        bullish_score = 0.0
        bearish_score = 0.0

        for p in patterns:
            conf = p["confidence"]
            if p["direction"] == "bullish":
                bullish_score += conf
            elif p["direction"] == "bearish":
                bearish_score += conf

        total = bullish_score + bearish_score
        if total == 0:
            return {"direction": "neutral", "strength": 0.0}

        if bullish_score > bearish_score:
            return {
                "direction": "bullish",
                "strength": round((bullish_score - bearish_score) / total, 4),
            }
        elif bearish_score > bullish_score:
            return {
                "direction": "bearish",
                "strength": round((bearish_score - bullish_score) / total, 4),
            }
        return {"direction": "neutral", "strength": 0.0}

    def _summarize(self, patterns: list[dict]) -> str:
        """creates a human-readable summary of detected patterns"""
        if not patterns:
            return "no patterns detected"

        names = [p["name"].replace("_", " ") for p in patterns]
        return f"detected {len(patterns)} pattern(s): {', '.join(names)}"
