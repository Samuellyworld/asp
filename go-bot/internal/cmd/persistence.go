// adapters that bridge database.PositionRepository to the PositionStore and
// LeveragePositionStore interfaces required by the paper trading executors.
// also provides startup recovery logic to reload open positions from the database.
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/database"
	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/leverage"
	"github.com/trading-bot/go-bot/internal/livetrading"
	"github.com/trading-bot/go-bot/internal/papertrading"
	"github.com/trading-bot/go-bot/internal/pipeline"
)

// implements papertrading.PositionStore by wrapping PositionRepository
type spotPositionStoreAdapter struct {
	repo *database.PositionRepository
}

func (a *spotPositionStoreAdapter) SavePosition(ctx context.Context, pos *papertrading.Position) error {
	p := &database.PersistedPosition{
		InternalID:   pos.ID,
		UserID:       pos.UserID,
		Symbol:       pos.Symbol,
		Side:         spotSide(pos.Action),
		Action:       string(pos.Action),
		PositionType: "SPOT",
		Status:       "OPEN",
		EntryPrice:   pos.EntryPrice,
		CurrentPrice: pos.CurrentPrice,
		Quantity:     pos.Quantity,
		PositionSize: pos.PositionSize,
		StopLoss:     pos.StopLoss,
		TakeProfit:   pos.TakeProfit,
		IsPaper:      true,
		Platform:     pos.Platform,
		OpenedAt:     pos.OpenedAt,
	}
	return a.repo.Insert(ctx, p)
}

func (a *spotPositionStoreAdapter) ClosePosition(ctx context.Context, pos *papertrading.Position) error {
	closedAt := time.Now()
	if pos.ClosedAt != nil {
		closedAt = *pos.ClosedAt
	}
	return a.repo.UpdateClose(ctx, pos.ID, "CLOSED", string(pos.CloseReason),
		pos.ClosePrice, pos.PnL(), 0, closedAt)
}

func (a *spotPositionStoreAdapter) AdjustPosition(ctx context.Context, posID string, sl, tp float64) error {
	return a.repo.UpdateSLTP(ctx, posID, sl, tp)
}

func spotSide(action claude.Action) string {
	switch action {
	case claude.ActionBuy:
		return "LONG"
	default:
		return "SHORT"
	}
}

// implements leverage.LeveragePositionStore by wrapping PositionRepository
type leveragePositionStoreAdapter struct {
	repo *database.PositionRepository
}

func (a *leveragePositionStoreAdapter) SavePosition(ctx context.Context, pos *leverage.LeveragePosition) error {
	p := &database.PersistedPosition{
		InternalID:       pos.ID,
		UserID:           pos.UserID,
		Symbol:           pos.Symbol,
		Side:             string(pos.Side),
		PositionType:     "FUTURES",
		Status:           "OPEN",
		EntryPrice:       pos.EntryPrice,
		MarkPrice:        pos.MarkPrice,
		Quantity:         pos.Quantity,
		Margin:           pos.Margin,
		NotionalValue:    pos.NotionalValue,
		Leverage:         pos.Leverage,
		StopLoss:         pos.StopLoss,
		TakeProfit:       pos.TakeProfit,
		LiquidationPrice: pos.LiquidationPrice,
		FundingPaid:      pos.FundingPaid,
		MarginType:       pos.MarginType,
		IsPaper:          true,
		Platform:         pos.Platform,
		OpenedAt:         pos.OpenedAt,
	}
	return a.repo.Insert(ctx, p)
}

func (a *leveragePositionStoreAdapter) ClosePosition(ctx context.Context, pos *leverage.LeveragePosition) error {
	closedAt := time.Now()
	if pos.ClosedAt != nil {
		closedAt = *pos.ClosedAt
	}
	return a.repo.UpdateClose(ctx, pos.ID, "CLOSED", pos.CloseReason,
		pos.ClosePrice, pos.PnL, pos.FundingPaid, closedAt)
}

