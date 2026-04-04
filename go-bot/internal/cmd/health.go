// health check HTTP endpoint for container orchestration and monitoring
package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type healthStatus struct {
	Status   string            `json:"status"`
	Uptime   string            `json:"uptime"`
	Services map[string]string `json:"services"`
}

type healthServer struct {
	pg      *pgxpool.Pool
	redis   *redis.Client
	startAt time.Time
}

func newHealthServer(pg *pgxpool.Pool, redis *redis.Client) *healthServer {
	return &healthServer{
		pg:      pg,
		redis:   redis,
		startAt: time.Now(),
	}
}

func (h *healthServer) start(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/health/ready", h.handleReady)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			// health server failed but don't kill the main process
		}
	}()

	return srv
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
		} else {
			status.Services["postgres"] = "up"
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
