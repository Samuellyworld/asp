// average directional index calculation
// measures trend strength on a 0-100 scale using +DI and -DI

use super::ema;

/// adx result with trend strength and directional indicators
pub struct ADXResult {
    pub value: f64,          // adx value 0-100
    pub plus_di: f64,        // +di (bullish directional indicator)
    pub minus_di: f64,       // -di (bearish directional indicator)
    pub signal: String,      // STRONG_TREND, TRENDING, WEAK, NO_TREND
    pub trend_dir: String,   // UP (+di > -di), DOWN (-di > +di), NEUTRAL
    pub series: Vec<f64>,    // full adx series
}

/// calculates ADX from ohlc data.
/// formula:
///   +DM = high - prev_high (if > prev_low - low and > 0, else 0)
///   -DM = prev_low - low (if > high - prev_high and > 0, else 0)
///   +DI = smoothed(+DM) / smoothed(TR) * 100
///   -DI = smoothed(-DM) / smoothed(TR) * 100
///   DX = |+DI - -DI| / (+DI + -DI) * 100
///   ADX = smoothed(DX)
pub fn calculate(highs: &[f64], lows: &[f64], closes: &[f64], period: usize) -> Option<ADXResult> {
    let len = highs.len();
    if len < period * 2 + 1 || len != lows.len() || len != closes.len() || period == 0 {
        return None;
    }

    // step 1: compute +DM, -DM, and TR for each bar
    let mut plus_dm = Vec::with_capacity(len - 1);
    let mut minus_dm = Vec::with_capacity(len - 1);
    let mut tr = Vec::with_capacity(len - 1);

    for i in 1..len {
        let up = highs[i] - highs[i - 1];
        let down = lows[i - 1] - lows[i];

        if up > down && up > 0.0 {
            plus_dm.push(up);
        } else {
            plus_dm.push(0.0);
        }

        if down > up && down > 0.0 {
            minus_dm.push(down);
        } else {
            minus_dm.push(0.0);
        }

        let hl = highs[i] - lows[i];
        let hc = (highs[i] - closes[i - 1]).abs();
        let lc = (lows[i] - closes[i - 1]).abs();
        tr.push(hl.max(hc).max(lc));
    }

    // step 2: smooth +DM, -DM, TR using wilder's method (period-smoothed)
    let smoothed_plus_dm = wilder_smooth(&plus_dm, period);
    let smoothed_minus_dm = wilder_smooth(&minus_dm, period);
    let smoothed_tr = wilder_smooth(&tr, period);

    if smoothed_plus_dm.is_empty() {
        return None;
    }

    // step 3: compute +DI and -DI
    let n = smoothed_tr.len();
    let mut plus_di_series = Vec::with_capacity(n);
    let mut minus_di_series = Vec::with_capacity(n);
    let mut dx_series = Vec::with_capacity(n);

    for i in 0..n {
        let atr = smoothed_tr[i];
        let pdi = if atr > 1e-10 { smoothed_plus_dm[i] / atr * 100.0 } else { 0.0 };
        let mdi = if atr > 1e-10 { smoothed_minus_dm[i] / atr * 100.0 } else { 0.0 };
        plus_di_series.push(pdi);
        minus_di_series.push(mdi);

        let sum = pdi + mdi;
        let dx = if sum > 1e-10 { (pdi - mdi).abs() / sum * 100.0 } else { 0.0 };
        dx_series.push(dx);
    }

    // step 4: smooth DX to get ADX using wilder's method
    let adx_raw = wilder_smooth(&dx_series, period);
    if adx_raw.is_empty() {
        return None;
    }

    // build full output series with leading zeros
    // total leading zeros in final adx series = period*2 - 1
    let mut series = vec![0.0; period * 2 - 1];
    series.extend_from_slice(&adx_raw);

    // pad if needed to match len-1
    while series.len() < len - 1 {
        series.push(0.0);
    }
    series.truncate(len - 1);

    let value = *adx_raw.last().unwrap_or(&0.0);
    let plus_di = *plus_di_series.last().unwrap_or(&0.0);
    let minus_di = *minus_di_series.last().unwrap_or(&0.0);
    let signal = classify(value);
    let trend_dir = if plus_di > minus_di + 2.0 {
        "UP".to_string()
    } else if minus_di > plus_di + 2.0 {
        "DOWN".to_string()
    } else {
        "NEUTRAL".to_string()
    };

    Some(ADXResult { value, plus_di, minus_di, signal, trend_dir, series })
}

/// wilder's smoothed average (first value = simple sum, then recursive)
fn wilder_smooth(data: &[f64], period: usize) -> Vec<f64> {
    if data.len() < period || period == 0 {
        return vec![];
    }

    let mut result = Vec::with_capacity(data.len() - period + 1);
    let first: f64 = data[..period].iter().sum::<f64>();
    result.push(first / period as f64);

    for i in period..data.len() {
        let prev = *result.last().unwrap();
        let smoothed = (prev * (period as f64 - 1.0) + data[i]) / period as f64;
        result.push(smoothed);
    }

    result
}