func (a *leveragePositionStoreAdapter) AdjustPosition(ctx context.Context, posID string, sl, tp float64) error {
	return a.repo.UpdateSLTP(ctx, posID, sl, tp)
}

// recoverSpotPositions loads open paper SPOT positions from the database
// and restores them into the executor's in-memory map.
func recoverSpotPositions(ctx context.Context, repo *database.PositionRepository, executor *papertrading.Executor) error {
	rows, err := repo.LoadOpenPaperSpot(ctx)
	if err != nil {
		return fmt.Errorf("failed to load open spot positions: %w", err)
	}

	for _, r := range rows {
		pos := &papertrading.Position{
			ID:            r.InternalID,
			UserID:        r.UserID,
			Symbol:        r.Symbol,
			Action:        claude.Action(r.Action),
			EntryPrice:    r.EntryPrice,
			CurrentPrice:  r.CurrentPrice,
			Quantity:      r.Quantity,
			StopLoss:      r.StopLoss,
			TakeProfit:    r.TakeProfit,
			PositionSize:  r.PositionSize,
			Status:        papertrading.PositionOpen,
			OpenedAt:      r.OpenedAt,
			Platform:      r.Platform,
			HitMilestones: make(map[float64]bool),
			LastNotified:  time.Now(),
		}
		executor.RestorePosition(pos)
		slog.Info("recovered spot paper position", "id", r.InternalID, "symbol", r.Symbol)
	}

	// resume ID generation
	maxID, err := repo.MaxInternalID(ctx, "pt_")
	if err != nil {
		return fmt.Errorf("failed to get max spot position ID: %w", err)
	}
	executor.SetNextID(maxID + 1)

	if len(rows) > 0 {
		slog.Info("spot paper position recovery complete", "count", len(rows))
	}
	return nil
}

// recoverLeveragePositions loads open paper FUTURES positions from the database
// and restores them into the leverage executor's in-memory map.
func recoverLeveragePositions(ctx context.Context, repo *database.PositionRepository, executor *leverage.PaperExecutor) error {
	rows, err := repo.LoadOpenPaperFutures(ctx)
	if err != nil {
		return fmt.Errorf("failed to load open leverage positions: %w", err)
	}

	for _, r := range rows {
		pos := &leverage.LeveragePosition{
			ID:               r.InternalID,
			UserID:           r.UserID,
			Symbol:           r.Symbol,
			Side:             leverage.PositionSide(r.Side),
			Leverage:         r.Leverage,
			EntryPrice:       r.EntryPrice,
			MarkPrice:        r.MarkPrice,
			Quantity:         r.Quantity,
			Margin:           r.Margin,
			NotionalValue:    r.NotionalValue,
			LiquidationPrice: r.LiquidationPrice,
			StopLoss:         r.StopLoss,
			TakeProfit:       r.TakeProfit,
			FundingPaid:      r.FundingPaid,
			MarginType:       r.MarginType,
			IsPaper:          true,
			Status:           "open",
			OpenedAt:         r.OpenedAt,
			Platform:         r.Platform,
		}
		executor.RestorePosition(pos)
		slog.Info("recovered leverage paper position", "id", r.InternalID, "symbol", r.Symbol)
	}

	// resume ID generation
	maxID, err := repo.MaxInternalID(ctx, "lp_")
	if err != nil {
		return fmt.Errorf("failed to get max leverage position ID: %w", err)
	}
	executor.SetNextID(maxID + 1)

	if len(rows) > 0 {
		slog.Info("leverage paper position recovery complete", "count", len(rows))
	}
	return nil
}

// parseIDSuffix extracts the numeric suffix from an internal ID like "pt_42".
func parseIDSuffix(internalID, prefix string) int {
	s := strings.TrimPrefix(internalID, prefix)
	n, _ := strconv.Atoi(s)
	return n
}

