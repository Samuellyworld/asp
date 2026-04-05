// health check HTTP endpoint for container orchestration and monitoring
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/trading-bot/go-bot/internal/database"
)

type healthStatus struct {
	Status   string            `json:"status"`
	Uptime   string            `json:"uptime"`
	Services map[string]string `json:"services"`
}

type healthServer struct {
	pg         *pgxpool.Pool
	redis      *redis.Client
	dbBreaker  *database.DBCircuitBreaker
	startAt    time.Time
	binanceURL string // optional: Binance API base URL for health check
	mlURL      string // optional: ML service base URL for health check
}

func newHealthServer(pg *pgxpool.Pool, redis *redis.Client) *healthServer {
	return &healthServer{
		pg:      pg,
		redis:   redis,
		startAt: time.Now(),
	}
}

func (h *healthServer) SetBinanceURL(url string) { h.binanceURL = url }
func (h *healthServer) SetMLURL(url string)      { h.mlURL = url }
func (h *healthServer) SetDBBreaker(b *database.DBCircuitBreaker) { h.dbBreaker = b }

func (h *healthServer) start(addr string) (*http.ServeMux, *http.Server) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/health/ready", h.handleReady)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			// health server failed but don't kill the main process
		}
	}()

	return mux, srv
}

// liveness probe — always returns 200 if the process is up
func (h *healthServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := healthStatus{
		Status:   "ok",
		Uptime:   time.Since(h.startAt).Round(time.Second).String(),
		Services: make(map[string]string),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}

// readiness probe — checks postgres and redis connectivity
func (h *healthServer) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	status := healthStatus{
		Status:   "ok",
		Uptime:   time.Since(h.startAt).Round(time.Second).String(),
		Services: make(map[string]string),
	}

	allHealthy := true

	// check postgres
	if h.pg != nil {
		if err := h.pg.Ping(ctx); err != nil {
			status.Services["postgres"] = "down"
			allHealthy = false
			if h.dbBreaker != nil {
				h.dbBreaker.RecordFailure()
			}
		} else {
			status.Services["postgres"] = "up"
			if h.dbBreaker != nil {
				h.dbBreaker.RecordSuccess()
			}
		}
	}

	// check redis
	if h.redis != nil {
		if err := h.redis.Ping(ctx).Err(); err != nil {
			status.Services["redis"] = "down"
			allHealthy = false
		} else {
			status.Services["redis"] = "up"
		}
	}

	// check binance api
	if h.binanceURL != "" {
		if err := httpPing(ctx, h.binanceURL+"/api/v3/ping"); err != nil {
			status.Services["binance"] = "down"
		} else {
			status.Services["binance"] = "up"
		}
	}

	// check ml service
	if h.mlURL != "" {
		if err := httpPing(ctx, h.mlURL+"/health"); err != nil {
			status.Services["ml_service"] = "down"
		} else {
			status.Services["ml_service"] = "up"
		}
	}

	if !allHealthy {
		status.Status = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	if allHealthy {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(status)
}

// httpPing performs a quick GET request to check if a service is reachable
func httpPing(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
