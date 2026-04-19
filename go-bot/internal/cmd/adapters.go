// bridge adapters that connect existing services to trading interfaces.
// keeps run.go clean by isolating small adapter types here.
package cmd

import (
	"context"
	"log"
	"log/slog"
	"time"

	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/database"
	"github.com/trading-bot/go-bot/internal/datasources"
	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/livetrading"
	"github.com/trading-bot/go-bot/internal/pipeline"
	"github.com/trading-bot/go-bot/internal/user"
	"github.com/trading-bot/go-bot/internal/watchlist"
)

// adapts binance.WSPriceCache to the PriceProvider interface.
// uses the websocket cache for instant prices, with REST fallback.
type wsPriceAdapter struct {
	ws   *binance.WSPriceCache
	rest *binance.Client
}

func (a *wsPriceAdapter) GetPrice(symbol string) (float64, error) {
	price, err := a.ws.GetPrice(symbol)
	if err == nil {
		return price, nil
	}
	// fallback to REST if websocket hasn't received this symbol yet
	ticker, err := a.rest.GetPrice(context.Background(), symbol)
	if err != nil {
		return 0, err
	}
	return ticker.Price, nil
}

// adapts binance.Client (which takes context) to the simpler
// PriceProvider interface used by paper/live trading monitors
type priceAdapter struct {
	client *binance.Client
}

func (a *priceAdapter) GetPrice(symbol string) (float64, error) {
	ticker, err := a.client.GetPrice(context.Background(), symbol)
	if err != nil {
		return 0, err
	}
	return ticker.Price, nil
}

// bridges user.Repository to livetrading's credentialRepository interface
// by converting user.Credentials to livetrading.credentials
type credRepoAdapter struct {
	repo *user.Repository
}

func (a *credRepoAdapter) GetCredentials(ctx context.Context, userID int, exchange string) (*livetrading.Credentials, error) {
	cred, err := a.repo.GetCredentials(ctx, userID, exchange)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, nil
	}
	return &livetrading.Credentials{
		ID:                 cred.ID,
		UserID:             cred.UserID,
		APIKeyEncrypted:    cred.APIKeyEncrypted,
		APISecretEncrypted: cred.APISecretEncrypted,
		Salt:               cred.Salt,
	}, nil
}

// adapts binance.FuturesClient to the leverage.MarkPriceProvider interface
type markPriceAdapter struct {
	client *binance.FuturesClient
}

func (a *markPriceAdapter) GetMarkPrice(ctx context.Context, symbol string) (float64, error) {
	mp, err := a.client.GetMarkPrice(ctx, symbol)
	if err != nil {
		return 0, err
	}
	return mp.MarkPrice, nil
}

// adapts binance.FuturesClient to the leverage.FuturesBalanceProvider interface.
// decrypts keys first, then fetches the futures USDT balance.
type futuresBalanceAdapter struct {
	futures *binance.FuturesClient
	keys    interface {
		DecryptKeys(userID int) (string, string, error)
	}
}

func (a *futuresBalanceAdapter) GetFuturesBalance(ctx context.Context, userID int, asset string) (float64, error) {
	apiKey, apiSecret, err := a.keys.DecryptKeys(userID)
	if err != nil {
		return 0, err
	}
	balances, err := a.futures.GetFuturesBalance(ctx, apiKey, apiSecret)
	if err != nil {
		return 0, err
	}
	for _, b := range balances {
		if b.Asset == asset {
			return b.AvailableBalance, nil
		}
	}
	return 0, nil
}

// adapts the user service to the leverage.LeverageStatusProvider interface
type leverageStatusAdapter struct {
	userSvc *user.Service
}

func (a *leverageStatusAdapter) IsLeverageEnabled(userID int) bool {
	enabled, err := a.userSvc.IsLeverageEnabled(context.Background(), userID)
	if err != nil {
		return false
	}
	return enabled
}

// implements scanner.Notifier by sending messages via telegram, discord, and whatsapp bots
type scannerNotifier struct {
	telegramBot interface {
		SendMessage(chatID int64, text string) error
	}
	discordBot interface {
		SendMessage(channelID string, content string) error
	}
	whatsappBot interface {
		SendMessage(recipientID string, text string) error
	}
}

func (n *scannerNotifier) NotifyTelegram(chatID int64, message string) error {
	if n.telegramBot == nil {
		log.Printf("telegram not configured, skipping notification for chat %d", chatID)
		return nil
	}
	return n.telegramBot.SendMessage(chatID, message)
}

func (n *scannerNotifier) NotifyDiscord(channelID string, title, description string, fields []pipeline.DiscordField, color int) error {
	if n.discordBot == nil {
		log.Printf("discord not configured, skipping notification for channel %s", channelID)
		return nil
	}
	// format a simple text message from the discord fields
	msg := "**" + title + "**\n" + description
	return n.discordBot.SendMessage(channelID, msg)
}

func (n *scannerNotifier) NotifyWhatsApp(recipientID string, message string) error {
	if n.whatsappBot == nil {
		log.Printf("whatsapp not configured, skipping notification for %s", recipientID)
		return nil
	}
	return n.whatsappBot.SendMessage(recipientID, message)
}

// --- data ingestion adapters ---

// adapts database.CandleRepository to pipeline.CandleStore interface.
// converts between pipeline.CandleRecord and database.CandleRecord.
type candleStoreAdapter struct {
	repo *database.CandleRepository
}

