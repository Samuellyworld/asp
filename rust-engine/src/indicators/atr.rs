// average true range calculation
// measures market volatility using high-low-close data

/// atr result with the current value and full series
pub struct ATRResult {
    pub value: f64,
    pub series: Vec<f64>,
    pub signal: String,      // HIGH, NORMAL, LOW
    pub percent: f64,        // atr as percentage of current close
}

/// calculates average true range from ohlc data.
/// true range = max(high-low, |high-prev_close|, |low-prev_close|)
pub fn calculate(highs: &[f64], lows: &[f64], closes: &[f64], period: usize) -> Option<ATRResult> {
    let len = highs.len();
    if len < period + 1 || len != lows.len() || len != closes.len() || period == 0 {
        return None;
    }

    // compute true range series
    let mut tr = Vec::with_capacity(len - 1);
    for i in 1..len {
        let hl = highs[i] - lows[i];
        let hc = (highs[i] - closes[i - 1]).abs();
        let lc = (lows[i] - closes[i - 1]).abs();
        tr.push(hl.max(hc).max(lc));
    }

    // first atr = simple average of first `period` true ranges
    let first_atr: f64 = tr[..period].iter().sum::<f64>() / period as f64;

    let mut series = Vec::with_capacity(tr.len());
    // fill leading positions
    for _ in 0..period - 1 {
        series.push(0.0);
    }
    series.push(first_atr);

    // wilder's smoothed average (same as rsi)
    let mut atr = first_atr;
    for i in period..tr.len() {
        atr = (atr * (period as f64 - 1.0) + tr[i]) / period as f64;
        series.push(atr);
    }

    let value = *series.last().unwrap_or(&0.0);
    let last_close = *closes.last().unwrap_or(&1.0);
    let percent = if last_close.abs() > 1e-10 { value / last_close * 100.0 } else { 0.0 };
    let signal = classify_volatility(percent);

    Some(ATRResult { value, series, signal, percent })
}

/// classifies volatility level from atr percent
pub fn classify_volatility(atr_pct: f64) -> String {
    if atr_pct >= 3.0 {
        "HIGH".to_string()
    } else if atr_pct <= 1.0 {
        "LOW".to_string()
    } else {
        "NORMAL".to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_trending_data(n: usize) -> (Vec<f64>, Vec<f64>, Vec<f64>) {
        let mut highs = Vec::with_capacity(n);
        let mut lows = Vec::with_capacity(n);
        let mut closes = Vec::with_capacity(n);
        for i in 0..n {
            let base = 100.0 + i as f64;
            highs.push(base + 1.5);
            lows.push(base - 1.5);
            closes.push(base);
        }
        (highs, lows, closes)
    }

    fn make_volatile_data(n: usize) -> (Vec<f64>, Vec<f64>, Vec<f64>) {
        let mut highs = Vec::with_capacity(n);
        let mut lows = Vec::with_capacity(n);
        let mut closes = Vec::with_capacity(n);
        for i in 0..n {
            let base = 100.0 + (i as f64 * 0.5).sin() * 10.0;
            highs.push(base + 5.0);
            lows.push(base - 5.0);
            closes.push(base);
        }
        (highs, lows, closes)
    }

    #[test]
    fn test_atr_basic() {
        let (h, l, c) = make_trending_data(30);
        let result = calculate(&h, &l, &c, 14).unwrap();
        assert!(result.value > 0.0, "atr should be positive, got {}", result.value);
        assert!(result.percent > 0.0);
    }

    #[test]
    fn test_atr_series_length() {
        let (h, l, c) = make_trending_data(30);
        let result = calculate(&h, &l, &c, 14).unwrap();
        // series length = len-1 (true range has one fewer element than input)
        assert_eq!(result.series.len(), 29);
    }

    #[test]
    fn test_atr_volatile_higher() {
        let (h1, l1, c1) = make_trending_data(30);
        let (h2, l2, c2) = make_volatile_data(30);
        let calm = calculate(&h1, &l1, &c1, 14).unwrap();
        let volatile = calculate(&h2, &l2, &c2, 14).unwrap();
        assert!(volatile.value > calm.value,
            "volatile data should have higher atr: {} vs {}", volatile.value, calm.value);
    }

    #[test]
    fn test_atr_constant_range() {
        // constant range = constant atr
        let n = 30;
        let highs = vec![102.0; n];
        let lows = vec![98.0; n];
        let closes = vec![100.0; n];
        let result = calculate(&highs, &lows, &closes, 14).unwrap();
        // true range should be 4.0 for all bars (high - low = 4, no gaps)
        assert!((result.value - 4.0).abs() < 0.1,
            "constant 4-point range should give atr near 4.0, got {}", result.value);
    }

    #[test]
    fn test_atr_insufficient_data() {
        let h = vec![101.0; 10];
        let l = vec![99.0; 10];
        let c = vec![100.0; 10];
        assert!(calculate(&h, &l, &c, 14).is_none());
    }

    #[test]
    fn test_atr_period_zero() {
        let (h, l, c) = make_trending_data(30);
        assert!(calculate(&h, &l, &c, 0).is_none());
    }

    #[test]
    fn test_atr_mismatched_lengths() {
        let h = vec![102.0; 30];
        let l = vec![98.0; 25]; // different length
        let c = vec![100.0; 30];
        assert!(calculate(&h, &l, &c, 14).is_none());
    }

    #[test]
    fn test_classify_high() {
        assert_eq!(classify_volatility(4.0), "HIGH");
        assert_eq!(classify_volatility(3.0), "HIGH");
    }

    #[test]
    fn test_classify_low() {
        assert_eq!(classify_volatility(0.5), "LOW");
        assert_eq!(classify_volatility(1.0), "LOW");
    }

    #[test]
    fn test_classify_normal() {
        assert_eq!(classify_volatility(2.0), "NORMAL");
        assert_eq!(classify_volatility(1.5), "NORMAL");
    }

    #[test]
    fn test_atr_leading_zeros() {
        let (h, l, c) = make_trending_data(30);
        let result = calculate(&h, &l, &c, 14).unwrap();
        for i in 0..13 {
            assert!((result.series[i]).abs() < 1e-10, "index {} should be 0", i);
        }
        assert!(result.series[13] > 0.0, "index 13 should be non-zero");
    }

    #[test]
    fn test_atr_gap_up_contributes() {
        // gap up: prev close 100, current high 110, low 108
        // true range = max(110-108, |110-100|, |108-100|) = 10
        let highs = vec![101.0; 15];
        let lows = vec![99.0; 15];
        let mut closes = vec![100.0; 15];
        // create a gap after the first 14 bars
        let mut h = highs; h.push(110.0);
        let mut l = lows; l.push(108.0);
        closes.push(109.0);
        let result = calculate(&h, &l, &closes, 14).unwrap();
        // last atr should be higher due to the gap
        let second_to_last = result.series[result.series.len() - 2];
        let last = result.value;
        assert!(last > second_to_last, "gap should increase atr");
    }
}
