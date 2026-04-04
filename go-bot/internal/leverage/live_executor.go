// live leverage executor. decrypts user keys, runs safety checks,
// and places real futures orders on binance with sl/tp.
package leverage

import (
	"fmt"
	"sync"
	"time"

	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/exchange"
)

// decrypts stored api credentials
type KeyDecryptor interface {
	DecryptKeys(userID int) (apiKey, apiSecret string, err error)
}

// places and manages futures orders
type FuturesOrderClient interface {
	SetLeverage(symbol string, leverage int, apiKey, apiSecret string) error
	SetMarginType(symbol string, marginType string, apiKey, apiSecret string) error
	PlaceOrder(symbol string, side exchange.OrderSide, orderType exchange.OrderType, quantity, price float64, apiKey, apiSecret string) (*binance.FuturesOrder, error)
	PlaceStopMarket(symbol string, side exchange.OrderSide, quantity, stopPrice float64, apiKey, apiSecret string) (*binance.FuturesOrder, error)
	PlaceTakeProfitMarket(symbol string, side exchange.OrderSide, quantity, stopPrice float64, apiKey, apiSecret string) (*binance.FuturesOrder, error)
	CancelOrder(symbol string, orderID int64, apiKey, apiSecret string) error
	GetPositions(apiKey, apiSecret string) ([]binance.FuturesPosition, error)
}

// executes real leveraged futures trades on binance
type LiveExecutor struct {
	mu        sync.RWMutex
	positions map[string]*LeveragePosition
	closed    []*LeveragePosition
	futures   FuturesOrderClient
	keys      KeyDecryptor
	safety    *SafetyChecker
	funding   *FundingTracker
	prices    MarkPriceProvider
	nextID    int
}

// creates a new live leverage executor
func NewLiveExecutor(
	futures FuturesOrderClient,
	keys KeyDecryptor,
	safety *SafetyChecker,
	funding *FundingTracker,
	prices MarkPriceProvider,
) *LiveExecutor {
	return &LiveExecutor{
		positions: make(map[string]*LeveragePosition),
		futures:   futures,
		keys:      keys,
		safety:    safety,
		funding:   funding,
		prices:    prices,
	}
}