func (a *candleStoreAdapter) UpsertBatch(ctx context.Context, candles []*pipeline.CandleRecord) (int, error) {
	dbCandles := make([]*database.CandleRecord, len(candles))
	for i, c := range candles {
		dbCandles[i] = &database.CandleRecord{
			Time:        c.Time,
			Symbol:      c.Symbol,
			Interval:    c.Interval,
			Open:        c.Open,
			High:        c.High,
			Low:         c.Low,
			Close:       c.Close,
			Volume:      c.Volume,
			QuoteVolume: c.QuoteVolume,
			TradeCount:  c.TradeCount,
		}
	}
	return a.repo.UpsertBatch(ctx, dbCandles)
}

func (a *candleStoreAdapter) LatestTime(ctx context.Context, symbol, interval string) (time.Time, error) {
	return a.repo.LatestTime(ctx, symbol, interval)
}

// aggregates all unique symbols across all users' watchlists.
// implements pipeline.SymbolProvider.
type watchlistSymbolProvider struct {
	userSvc  *user.Service
	watchSvc *watchlist.Service
}

func (p *watchlistSymbolProvider) ActiveSymbols(ctx context.Context) ([]string, error) {
	users, err := p.userSvc.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	for _, u := range users {
		items, err := p.watchSvc.List(ctx, u.ID)
		if err != nil {
			continue
		}
		for _, item := range items {
			seen[item.Symbol] = true
		}
	}

	symbols := make([]string, 0, len(seen))
	for s := range seen {
		symbols = append(symbols, s)
	}
	return symbols, nil
}

// altDataAdapter bridges datasources.Aggregator to pipeline.AltDataProvider.
// Converts datasources.AlternativeData -> claude.AltData for the pipeline.
type altDataAdapter struct {
	agg *datasources.Aggregator
}

func (a *altDataAdapter) Fetch(ctx context.Context, symbol string) *claude.AltData {
	raw := a.agg.Fetch(ctx, symbol)
	if raw == nil {
		return nil
	}

	result := &claude.AltData{}

	if raw.OrderFlow != nil {
		result.OrderFlow = &claude.OrderFlowData{
			BuySellRatio:    raw.OrderFlow.BuySellRatio,
			DepthImbalance:  raw.OrderFlow.DepthImbalance,
			LargeBuyOrders:  raw.OrderFlow.LargeBuyOrders,
			LargeSellOrders: raw.OrderFlow.LargeSellOrders,
			SpreadBps:       raw.OrderFlow.SpreadBps,
		}
	}

	if raw.OnChain != nil {
		result.OnChain = &claude.OnChainData{
			NetFlow:           raw.OnChain.NetFlow,
			WhaleTransactions: raw.OnChain.WhaleTransactions,
			ActiveAddresses:   raw.OnChain.ActiveAddresses24h,
			NVTRatio:          raw.OnChain.NVTRatio,
		}
	}

	if raw.FundingRate != nil {
		// use the max rate as primary (from whatever exchange has highest deviation)
		result.FundingRate = &claude.FundingData{
			Rate:       raw.FundingRate.MaxRate,
			Annualized: raw.FundingRate.Annualized,
		}
	}

	if raw.Sentiment != nil {
		result.Sentiment = &claude.SentimentData{
			OverallScore:   raw.Sentiment.OverallScore,
			OverallLabel:   raw.Sentiment.OverallLabel,
			FearGreedIndex: raw.Sentiment.FearGreedIndex,
		}
	}

	return result
}

// tradeHistoryAdapter bridges AIDecisionRepository to pipeline.TradeHistoryProvider.
type tradeHistoryAdapter struct {
	repo *database.AIDecisionRepository
}

func (a *tradeHistoryAdapter) RecentOutcomes(ctx context.Context, limit int) ([]claude.TradeOutcome, error) {
	rows, err := a.repo.RecentOutcomes(ctx, limit)
	if err != nil {
		return nil, err
	}

	outcomes := make([]claude.TradeOutcome, len(rows))
	for i, r := range rows {
		correct := (r.Decision == "BUY" && r.PnLPct > 0) || (r.Decision == "SELL" && r.PnLPct > 0)
		outcomes[i] = claude.TradeOutcome{
			Symbol:     r.Symbol,
			Action:     r.Decision,
			EntryPrice: r.EntryPrice,
			ExitPrice:  r.ExitPrice,
			PnLPct:     r.PnLPct,
			Confidence: float64(r.Confidence),
			Correct:    correct,
			Timeframe:  r.Timeframe,
			Timestamp:  r.CreatedAt.Format("2006-01-02 15:04"),
		}
	}
	return outcomes, nil
}

// failedOrderAdapter implements livetrading.FailedOrderRecorder using FailedOrderRepository.
type failedOrderAdapter struct {
	repo *database.FailedOrderRepository
}

func (a *failedOrderAdapter) RecordFailedOrder(ctx context.Context, userID int, positionID, symbol, side, orderType string, quantity, price, stopPrice float64, tradeType, errorMsg string) error {
	_, err := a.repo.Insert(ctx, &database.FailedOrder{
		UserID:       userID,
		PositionID:   positionID,
		Symbol:       symbol,
		Side:         side,
		OrderType:    orderType,
		Quantity:     quantity,
		Price:        price,
		StopPrice:    stopPrice,
		TradeType:    tradeType,
		ErrorMessage: errorMsg,
	})
	if err != nil {
		slog.Error("failed to record failed order to dead-letter queue", "symbol", symbol, "error", err)
	}
	return err
}

// dcaPriceAdapter wraps wsPriceAdapter to match the dca.PriceProvider interface
type dcaPriceAdapter struct {
	ws   *binance.WSPriceCache
	rest *binance.Client
}

func (a *dcaPriceAdapter) GetPrice(ctx context.Context, symbol string) (*exchange.Ticker, error) {
	price, err := a.ws.GetPrice(symbol)
	if err == nil {
		return &exchange.Ticker{Symbol: symbol, Price: price}, nil
	}
	return a.rest.GetPrice(ctx, symbol)
}
