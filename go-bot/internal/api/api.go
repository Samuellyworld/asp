// analytics REST API — read-only HTTP endpoints for positions, trades, AI decisions, and stats.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/trading-bot/go-bot/internal/database"
)

// Server serves the analytics REST API.
type Server struct {
	positions *database.PositionRepository
	trades    *database.TradeRepository
	decisions *database.AIDecisionRepository
	stats     *database.DailyStatsRepository
	candles   *database.CandleRepository
	apiKey    string
}

// NewServer creates a new analytics API server.
func NewServer(
	positions *database.PositionRepository,
	trades *database.TradeRepository,
	decisions *database.AIDecisionRepository,
	stats *database.DailyStatsRepository,
	candles *database.CandleRepository,
	apiKey string,
) *Server {
	return &Server{
		positions: positions,
		trades:    trades,
		decisions: decisions,
		stats:     stats,
		candles:   candles,
		apiKey:    apiKey,
	}
}

// RegisterRoutes adds all API routes to the given mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/positions", s.auth(s.handlePositions))
	mux.HandleFunc("/api/positions/", s.auth(s.handlePositionByID))
	mux.HandleFunc("/api/trades", s.auth(s.handleTrades))
	mux.HandleFunc("/api/decisions", s.auth(s.handleDecisions))
	mux.HandleFunc("/api/stats/daily", s.auth(s.handleDailyStats))
	mux.HandleFunc("/api/stats/summary", s.auth(s.handleSummary))
	mux.HandleFunc("/api/candles", s.auth(s.handleCandles))
}

// auth wraps a handler with API key authentication.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "only GET allowed")
			return
		}

		if s.apiKey != "" {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				key = r.URL.Query().Get("api_key")
			}
			if key != s.apiKey {
				writeError(w, http.StatusUnauthorized, "invalid or missing API key")
				return
			}
		}

		next(w, r)
	}
}

// GET /api/positions?user_id=1&status=OPEN&limit=50
func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	userID, err := requiredIntParam(r, "user_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "user_id is required and must be an integer")
		return
	}

	status := strings.ToUpper(r.URL.Query().Get("status"))
	if status != "" && status != "OPEN" && status != "CLOSED" && status != "LIQUIDATED" {
		writeError(w, http.StatusBadRequest, "status must be OPEN, CLOSED, or LIQUIDATED")
		return
	}

	limit := intParam(r, "limit", 100)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	positions, err := s.positions.ListByUser(ctx, userID, status, limit)
	if err != nil {
		slog.Error("api: list positions", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list positions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"positions": positionsToAPI(positions),
		"count":     len(positions),
	})
}

// GET /api/positions/123
func (s *Server) handlePositionByID(w http.ResponseWriter, r *http.Request) {
	// parse ID from path: /api/positions/123
	path := strings.TrimPrefix(r.URL.Path, "/api/positions/")
	id, err := strconv.Atoi(path)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid position ID")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	pos, err := s.positions.GetByID(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			writeError(w, http.StatusNotFound, "position not found")
			return
		}
		slog.Error("api: get position", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "failed to get position")
		return
	}

	writeJSON(w, http.StatusOK, positionToAPI(pos))
}

// GET /api/trades?user_id=1&from=2024-01-01&to=2024-12-31
func (s *Server) handleTrades(w http.ResponseWriter, r *http.Request) {
	userID, err := requiredIntParam(r, "user_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "user_id is required and must be an integer")
		return
	}

	from, to := parseDateRangeParams(r, 30) // default: last 30 days

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	trades, err := s.trades.ListByUser(ctx, userID, from, to)
	if err != nil {
		slog.Error("api: list trades", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list trades")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"trades": tradesToAPI(trades),
		"count":  len(trades),
		"from":   from.Format("2006-01-02"),
		"to":     to.Format("2006-01-02"),
	})
}

// GET /api/decisions?user_id=1&symbol=BTC/USDT&limit=20
func (s *Server) handleDecisions(w http.ResponseWriter, r *http.Request) {
	userID, err := requiredIntParam(r, "user_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "user_id is required and must be an integer")
		return
	}

	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		writeError(w, http.StatusBadRequest, "symbol is required")
		return
	}

	limit := intParam(r, "limit", 20)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	decisions, err := s.decisions.RecentBySymbol(ctx, userID, symbol, limit)
	if err != nil {
		slog.Error("api: list decisions", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list decisions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"decisions": decisionsToAPI(decisions),
		"count":     len(decisions),
	})
}

// GET /api/stats/daily?user_id=1&from=2024-01-01&to=2024-12-31
func (s *Server) handleDailyStats(w http.ResponseWriter, r *http.Request) {
	userID, err := requiredIntParam(r, "user_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "user_id is required and must be an integer")
		return
	}

	from, to := parseDateRangeParams(r, 30)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats, err := s.stats.GetRange(ctx, userID, from, to)
	if err != nil {
		slog.Error("api: daily stats", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get daily stats")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"stats": dailyStatsToAPI(stats),
		"count": len(stats),
		"from":  from.Format("2006-01-02"),
		"to":    to.Format("2006-01-02"),
	})
}

