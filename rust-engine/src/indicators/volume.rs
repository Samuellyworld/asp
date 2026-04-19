// volume spike detection
// compares current volume against moving average to detect anomalies

// volume analysis result
pub struct VolumeResult {
    pub current_volume: f64,
    pub average_volume: f64,
    pub ratio: f64,
    pub is_spike: bool,
    pub signal: String,
}

// detects volume spikes by comparing current volume to the average
// over a lookback period. threshold is the multiplier (e.g., 2.0 means
// a spike is when volume is 2x the average)
pub fn detect(volumes: &[f64], lookback: usize, threshold: f64) -> Option<VolumeResult> {
    if volumes.is_empty() || lookback == 0 || volumes.len() < lookback + 1 {
        return None;
    }

    let current = *volumes.last().unwrap();
    let lookback_slice = &volumes[volumes.len() - 1 - lookback..volumes.len() - 1];
    let avg = lookback_slice.iter().sum::<f64>() / lookback as f64;

    let ratio = if avg.abs() > 1e-10 {
        current / avg
    } else {
        0.0
    };

    let is_spike = ratio >= threshold;
    let signal = classify(ratio, threshold);

    Some(VolumeResult {
        current_volume: current,
        average_volume: avg,
        ratio,
        is_spike,
        signal,
    })
}

// classifies volume relative to average
pub fn classify(ratio: f64, threshold: f64) -> String {
    if ratio >= threshold {
        "SPIKE".to_string()
    } else if ratio < 0.5 {
        "LOW".to_string()
    } else {
        "NORMAL".to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_volume_spike_detected() {
        let mut volumes = vec![100.0; 20];
        volumes.push(300.0); // 3x average
        let result = detect(&volumes, 20, 2.0).unwrap();
        assert!(result.is_spike);
        assert_eq!(result.signal, "SPIKE");
        assert!((result.ratio - 3.0).abs() < 1e-10);
    }

    #[test]
    fn test_volume_normal() {
        let mut volumes = vec![100.0; 20];
        volumes.push(110.0);
        let result = detect(&volumes, 20, 2.0).unwrap();
        assert!(!result.is_spike);
        assert_eq!(result.signal, "NORMAL");
    }

    #[test]
    fn test_volume_low() {
        let mut volumes = vec![100.0; 20];
        volumes.push(30.0);
        let result = detect(&volumes, 20, 2.0).unwrap();
        assert!(!result.is_spike);
        assert_eq!(result.signal, "LOW");
        assert!(result.ratio < 0.5);
    }

    #[test]
    fn test_volume_empty() {
        assert!(detect(&[], 20, 2.0).is_none());
    }

    #[test]
    fn test_volume_insufficient_data() {
        let volumes = vec![100.0; 5];
        assert!(detect(&volumes, 20, 2.0).is_none());
    }

    #[test]
    fn test_volume_lookback_zero() {
        let volumes = vec![100.0; 20];
        assert!(detect(&volumes, 0, 2.0).is_none());
    }

    #[test]
    fn test_volume_exact_threshold() {
        let mut volumes = vec![100.0; 20];
        volumes.push(200.0);
        let result = detect(&volumes, 20, 2.0).unwrap();
        assert!(result.is_spike);
    }

    #[test]
    fn test_volume_just_below_threshold() {
        let mut volumes = vec![100.0; 20];
        volumes.push(199.0);
        let result = detect(&volumes, 20, 2.0).unwrap();
        assert!(!result.is_spike);
        assert_eq!(result.signal, "NORMAL");
    }

    #[test]
    fn test_volume_average_calculation() {
        let volumes = vec![10.0, 20.0, 30.0, 40.0, 50.0, 100.0];
        let result = detect(&volumes, 5, 2.0).unwrap();
        // average of 10,20,30,40,50 = 30
        assert!((result.average_volume - 30.0).abs() < 1e-10);
        assert!((result.current_volume - 100.0).abs() < 1e-10);
    }

    #[test]
    fn test_volume_classify_spike() {
        assert_eq!(classify(3.0, 2.0), "SPIKE");
    }

    #[test]
    fn test_volume_classify_normal() {
        assert_eq!(classify(1.2, 2.0), "NORMAL");
    }

    #[test]
    fn test_volume_classify_low() {
        assert_eq!(classify(0.3, 2.0), "LOW");
    }

    #[test]
    fn test_volume_zero_average() {
        let mut volumes = vec![0.0; 20];
        volumes.push(100.0);
        let result = detect(&volumes, 20, 2.0).unwrap();
        assert!(
            (result.ratio).abs() < 1e-10,
            "zero average should give 0 ratio"
        );
    }
}
