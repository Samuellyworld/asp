// stochastic oscillator calculation
// measures price momentum relative to high-low range over a lookback period

/// stochastic result with %K, %D, and signal
pub struct StochasticResult {
    pub k: f64,              // %K (fast stochastic) 0-100
    pub d: f64,              // %D (signal line = SMA of %K) 0-100
    pub signal: String,      // OVERSOLD, NEUTRAL, OVERBOUGHT, BULLISH_CROSS, BEARISH_CROSS
    pub k_series: Vec<f64>,
    pub d_series: Vec<f64>,
}

/// calculates stochastic oscillator
/// %K = (close - lowest_low) / (highest_high - lowest_low) * 100
/// %D = SMA(%K, d_period)
pub fn calculate(
    highs: &[f64],
    lows: &[f64],
    closes: &[f64],
    k_period: usize,
    d_period: usize,
    smooth: usize, // smoothing period for %K (typically 3)
) -> Option<StochasticResult> {
    let len = highs.len();
    if len < k_period + d_period + smooth
        || len != lows.len()
        || len != closes.len()
        || k_period == 0
        || d_period == 0
        || smooth == 0
    {
        return None;
    }

    // step 1: compute raw %K
    let mut raw_k = Vec::with_capacity(len);
    for i in 0..len {
        if i < k_period - 1 {
            raw_k.push(0.0);
            continue;
        }
        let window_start = i + 1 - k_period;
        let highest = highs[window_start..=i]
            .iter()
            .cloned()
            .fold(f64::NEG_INFINITY, f64::max);
        let lowest = lows[window_start..=i]
            .iter()
            .cloned()
            .fold(f64::INFINITY, f64::min);
        let range = highest - lowest;
        let k = if range > 1e-10 {
            (closes[i] - lowest) / range * 100.0
        } else {
            50.0 // flat market
        };
        raw_k.push(k);
    }

    // step 2: smooth %K with SMA
    let mut k_series = Vec::with_capacity(len);
    for i in 0..len {
        if i < k_period - 1 + smooth - 1 {
            k_series.push(0.0);
            continue;
        }
        let start = i + 1 - smooth;
        let sum: f64 = raw_k[start..=i].iter().sum();
        k_series.push(sum / smooth as f64);
    }

    // step 3: compute %D = SMA of smoothed %K
    let valid_start = k_period - 1 + smooth - 1;
    let mut d_series = Vec::with_capacity(len);
    for i in 0..len {
        if i < valid_start + d_period - 1 {
            d_series.push(0.0);
            continue;
        }
        let start = i + 1 - d_period;
        let sum: f64 = k_series[start..=i].iter().sum();
        d_series.push(sum / d_period as f64);
    }

    let k = *k_series.last().unwrap_or(&50.0);
    let d = *d_series.last().unwrap_or(&50.0);

    // detect crossovers
    let signal = if len >= 2 {
        let prev_k = k_series[len - 2];
        let prev_d = d_series[len - 2];
        classify(k, d, prev_k, prev_d)
    } else {
        classify_level(k)
    };

    Some(StochasticResult {
        k,
        d,
        signal,
        k_series,
        d_series,
    })
}

/// classifies stochastic with crossover detection
fn classify(k: f64, d: f64, prev_k: f64, prev_d: f64) -> String {
    // bullish cross: %K crosses above %D in oversold region
    if prev_k <= prev_d && k > d && k < 30.0 {
        return "BULLISH_CROSS".to_string();
    }
    // bearish cross: %K crosses below %D in overbought region
    if prev_k >= prev_d && k < d && k > 70.0 {
        return "BEARISH_CROSS".to_string();
    }
    classify_level(k)
}

