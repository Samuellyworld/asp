// relative strength index calculation
// measures momentum on a 0-100 scale

// rsi result with value and signal classification
pub struct RSIResult {
    pub value: f64,
    pub signal: String,
    pub series: Vec<f64>,
}

// calculates rsi from closing prices
pub fn calculate(closes: &[f64], period: usize) -> Option<RSIResult> {
    if closes.len() < period + 1 || period == 0 {
        return None;
    }

    let mut gains = Vec::with_capacity(closes.len() - 1);
    let mut losses = Vec::with_capacity(closes.len() - 1);

    for i in 1..closes.len() {
        let change = closes[i] - closes[i - 1];
        if change > 0.0 {
            gains.push(change);
            losses.push(0.0);
        } else {
            gains.push(0.0);
            losses.push(change.abs());
        }
    }

    let series = compute_rsi_series(&gains, &losses, period);

    let value = *series.last().unwrap_or(&50.0);
    let signal = classify(value);

    Some(RSIResult {
        value,
        signal,
        series,
    })
}

// computes the full rsi series using wilder's smoothed moving average
fn compute_rsi_series(gains: &[f64], losses: &[f64], period: usize) -> Vec<f64> {
    if gains.len() < period {
        return vec![];
    }

    let mut result = Vec::with_capacity(gains.len());

    // initial average gain/loss using simple mean
    let mut avg_gain: f64 = gains[..period].iter().sum::<f64>() / period as f64;
    let mut avg_loss: f64 = losses[..period].iter().sum::<f64>() / period as f64;

    // fill leading positions
    for _ in 0..period - 1 {
        result.push(0.0);
    }

    let rsi = if avg_loss == 0.0 {
        100.0
    } else {
        let rs = avg_gain / avg_loss;
        100.0 - (100.0 / (1.0 + rs))
    };
    result.push(rsi);

    // wilder's smoothed average
    for i in period..gains.len() {
        avg_gain = (avg_gain * (period as f64 - 1.0) + gains[i]) / period as f64;
        avg_loss = (avg_loss * (period as f64 - 1.0) + losses[i]) / period as f64;

        let rsi = if avg_loss == 0.0 {
            100.0
        } else {
            let rs = avg_gain / avg_loss;
            100.0 - (100.0 / (1.0 + rs))
        };
        result.push(rsi);
    }

    result
}

/// classifies rsi value into a signal
pub fn classify(value: f64) -> String {
    if value <= 30.0 {
        "OVERSOLD".to_string()
    } else if value >= 70.0 {
        "OVERBOUGHT".to_string()
    } else {
        "NEUTRAL".to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn rising_prices(n: usize) -> Vec<f64> {
        (0..n).map(|i| 100.0 + i as f64).collect()
    }

    fn falling_prices(n: usize) -> Vec<f64> {
        (0..n).map(|i| 100.0 - i as f64).collect()
    }

    fn sideways_prices() -> Vec<f64> {
        vec![100.0, 101.0, 100.0, 101.0, 100.0, 101.0, 100.0, 101.0,
             100.0, 101.0, 100.0, 101.0, 100.0, 101.0, 100.0, 101.0]
    }

    #[test]
    fn test_rsi_rising_is_high() {
        let prices = rising_prices(30);
        let result = calculate(&prices, 14).unwrap();
        assert!(result.value > 70.0, "rising prices should give high rsi, got {}", result.value);
        assert_eq!(result.signal, "OVERBOUGHT");
    }

    #[test]
    fn test_rsi_falling_is_low() {
        let prices = falling_prices(30);
        let result = calculate(&prices, 14).unwrap();
        assert!(result.value < 30.0, "falling prices should give low rsi, got {}", result.value);
        assert_eq!(result.signal, "OVERSOLD");
    }

    #[test]
    fn test_rsi_sideways_is_neutral() {
        let prices = sideways_prices();
        let result = calculate(&prices, 14).unwrap();
        assert!(result.value > 30.0 && result.value < 70.0,
            "sideways prices should give neutral rsi, got {}", result.value);
        assert_eq!(result.signal, "NEUTRAL");
    }

    #[test]
    fn test_rsi_range_0_100() {
        let prices = rising_prices(50);
        let result = calculate(&prices, 14).unwrap();
        for val in &result.series {
            if *val != 0.0 { // skip leading zeros
                assert!(*val >= 0.0 && *val <= 100.0, "rsi out of range: {}", val);
            }
        }
    }

    #[test]
    fn test_rsi_series_length() {
        let prices = rising_prices(30);
        let result = calculate(&prices, 14).unwrap();
        // series length = closes.len() - 1 (price changes)
        assert_eq!(result.series.len(), 29);
    }

    #[test]
    fn test_rsi_insufficient_data() {
        let prices = vec![1.0, 2.0, 3.0];
        assert!(calculate(&prices, 14).is_none());
    }

    #[test]
    fn test_rsi_period_zero() {
        let prices = rising_prices(20);
        assert!(calculate(&prices, 0).is_none());
    }

    #[test]
    fn test_rsi_known_value() {
        // known rsi(14) calculation
        // prices that produce a calculable rsi
        let prices = vec![
            44.34, 44.09, 44.15, 43.61, 44.33,
            44.83, 45.10, 45.42, 45.84, 46.08,
            45.89, 46.03, 45.61, 46.28, 46.28,
            46.00, 46.03, 46.41, 46.22, 45.64,
        ];
        let result = calculate(&prices, 14).unwrap();
        // rsi should be in a reasonable range for this data
        assert!(result.value > 40.0 && result.value < 80.0,
            "unexpected rsi value: {}", result.value);
    }

    #[test]
    fn test_rsi_all_gains() {
        // every bar is a gain -> rsi should approach 100
        let prices: Vec<f64> = (0..30).map(|i| 100.0 + (i as f64) * 2.0).collect();
        let result = calculate(&prices, 14).unwrap();
        assert!(result.value > 90.0, "all gains should give rsi near 100, got {}", result.value);
    }

    #[test]
    fn test_rsi_all_losses() {
        // every bar is a loss -> rsi should approach 0
        let prices: Vec<f64> = (0..30).map(|i| 100.0 - (i as f64) * 2.0).collect();
        let result = calculate(&prices, 14).unwrap();
        assert!(result.value < 10.0, "all losses should give rsi near 0, got {}", result.value);
    }

    #[test]
    fn test_rsi_classify_boundaries() {
        assert_eq!(classify(30.0), "OVERSOLD");
        assert_eq!(classify(30.1), "NEUTRAL");
        assert_eq!(classify(69.9), "NEUTRAL");
        assert_eq!(classify(70.0), "OVERBOUGHT");
        assert_eq!(classify(50.0), "NEUTRAL");
        assert_eq!(classify(0.0), "OVERSOLD");
        assert_eq!(classify(100.0), "OVERBOUGHT");
    }

    #[test]
    fn test_rsi_period_equals_data() {
        let prices = rising_prices(15); // 15 prices, 14 changes = exactly enough for period 14
        let result = calculate(&prices, 14).unwrap();
        assert!(result.value > 0.0);
    }

    #[test]
    fn test_rsi_large_dataset() {
        let mut prices = Vec::with_capacity(10000);
        let mut price = 100.0;
        for i in 0..10000 {
            price += (i as f64 * 0.01).sin() * 2.0;
            prices.push(price);
        }
        let result = calculate(&prices, 14).unwrap();
        assert!(result.value >= 0.0 && result.value <= 100.0);
        assert_eq!(result.series.len(), 9999);
    }
}