// decisionLoggerAdapter implements scanner.DecisionLogger by delegating
// to AIDecisionRepository and DailyStatsRepository (best-effort).
type decisionLoggerAdapter struct {
	decisions *database.AIDecisionRepository
	daily     *database.DailyStatsRepository
}

func (a *decisionLoggerAdapter) LogDecision(ctx context.Context, userID int, symbol string, result *pipeline.Result, filterReason string) int {
	if result == nil || result.Decision == nil {
		return 0
	}

	approved := filterReason == "none"
	wasApproved := &approved

	rec := &database.AIDecisionRecord{
		UserID:          userID,
		Symbol:          symbol,
		Decision:        string(result.Decision.Action),
		Confidence:      int(result.Decision.Confidence),
		EntryPrice:      result.Decision.Plan.Entry,
		StopLoss:        result.Decision.Plan.StopLoss,
		TakeProfit:      result.Decision.Plan.TakeProfit,
		PositionSizeUSD: result.Decision.Plan.PositionSize,
		RiskRewardRatio: result.Decision.Plan.RiskReward,
		Reasoning:       result.Decision.Reasoning,
		LatencyMs:       int(result.Latency.Milliseconds()),
		WasApproved:     wasApproved,
		WasExecuted:     false,
		FilterReason:    filterReason,
	}

	// populate indicator data if available
	if result.Indicators != nil {
		rec.IndicatorsData = map[string]interface{}{
			"overall_signal": result.Indicators.OverallSignal,
			"bullish_count":  result.Indicators.BullishCount,
			"bearish_count":  result.Indicators.BearishCount,
		}
		if result.Indicators.RSI != nil {
			rec.IndicatorsData["rsi"] = result.Indicators.RSI.Value
		}
	}

	// populate ML prediction if available
	if result.Prediction != nil {
		rec.MLPrediction = map[string]interface{}{
			"predicted_price": result.Prediction.PredictedPrice,
			"confidence":      result.Prediction.Confidence,
		}
	}

	// populate sentiment if available
	if result.Sentiment != nil {
		rec.SentimentData = map[string]interface{}{
			"label": result.Sentiment.Label,
			"score": result.Sentiment.Score,
		}
	}

	id, err := a.decisions.Insert(ctx, rec)
	if err != nil {
		slog.Error("failed to log ai decision", "symbol", symbol, "user_id", userID, "error", err)
		return 0
	}

	// increment daily stats counter (best-effort)
	if err := a.daily.IncrementDecision(ctx, userID, approved); err != nil {
		slog.Error("failed to increment decision stats", "user_id", userID, "error", err)
	}

	return id
}

func (a *decisionLoggerAdapter) IncrementNotification(ctx context.Context, userID int) {
	if err := a.daily.IncrementNotification(ctx, userID); err != nil {
		slog.Error("failed to increment notification stats", "user_id", userID, "error", err)
	}
}

// spotTradeLoggerAdapter implements papertrading.TradeLogger using TradeRepository + DailyStatsRepository.
type spotTradeLoggerAdapter struct {
	trades *database.TradeRepository
	daily  *database.DailyStatsRepository
}

func (a *spotTradeLoggerAdapter) LogOpen(ctx context.Context, pos *papertrading.Position) error {
	side := "BUY"
	if pos.Action == claude.ActionSell {
		side = "SELL"
	}
	rec := &database.TradeRecord{
		UserID:     pos.UserID,
		Symbol:     pos.Symbol,
		Side:       side,
		TradeType:  "SPOT",
		Quantity:   pos.Quantity,
		Price:      pos.EntryPrice,
		IsPaper:    true,
		ExecutedAt: pos.OpenedAt,
	}
	_, err := a.trades.Insert(ctx, rec)
	return err
}