/// classifies based on %K level alone
fn classify_level(k: f64) -> String {
    if k <= 20.0 {
        "OVERSOLD".to_string()
    } else if k >= 80.0 {
        "OVERBOUGHT".to_string()
    } else {
        "NEUTRAL".to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_uptrend(n: usize) -> (Vec<f64>, Vec<f64>, Vec<f64>) {
        let mut h = Vec::with_capacity(n);
        let mut l = Vec::with_capacity(n);
        let mut c = Vec::with_capacity(n);
        for i in 0..n {
            let base = 100.0 + i as f64 * 1.0;
            h.push(base + 1.0);
            l.push(base - 1.0);
            c.push(base + 0.5); // close near high
        }
        (h, l, c)
    }

    fn make_downtrend(n: usize) -> (Vec<f64>, Vec<f64>, Vec<f64>) {
        let mut h = Vec::with_capacity(n);
        let mut l = Vec::with_capacity(n);
        let mut c = Vec::with_capacity(n);
        for i in 0..n {
            let base = 200.0 - i as f64 * 1.0;
            h.push(base + 1.0);
            l.push(base - 1.0);
            c.push(base - 0.5); // close near low
        }
        (h, l, c)
    }

    fn make_oscillating(n: usize) -> (Vec<f64>, Vec<f64>, Vec<f64>) {
        let mut h = Vec::with_capacity(n);
        let mut l = Vec::with_capacity(n);
        let mut c = Vec::with_capacity(n);
        for i in 0..n {
            let base = 100.0 + (i as f64 * 0.5).sin() * 5.0;
            h.push(base + 1.0);
            l.push(base - 1.0);
            c.push(base);
        }
        (h, l, c)
    }

    #[test]
    fn test_stoch_uptrend_high() {
        let (h, l, c) = make_uptrend(40);
        let result = calculate(&h, &l, &c, 14, 3, 3).unwrap();
        assert!(result.k > 60.0,
            "uptrend should give high %K, got {}", result.k);
    }

    #[test]
    fn test_stoch_downtrend_low() {
        let (h, l, c) = make_downtrend(40);
        let result = calculate(&h, &l, &c, 14, 3, 3).unwrap();
        assert!(result.k < 40.0,
            "downtrend should give low %K, got {}", result.k);
    }

    #[test]
    fn test_stoch_range() {
        let (h, l, c) = make_oscillating(40);
        let result = calculate(&h, &l, &c, 14, 3, 3).unwrap();
        assert!(result.k >= 0.0 && result.k <= 100.0,
            "%K out of range: {}", result.k);
        assert!(result.d >= 0.0 && result.d <= 100.0,
            "%D out of range: {}", result.d);
    }

    #[test]
    fn test_stoch_series_length() {
        let (h, l, c) = make_uptrend(40);
        let result = calculate(&h, &l, &c, 14, 3, 3).unwrap();
        assert_eq!(result.k_series.len(), 40);
        assert_eq!(result.d_series.len(), 40);
    }

    #[test]
    fn test_stoch_insufficient_data() {
        let h = vec![101.0; 10];
        let l = vec![99.0; 10];
        let c = vec![100.0; 10];
        assert!(calculate(&h, &l, &c, 14, 3, 3).is_none());
    }

    #[test]
    fn test_stoch_period_zero() {
        let (h, l, c) = make_uptrend(40);
        assert!(calculate(&h, &l, &c, 0, 3, 3).is_none());
    }

    #[test]
    fn test_stoch_classify_oversold() {
        assert_eq!(classify_level(15.0), "OVERSOLD");
        assert_eq!(classify_level(20.0), "OVERSOLD");
    }

    #[test]
    fn test_stoch_classify_overbought() {
        assert_eq!(classify_level(85.0), "OVERBOUGHT");
        assert_eq!(classify_level(80.0), "OVERBOUGHT");
    }

    #[test]
    fn test_stoch_classify_neutral() {
        assert_eq!(classify_level(50.0), "NEUTRAL");
    }

    #[test]
    fn test_stoch_bullish_cross() {
        // prev: K below D in oversold, now: K above D still in oversold
        assert_eq!(classify(25.0, 22.0, 18.0, 20.0), "BULLISH_CROSS");
    }

    #[test]
    fn test_stoch_bearish_cross() {
        // prev: K above D in overbought, now: K below D still in overbought
        assert_eq!(classify(78.0, 82.0, 85.0, 83.0), "BEARISH_CROSS");
    }

    #[test]
    fn test_stoch_d_lags_k() {
        let (h, l, c) = make_uptrend(40);
        let result = calculate(&h, &l, &c, 14, 3, 3).unwrap();
        // in a strong uptrend, %K should generally lead %D
        // (both should be high, but K should be >= D)
        assert!(result.k >= result.d - 5.0,
            "in uptrend K ({}) should be near or above D ({})", result.k, result.d);
    }

    #[test]
    fn test_stoch_constant_price() {
        let h = vec![101.0; 40];
        let l = vec![99.0; 40];
        let c = vec![100.0; 40];
        let result = calculate(&h, &l, &c, 14, 3, 3).unwrap();
        // close is at midpoint of range -> %K should be around 50
        assert!((result.k - 50.0).abs() < 5.0,
            "constant price at midpoint should give %K near 50, got {}", result.k);
    }
}
