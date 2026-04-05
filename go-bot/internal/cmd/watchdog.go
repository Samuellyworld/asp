// infrastructure watchdog — periodically checks dependencies and sends alerts on failure
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// InfraAlertSender can send an alert message to a user or broadcast channel.
type InfraAlertSender interface {
	SendMessage(chatID int64, text string) error
}

// InfraWatchdog runs background checks against core infrastructure
// and sends alerts when consecutive failures exceed a threshold.
type InfraWatchdog struct {
	pg       *pgxpool.Pool
	redis    *redis.Client
	sender   InfraAlertSender
	chatID   int64 // admin chat to alert (0 = skip telegram)
	interval time.Duration

	mu             sync.Mutex
	pgFails        int
	redisFails     int
	pgAlerted      bool
	redisAlerted   bool
	failThreshold  int
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
}

func (w *InfraWatchdog) recordFailure(service string, err error, fails *int, alerted *bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	*fails++
	slog.Warn("infra check failed", "service", service, "consecutive_failures", *fails, "error", err)

	if *fails >= w.failThreshold && !*alerted {
		*alerted = true
		msg := fmt.Sprintf("🔴 INFRA ALERT: %s is DOWN (%d consecutive failures)\nError: %s", service, *fails, err)
		w.alert(msg)
	}
}

func (w *InfraWatchdog) recordRecovery(service string, fails *int, alerted *bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if *alerted {
		msg := fmt.Sprintf("🟢 INFRA RECOVERY: %s is back UP (was down for %d checks)", service, *fails)
		w.alert(msg)
	}
	*fails = 0
	*alerted = false
}

func (w *InfraWatchdog) alert(msg string) {
	slog.Error("infra alert", "message", msg)
	if w.sender != nil && w.chatID != 0 {
		if err := w.sender.SendMessage(w.chatID, msg); err != nil {
			slog.Warn("failed to send infra alert", "error", err)
		}
	}
}
