// preferences management service
package preferences

import (
	"context"
	"fmt"
	"strings"
)

//  handles preferences business logic
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// scanning preferences

func (s *Service) GetScanning(ctx context.Context, userID int) (*Scanning, error) {
	prefs, err := s.repo.GetScanning(ctx, userID)
	if err != nil {
		return nil, err
	}
	if prefs == nil {
		return nil, fmt.Errorf("scanning preferences not found, try /start first")
	}
	return prefs, nil
}

func (s *Service) SetMinConfidence(ctx context.Context, userID int, value int) error {
	if value < 0 || value > 100 {
		return fmt.Errorf("confidence must be between 0 and 100")
	}
	prefs, err := s.GetScanning(ctx, userID)
	if err != nil {
		return err
	}
	prefs.MinConfidence = value
	return s.repo.UpdateScanning(ctx, prefs)
}

func (s *Service) SetScanInterval(ctx context.Context, userID int, minutes int) error {
	if minutes < 1 || minutes > 60 {
		return fmt.Errorf("scan interval must be between 1 and 60 minutes")
	}
	prefs, err := s.GetScanning(ctx, userID)
	if err != nil {
		return err
	}
	prefs.ScanIntervalMins = minutes
	return s.repo.UpdateScanning(ctx, prefs)
}

func (s *Service) ToggleScanning(ctx context.Context, userID int, enabled bool) error {
	prefs, err := s.GetScanning(ctx, userID)
	if err != nil {
		return err
	}
	prefs.IsScanningEnabled = enabled
	return s.repo.UpdateScanning(ctx, prefs)
}

// notification preferences

func (s *Service) GetNotification(ctx context.Context, userID int) (*Notification, error) {
	prefs, err := s.repo.GetNotification(ctx, userID)
	if err != nil {
		return nil, err
	}
	if prefs == nil {
		return nil, fmt.Errorf("notification preferences not found, try /start first")
	}
	return prefs, nil
}

func (s *Service) SetMaxDailyNotifications(ctx context.Context, userID int, value int) error {
	if value < 1 || value > 100 {
		return fmt.Errorf("max daily notifications must be between 1 and 100")
	}
	prefs, err := s.GetNotification(ctx, userID)
	if err != nil {
		return err
	}
	prefs.MaxDailyNotifications = value
	return s.repo.UpdateNotification(ctx, prefs)
}

func (s *Service) SetTimezone(ctx context.Context, userID int, tz string) error {
	// basic validation
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return fmt.Errorf("timezone cannot be empty")
	}
	prefs, err := s.GetNotification(ctx, userID)
	if err != nil {
		return err
	}
	prefs.Timezone = tz
	return s.repo.UpdateNotification(ctx, prefs)
}

func (s *Service) SetDailySummaryHour(ctx context.Context, userID int, hour int) error {
	if hour < 0 || hour > 23 {
		return fmt.Errorf("hour must be between 0 and 23")
	}
	prefs, err := s.GetNotification(ctx, userID)
	if err != nil {
		return err
	}
	prefs.DailySummaryHour = hour
	return s.repo.UpdateNotification(ctx, prefs)
}

// trading preferences

func (s *Service) GetTrading(ctx context.Context, userID int) (*Trading, error) {
	prefs, err := s.repo.GetTrading(ctx, userID)
	if err != nil {
		return nil, err
	}
	if prefs == nil {
		return nil, fmt.Errorf("trading preferences not found, try /start first")
	}
	return prefs, nil
}

func (s *Service) SetPositionSize(ctx context.Context, userID int, defaultSize, maxSize float64) error {
	if defaultSize <= 0 {
		return fmt.Errorf("default position size must be greater than 0")
	}
	if maxSize < defaultSize {
		return fmt.Errorf("max position size cannot be less than default")
	}
	prefs, err := s.GetTrading(ctx, userID)
	if err != nil {
		return err
	}
	prefs.DefaultPositionSize = defaultSize
	prefs.MaxPositionSize = maxSize
	return s.repo.UpdateTrading(ctx, prefs)
}

func (s *Service) SetStopLoss(ctx context.Context, userID int, pct float64) error {
	if pct <= 0 || pct > 50 {
		return fmt.Errorf("stop loss must be between 0.1%% and 50%%")
	}
	prefs, err := s.GetTrading(ctx, userID)
	if err != nil {
		return err
	}
	prefs.DefaultStopLossPct = pct
	return s.repo.UpdateTrading(ctx, prefs)
}

func (s *Service) SetTakeProfit(ctx context.Context, userID int, pct float64) error {
	if pct <= 0 || pct > 100 {
		return fmt.Errorf("take profit must be between 0.1%% and 100%%")
	}
	prefs, err := s.GetTrading(ctx, userID)
	if err != nil {
		return err
	}
	prefs.DefaultTakeProfitPct = pct
	return s.repo.UpdateTrading(ctx, prefs)
}

func (s *Service) SetMaxLeverage(ctx context.Context, userID int, leverage int) error {
	if leverage < 1 || leverage > 125 {
		return fmt.Errorf("leverage must be between 1 and 125")
	}
	prefs, err := s.GetTrading(ctx, userID)
	if err != nil {
		return err
	}
	prefs.MaxLeverage = leverage
	return s.repo.UpdateTrading(ctx, prefs)
}

func (s *Service) SetRiskPerTrade(ctx context.Context, userID int, pct float64) error {
	if pct <= 0 || pct > 25 {
		return fmt.Errorf("risk per trade must be between 0.1%% and 25%%")
	}
	prefs, err := s.GetTrading(ctx, userID)
	if err != nil {
		return err
	}
	prefs.RiskPerTradePct = pct
	return s.repo.UpdateTrading(ctx, prefs)
}
