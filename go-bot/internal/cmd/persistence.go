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
	"github.com/trading-bot/go-bot/internal/leverage"
	"github.com/trading-bot/go-bot/internal/papertrading"
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
