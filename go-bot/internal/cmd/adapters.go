// bridge adapters that connect existing services to trading interfaces.
// keeps run.go clean by isolating small adapter types here.
package cmd

import (
	"context"
	"log"

	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/livetrading"
	"github.com/trading-bot/go-bot/internal/pipeline"
	"github.com/trading-bot/go-bot/internal/user"
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

func (a *markPriceAdapter) GetMarkPrice(symbol string) (float64, error) {
	mp, err := a.client.GetMarkPrice(symbol)
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

func (a *futuresBalanceAdapter) GetFuturesBalance(userID int, asset string) (float64, error) {
	apiKey, apiSecret, err := a.keys.DecryptKeys(userID)
	if err != nil {
		return 0, err
	}
	balances, err := a.futures.GetFuturesBalance(apiKey, apiSecret)
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