func (a *spotTradeLoggerAdapter) LogClose(ctx context.Context, pos *papertrading.Position) error {
	side := "SELL"
	if pos.Action == claude.ActionSell {
		side = "BUY"
	}
	executedAt := time.Now()
	if pos.ClosedAt != nil {
		executedAt = *pos.ClosedAt
	}
	rec := &database.TradeRecord{
		UserID:     pos.UserID,
		Symbol:     pos.Symbol,
		Side:       side,
		TradeType:  "SPOT",
		Quantity:   pos.Quantity,
		Price:      pos.ClosePrice,
		IsPaper:    true,
		ExecutedAt: executedAt,
	}
	if _, err := a.trades.Insert(ctx, rec); err != nil {
		return err
	}

	// increment daily trade stats
	pnl := pos.PnL()
	isWin := pnl > 0
	if err := a.daily.IncrementTrade(ctx, pos.UserID, pnl, 0, isWin); err != nil {
		slog.Error("failed to increment trade stats", "user_id", pos.UserID, "error", err)
	}
	return nil
}

// leverageTradeLoggerAdapter implements leverage.LeverageTradeLogger using TradeRepository + DailyStatsRepository.
type leverageTradeLoggerAdapter struct {
	trades *database.TradeRepository
	daily  *database.DailyStatsRepository
}

func (a *leverageTradeLoggerAdapter) LogOpen(ctx context.Context, pos *leverage.LeveragePosition) error {
	side := "BUY"
	tradeType := "FUTURES_LONG"
	if pos.Side == leverage.SideShort {
		side = "SELL"
		tradeType = "FUTURES_SHORT"
	}
	rec := &database.TradeRecord{
		UserID:     pos.UserID,
		Symbol:     pos.Symbol,
		Side:       side,
		TradeType:  tradeType,
		Quantity:   pos.Quantity,
		Price:      pos.EntryPrice,
		IsPaper:    pos.IsPaper,
		ExecutedAt: pos.OpenedAt,
	}
	_, err := a.trades.Insert(ctx, rec)
	return err
}

func (a *leverageTradeLoggerAdapter) LogClose(ctx context.Context, pos *leverage.LeveragePosition) error {
	// closing side is opposite of opening side
	side := "SELL"
	tradeType := "FUTURES_LONG"
	if pos.Side == leverage.SideShort {
		side = "BUY"
		tradeType = "FUTURES_SHORT"
	}
	executedAt := time.Now()
	if pos.ClosedAt != nil {
		executedAt = *pos.ClosedAt
	}
	rec := &database.TradeRecord{
		UserID:     pos.UserID,
		Symbol:     pos.Symbol,
		Side:       side,
		TradeType:  tradeType,
		Quantity:   pos.Quantity,
		Price:      pos.ClosePrice,
		IsPaper:    pos.IsPaper,
		ExecutedAt: executedAt,
	}
	if _, err := a.trades.Insert(ctx, rec); err != nil {
		return err
	}

	// increment daily trade stats
	isWin := pos.PnL > 0
	if err := a.daily.IncrementTrade(ctx, pos.UserID, pos.PnL, pos.FundingPaid, isWin); err != nil {
		slog.Error("failed to increment leverage trade stats", "user_id", pos.UserID, "error", err)
	}
	return nil
}

// --- live trading persistence adapters ---

// implements livetrading.PositionStore by wrapping PositionRepository
type livePositionStoreAdapter struct {
	repo *database.PositionRepository
}

func (a *livePositionStoreAdapter) SavePosition(ctx context.Context, pos *livetrading.LivePosition) error {
	side := "LONG"
	if pos.Side == exchange.SideSell {
		side = "SHORT"
	}
	p := &database.PersistedPosition{
		InternalID:   pos.ID,
		UserID:       pos.UserID,
		Symbol:       pos.Symbol,
		Side:         side,
		PositionType: "SPOT",
		Status:       "OPEN",
		EntryPrice:   pos.EntryPrice,
		Quantity:     pos.Quantity,
		PositionSize: pos.PositionSize,
		StopLoss:     pos.StopLoss,
		TakeProfit:   pos.TakeProfit,
		IsPaper:      false,
		Platform:     pos.Platform,
		OpenedAt:     pos.OpenedAt,
	}
	return a.repo.Insert(ctx, p)
}

