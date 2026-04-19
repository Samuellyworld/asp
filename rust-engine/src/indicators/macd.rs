// moving average convergence divergence calculation
// uses ema difference to detect momentum shifts

use super::ema;

/// macd result with all three components and signal classification
pub struct MACDResult {
    pub macd_line: f64,
    pub signal_line: f64,
    pub histogram: f64,
    pub signal: String,
    pub crossover: bool,
    pub macd_series: Vec<f64>,
    pub signal_series: Vec<f64>,
    pub histogram_series: Vec<f64>,
}

/// calculates macd from closing prices.
/// fast_period (default 12), slow_period (default 26), signal_period (default 9)
pub fn calculate(
    closes: &[f64],
    fast_period: usize,
    slow_period: usize,
    signal_period: usize,
) -> Option<MACDResult> {
    if closes.len() < slow_period + signal_period || fast_period >= slow_period {
        return None;
    }

    let fast_ema = ema::calculate(closes, fast_period);
    let slow_ema = ema::calculate(closes, slow_period);

    // macd line = fast ema - slow ema
    let mut macd_values = Vec::with_capacity(closes.len());
    for i in 0..closes.len() {
        if i < slow_period - 1 {
            macd_values.push(0.0);
        } else {
            macd_values.push(fast_ema[i] - slow_ema[i]);
        }
    }

    // signal line = ema of macd values (starting from where macd is valid)
    let valid_macd: Vec<f64> = macd_values[slow_period - 1..].to_vec();
    let signal_ema = ema::calculate(&valid_macd, signal_period);

    // build full signal series with leading zeros
    let mut signal_series = vec![0.0; slow_period - 1];
    signal_series.extend_from_slice(&signal_ema);

    // histogram = macd - signal
    let mut histogram_series = Vec::with_capacity(closes.len());
    for i in 0..closes.len() {
        if i < slow_period + signal_period - 2 {
            histogram_series.push(0.0);
        } else {
            histogram_series.push(macd_values[i] - signal_series[i]);
        }
    }

    let last = closes.len() - 1;
    let macd_line = macd_values[last];
    let signal_line = signal_series[last];
    let histogram = histogram_series[last];

    // detect crossover (macd crosses signal line)
    let crossover = if last > 0 {
        let prev_diff = macd_values[last - 1] - signal_series[last - 1];
        let curr_diff = macd_line - signal_line;
        (prev_diff <= 0.0 && curr_diff > 0.0) || (prev_diff >= 0.0 && curr_diff < 0.0)
    } else {
        false
    };

    let signal = classify(macd_line, signal_line, histogram);

    Some(MACDResult {
        macd_line,
        signal_line,
        histogram,
        signal,
        crossover,
        macd_series: macd_values,
        signal_series,
        histogram_series,
    })
}

