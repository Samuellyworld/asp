// grpc service implementation for technical indicators

use tonic::{Request, Response, Status};

pub mod proto {
    tonic::include_proto!("indicators");
}

use proto::technical_indicators_server::TechnicalIndicators;
use proto::*;

use crate::indicators::{bollinger, ema, macd, rsi, volume};

pub struct IndicatorService;

// extracts closing prices from candle data
fn extract_closes(candles: &[Candle]) -> Vec<f64> {
    candles.iter().map(|c| c.close).collect()
}

// extracts volumes from candle data
fn extract_volumes(candles: &[Candle]) -> Vec<f64> {
    candles.iter().map(|c| c.volume).collect()
}

#[tonic::async_trait]
impl TechnicalIndicators for IndicatorService {
    async fn calculate_rsi(
        &self,
        request: Request<RsiRequest>,
    ) -> Result<Response<RsiResponse>, Status> {
        let req = request.into_inner();
        let closes = extract_closes(&req.candles);
        let period = if req.period > 0 { req.period as usize } else { 14 };

        match rsi::calculate(&closes, period) {
            Some(result) => Ok(Response::new(RsiResponse {
                value: result.value,
                signal: result.signal,
                series: result.series,
            })),
            None => Err(Status::invalid_argument(
                "insufficient data for rsi calculation",
            )),
        }
    }

    async fn calculate_macd(
        &self,
        request: Request<MacdRequest>,
    ) -> Result<Response<MacdResponse>, Status> {
        let req = request.into_inner();
        let closes = extract_closes(&req.candles);
        let fast = if req.fast_period > 0 { req.fast_period as usize } else { 12 };
        let slow = if req.slow_period > 0 { req.slow_period as usize } else { 26 };
        let signal = if req.signal_period > 0 { req.signal_period as usize } else { 9 };

        match macd::calculate(&closes, fast, slow, signal) {
            Some(result) => Ok(Response::new(MacdResponse {
                macd_line: result.macd_line,
                signal_line: result.signal_line,
                histogram: result.histogram,
                signal: result.signal,
                crossover: result.crossover,
                macd_series: result.macd_series,
                signal_series: result.signal_series,
                histogram_series: result.histogram_series,
            })),
            None => Err(Status::invalid_argument(
                "insufficient data for macd calculation",
            )),
        }
    }

    async fn calculate_bollinger_bands(
        &self,
        request: Request<BollingerRequest>,
    ) -> Result<Response<BollingerResponse>, Status> {
        let req = request.into_inner();
        let closes = extract_closes(&req.candles);
        let period = if req.period > 0 { req.period as usize } else { 20 };
        let std_dev = if req.std_dev > 0.0 { req.std_dev } else { 2.0 };

        match bollinger::calculate(&closes, period, std_dev) {
            Some(result) => Ok(Response::new(BollingerResponse {
                upper: result.upper,
                middle: result.middle,
                lower: result.lower,
                bandwidth: result.bandwidth,
                percent_b: result.percent_b,
                signal: result.signal,
                upper_series: result.upper_series,
                middle_series: result.middle_series,
                lower_series: result.lower_series,
            })),
            None => Err(Status::invalid_argument(
                "insufficient data for bollinger bands calculation",
            )),
        }
    }

    async fn calculate_ema(
        &self,
        request: Request<EmaRequest>,
    ) -> Result<Response<EmaResponse>, Status> {
        let req = request.into_inner();
        let closes = extract_closes(&req.candles);
        let period = if req.period > 0 { req.period as usize } else { 21 };

        if closes.len() < period {
            return Err(Status::invalid_argument(
                "insufficient data for ema calculation",
            ));
        }

        let series = ema::calculate(&closes, period);
        let value = *series.last().unwrap_or(&0.0);
        let trend = ema::trend(*closes.last().unwrap_or(&0.0), value).to_string();

        Ok(Response::new(EmaResponse {
            value,
            trend,
            series,
        }))
    }