/// classifies adx strength
pub fn classify(adx: f64) -> String {
    if adx >= 50.0 {
        "STRONG_TREND".to_string()
    } else if adx >= 25.0 {
        "TRENDING".to_string()
    } else if adx >= 15.0 {
        "WEAK".to_string()
    } else {
        "NO_TREND".to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_strong_uptrend(n: usize) -> (Vec<f64>, Vec<f64>, Vec<f64>) {
        let mut highs = Vec::with_capacity(n);
        let mut lows = Vec::with_capacity(n);
        let mut closes = Vec::with_capacity(n);
        for i in 0..n {
            let base = 100.0 + i as f64 * 2.0;
            highs.push(base + 1.0);
            lows.push(base - 1.0);
            closes.push(base);
        }
        (highs, lows, closes)
    }

    fn make_strong_downtrend(n: usize) -> (Vec<f64>, Vec<f64>, Vec<f64>) {
        let mut highs = Vec::with_capacity(n);
        let mut lows = Vec::with_capacity(n);
        let mut closes = Vec::with_capacity(n);
        for i in 0..n {
            let base = 200.0 - i as f64 * 2.0;
            highs.push(base + 1.0);
            lows.push(base - 1.0);
            closes.push(base);
        }
        (highs, lows, closes)
    }

    fn make_sideways(n: usize) -> (Vec<f64>, Vec<f64>, Vec<f64>) {
        let mut highs = Vec::with_capacity(n);
        let mut lows = Vec::with_capacity(n);
        let mut closes = Vec::with_capacity(n);
        for i in 0..n {
            let base = 100.0 + (i as f64 * 0.5).sin() * 0.5;
            highs.push(base + 0.5);
            lows.push(base - 0.5);
            closes.push(base);
        }
        (highs, lows, closes)
    }

    #[test]
    fn test_adx_uptrend_shows_trend() {
        let (h, l, c) = make_strong_uptrend(60);
        let result = calculate(&h, &l, &c, 14).unwrap();
        assert!(result.value > 20.0,
            "strong uptrend should have high adx, got {}", result.value);
        assert_eq!(result.trend_dir, "UP");
    }

    #[test]
    fn test_adx_downtrend_shows_trend() {
        let (h, l, c) = make_strong_downtrend(60);
        let result = calculate(&h, &l, &c, 14).unwrap();
        assert!(result.value > 20.0,
            "strong downtrend should have high adx, got {}", result.value);
        assert_eq!(result.trend_dir, "DOWN");
    }

    #[test]
    fn test_adx_sideways_low() {
        let (h, l, c) = make_sideways(60);
        let result = calculate(&h, &l, &c, 14).unwrap();
        assert!(result.value < 30.0,
            "sideways market should have low adx, got {}", result.value);
    }

    #[test]
    fn test_adx_range() {
        let (h, l, c) = make_strong_uptrend(60);
        let result = calculate(&h, &l, &c, 14).unwrap();
        assert!(result.value >= 0.0 && result.value <= 100.0,
            "adx should be 0-100, got {}", result.value);
    }

    #[test]
    fn test_adx_di_positive() {
        let (h, l, c) = make_strong_uptrend(60);
        let result = calculate(&h, &l, &c, 14).unwrap();
        assert!(result.plus_di >= 0.0);
        assert!(result.minus_di >= 0.0);
    }

    #[test]
    fn test_adx_uptrend_plus_di_greater() {
        let (h, l, c) = make_strong_uptrend(60);
        let result = calculate(&h, &l, &c, 14).unwrap();
        assert!(result.plus_di > result.minus_di,
            "+DI should be > -DI in uptrend: {} vs {}", result.plus_di, result.minus_di);
    }

    #[test]
    fn test_adx_downtrend_minus_di_greater() {
        let (h, l, c) = make_strong_downtrend(60);
        let result = calculate(&h, &l, &c, 14).unwrap();
        assert!(result.minus_di > result.plus_di,
            "-DI should be > +DI in downtrend: {} vs {}", result.minus_di, result.plus_di);
    }

    #[test]
    fn test_adx_insufficient_data() {
        let h = vec![101.0; 20];
        let l = vec![99.0; 20];
        let c = vec![100.0; 20];
        assert!(calculate(&h, &l, &c, 14).is_none());
    }

    #[test]
    fn test_adx_period_zero() {
        let (h, l, c) = make_strong_uptrend(60);
        assert!(calculate(&h, &l, &c, 0).is_none());
    }

    #[test]
    fn test_classify_strong() {
        assert_eq!(classify(55.0), "STRONG_TREND");
        assert_eq!(classify(50.0), "STRONG_TREND");
    }

    #[test]
    fn test_classify_trending() {
        assert_eq!(classify(30.0), "TRENDING");
        assert_eq!(classify(25.0), "TRENDING");
    }

    #[test]
    fn test_classify_weak() {
        assert_eq!(classify(20.0), "WEAK");
        assert_eq!(classify(15.0), "WEAK");
    }

    #[test]
    fn test_classify_no_trend() {
        assert_eq!(classify(10.0), "NO_TREND");
        assert_eq!(classify(0.0), "NO_TREND");
    }

    #[test]
    fn test_wilder_smooth_basic() {
        let data = vec![1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0];
        let result = wilder_smooth(&data, 3);
        assert!(!result.is_empty());
        // first value = (1+2+3)/3 = 2.0
        assert!((result[0] - 2.0).abs() < 1e-10);
    }

    #[test]
    fn test_wilder_smooth_insufficient() {
        let data = vec![1.0, 2.0];
        assert!(wilder_smooth(&data, 5).is_empty());
    }

    #[test]
    fn test_adx_series_has_values() {
        let (h, l, c) = make_strong_uptrend(60);
        let result = calculate(&h, &l, &c, 14).unwrap();
        assert!(!result.series.is_empty());
        // should have some non-zero values
        let non_zero: Vec<_> = result.series.iter().filter(|v| v.abs() > 1e-10).collect();
        assert!(!non_zero.is_empty(), "series should have non-zero values");
    }
}
