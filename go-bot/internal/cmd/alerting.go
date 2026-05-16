package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/trading-bot/go-bot/internal/config"
	"github.com/trading-bot/go-bot/internal/opsalert"
)

func buildOpsAlertManager(cfg *config.Config) *opsalert.Manager {
	if cfg == nil || !cfg.Alerting.Enabled {
		return nil
	}

	manager := opsalert.NewManager(time.Duration(cfg.Alerting.DedupMinutes) * time.Minute)

	if cfg.Alerting.Email.Enabled {
		email, err := opsalert.NewEmailNotifier(opsalert.EmailConfig{
			Host:     cfg.Alerting.Email.SMTPHost,
			Port:     cfg.Alerting.Email.SMTPPort,
			Username: cfg.Alerting.Email.Username,
			Password: cfg.Alerting.Email.Password,
			From:     cfg.Alerting.Email.From,
			To:       cfg.Alerting.Email.To,
		})
		if err != nil {
			slog.Warn("email alerting disabled due to invalid config", "error", err)
		} else {
			manager.AddNotifier(email)
			slog.Info("email operational alerting enabled", "recipients", len(cfg.Alerting.Email.To))
		}
	}

	return manager
}

func addTelegramOpsAlerts(manager *opsalert.Manager, bot interface {
	SendMessage(chatID int64, text string) error
}, chatID int64) {
	if manager == nil || bot == nil || chatID == 0 {
		return
	}
	manager.AddNotifier(opsalert.NotifierFunc(func(ctx context.Context, alert opsalert.Alert) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		return bot.SendMessage(chatID, fmt.Sprintf("🚨 Operational Alert\n\n%s", opsalert.FormatText(alert)))
	}))
}

func opsalertSeverityForMismatch(mismatchType string) opsalert.Severity {
	switch mismatchType {
	case "orphaned_sl_tp":
		return opsalert.SeverityCritical
	case "partial_fill", "quantity_mismatch", "stale_order":
		return opsalert.SeverityWarning
	default:
		return opsalert.SeverityWarning
	}
}