    async fn detect_volume_spike(
        &self,
        request: Request<VolumeRequest>,
    ) -> Result<Response<VolumeResponse>, Status> {
        let req = request.into_inner();
        let volumes = extract_volumes(&req.candles);
        let lookback = if req.lookback > 0 { req.lookback as usize } else { 20 };
        let threshold = if req.threshold > 0.0 { req.threshold } else { 2.0 };

        match volume::detect(&volumes, lookback, threshold) {
            Some(result) => Ok(Response::new(VolumeResponse {
                is_spike: result.is_spike,
                current_volume: result.current_volume,
                average_volume: result.average_volume,
                ratio: result.ratio,
                signal: result.signal,
            })),
            None => Err(Status::invalid_argument(
                "insufficient data for volume analysis",
            )),
        }
    }

    async fn analyze_all(
        &self,
        request: Request<AnalyzeAllRequest>,
    ) -> Result<Response<AnalyzeAllResponse>, Status> {
        let req = request.into_inner();
        let closes = extract_closes(&req.candles);
        let volumes = extract_volumes(&req.candles);

        // defaults
        let rsi_period = if req.rsi_period > 0 { req.rsi_period as usize } else { 14 };
        let macd_fast = if req.macd_fast > 0 { req.macd_fast as usize } else { 12 };
        let macd_slow = if req.macd_slow > 0 { req.macd_slow as usize } else { 26 };
        let macd_signal = if req.macd_signal > 0 { req.macd_signal as usize } else { 9 };
        let bb_period = if req.bb_period > 0 { req.bb_period as usize } else { 20 };
        let bb_std_dev = if req.bb_std_dev > 0.0 { req.bb_std_dev } else { 2.0 };
        let ema_period = if req.ema_period > 0 { req.ema_period as usize } else { 21 };
        let vol_lookback = if req.volume_lookback > 0 { req.volume_lookback as usize } else { 20 };
        let vol_threshold = if req.volume_threshold > 0.0 { req.volume_threshold } else { 2.0 };

        // compute each indicator, use None if insufficient data
        let rsi_result = rsi::calculate(&closes, rsi_period);
        let macd_result = macd::calculate(&closes, macd_fast, macd_slow, macd_signal);
        let bb_result = bollinger::calculate(&closes, bb_period, bb_std_dev);
        let vol_result = volume::detect(&volumes, vol_lookback, vol_threshold);

        let ema_series = if closes.len() >= ema_period {
            Some(ema::calculate(&closes, ema_period))
        } else {
            None
        };

        // build signal scoring
        let mut bullish = 0i32;
        let mut bearish = 0i32;

        let rsi_resp = rsi_result.map(|r| {
            match r.signal.as_str() {
                "OVERSOLD" => bullish += 1,   // oversold = potential buy
                "OVERBOUGHT" => bearish += 1,
                _ => {}
            }
            RsiResponse {
                value: r.value,
                signal: r.signal,
                series: r.series,
            }
        });

        let macd_resp = macd_result.map(|r| {
            match r.signal.as_str() {
                "BULLISH" => bullish += 1,
                "BEARISH" => bearish += 1,
                _ => {}
            }
            MacdResponse {
                macd_line: r.macd_line,
                signal_line: r.signal_line,
                histogram: r.histogram,
                signal: r.signal,
                crossover: r.crossover,
                macd_series: r.macd_series,
                signal_series: r.signal_series,
                histogram_series: r.histogram_series,
            }
        });

        let bb_resp = bb_result.map(|r| {
            match r.signal.as_str() {
                "LOWER_BAND" => bullish += 1, // price at lower band = potential buy
                "UPPER_BAND" => bearish += 1,
                _ => {}
            }
            BollingerResponse {
                upper: r.upper,
                middle: r.middle,
                lower: r.lower,
                bandwidth: r.bandwidth,
                percent_b: r.percent_b,
                signal: r.signal,
                upper_series: r.upper_series,
                middle_series: r.middle_series,
                lower_series: r.lower_series,
            }
        });

        let ema_resp = ema_series.map(|s| {
            let value = *s.last().unwrap_or(&0.0);
            let last_close = *closes.last().unwrap_or(&0.0);
            let trend = ema::trend(last_close, value).to_string();
            match trend.as_str() {
                "ABOVE" => bullish += 1,
                "BELOW" => bearish += 1,
                _ => {}
            }
            EmaResponse {
                value,
                trend,
                series: s,
            }
        });

        let vol_resp = vol_result.map(|r| {
            // volume spike is neutral signal on its own
            VolumeResponse {
                is_spike: r.is_spike,
                current_volume: r.current_volume,
                average_volume: r.average_volume,
                ratio: r.ratio,
                signal: r.signal,
            }
        });

        let overall_signal = determine_overall_signal(bullish, bearish);

        Ok(Response::new(AnalyzeAllResponse {
            rsi: rsi_resp,
            macd: macd_resp,
            bollinger: bb_resp,
            ema: ema_resp,
            volume: vol_resp,
            overall_signal,
            bullish_count: bullish,
            bearish_count: bearish,
        }))
    }
}

