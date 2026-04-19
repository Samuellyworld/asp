// market regime classification
// uses atr + adx to classify market into trending/ranging/volatile/quiet

use super::{adx, atr};

/// market regime categories
#[derive(Debug, Clone, PartialEq)]
pub enum Regime {
    Trending,
    Ranging,
    Volatile,
    Quiet,
}

impl Regime {
    pub fn as_str(&self) -> &str {
        match self {
            Regime::Trending => "trending",
            Regime::Ranging => "ranging",
            Regime::Volatile => "volatile",
            Regime::Quiet => "quiet",
        }
    }
}

/// composite regime classification result
pub struct RegimeResult {
    pub regime: String,      // trending, ranging, volatile, quiet
    pub adx: f64,            // trend strength 0-100
    pub atr_percent: f64,    // volatility as % of price
    pub plus_di: f64,        // bullish directional indicator
    pub minus_di: f64,       // bearish directional indicator
    pub trend_dir: String,   // UP, DOWN, NEUTRAL
    pub confidence: f64,     // classification confidence 0-100
    pub description: String, // human-readable assessment
}

/// classifies market regime from ohlc data
pub fn classify(
    highs: &[f64],
    lows: &[f64],
    closes: &[f64],
    period: usize,
) -> Option<RegimeResult> {
    let adx_result = adx::calculate(highs, lows, closes, period)?;
    let atr_result = atr::calculate(highs, lows, closes, period)?;

    let adx_val = adx_result.value;
    let atr_pct = atr_result.percent;

    // classification logic:
    // volatile: high atr regardless of trend (atr% >= 3.0)
    // trending: adx >= 25 and moderate-to-high atr
    // quiet: low atr (< 1.0%) and low adx
    // ranging: everything else (low adx, normal volatility)
    let (regime, confidence) = if atr_pct >= 3.0 {
        // high volatility dominates
        let conf = 60.0 + (atr_pct - 3.0).min(5.0) * 8.0;
        (Regime::Volatile, conf.min(100.0))
    } else if adx_val >= 25.0 {
        // strong directional movement
        let conf = 50.0 + (adx_val - 25.0).min(25.0) * 2.0;
        (Regime::Trending, conf.min(100.0))
    } else if atr_pct <= 1.0 && adx_val < 20.0 {
        // low everything = quiet
        let conf = 50.0 + (20.0 - adx_val).min(10.0) * 3.0 + (1.0 - atr_pct).min(0.5) * 20.0;
        (Regime::Quiet, conf.min(100.0))
    } else {
        // moderate volatility, no strong trend
        let conf = 40.0 + (25.0 - adx_val).min(15.0) * 2.0;
        (Regime::Ranging, conf.min(100.0))
    };

    let description = build_description(&regime, adx_val, atr_pct, &adx_result.trend_dir);

    Some(RegimeResult {
        regime: regime.as_str().to_string(),
        adx: adx_val,
        atr_percent: atr_pct,
        plus_di: adx_result.plus_di,
        minus_di: adx_result.minus_di,
        trend_dir: adx_result.trend_dir,
        confidence,
        description,
    })
}