// GET /api/stats/summary?user_id=1
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	userID, err := requiredIntParam(r, "user_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "user_id is required and must be an integer")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// get all-time stats
	allTimeFrom := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	allTimeTo := time.Now().AddDate(0, 0, 1)

	stats, err := s.stats.GetRange(ctx, userID, allTimeFrom, allTimeTo)
	if err != nil {
		slog.Error("api: summary stats", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	// aggregate
	var totalTrades, wins, losses int
	var totalPnL, totalFees, totalFunding float64
	var tradingDays int

	for _, s := range stats {
		totalTrades += s.TotalTrades
		wins += s.WinningTrades
		losses += s.LosingTrades
		totalPnL += s.RealizedPnL
		totalFees += s.FeesPaid
		totalFunding += s.FundingPaid
		if s.TotalTrades > 0 {
			tradingDays++
		}
	}

	var winRate float64
	if totalTrades > 0 {
		winRate = float64(wins) / float64(totalTrades) * 100
	}

	// today's decisions
	todayTotal, todayApproved, _ := s.decisions.CountByUserToday(ctx, userID)

	// open positions count
	openPositions, _ := s.positions.ListByUser(ctx, userID, "OPEN", 500)

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":         userID,
		"total_trades":    totalTrades,
		"winning_trades":  wins,
		"losing_trades":   losses,
		"win_rate":        winRate,
		"total_pnl":       totalPnL,
		"total_fees":      totalFees,
		"total_funding":   totalFunding,
		"net_pnl":         totalPnL - totalFees - totalFunding,
		"trading_days":    tradingDays,
		"open_positions":  len(openPositions),
		"today_decisions": todayTotal,
		"today_approved":  todayApproved,
	})
}

// GET /api/candles?symbol=BTC/USDT&interval=4h&from=2024-01-01&to=2024-12-31
func (s *Server) handleCandles(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		writeError(w, http.StatusBadRequest, "symbol is required")
		return
	}
	interval := r.URL.Query().Get("interval")
	if interval == "" {
		interval = "4h"
	}

	from, to := parseDateRangeParams(r, 7) // default: last 7 days

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	candles, err := s.candles.GetRange(ctx, symbol, interval, from, to)
	if err != nil {
		slog.Error("api: candles", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get candles")
		return
	}

	apiCandles := make([]map[string]any, len(candles))
	for i, c := range candles {
		apiCandles[i] = map[string]any{
			"time":         c.Time.Format(time.RFC3339),
			"open":         c.Open,
			"high":         c.High,
			"low":          c.Low,
			"close":        c.Close,
			"volume":       c.Volume,
			"quote_volume": c.QuoteVolume,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"candles": apiCandles,
		"count":   len(apiCandles),
		"symbol":  symbol,
		"interval": interval,
	})
}

// --- response helpers ---

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// --- parameter parsing ---

func requiredIntParam(r *http.Request, name string) (int, error) {
	s := r.URL.Query().Get(name)
	return strconv.Atoi(s)
}

func intParam(r *http.Request, name string, defaultVal int) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return defaultVal
	}
	return v
}

func parseDateRangeParams(r *http.Request, defaultDays int) (time.Time, time.Time) {
	layout := "2006-01-02"
	now := time.Now()

	from := now.AddDate(0, 0, -defaultDays)
	to := now.AddDate(0, 0, 1) // include today

	if s := r.URL.Query().Get("from"); s != "" {
		if t, err := time.Parse(layout, s); err == nil {
			from = t
		}
	}
	if s := r.URL.Query().Get("to"); s != "" {
		if t, err := time.Parse(layout, s); err == nil {
			to = t
		}
	}

	return from, to
}

// --- model converters ---

func positionsToAPI(positions []*database.PersistedPosition) []map[string]any {
	result := make([]map[string]any, len(positions))
	for i, p := range positions {
		result[i] = positionToAPI(p)
	}
	return result
}

