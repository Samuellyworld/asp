// futures api types for binance usdt-m futures
package binance

import (
	"strconv"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// futures order returned from binance futures api
type FuturesOrder struct {
	OrderID       int64
	ClientOrderID string
	Symbol        string
	Side          exchange.OrderSide
	Type          string
	Status        exchange.OrderStatus
	Price         float64
	StopPrice     float64
	Quantity      float64
	ExecutedQty   float64
	AvgPrice      float64
	CreatedAt     time.Time
}

// futures position from binance position risk endpoint
type FuturesPosition struct {
	Symbol           string
	PositionSide     string // "BOTH", "LONG", "SHORT"
	PositionAmt      float64
	EntryPrice       float64
	MarkPrice        float64
	UnrealizedProfit float64
	LiquidationPrice float64
	Leverage         int
	MarginType       string // "isolated" or "cross"
	IsolatedMargin   float64
	Notional         float64
}

// futures wallet balance entry
type FuturesBalance struct {
	Asset              string
	Balance            float64
	AvailableBalance   float64
	CrossWalletBalance float64
}

// mark price data from premium index endpoint
type MarkPrice struct {
	Symbol          string
	MarkPrice       float64
	IndexPrice      float64
	LastFundingRate float64
	NextFundingTime int64
}

// funding rate info
type FundingRate struct {
	Symbol      string
	FundingRate float64
	FundingTime int64
}

// raw response from POST /fapi/v1/order
type futuresOrderResponse struct {
	OrderID       int64  `json:"orderId"`
	ClientOrderID string `json:"clientOrderId"`
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	Price         string `json:"price"`
	StopPrice     string `json:"stopPrice"`
	OrigQty       string `json:"origQty"`
	ExecutedQty   string `json:"executedQty"`
	AvgPrice      string `json:"avgPrice"`
	UpdateTime    int64  `json:"updateTime"`
}

type futuresAlgoOrderResponse struct {
	AlgoID        int64  `json:"algoId"`
	ClientAlgoID  string `json:"clientAlgoId"`
	AlgoType      string `json:"algoType"`
	OrderType     string `json:"orderType"`
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	PositionSide  string `json:"positionSide"`
	Quantity      string `json:"quantity"`
	AlgoStatus    string `json:"algoStatus"`
	ActualOrderID string `json:"actualOrderId"`
	ActualPrice   string `json:"actualPrice"`
	ActualQty     string `json:"actualQty"`
	TriggerPrice  string `json:"triggerPrice"`
	Price         string `json:"price"`
	CreateTime    int64  `json:"createTime"`
	UpdateTime    int64  `json:"updateTime"`
	TriggerTime   int64  `json:"triggerTime"`
}

func (r *futuresOrderResponse) toFuturesOrder() *FuturesOrder {
	price, _ := strconv.ParseFloat(r.Price, 64)
	stopPrice, _ := strconv.ParseFloat(r.StopPrice, 64)
	origQty, _ := strconv.ParseFloat(r.OrigQty, 64)
	execQty, _ := strconv.ParseFloat(r.ExecutedQty, 64)
	avgPrice, _ := strconv.ParseFloat(r.AvgPrice, 64)

	var createdAt time.Time
	if r.UpdateTime > 0 {
		createdAt = time.UnixMilli(r.UpdateTime)
	}

	return &FuturesOrder{
		OrderID:       r.OrderID,
		ClientOrderID: r.ClientOrderID,
		Symbol:        r.Symbol,
		Side:          exchange.OrderSide(r.Side),
		Type:          r.Type,
		Status:        exchange.OrderStatus(r.Status),
		Price:         price,
		StopPrice:     stopPrice,
		Quantity:      origQty,
		ExecutedQty:   execQty,
		AvgPrice:      avgPrice,
		CreatedAt:     createdAt,
	}
}

func (r *futuresAlgoOrderResponse) toFuturesOrder() *FuturesOrder {
	price, _ := strconv.ParseFloat(r.Price, 64)
	stopPrice, _ := strconv.ParseFloat(r.TriggerPrice, 64)
	quantity, _ := strconv.ParseFloat(r.Quantity, 64)
	actualQty, _ := strconv.ParseFloat(r.ActualQty, 64)
	actualPrice, _ := strconv.ParseFloat(r.ActualPrice, 64)

	status := exchange.OrderStatus(r.AlgoStatus)
	switch r.AlgoStatus {
	case "NEW":
		status = exchange.OrderStatusNew
	case "CANCELED":
		status = exchange.OrderStatusCanceled
	case "EXPIRED":
		status = exchange.OrderStatusExpired
	case "REJECTED":
		status = exchange.OrderStatusRejected
	}
	if r.ActualOrderID != "" && actualQty > 0 {
		status = exchange.OrderStatusFilled
	}

	var createdAt time.Time
	if r.UpdateTime > 0 {
		createdAt = time.UnixMilli(r.UpdateTime)
	} else if r.CreateTime > 0 {
		createdAt = time.UnixMilli(r.CreateTime)
	}

	return &FuturesOrder{
		OrderID:       r.AlgoID,
		ClientOrderID: r.ClientAlgoID,
		Symbol:        r.Symbol,
		Side:          exchange.OrderSide(r.Side),
		Type:          r.OrderType,
		Status:        status,
		Price:         price,
		StopPrice:     stopPrice,
		Quantity:      quantity,
		ExecutedQty:   actualQty,
		AvgPrice:      actualPrice,
		CreatedAt:     createdAt,
	}
}

// raw response from GET /fapi/v2/positionRisk
type futuresPositionResponse struct {
	Symbol           string `json:"symbol"`
	PositionSide     string `json:"positionSide"`
	PositionAmt      string `json:"positionAmt"`
	EntryPrice       string `json:"entryPrice"`
	MarkPrice        string `json:"markPrice"`
	UnrealizedProfit string `json:"unRealizedProfit"`
	LiquidationPrice string `json:"liquidationPrice"`
	Leverage         string `json:"leverage"`
	MarginType       string `json:"marginType"`
	IsolatedMargin   string `json:"isolatedMargin"`
	Notional         string `json:"notional"`
}

func (r *futuresPositionResponse) toFuturesPosition() FuturesPosition {
	posAmt, _ := strconv.ParseFloat(r.PositionAmt, 64)
	entryPrice, _ := strconv.ParseFloat(r.EntryPrice, 64)
	markPrice, _ := strconv.ParseFloat(r.MarkPrice, 64)
	unrealized, _ := strconv.ParseFloat(r.UnrealizedProfit, 64)
	liqPrice, _ := strconv.ParseFloat(r.LiquidationPrice, 64)
	leverage, _ := strconv.Atoi(r.Leverage)
	isoMargin, _ := strconv.ParseFloat(r.IsolatedMargin, 64)
	notional, _ := strconv.ParseFloat(r.Notional, 64)

	return FuturesPosition{
		Symbol:           r.Symbol,
		PositionSide:     r.PositionSide,
		PositionAmt:      posAmt,
		EntryPrice:       entryPrice,
		MarkPrice:        markPrice,
		UnrealizedProfit: unrealized,
		LiquidationPrice: liqPrice,
		Leverage:         leverage,
		MarginType:       r.MarginType,
		IsolatedMargin:   isoMargin,
		Notional:         notional,
	}
}

// raw response from GET /fapi/v2/balance
type futuresBalanceResponse struct {
	Asset              string `json:"asset"`
	Balance            string `json:"balance"`
	AvailableBalance   string `json:"availableBalance"`
	CrossWalletBalance string `json:"crossWalletBalance"`
}

func (r *futuresBalanceResponse) toFuturesBalance() FuturesBalance {
	bal, _ := strconv.ParseFloat(r.Balance, 64)
	avail, _ := strconv.ParseFloat(r.AvailableBalance, 64)
	crossWallet, _ := strconv.ParseFloat(r.CrossWalletBalance, 64)

	return FuturesBalance{
		Asset:              r.Asset,
		Balance:            bal,
		AvailableBalance:   avail,
		CrossWalletBalance: crossWallet,
	}
}

// raw response from GET /fapi/v1/premiumIndex
type markPriceResponse struct {
	Symbol          string `json:"symbol"`
	MarkPrice       string `json:"markPrice"`
	IndexPrice      string `json:"indexPrice"`
	LastFundingRate string `json:"lastFundingRate"`
	NextFundingTime int64  `json:"nextFundingTime"`
}

func (r *markPriceResponse) toMarkPrice() *MarkPrice {
	mp, _ := strconv.ParseFloat(r.MarkPrice, 64)
	ip, _ := strconv.ParseFloat(r.IndexPrice, 64)
	fr, _ := strconv.ParseFloat(r.LastFundingRate, 64)

	return &MarkPrice{
		Symbol:          r.Symbol,
		MarkPrice:       mp,
		IndexPrice:      ip,
		LastFundingRate: fr,
		NextFundingTime: r.NextFundingTime,
	}
}

// raw response from GET /fapi/v1/fundingRate
type fundingRateResponse struct {
	Symbol      string `json:"symbol"`
	FundingRate string `json:"fundingRate"`
	FundingTime int64  `json:"fundingTime"`
}

func (r *fundingRateResponse) toFundingRate() *FundingRate {
	fr, _ := strconv.ParseFloat(r.FundingRate, 64)

	return &FundingRate{
		Symbol:      r.Symbol,
		FundingRate: fr,
		FundingTime: r.FundingTime,
	}
}