// opens a live leveraged position on binance futures.
// decrypts keys, runs safety checks, configures leverage and margin type,
// places market order, then sets sl/tp orders.
func (e *LiveExecutor) OpenPosition(
	userID int,
	symbol string,
	side PositionSide,
	leverage int,
	margin float64,
	stopLoss, takeProfit float64,
	platform string,
) (*LeveragePosition, error) {
	// get current mark price for quantity calculation and safety checks
	markPrice, err := e.prices.GetMarkPrice(symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get mark price: %w", err)
	}
	if markPrice <= 0 {
		return nil, fmt.Errorf("invalid mark price %.8f for %s", markPrice, symbol)
	}

	// run safety checks if checker is configured
	if e.safety != nil {
		result := e.safety.Check(userID, symbol, leverage, margin, markPrice, string(side))
		if !result.Passed {
			return nil, fmt.Errorf("safety check failed: %s", result.Blocked)
		}
	}

	// decrypt user api keys
	apiKey, apiSecret, err := e.keys.DecryptKeys(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt keys: %w", err)
	}

	// configure leverage on exchange
	if err := e.futures.SetLeverage(symbol, leverage, apiKey, apiSecret); err != nil {
		return nil, fmt.Errorf("failed to set leverage: %w", err)
	}

	// set isolated margin mode
	if err := e.futures.SetMarginType(symbol, "ISOLATED", apiKey, apiSecret); err != nil {
		return nil, fmt.Errorf("failed to set margin type: %w", err)
	}

	// determine exchange order side
	orderSide := exchange.SideBuy
	closeSide := exchange.SideSell
	if side == SideShort {
		orderSide = exchange.SideSell
		closeSide = exchange.SideBuy
	}

	// calculate quantity from margin and leverage
	notional := margin * float64(leverage)
	quantity := notional / markPrice

	// place market order
	mainOrder, err := e.futures.PlaceOrder(
		symbol, orderSide, exchange.OrderTypeMarket,
		quantity, 0,
		apiKey, apiSecret,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	// use actual fill price if available, otherwise fall back to mark price
	entryPrice := mainOrder.AvgPrice
	if entryPrice <= 0 {
		entryPrice = markPrice
	}

	// use actual executed quantity if available
	filledQty := mainOrder.ExecutedQty
	if filledQty <= 0 {
		filledQty = quantity
	}

	// recalculate actual notional based on fill
	actualNotional := entryPrice * filledQty

	// calculate liquidation price
	liqPrice := CalculateLiquidationPrice(entryPrice, leverage, string(side), DefaultMaintenanceMarginRate)

	// place stop loss order if specified
	var slOrderID int64
	if stopLoss > 0 {
		slOrder, err := e.futures.PlaceStopMarket(
			symbol, closeSide, filledQty, stopLoss,
			apiKey, apiSecret,
		)
		if err == nil {
			slOrderID = slOrder.OrderID
		}
	}

	// place take profit order if specified
	var tpOrderID int64
	if takeProfit > 0 {
		tpOrder, err := e.futures.PlaceTakeProfitMarket(
			symbol, closeSide, filledQty, takeProfit,
			apiKey, apiSecret,
		)
		if err == nil {
			tpOrderID = tpOrder.OrderID
		}
	}

	// try to get exchange liquidation price for higher accuracy
	positions, err := e.futures.GetPositions(apiKey, apiSecret)
	if err == nil {
		for _, p := range positions {
			if p.Symbol == symbol && p.LiquidationPrice > 0 {
				liqPrice = p.LiquidationPrice
				break
			}
		}
	}

	e.mu.Lock()
	e.nextID++
	id := fmt.Sprintf("lev_%d", e.nextID)

	pos := &LeveragePosition{
		ID:               id,
		UserID:           userID,
		Symbol:           symbol,
		Side:             side,
		Leverage:         leverage,
		EntryPrice:       entryPrice,
		MarkPrice:        entryPrice,
		Quantity:         filledQty,
		Margin:           margin,
		NotionalValue:    actualNotional,
		LiquidationPrice: liqPrice,
		StopLoss:         stopLoss,
		TakeProfit:       takeProfit,
		MarginType:       "isolated",
		IsPaper:          false,
		Status:           "open",
		OpenedAt:         time.Now(),
		Platform:         platform,
		MainOrderID:      mainOrder.OrderID,
		SLOrderID:        slOrderID,
		TPOrderID:        tpOrderID,
	}

	e.positions[id] = pos
	e.mu.Unlock()

	return pos, nil
}

// closes a live leveraged position by canceling sl/tp and placing a closing order.
// calculates realized pnl including cumulative funding fees.
func (e *LiveExecutor) Close(posID string, reason string) (*LeveragePosition, error) {
	e.mu.Lock()
	pos, ok := e.positions[posID]
	if !ok {
		e.mu.Unlock()
		return nil, fmt.Errorf("position not found: %s", posID)
	}
	if pos.Status == "closed" {
		e.mu.Unlock()
		return nil, fmt.Errorf("position already closed: %s", posID)
	}
	e.mu.Unlock()

	// decrypt user api keys
	apiKey, apiSecret, err := e.keys.DecryptKeys(pos.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt keys: %w", err)
	}

	// cancel existing sl/tp orders
	if pos.SLOrderID > 0 {
		_ = e.futures.CancelOrder(pos.Symbol, pos.SLOrderID, apiKey, apiSecret)
	}
	if pos.TPOrderID > 0 {
		_ = e.futures.CancelOrder(pos.Symbol, pos.TPOrderID, apiKey, apiSecret)
	}

	// determine closing side
	closeSide := exchange.SideSell
	if pos.Side == SideShort {
		closeSide = exchange.SideBuy
	}

	// place closing market order
	closeOrder, err := e.futures.PlaceOrder(
		pos.Symbol, closeSide, exchange.OrderTypeMarket,
		pos.Quantity, 0,
		apiKey, apiSecret,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to close position: %w", err)
	}

	closePrice := closeOrder.AvgPrice
	if closePrice <= 0 {
		closePrice = pos.MarkPrice
	}

	// calculate pnl
	var pnl float64
	if pos.Side == SideLong {
		pnl = (closePrice - pos.EntryPrice) * pos.Quantity
	} else {
		pnl = (pos.EntryPrice - closePrice) * pos.Quantity
	}

	// subtract funding fees from pnl
	var fundingFees float64
	if e.funding != nil {
		fundingFees = e.funding.CumulativeFees(posID)
		e.funding.Cleanup(posID)
	}
	pnl -= fundingFees

	e.mu.Lock()
	now := time.Now()
	pos.Status = "closed"
	pos.CloseReason = reason
	pos.ClosePrice = closePrice
	pos.ClosedAt = &now
	pos.PnL = pnl
	pos.FundingPaid = fundingFees

	e.closed = append(e.closed, pos)
	delete(e.positions, posID)
	e.mu.Unlock()

	return pos, nil
}

// returns a position by id from open or closed positions
func (e *LiveExecutor) Get(posID string) *LeveragePosition {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if pos, ok := e.positions[posID]; ok {
		return pos
	}
	for _, pos := range e.closed {
		if pos.ID == posID {
			return pos
		}
	}
	return nil
}

// returns all open positions for a specific user
func (e *LiveExecutor) OpenPositions(userID int) []*LeveragePosition {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []*LeveragePosition
	for _, pos := range e.positions {
		if pos.UserID == userID {
			result = append(result, pos)
		}
	}
	return result
}

// returns all open positions across all users
func (e *LiveExecutor) AllOpen() []*LeveragePosition {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*LeveragePosition, 0, len(e.positions))
	for _, pos := range e.positions {
		result = append(result, pos)
	}
	return result
}

// returns the total count of open positions
func (e *LiveExecutor) Count() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.positions)
}
