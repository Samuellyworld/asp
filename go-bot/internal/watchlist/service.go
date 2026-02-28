// watchlist management service
package watchlist

import (
	"context"
	"fmt"
	"strings"
)

const maxWatchlistSize = 25

// repository defines the data operations for watchlist management
type repository interface {
	GetByUserID(ctx context.Context, userID int) ([]Item, error)
	Add(ctx context.Context, userID int, symbol string) error
	Remove(ctx context.Context, userID int, symbol string) error
	Exists(ctx context.Context, userID int, symbol string) (bool, error)
	Count(ctx context.Context, userID int) (int, error)
	Reset(ctx context.Context, userID int) error
}

// handles watchlist business logic
type Service struct {
	repo repository
}

func NewService(repo repository) *Service {
	return &Service{repo: repo}
}

//  returns all active watchlist symbols for a user
func (s *Service) List(ctx context.Context, userID int) ([]Item, error) {
	return s.repo.GetByUserID(ctx, userID)
}

//  adds a symbol to the user's watchlist after validation
func (s *Service) Add(ctx context.Context, userID int, symbol string) error {
	symbol = normalizeSymbol(symbol)

	if !isValidSymbol(symbol) {
		return fmt.Errorf("invalid symbol format: %s (expected format: BTC/USDT)", symbol)
	}

	// check if already exists
	exists, err := s.repo.Exists(ctx, userID, symbol)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%s is already in your watchlist", symbol)
	}

	// check max size
	count, err := s.repo.Count(ctx, userID)
	if err != nil {
		return err
	}
	if count >= maxWatchlistSize {
		return fmt.Errorf("watchlist is full (max %d symbols). remove one first with /watchremove", maxWatchlistSize)
	}

	return s.repo.Add(ctx, userID, symbol)
}

// removes a symbol from the user's watchlist
func (s *Service) Remove(ctx context.Context, userID int, symbol string) error {
	symbol = normalizeSymbol(symbol)
	return s.repo.Remove(ctx, userID, symbol)
}

//  restores the default top-10 watchlist
func (s *Service) Reset(ctx context.Context, userID int) error {
	return s.repo.Reset(ctx, userID)
}

//  converts user input to the standard format
func normalizeSymbol(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	// handle formats like "BTCUSDT" -> "BTC/USDT"
	if !strings.Contains(symbol, "/") {
		// try to split at common quote currencies
		for _, quote := range []string{"USDT", "BUSD", "USDC", "BTC", "ETH", "BNB"} {
			if strings.HasSuffix(symbol, quote) {
				base := strings.TrimSuffix(symbol, quote)
				if len(base) > 0 {
					return base + "/" + quote
				}
			}
		}
	}

	return symbol
}

//  checks if a symbol follows the expected format
func isValidSymbol(symbol string) bool {
	parts := strings.Split(symbol, "/")
	if len(parts) != 2 {
		return false
	}
	if len(parts[0]) < 2 || len(parts[0]) > 10 {
		return false
	}
	if len(parts[1]) < 2 || len(parts[1]) > 10 {
		return false
	}
	return true
}