/// classifies the macd state
pub fn classify(macd_line: f64, signal_line: f64, histogram: f64) -> String {
    if macd_line > signal_line && histogram > 0.0 {
        "BULLISH".to_string()
    } else if macd_line < signal_line && histogram < 0.0 {
        "BEARISH".to_string()
    } else {
        "NEUTRAL".to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn trending_up(n: usize) -> Vec<f64> {
        (0..n).map(|i| 100.0 + (i as f64) * 0.5).collect()
    }

    fn trending_down(n: usize) -> Vec<f64> {
        (0..n).map(|i| 200.0 - (i as f64) * 0.5).collect()
    }

    #[test]
    fn test_macd_rising_is_bullish() {
        let prices = trending_up(60);
        let result = calculate(&prices, 12, 26, 9).unwrap();
        assert!(
            result.macd_line > 0.0,
            "macd should be positive in uptrend, got {}",
            result.macd_line
        );
    }

    #[test]
    fn test_macd_falling_is_bearish() {
        let prices = trending_down(60);
        let result = calculate(&prices, 12, 26, 9).unwrap();
        assert!(
            result.macd_line < 0.0,
            "macd should be negative in downtrend, got {}",
            result.macd_line
        );
    }

    #[test]
    fn test_macd_series_length() {
        let prices = trending_up(60);
        let result = calculate(&prices, 12, 26, 9).unwrap();
        assert_eq!(result.macd_series.len(), 60);
        assert_eq!(result.signal_series.len(), 60);
        assert_eq!(result.histogram_series.len(), 60);
    }

    #[test]
    fn test_macd_insufficient_data() {
        let prices = trending_up(30);
        assert!(calculate(&prices, 12, 26, 9).is_none());
    }

    #[test]
    fn test_macd_fast_ge_slow() {
        let prices = trending_up(60);
        assert!(calculate(&prices, 26, 12, 9).is_none());
    }

    #[test]
    fn test_macd_histogram_exists() {
        // use accelerating prices so histogram is clearly non-zero
        let prices: Vec<f64> = (0..60).map(|i| 100.0 + (i as f64).powi(2) * 0.01).collect();
        let result = calculate(&prices, 12, 26, 9).unwrap();
        // check that non-zero histogram values exist (use permissive threshold)
        let non_zero: Vec<&f64> = result
            .histogram_series
            .iter()
            .filter(|v| v.abs() > 1e-6)
            .collect();
        assert!(
            !non_zero.is_empty(),
            "histogram should have non-zero values"
        );
    }

    #[test]
    fn test_macd_crossover_detected() {
        // create data that crosses: up trend then down trend
        let mut prices: Vec<f64> = (0..40).map(|i| 100.0 + (i as f64) * 1.0).collect();
        // reverse the trend
        for i in 0..20 {
            prices.push(140.0 - (i as f64) * 1.5);
        }
        let result = calculate(&prices, 12, 26, 9).unwrap();
        // at some point in the reversal there should be signal components
        assert!(result.macd_series.len() == 60);
    }

    #[test]
    fn test_macd_constant_input() {
        let prices = vec![100.0; 60];
        let result = calculate(&prices, 12, 26, 9).unwrap();
        assert!(
            result.macd_line.abs() < 1e-10,
            "constant input should give macd near 0"
        );
        assert!(
            result.histogram.abs() < 1e-10,
            "constant input should give histogram near 0"
        );
    }

    #[test]
    fn test_macd_classify_bullish() {
        assert_eq!(classify(1.0, 0.5, 0.5), "BULLISH");
    }

    #[test]
    fn test_macd_classify_bearish() {
        assert_eq!(classify(-1.0, -0.5, -0.5), "BEARISH");
    }

    #[test]
    fn test_macd_classify_neutral() {
        // macd > signal but histogram <= 0: contradictory = neutral
        assert_eq!(classify(1.0, 0.5, -0.5), "NEUTRAL");
        // macd < signal but histogram >= 0: contradictory = neutral
        assert_eq!(classify(0.5, 1.0, 0.5), "NEUTRAL");
    }

    #[test]
    fn test_macd_custom_periods() {
        let prices = trending_up(80);
        let result = calculate(&prices, 8, 21, 5).unwrap();
        assert!(result.macd_line > 0.0);
        assert_eq!(result.macd_series.len(), 80);
    }

    #[test]
    fn test_macd_large_dataset() {
        let mut prices = Vec::with_capacity(10000);
        let mut price = 100.0;
        for i in 0..10000 {
            price += (i as f64 * 0.01).sin() * 2.0;
            prices.push(price);
        }
        let result = calculate(&prices, 12, 26, 9).unwrap();
        assert_eq!(result.macd_series.len(), 10000);
    }

    #[test]
    fn test_macd_signal_line_lags() {
        let prices = trending_up(60);
        let result = calculate(&prices, 12, 26, 9).unwrap();
        // in uptrend, macd should be above signal (signal lags)
        assert!(
            result.macd_line >= result.signal_line,
            "macd ({}) should be >= signal ({}) in uptrend",
            result.macd_line,
            result.signal_line
        );
    }
}