func positionToAPI(p *database.PersistedPosition) map[string]any {
	m := map[string]any{
		"id":            p.ID,
		"internal_id":   p.InternalID,
		"user_id":       p.UserID,
		"symbol":        p.Symbol,
		"side":          p.Side,
		"position_type": p.PositionType,
		"status":        p.Status,
		"entry_price":   p.EntryPrice,
		"current_price": p.CurrentPrice,
		"quantity":      p.Quantity,
		"position_size": p.PositionSize,
		"leverage":      p.Leverage,
		"is_paper":      p.IsPaper,
		"platform":      p.Platform,
		"opened_at":     p.OpenedAt.Format(time.RFC3339),
	}

	if p.Action != "" {
		m["action"] = p.Action
	}
	if p.StopLoss != 0 {
		m["stop_loss"] = p.StopLoss
	}
	if p.TakeProfit != 0 {
		m["take_profit"] = p.TakeProfit
	}
	if p.MarkPrice != 0 {
		m["mark_price"] = p.MarkPrice
	}
	if p.ClosePrice != 0 {
		m["close_price"] = p.ClosePrice
	}
	if p.LiquidationPrice != 0 {
		m["liquidation_price"] = p.LiquidationPrice
	}
	if p.Margin != 0 {
		m["margin"] = p.Margin
	}
	if p.UnrealizedPnL != 0 {
		m["unrealized_pnl"] = p.UnrealizedPnL
	}
	if p.RealizedPnL != 0 {
		m["realized_pnl"] = p.RealizedPnL
	}
	if p.FundingPaid != 0 {
		m["funding_paid"] = p.FundingPaid
	}
	if p.CloseReason != "" {
		m["close_reason"] = p.CloseReason
	}
	if p.ClosedAt != nil {
		m["closed_at"] = p.ClosedAt.Format(time.RFC3339)
	}

	return m
}

func tradesToAPI(trades []*database.TradeRecord) []map[string]any {
	result := make([]map[string]any, len(trades))
	for i, t := range trades {
		m := map[string]any{
			"id":          t.ID,
			"user_id":     t.UserID,
			"symbol":      t.Symbol,
			"side":        t.Side,
			"trade_type":  t.TradeType,
			"quantity":    t.Quantity,
			"price":       t.Price,
			"fee":         t.Fee,
			"is_paper":    t.IsPaper,
			"executed_at": t.ExecutedAt.Format(time.RFC3339),
		}
		if t.PositionID != nil {
			m["position_id"] = *t.PositionID
		}
		if t.ExchangeOrderID != "" {
			m["exchange_order_id"] = t.ExchangeOrderID
		}
		if t.FeeCurrency != "" {
			m["fee_currency"] = t.FeeCurrency
		}
		result[i] = m
	}
	return result
}

func decisionsToAPI(decisions []*database.AIDecisionRecord) []map[string]any {
	result := make([]map[string]any, len(decisions))
	for i, d := range decisions {
		m := map[string]any{
			"id":           d.ID,
			"user_id":      d.UserID,
			"symbol":       d.Symbol,
			"decision":     d.Decision,
			"confidence":   d.Confidence,
			"was_executed": d.WasExecuted,
			"created_at":   d.CreatedAt.Format(time.RFC3339),
		}
		if d.Timeframe != "" {
			m["timeframe"] = d.Timeframe
		}
		if d.EntryPrice != 0 {
			m["entry_price"] = d.EntryPrice
		}
		if d.StopLoss != 0 {
			m["stop_loss"] = d.StopLoss
		}
		if d.TakeProfit != 0 {
			m["take_profit"] = d.TakeProfit
		}
		if d.PositionSizeUSD != 0 {
			m["position_size_usd"] = d.PositionSizeUSD
		}
		if d.RiskRewardRatio != 0 {
			m["risk_reward_ratio"] = d.RiskRewardRatio
		}
		if d.Reasoning != "" {
			m["reasoning"] = d.Reasoning
		}
		if d.PromptTokens > 0 {
			m["prompt_tokens"] = d.PromptTokens
		}
		if d.CompletionTokens > 0 {
			m["completion_tokens"] = d.CompletionTokens
		}
		if d.LatencyMs > 0 {
			m["latency_ms"] = d.LatencyMs
		}
		if d.WasApproved != nil {
			m["was_approved"] = *d.WasApproved
		}
		if len(d.IndicatorsData) > 0 {
			m["indicators"] = d.IndicatorsData
		}
		if len(d.MLPrediction) > 0 {
			m["ml_prediction"] = d.MLPrediction
		}
		if len(d.SentimentData) > 0 {
			m["sentiment"] = d.SentimentData
		}
		result[i] = m
	}
	return result
}

func dailyStatsToAPI(stats []*database.DailyStatsRecord) []map[string]any {
	result := make([]map[string]any, len(stats))
	for i, s := range stats {
		result[i] = map[string]any{
			"date":                s.Date.Format("2006-01-02"),
			"total_trades":        s.TotalTrades,
			"winning_trades":      s.WinningTrades,
			"losing_trades":       s.LosingTrades,
			"realized_pnl":        s.RealizedPnL,
			"unrealized_pnl":      s.UnrealizedPnL,
			"fees_paid":           s.FeesPaid,
			"funding_paid":        s.FundingPaid,
			"ai_decisions_made":   s.AIDecisionsMade,
			"ai_decisions_approved": s.AIDecisionsApproved,
			"notifications_sent":  s.NotificationsSent,
		}
	}
	return result
}