func (a *livePositionStoreAdapter) ClosePosition(ctx context.Context, pos *livetrading.LivePosition) error {
	closedAt := time.Now()
	if pos.ClosedAt != nil {
		closedAt = *pos.ClosedAt
	}
	return a.repo.UpdateClose(ctx, pos.ID, "CLOSED", pos.CloseReason,
		pos.ClosePrice, pos.PnL, 0, closedAt)
}

// implements livetrading.TradeLogger by wrapping TradeRepository + DailyStatsRepository
type liveTradeLoggerAdapter struct {
	trades *database.TradeRepository
	daily  *database.DailyStatsRepository
}

func (a *liveTradeLoggerAdapter) LogOpen(ctx context.Context, pos *livetrading.LivePosition) error {
	side := "BUY"
	if pos.Side == exchange.SideSell {
		side = "SELL"
	}
	rec := &database.TradeRecord{
		UserID:          pos.UserID,
		Symbol:          pos.Symbol,
		Side:            side,
		TradeType:       "SPOT",
		Quantity:        pos.Quantity,
		Price:           pos.EntryPrice,
		IsPaper:         false,
		ExchangeOrderID: fmt.Sprintf("%d", pos.MainOrderID),
		ExecutedAt:      pos.OpenedAt,
	}
	_, err := a.trades.Insert(ctx, rec)
	return err
}

func (a *liveTradeLoggerAdapter) LogClose(ctx context.Context, pos *livetrading.LivePosition) error {
	side := "SELL"
	if pos.Side == exchange.SideSell {
		side = "BUY"
	}
	executedAt := time.Now()
	if pos.ClosedAt != nil {
		executedAt = *pos.ClosedAt
	}
	rec := &database.TradeRecord{
		UserID:     pos.UserID,
		Symbol:     pos.Symbol,
		Side:       side,
		TradeType:  "SPOT",
		Quantity:   pos.Quantity,
		Price:      pos.ClosePrice,
		IsPaper:    false,
		ExecutedAt: executedAt,
	}
	if _, err := a.trades.Insert(ctx, rec); err != nil {
		return err
	}

	// increment daily trade stats
	isWin := pos.PnL > 0
	if err := a.daily.IncrementTrade(ctx, pos.UserID, pos.PnL, 0, isWin); err != nil {
		slog.Error("failed to increment live trade stats", "user_id", pos.UserID, "error", err)
	}
	return nil
}

// implements leverage.LeveragePositionStore for live leverage positions
type liveLeveragePositionStoreAdapter struct {
	repo *database.PositionRepository
}

func (a *liveLeveragePositionStoreAdapter) SavePosition(ctx context.Context, pos *leverage.LeveragePosition) error {
	p := &database.PersistedPosition{
		InternalID:       pos.ID,
		UserID:           pos.UserID,
		Symbol:           pos.Symbol,
		Side:             string(pos.Side),
		PositionType:     "FUTURES",
		Status:           "OPEN",
		EntryPrice:       pos.EntryPrice,
		MarkPrice:        pos.MarkPrice,
		Quantity:         pos.Quantity,
		Margin:           pos.Margin,
		NotionalValue:    pos.NotionalValue,
		Leverage:         pos.Leverage,
		StopLoss:         pos.StopLoss,
		TakeProfit:       pos.TakeProfit,
		LiquidationPrice: pos.LiquidationPrice,
		FundingPaid:      pos.FundingPaid,
		MarginType:       pos.MarginType,
		IsPaper:          false,
		Platform:         pos.Platform,
		OpenedAt:         pos.OpenedAt,
	}
	return a.repo.Insert(ctx, p)
}

