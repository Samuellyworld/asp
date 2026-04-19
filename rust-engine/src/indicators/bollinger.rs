// bollinger bands calculation
// uses sma and standard deviation to create price channels

/// bollinger bands result with upper, middle, lower bands and signals
pub struct BollingerResult {
    pub upper: f64,
    pub middle: f64,
    pub lower: f64,
    pub bandwidth: f64,
    pub percent_b: f64,
    pub signal: String,
    pub upper_series: Vec<f64>,
    pub middle_series: Vec<f64>,
    pub lower_series: Vec<f64>,
}

/// calculates bollinger bands from closing prices.
/// period (default 20), num_std_dev (default 2.0)
pub fn calculate(closes: &[f64], period: usize, num_std_dev: f64) -> Option<BollingerResult> {
    if period == 0 || closes.len() < period {
        return None;
    }

    let len = closes.len();
    let mut upper_series = Vec::with_capacity(len);
    let mut middle_series = Vec::with_capacity(len);
    let mut lower_series = Vec::with_capacity(len);

    for i in 0..len {
        if i < period - 1 {
            upper_series.push(0.0);
            middle_series.push(0.0);
            lower_series.push(0.0);
        } else {
            let window = &closes[i + 1 - period..=i];
            let sma = window.iter().sum::<f64>() / period as f64;
            let variance = window.iter().map(|v| (v - sma).powi(2)).sum::<f64>() / period as f64;
            let std_dev = variance.sqrt();

            upper_series.push(sma + num_std_dev * std_dev);
            middle_series.push(sma);
            lower_series.push(sma - num_std_dev * std_dev);
        }
    }

    let last = len - 1;
    let upper = upper_series[last];
    let middle = middle_series[last];
    let lower = lower_series[last];

    let bandwidth = if middle.abs() > 1e-10 {
        (upper - lower) / middle
    } else {
        0.0
    };

    let percent_b = if (upper - lower).abs() > 1e-10 {
        (closes[last] - lower) / (upper - lower)
    } else {
        0.5
    };

    let signal = classify(closes[last], upper, lower, bandwidth);

    Some(BollingerResult {
        upper,
        middle,
        lower,
        bandwidth,
        percent_b,
        signal,
        upper_series,
        middle_series,
        lower_series,
    })
}

/// classifies price position relative to bands
pub fn classify(price: f64, upper: f64, lower: f64, bandwidth: f64) -> String {
    if bandwidth < 0.02 {
        "SQUEEZE".to_string()
    } else if price >= upper {
        "UPPER_BAND".to_string()
    } else if price <= lower {
        "LOWER_BAND".to_string()
    } else {
        "MIDDLE".to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn trending_up(n: usize) -> Vec<f64> {
        (0..n).map(|i| 100.0 + (i as f64) * 0.5).collect()
    }

    #[test]
    fn test_bollinger_basic() {
        let prices = trending_up(30);
        let result = calculate(&prices, 20, 2.0).unwrap();
        assert!(result.upper > result.middle);
        assert!(result.middle > result.lower);
    }

    #[test]
    fn test_bollinger_constant_input() {
        let prices = vec![100.0; 30];
        let result = calculate(&prices, 20, 2.0).unwrap();
        assert!((result.middle - 100.0).abs() < 1e-10);
        assert!((result.upper - 100.0).abs() < 1e-10);
        assert!((result.lower - 100.0).abs() < 1e-10);
        assert!(result.bandwidth < 1e-10);
    }

    #[test]
    fn test_bollinger_series_length() {
        let prices = trending_up(30);
        let result = calculate(&prices, 20, 2.0).unwrap();
        assert_eq!(result.upper_series.len(), 30);
        assert_eq!(result.middle_series.len(), 30);
        assert_eq!(result.lower_series.len(), 30);
    }

    #[test]
    fn test_bollinger_insufficient_data() {
        let prices = trending_up(10);
        assert!(calculate(&prices, 20, 2.0).is_none());
    }

    #[test]
    fn test_bollinger_period_zero() {
        let prices = trending_up(30);
        assert!(calculate(&prices, 0, 2.0).is_none());
    }

    #[test]
    fn test_bollinger_bandwidth_positive() {
        let prices: Vec<f64> = (0..40)
            .map(|i| 100.0 + (i as f64 * 0.3).sin() * 10.0)
            .collect();
        let result = calculate(&prices, 20, 2.0).unwrap();
        assert!(result.bandwidth >= 0.0, "bandwidth should be non-negative");
    }

    #[test]
    fn test_bollinger_percent_b_range() {
        let prices = trending_up(30);
        let result = calculate(&prices, 20, 2.0).unwrap();
        // percent_b can be outside 0-1 when price is outside bands
        // but in a smooth uptrend it should be defined
        assert!(result.percent_b.is_finite());
    }

    #[test]
    fn test_bollinger_wider_std_dev() {
        let prices = trending_up(30);
        let narrow = calculate(&prices, 20, 1.0).unwrap();
        let wide = calculate(&prices, 20, 3.0).unwrap();
        assert!(
            wide.upper > narrow.upper,
            "wider std_dev should give wider bands"
        );
        assert!(
            wide.lower < narrow.lower,
            "wider std_dev should give wider bands"
        );
    }

    #[test]
    fn test_bollinger_classify_upper() {
        assert_eq!(classify(110.0, 105.0, 95.0, 0.1), "UPPER_BAND");
    }

    #[test]
    fn test_bollinger_classify_lower() {
        assert_eq!(classify(90.0, 105.0, 95.0, 0.1), "LOWER_BAND");
    }

    #[test]
    fn test_bollinger_classify_middle() {
        assert_eq!(classify(100.0, 105.0, 95.0, 0.1), "MIDDLE");
    }

    #[test]
    fn test_bollinger_classify_squeeze() {
        assert_eq!(classify(100.0, 100.5, 99.5, 0.01), "SQUEEZE");
    }

    #[test]
    fn test_bollinger_period_equals_length() {
        let prices = vec![10.0, 20.0, 30.0, 40.0, 50.0];
        let result = calculate(&prices, 5, 2.0).unwrap();
        assert!((result.middle - 30.0).abs() < 1e-10);
    }

    #[test]
    fn test_bollinger_large_dataset() {
        let mut prices = Vec::with_capacity(10000);
        let mut price = 100.0;
        for i in 0..10000 {
            price += (i as f64 * 0.01).sin() * 2.0;
            prices.push(price);
        }
        let result = calculate(&prices, 20, 2.0).unwrap();
        assert_eq!(result.upper_series.len(), 10000);
        assert!(result.upper > result.lower);
    }

    #[test]
    fn test_bollinger_leading_zeros() {
        let prices = trending_up(30);
        let result = calculate(&prices, 20, 2.0).unwrap();
        // first 19 elements should be 0 (period - 1 leading zeros)
        for i in 0..19 {
            assert!(
                (result.upper_series[i]).abs() < 1e-10,
                "index {} should be 0",
                i
            );
        }
        // element 19 should be non-zero
        assert!(
            result.upper_series[19].abs() > 1e-10,
            "index 19 should be non-zero"
        );
    }
}