fn build_description(regime: &Regime, adx: f64, atr_pct: f64, trend_dir: &str) -> String {
    match regime {
        Regime::Trending => {
            format!(
                "Strong {} trend detected (ADX={:.1}). Favor trend-following entries with wider stops.",
                trend_dir.to_lowercase(), adx
            )
        }
        Regime::Ranging => {
            format!(
                "Range-bound market (ADX={:.1}, ATR={:.2}%). Favor mean-reversion at support/resistance.",
                adx, atr_pct
            )
        }
        Regime::Volatile => {
            format!(
                "High volatility detected (ATR={:.2}%). Reduce position size and use wider stops.",
                atr_pct
            )
        }
        Regime::Quiet => {
            format!(
                "Quiet market (ADX={:.1}, ATR={:.2}%). Watch for breakout setups, wait for confirmation.",
                adx, atr_pct
            )
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_strong_trend(n: usize) -> (Vec<f64>, Vec<f64>, Vec<f64>) {
        let mut h = Vec::with_capacity(n);
        let mut l = Vec::with_capacity(n);
        let mut c = Vec::with_capacity(n);
        for i in 0..n {
            let base = 100.0 + i as f64 * 3.0; // strong upward movement
            h.push(base + 2.0);
            l.push(base - 1.0);
            c.push(base + 1.0);
        }
        (h, l, c)
    }

    fn make_quiet_market(n: usize) -> (Vec<f64>, Vec<f64>, Vec<f64>) {
        let mut h = Vec::with_capacity(n);
        let mut l = Vec::with_capacity(n);
        let mut c = Vec::with_capacity(n);
        for i in 0..n {
            let base = 100.0 + (i as f64 * 0.3).sin() * 0.2;
            h.push(base + 0.3);
            l.push(base - 0.3);
            c.push(base);
        }
        (h, l, c)
    }

    fn make_volatile_market(n: usize) -> (Vec<f64>, Vec<f64>, Vec<f64>) {
        let mut h = Vec::with_capacity(n);
        let mut l = Vec::with_capacity(n);
        let mut c = Vec::with_capacity(n);
        for i in 0..n {
            let base = 100.0 + (i as f64 * 0.8).sin() * 5.0;
            h.push(base + 5.0);
            l.push(base - 5.0);
            c.push(base);
        }
        (h, l, c)
    }

    #[test]
    fn test_regime_trending() {
        let (h, l, c) = make_strong_trend(60);
        let result = classify(&h, &l, &c, 14).unwrap();
        // a strong trend should be classified as trending or volatile
        assert!(
            result.regime == "trending" || result.regime == "volatile",
            "strong trend should be trending or volatile, got {}",
            result.regime
        );
    }

    #[test]
    fn test_regime_quiet() {
        let (h, l, c) = make_quiet_market(60);
        let result = classify(&h, &l, &c, 14).unwrap();
        // quiet market should show low volatility
        assert!(
            result.atr_percent < 2.0,
            "quiet market should have low atr%, got {}",
            result.atr_percent
        );
    }

    #[test]
    fn test_regime_volatile() {
        let (h, l, c) = make_volatile_market(60);
        let result = classify(&h, &l, &c, 14).unwrap();
        assert!(
            result.atr_percent > 1.5,
            "volatile market should have high atr%, got {}",
            result.atr_percent
        );
    }

    #[test]
    fn test_regime_has_description() {
        let (h, l, c) = make_strong_trend(60);
        let result = classify(&h, &l, &c, 14).unwrap();
        assert!(!result.description.is_empty());
    }

    #[test]
    fn test_regime_confidence_range() {
        let (h, l, c) = make_strong_trend(60);
        let result = classify(&h, &l, &c, 14).unwrap();
        assert!(
            result.confidence >= 0.0 && result.confidence <= 100.0,
            "confidence out of range: {}",
            result.confidence
        );
    }

    #[test]
    fn test_regime_adx_populated() {
        let (h, l, c) = make_strong_trend(60);
        let result = classify(&h, &l, &c, 14).unwrap();
        assert!(result.adx > 0.0);
        assert!(result.plus_di > 0.0 || result.minus_di > 0.0);
    }

    #[test]
    fn test_regime_insufficient_data() {
        let h = vec![101.0; 20];
        let l = vec![99.0; 20];
        let c = vec![100.0; 20];
        assert!(classify(&h, &l, &c, 14).is_none());
    }

    #[test]
    fn test_regime_trend_dir() {
        let (h, l, c) = make_strong_trend(60);
        let result = classify(&h, &l, &c, 14).unwrap();
        // uptrend data -> trend direction should not be DOWN
        assert_ne!(
            result.trend_dir, "DOWN",
            "uptrend data should not have DOWN direction"
        );
    }
}