/// determines overall signal from bullish/bearish indicator counts
fn determine_overall_signal(bullish: i32, bearish: i32) -> String {
    let net = bullish - bearish;
    match net {
        n if n >= 3 => "STRONG_BUY".to_string(),
        n if n >= 1 => "BUY".to_string(),
        0 => "NEUTRAL".to_string(),
        n if n >= -2 => "SELL".to_string(),
        _ => "STRONG_SELL".to_string(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_candles(prices: &[f64], volumes: &[f64]) -> Vec<Candle> {
        prices.iter().zip(volumes.iter()).enumerate().map(|(i, (p, v))| {
            Candle {
                open: *p,
                high: *p * 1.01,
                low: *p * 0.99,
                close: *p,
                volume: *v,
                timestamp: i as i64,
            }
        }).collect()
    }

    fn make_price_candles(prices: &[f64]) -> Vec<Candle> {
        let volumes = vec![100.0; prices.len()];
        make_candles(prices, &volumes)
    }

    #[tokio::test]
    async fn test_rsi_endpoint() {
        let service = IndicatorService;
        let prices: Vec<f64> = (0..30).map(|i| 100.0 + (i as f64) * 0.5).collect();
        let req = Request::new(RsiRequest {
            candles: make_price_candles(&prices),
            period: 14,
        });
        let resp = service.calculate_rsi(req).await.unwrap();
        let inner = resp.into_inner();
        assert!(inner.value >= 0.0 && inner.value <= 100.0);
        assert!(!inner.signal.is_empty());
    }

    #[tokio::test]
    async fn test_rsi_insufficient_data() {
        let service = IndicatorService;
        let req = Request::new(RsiRequest {
            candles: make_price_candles(&[100.0, 101.0]),
            period: 14,
        });
        let resp = service.calculate_rsi(req).await;
        assert!(resp.is_err());
    }

    #[tokio::test]
    async fn test_macd_endpoint() {
        let service = IndicatorService;
        let prices: Vec<f64> = (0..60).map(|i| 100.0 + (i as f64) * 0.3).collect();
        let req = Request::new(MacdRequest {
            candles: make_price_candles(&prices),
            fast_period: 12,
            slow_period: 26,
            signal_period: 9,
        });
        let resp = service.calculate_macd(req).await.unwrap();
        let inner = resp.into_inner();
        assert!(!inner.signal.is_empty());
    }

    #[tokio::test]
    async fn test_bollinger_endpoint() {
        let service = IndicatorService;
        let prices: Vec<f64> = (0..30).map(|i| 100.0 + (i as f64) * 0.2).collect();
        let req = Request::new(BollingerRequest {
            candles: make_price_candles(&prices),
            period: 20,
            std_dev: 2.0,
        });
        let resp = service.calculate_bollinger_bands(req).await.unwrap();
        let inner = resp.into_inner();
        assert!(inner.upper > inner.lower);
    }

    #[tokio::test]
    async fn test_ema_endpoint() {
        let service = IndicatorService;
        let prices: Vec<f64> = (0..30).map(|i| 100.0 + (i as f64) * 0.5).collect();
        let req = Request::new(EmaRequest {
            candles: make_price_candles(&prices),
            period: 9,
        });
        let resp = service.calculate_ema(req).await.unwrap();
        let inner = resp.into_inner();
        assert!(inner.value > 0.0);
        assert!(!inner.trend.is_empty());
    }

    #[tokio::test]
    async fn test_volume_endpoint() {
        let service = IndicatorService;
        let prices = vec![100.0; 25];
        let mut volumes = vec![1000.0; 24];
        volumes.push(5000.0);
        let req = Request::new(VolumeRequest {
            candles: make_candles(&prices, &volumes),
            lookback: 20,
            threshold: 2.0,
        });
        let resp = service.detect_volume_spike(req).await.unwrap();
        let inner = resp.into_inner();
        assert!(inner.is_spike);
        assert_eq!(inner.signal, "SPIKE");
    }

    #[tokio::test]
    async fn test_analyze_all_endpoint() {
        let service = IndicatorService;
        let prices: Vec<f64> = (0..60).map(|i| 100.0 + (i as f64) * 0.3).collect();
        let volumes = vec![1000.0; 60];
        let req = Request::new(AnalyzeAllRequest {
            candles: make_candles(&prices, &volumes),
            rsi_period: 14,
            macd_fast: 12,
            macd_slow: 26,
            macd_signal: 9,
            bb_period: 20,
            bb_std_dev: 2.0,
            ema_period: 21,
            volume_lookback: 20,
            volume_threshold: 2.0,
        });
        let resp = service.analyze_all(req).await.unwrap();
        let inner = resp.into_inner();
        assert!(inner.rsi.is_some());
        assert!(inner.macd.is_some());
        assert!(inner.bollinger.is_some());
        assert!(inner.ema.is_some());
        assert!(inner.volume.is_some());
        assert!(!inner.overall_signal.is_empty());
    }

    #[tokio::test]
    async fn test_analyze_all_defaults() {
        let service = IndicatorService;
        let prices: Vec<f64> = (0..60).map(|i| 100.0 + (i as f64) * 0.3).collect();
        let volumes = vec![1000.0; 60];
        // all zeros = use defaults
        let req = Request::new(AnalyzeAllRequest {
            candles: make_candles(&prices, &volumes),
            rsi_period: 0,
            macd_fast: 0,
            macd_slow: 0,
            macd_signal: 0,
            bb_period: 0,
            bb_std_dev: 0.0,
            ema_period: 0,
            volume_lookback: 0,
            volume_threshold: 0.0,
        });
        let resp = service.analyze_all(req).await.unwrap();
        let inner = resp.into_inner();
        assert!(inner.rsi.is_some());
    }

    #[test]
    fn test_overall_signal_strong_buy() {
        assert_eq!(determine_overall_signal(4, 0), "STRONG_BUY");
    }

    #[test]
    fn test_overall_signal_buy() {
        assert_eq!(determine_overall_signal(2, 1), "BUY");
    }

    #[test]
    fn test_overall_signal_neutral() {
        assert_eq!(determine_overall_signal(2, 2), "NEUTRAL");
    }

    #[test]
    fn test_overall_signal_sell() {
        assert_eq!(determine_overall_signal(1, 2), "SELL");
    }

    #[test]
    fn test_overall_signal_strong_sell() {
        assert_eq!(determine_overall_signal(0, 4), "STRONG_SELL");
    }
}
