// exponential moving average calculation
// used as a building block by rsi, macd, and bollinger bands

/// calculates ema for a slice of values with the given period.
/// returns the full ema series (length = values.len()).
/// the first `period - 1` values use a simple moving average seed.
pub fn calculate(values: &[f64], period: usize) -> Vec<f64> {
    if values.is_empty() || period == 0 {
        return vec![];
    }
    if period > values.len() {
        return vec![];
    }

    let mut result = Vec::with_capacity(values.len());
    let multiplier = 2.0 / (period as f64 + 1.0);

    // seed with sma of first `period` values
    let sma: f64 = values[..period].iter().sum::<f64>() / period as f64;

    // fill leading values with 0.0 (not enough data)
    for _ in 0..period - 1 {
        result.push(0.0);
    }
    result.push(sma);

    // compute ema from period onwards
    for i in period..values.len() {
        let prev = result[i - 1];
        let ema = (values[i] - prev) * multiplier + prev;
        result.push(ema);
    }

    result
}

/// returns just the latest ema value
pub fn latest(values: &[f64], period: usize) -> Option<f64> {
    let series = calculate(values, period);
    series.last().copied()
}

/// determines trend: whether the latest price is above or below the ema
pub fn trend(price: f64, ema_value: f64) -> &'static str {
    if price > ema_value {
        "ABOVE"
    } else {
        "BELOW"
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_ema_basic() {
        let values = vec![10.0, 11.0, 12.0, 13.0, 14.0, 15.0, 16.0, 17.0, 18.0, 19.0];
        let result = calculate(&values, 3);
        assert_eq!(result.len(), 10);
        // sma of first 3: (10+11+12)/3 = 11.0
        assert!((result[2] - 11.0).abs() < 1e-10);
        // ema[3] = (13 - 11) * 0.5 + 11 = 12.0
        assert!((result[3] - 12.0).abs() < 1e-10);
    }

    #[test]
    fn test_ema_period_5() {
        let values: Vec<f64> = (1..=20).map(|x| x as f64).collect();
        let result = calculate(&values, 5);
        assert_eq!(result.len(), 20);
        // sma of first 5: (1+2+3+4+5)/5 = 3.0
        assert!((result[4] - 3.0).abs() < 1e-10);
        // each subsequent value should increase since input is rising
        for i in 5..20 {
            assert!(result[i] > result[i - 1], "ema should rise with rising input");
        }
    }

    #[test]
    fn test_ema_constant_input() {
        let values = vec![50.0; 20];
        let result = calculate(&values, 10);
        // ema of constant values should be that constant
        for i in 9..20 {
            assert!((result[i] - 50.0).abs() < 1e-10);
        }
    }

    #[test]
    fn test_ema_empty() {
        assert!(calculate(&[], 5).is_empty());
    }

    #[test]
    fn test_ema_period_too_large() {
        let values = vec![1.0, 2.0, 3.0];
        assert!(calculate(&values, 5).is_empty());
    }

    #[test]
    fn test_ema_period_zero() {
        let values = vec![1.0, 2.0, 3.0];
        assert!(calculate(&values, 0).is_empty());
    }

    #[test]
    fn test_ema_latest() {
        let values = vec![10.0, 11.0, 12.0, 13.0, 14.0];
        let val = latest(&values, 3).unwrap();
        let series = calculate(&values, 3);
        assert!((val - series[4]).abs() < 1e-10);
    }

    #[test]
    fn test_ema_known_values() {
        // known ema(10) for sequential data
        let values: Vec<f64> = vec![
            22.27, 22.19, 22.08, 22.17, 22.18,
            22.13, 22.23, 22.43, 22.24, 22.29,
            22.15, 22.39, 22.38, 22.61, 22.45,
        ];
        let result = calculate(&values, 10);
        // sma of first 10: sum / 10
        let sma: f64 = values[..10].iter().sum::<f64>() / 10.0;
        assert!((result[9] - sma).abs() < 1e-10);
        // ema should follow the trend
        assert!(result[14] > result[9], "ema should rise with generally rising prices");
    }

    #[test]
    fn test_trend_above() {
        assert_eq!(trend(100.0, 95.0), "ABOVE");
    }

    #[test]
    fn test_trend_below() {
        assert_eq!(trend(90.0, 95.0), "BELOW");
    }

    #[test]
    fn test_trend_equal() {
        assert_eq!(trend(95.0, 95.0), "BELOW");
    }

    #[test]
    fn test_ema_single_period() {
        let values = vec![5.0, 10.0, 15.0, 20.0];
        let result = calculate(&values, 1);
        // ema(1) = price itself
        for (i, v) in values.iter().enumerate() {
            assert!((result[i] - v).abs() < 1e-10);
        }
    }

    #[test]
    fn test_ema_period_equals_length() {
        let values = vec![1.0, 2.0, 3.0, 4.0, 5.0];
        let result = calculate(&values, 5);
        assert_eq!(result.len(), 5);
        assert!((result[4] - 3.0).abs() < 1e-10); // sma = 15/5 = 3
    }
}
