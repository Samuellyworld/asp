// order types for exchange interactions.
// defines order side, type, status, and the order struct used across trading packages.
package exchange

import "time"

// side of the order
type OrderSide string

const (
	SideBuy  OrderSide = "BUY"
	SideSell OrderSide = "SELL"
)

// order execution type
type OrderType string

const (
	OrderTypeMarket   OrderType = "MARKET"
	OrderTypeLimit    OrderType = "LIMIT"
	OrderTypeStopLoss OrderType = "STOP_LOSS_LIMIT"
	OrderTypeTakeProfit OrderType = "TAKE_PROFIT_LIMIT"
)

// current state of an order on the exchange
type OrderStatus string

const (
	OrderStatusNew             OrderStatus = "NEW"
	OrderStatusPartiallyFilled OrderStatus = "PARTIALLY_FILLED"
	OrderStatusFilled          OrderStatus = "FILLED"
	OrderStatusCanceled        OrderStatus = "CANCELED"
	OrderStatusRejected        OrderStatus = "REJECTED"
	OrderStatusExpired         OrderStatus = "EXPIRED"
)

// a single fill within an order execution
type Fill struct {
	Price      float64
	Quantity   float64
	Commission float64
	CommissionAsset string
}

// represents an order placed or returned from an exchange
type Order struct {
	OrderID       int64
	ClientOrderID string
	Symbol        string
	Side          OrderSide
	Type          OrderType
	Status        OrderStatus
	Price         float64
	StopPrice     float64
	Quantity      float64
	ExecutedQty   float64
	AvgPrice      float64
	Fills         []Fill
	CreatedAt     time.Time
}

// interface for placing and managing orders on an exchange
type OrderExecutor interface {
	PlaceOrder(symbol string, side OrderSide, orderType OrderType, quantity, price float64, apiKey, apiSecret string) (*Order, error)
	PlaceStopLoss(symbol string, side OrderSide, quantity, stopPrice, price float64, apiKey, apiSecret string) (*Order, error)
	PlaceTakeProfit(symbol string, side OrderSide, quantity, stopPrice, price float64, apiKey, apiSecret string) (*Order, error)
	CancelOrder(symbol string, orderID int64, apiKey, apiSecret string) error
	GetOrder(symbol string, orderID int64, apiKey, apiSecret string) (*Order, error)
	GetOpenOrders(symbol string, apiKey, apiSecret string) ([]Order, error)
}
