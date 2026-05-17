// infrastructure watchdog — periodically checks dependencies and sends alerts on failure
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/trading-bot/go-bot/internal/opsalert"
)

// InfraAlertSender can send an alert message to a user or broadcast channel.
type InfraAlertSender interface {
	SendMessage(chatID int64, text string) error
}

type operationalAlerter interface {
	Notify(ctx context.Context, alert opsalert.Alert) error
}

type scannerStatus interface {
	IsRunning() bool
	LastCycleAt() time.Time
}

// InfraWatchdog runs background checks against core infrastructure
// and sends alerts when consecutive failures exceed a threshold.
type InfraWatchdog struct {
	pg       *pgxpool.Pool
	redis    *redis.Client
	sender   InfraAlertSender
	alerter  operationalAlerter
	chatID   int64 // admin chat to alert (0 = skip telegram)
	interval time.Duration

	mu             sync.Mutex
	pgFails        int
	redisFails     int
	mlFails        int
	rustFails      int
	scannerFails   int
	pgAlerted      bool
	redisAlerted   bool
	mlAlerted      bool
	rustAlerted    bool
	scannerAlerted bool
	failThreshold  int
	mlURL          string
	rustAddress    string
	scanner        scannerStatus
	scannerSetAt   time.Time
	scannerStale   time.Duration
	stopCh         chan struct{}
	done           chan struct{}
}

func NewInfraWatchdog(pg *pgxpool.Pool, redis *redis.Client, interval time.Duration) *InfraWatchdog {
	return &InfraWatchdog{
		pg:            pg,
		redis:         redis,
		interval:      interval,
		failThreshold: 3,
		stopCh:        make(chan struct{}),
		done:          make(chan struct{}),
	}
}

func (w *InfraWatchdog) SetAlertSender(sender InfraAlertSender, chatID int64) {
	w.sender = sender
	w.chatID = chatID
}

func (w *InfraWatchdog) SetAlerter(alerter operationalAlerter) {
	w.alerter = alerter
}

func (w *InfraWatchdog) SetMLURL(url string) {
	w.mlURL = url
}

func (w *InfraWatchdog) SetRustAddress(addr string) {
	w.rustAddress = addr
}

func (w *InfraWatchdog) SetScanner(status scannerStatus, staleAfter time.Duration) {
	w.scanner = status
	w.scannerSetAt = time.Now()
	w.scannerStale = staleAfter
}

func (w *InfraWatchdog) Start(ctx context.Context) {
	go w.loop(ctx)
}

func (w *InfraWatchdog) Stop() {
	close(w.stopCh)
	<-w.done
}

func (w *InfraWatchdog) loop(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.check(ctx)
		}
	}
}

func (w *InfraWatchdog) check(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// postgres
	if w.pg != nil {
		if err := w.pg.Ping(checkCtx); err != nil {
			w.recordFailure("postgres", err, &w.pgFails, &w.pgAlerted)
		} else {
			w.recordRecovery("postgres", &w.pgFails, &w.pgAlerted)
		}
	}

	// redis
	if w.redis != nil {
		if err := w.redis.Ping(checkCtx).Err(); err != nil {
			w.recordFailure("redis", err, &w.redisFails, &w.redisAlerted)
		} else {
			w.recordRecovery("redis", &w.redisFails, &w.redisAlerted)
		}
	}

	// python ML service
	if w.mlURL != "" {
		if err := httpPing(checkCtx, w.mlURL+"/health"); err != nil {
			w.recordFailure("ml_service", err, &w.mlFails, &w.mlAlerted)
		} else {
			w.recordRecovery("ml_service", &w.mlFails, &w.mlAlerted)
		}
	}

	// rust indicator engine
	if w.rustAddress != "" {
		if err := tcpPing(checkCtx, w.rustAddress); err != nil {
			w.recordFailure("rust_engine", err, &w.rustFails, &w.rustAlerted)
		} else {
			w.recordRecovery("rust_engine", &w.rustFails, &w.rustAlerted)
		}
	}

	// scanner staleness
	if w.scanner != nil {
		if err := w.checkScannerFreshness(); err != nil {
			w.recordFailure("scanner", err, &w.scannerFails, &w.scannerAlerted)
		} else {
			w.recordRecovery("scanner", &w.scannerFails, &w.scannerAlerted)
		}
	}
}

func (w *InfraWatchdog) recordFailure(service string, err error, fails *int, alerted *bool) {
	w.mu.Lock()
	*fails++
	failCount := *fails
	shouldAlert := false
	if failCount >= w.failThreshold && !*alerted {
		*alerted = true
		shouldAlert = true
	}
	w.mu.Unlock()

	slog.Warn("infra check failed", "service", service, "consecutive_failures", failCount, "error", err)

	if shouldAlert {
		msg := fmt.Sprintf("🔴 INFRA ALERT: %s is DOWN (%d consecutive failures)\nError: %s", service, failCount, err)
		infraAlerts.WithLabelValues(service, "down").Inc()
		w.alert(service, "down", opsalert.SeverityCritical, msg, err)
	}
}

func (w *InfraWatchdog) recordRecovery(service string, fails *int, alerted *bool) {
	w.mu.Lock()
	wasAlerted := *alerted
	failCount := *fails
	*fails = 0
	*alerted = false
	w.mu.Unlock()

	if wasAlerted {
		msg := fmt.Sprintf("🟢 INFRA RECOVERY: %s is back UP (was down for %d checks)", service, failCount)
		infraAlerts.WithLabelValues(service, "recovery").Inc()
		w.alert(service, "recovery", opsalert.SeverityInfo, msg, nil)
	}
}

func (w *InfraWatchdog) alert(service, alertType string, severity opsalert.Severity, msg string, err error) {
	slog.Error("infra alert", "message", msg)
	if w.sender != nil && w.chatID != 0 {
		if err := w.sender.SendMessage(w.chatID, msg); err != nil {
			slog.Warn("failed to send infra alert", "error", err)
		}
	}
	if w.alerter != nil {
		alert := opsalert.Alert{
			Key:       fmt.Sprintf("infra:%s:%s", service, alertType),
			Severity:  severity,
			Component: "infra_watchdog",
			Title:     fmt.Sprintf("%s %s", service, alertType),
			Summary:   msg,
			Fields: map[string]string{
				"service": service,
				"type":    alertType,
			},
		}
		if err != nil {
			alert.Error = err.Error()
		}
		if notifyErr := w.alerter.Notify(context.Background(), alert); notifyErr != nil {
			slog.Warn("failed to send operational alert", "service", service, "error", notifyErr)
		}
	}
}

func (w *InfraWatchdog) checkScannerFreshness() error {
	if !w.scanner.IsRunning() {
		return fmt.Errorf("scanner is not running")
	}
	staleAfter := w.scannerStale
	if staleAfter <= 0 {
		staleAfter = 15 * time.Minute
	}
	last := w.scanner.LastCycleAt()
	if last.IsZero() {
		if time.Since(w.scannerSetAt) > staleAfter {
			return fmt.Errorf("scanner has not completed an initial cycle after %s", time.Since(w.scannerSetAt).Round(time.Second))
		}
		return nil
	}
	if age := time.Since(last); age > staleAfter {
		return fmt.Errorf("scanner stale: last completed cycle %s ago", age.Round(time.Second))
	}
	return nil
}

func tcpPing(ctx context.Context, addr string) error {
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return conn.Close()
}