func (a *liveLeveragePositionStoreAdapter) ClosePosition(ctx context.Context, pos *leverage.LeveragePosition) error {
	closedAt := time.Now()
	if pos.ClosedAt != nil {
		closedAt = *pos.ClosedAt
	}
	return a.repo.UpdateClose(ctx, pos.ID, "CLOSED", pos.CloseReason,
		pos.ClosePrice, pos.PnL, pos.FundingPaid, closedAt)
}

func (a *liveLeveragePositionStoreAdapter) AdjustPosition(ctx context.Context, posID string, sl, tp float64) error {
	return a.repo.UpdateSLTP(ctx, posID, sl, tp)
}

// recoverLivePositions loads open non-paper SPOT positions from the database
// and restores them into the live executor's in-memory map.
func recoverLivePositions(ctx context.Context, repo *database.PositionRepository, executor *livetrading.Executor) error {
	rows, err := repo.LoadOpenLiveSpot(ctx)
	if err != nil {
		return fmt.Errorf("failed to load open live positions: %w", err)
	}

	for _, r := range rows {
		side := exchange.SideBuy
		if r.Side == "SHORT" || r.Side == "SELL" {
			side = exchange.SideSell
		}
		pos := &livetrading.LivePosition{
			ID:           r.InternalID,
			UserID:       r.UserID,
			Symbol:       r.Symbol,
			Side:         side,
			EntryPrice:   r.EntryPrice,
			Quantity:     r.Quantity,
			PositionSize: r.PositionSize,
			StopLoss:     r.StopLoss,
			TakeProfit:   r.TakeProfit,
			Status:       "open",
			OpenedAt:     r.OpenedAt,
			Platform:     r.Platform,
		}
		executor.RestorePosition(pos)
		slog.Info("recovered live position", "id", r.InternalID, "symbol", r.Symbol)
	}

	// resume ID generation
	maxID, err := repo.MaxInternalID(ctx, "live_")
	if err != nil {
		return fmt.Errorf("failed to get max live position ID: %w", err)
	}
	executor.SetNextID(maxID + 1)

	if len(rows) > 0 {
		slog.Info("live position recovery complete", "count", len(rows))
	}
	return nil
}

// recoverLiveLeveragePositions loads open non-paper FUTURES positions from the database
// and restores them into the live leverage executor's in-memory map.
func recoverLiveLeveragePositions(ctx context.Context, repo *database.PositionRepository, executor *leverage.LiveExecutor) error {
	rows, err := repo.LoadOpenLiveFutures(ctx)
	if err != nil {
		return fmt.Errorf("failed to load open live leverage positions: %w", err)
	}

	for _, r := range rows {
		pos := &leverage.LeveragePosition{
			ID:               r.InternalID,
			UserID:           r.UserID,
			Symbol:           r.Symbol,
			Side:             leverage.PositionSide(r.Side),
			Leverage:         r.Leverage,
			EntryPrice:       r.EntryPrice,
			MarkPrice:        r.MarkPrice,
			Quantity:         r.Quantity,
			Margin:           r.Margin,
			NotionalValue:    r.NotionalValue,
			LiquidationPrice: r.LiquidationPrice,
			StopLoss:         r.StopLoss,
			TakeProfit:       r.TakeProfit,
			FundingPaid:      r.FundingPaid,
			MarginType:       r.MarginType,
			IsPaper:          false,
			Status:           "open",
			OpenedAt:         r.OpenedAt,
			Platform:         r.Platform,
		}
		executor.RestorePosition(pos)
		slog.Info("recovered live leverage position", "id", r.InternalID, "symbol", r.Symbol)
	}

	// resume ID generation
	maxID, err := repo.MaxInternalID(ctx, "lev_")
	if err != nil {
		return fmt.Errorf("failed to get max live leverage position ID: %w", err)
	}
	executor.SetNextID(maxID + 1)

	if len(rows) > 0 {
		slog.Info("live leverage position recovery complete", "count", len(rows))
	}
	return nil
}
