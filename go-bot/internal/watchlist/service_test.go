package watchlist

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mock repository for testing
type mockRepo struct {
	items     map[int][]Item
	addErr    error
	removeErr error
	resetErr  error
	existsErr error
	countErr  error
	getErr    error
}

func newMockRepo() *mockRepo {
	return &mockRepo{items: make(map[int][]Item)}
}

func (m *mockRepo) GetByUserID(_ context.Context, userID int) ([]Item, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.items[userID], nil
}

func (m *mockRepo) Add(_ context.Context, userID int, symbol string) error {
	if m.addErr != nil {
		return m.addErr
	}
	m.items[userID] = append(m.items[userID], Item{
		UserID:   userID,
		Symbol:   symbol,
		IsActive: true,
		Priority: len(m.items[userID]) + 1,
		AddedAt:  time.Now(),
	})
	return nil
}

func (m *mockRepo) Remove(_ context.Context, userID int, symbol string) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	items := m.items[userID]
	for i, item := range items {
		if item.Symbol == symbol {
			m.items[userID] = append(items[:i], items[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("symbol %s not found in your watchlist", symbol)
}

func (m *mockRepo) Exists(_ context.Context, userID int, symbol string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	for _, item := range m.items[userID] {
		if item.Symbol == symbol {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockRepo) Count(_ context.Context, userID int) (int, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	return len(m.items[userID]), nil
}

func (m *mockRepo) Reset(_ context.Context, userID int) error {
	if m.resetErr != nil {
		return m.resetErr
	}
	m.items[userID] = []Item{
		{Symbol: "BTC/USDT", IsActive: true, Priority: 1},
		{Symbol: "ETH/USDT", IsActive: true, Priority: 2},
	}
	return nil
}

// seed helper
func (m *mockRepo) seed(userID int, symbols ...string) {
	for i, s := range symbols {
		m.items[userID] = append(m.items[userID], Item{
			UserID:   userID,
			Symbol:   s,
			IsActive: true,
			Priority: i + 1,
			AddedAt:  time.Now(),
		})
	}
}

// --- normalizeSymbol tests ---

func TestNormalizeSymbol(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"BTCUSDT", "BTC/USDT"},
		{"btcusdt", "BTC/USDT"},
		{"  btcusdt  ", "BTC/USDT"},
		{"ETHUSDT", "ETH/USDT"},
		{"SOLUSDC", "SOL/USDC"},
		{"ETHBTC", "ETH/BTC"},
		{"BNBETH", "BNB/ETH"},
		{"DOGEBUSD", "DOGE/BUSD"},
		{"ADABNB", "ADA/BNB"},
		{"BTC/USDT", "BTC/USDT"},
		{"ETH/BTC", "ETH/BTC"},
		{"btc/usdt", "BTC/USDT"},
		{"XYZABC", "XYZABC"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeSymbol(tt.input)
			if got != tt.want {
				t.Errorf("normalizeSymbol(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidSymbol(t *testing.T) {
	tests := []struct {
		symbol string
		want   bool
	}{
		{"BTC/USDT", true},
		{"ETH/BTC", true},
		{"DOGE/USDT", true},
		{"AB/CD", true},
		{"ABCDEFGHIJ/ABCDEFGHIJ", true},
		{"BTCUSDT", false},
		{"A/USDT", false},
		{"BTC/U", false},
		{"ABCDEFGHIJK/USDT", false},
		{"BTC/ABCDEFGHIJK", false},
		{"/USDT", false},
		{"BTC/", false},
		{"/", false},
		{"", false},
		{"BTC/USDT/ETH", false},
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			got := isValidSymbol(tt.symbol)
			if got != tt.want {
				t.Errorf("isValidSymbol(%q) = %v, want %v", tt.symbol, got, tt.want)
			}
		})
	}
}

// --- service List tests ---

func TestList_Empty(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	items, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("List() returned %d items, want 0", len(items))
	}
}

func TestList_WithItems(t *testing.T) {
	repo := newMockRepo()
	repo.seed(1, "BTC/USDT", "ETH/USDT", "SOL/USDT")
	svc := NewService(repo)

	items, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("List() returned %d items, want 3", len(items))
	}
}

func TestList_RepoError(t *testing.T) {
	repo := newMockRepo()
	repo.getErr = fmt.Errorf("db connection failed")
	svc := NewService(repo)

	_, err := svc.List(context.Background(), 1)
	if err == nil {
		t.Fatal("List() expected error, got nil")
	}
}

func TestList_IsolatedByUser(t *testing.T) {
	repo := newMockRepo()
	repo.seed(1, "BTC/USDT")
	repo.seed(2, "ETH/USDT", "SOL/USDT")
	svc := NewService(repo)

	items1, _ := svc.List(context.Background(), 1)
	items2, _ := svc.List(context.Background(), 2)

	if len(items1) != 1 {
		t.Errorf("user 1 has %d items, want 1", len(items1))
	}
	if len(items2) != 2 {
		t.Errorf("user 2 has %d items, want 2", len(items2))
	}
}

// --- service Add tests ---

func TestAdd_Success(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	err := svc.Add(context.Background(), 1, "BTCUSDT")
	if err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	items, _ := svc.List(context.Background(), 1)
	if len(items) != 1 {
		t.Fatalf("expected 1 item after add, got %d", len(items))
	}
	if items[0].Symbol != "BTC/USDT" {
		t.Errorf("symbol = %q, want %q", items[0].Symbol, "BTC/USDT")
	}
}

func TestAdd_NormalizesInput(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_ = svc.Add(context.Background(), 1, "  ethusdt  ")

	items, _ := svc.List(context.Background(), 1)
	if len(items) != 1 || items[0].Symbol != "ETH/USDT" {
		t.Errorf("expected normalized ETH/USDT, got %v", items)
	}
}

func TestAdd_InvalidSymbol(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	err := svc.Add(context.Background(), 1, "X")
	if err == nil {
		t.Fatal("Add() expected error for invalid symbol")
	}
}

func TestAdd_AlreadyExists(t *testing.T) {
	repo := newMockRepo()
	repo.seed(1, "BTC/USDT")
	svc := NewService(repo)

	err := svc.Add(context.Background(), 1, "BTCUSDT")
	if err == nil {
		t.Fatal("Add() expected error for duplicate symbol")
	}
}

func TestAdd_WatchlistFull(t *testing.T) {
	repo := newMockRepo()
	// fill up to max
	symbols := make([]string, maxWatchlistSize)
	for i := 0; i < maxWatchlistSize; i++ {
		symbols[i] = fmt.Sprintf("SYM%d/USDT", i)
	}
	repo.seed(1, symbols...)
	svc := NewService(repo)

	err := svc.Add(context.Background(), 1, "NEW/USDT")
	if err == nil {
		t.Fatal("Add() expected error when watchlist is full")
	}
}

func TestAdd_ExistsCheckError(t *testing.T) {
	repo := newMockRepo()
	repo.existsErr = fmt.Errorf("db error")
	svc := NewService(repo)

	err := svc.Add(context.Background(), 1, "BTC/USDT")
	if err == nil {
		t.Fatal("Add() expected error when exists check fails")
	}
}

func TestAdd_CountError(t *testing.T) {
	repo := newMockRepo()
	repo.countErr = fmt.Errorf("db error")
	svc := NewService(repo)

	err := svc.Add(context.Background(), 1, "BTC/USDT")
	if err == nil {
		t.Fatal("Add() expected error when count fails")
	}
}

func TestAdd_RepoAddError(t *testing.T) {
	repo := newMockRepo()
	repo.addErr = fmt.Errorf("insert failed")
	svc := NewService(repo)

	err := svc.Add(context.Background(), 1, "BTC/USDT")
	if err == nil {
		t.Fatal("Add() expected error when repo add fails")
	}
}

// --- service Remove tests ---

func TestRemove_Success(t *testing.T) {
	repo := newMockRepo()
	repo.seed(1, "BTC/USDT", "ETH/USDT")
	svc := NewService(repo)

	err := svc.Remove(context.Background(), 1, "BTCUSDT")
	if err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	items, _ := svc.List(context.Background(), 1)
	if len(items) != 1 {
		t.Errorf("expected 1 item after remove, got %d", len(items))
	}
}

func TestRemove_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	err := svc.Remove(context.Background(), 1, "BTC/USDT")
	if err == nil {
		t.Fatal("Remove() expected error for non-existent symbol")
	}
}

func TestRemove_RepoError(t *testing.T) {
	repo := newMockRepo()
	repo.removeErr = fmt.Errorf("db error")
	svc := NewService(repo)

	err := svc.Remove(context.Background(), 1, "BTC/USDT")
	if err == nil {
		t.Fatal("Remove() expected error when repo fails")
	}
}

// --- service Reset tests ---

func TestReset_Success(t *testing.T) {
	repo := newMockRepo()
	repo.seed(1, "DOGE/USDT")
	svc := NewService(repo)

	err := svc.Reset(context.Background(), 1)
	if err != nil {
		t.Fatalf("Reset() error: %v", err)
	}

	items, _ := svc.List(context.Background(), 1)
	if len(items) != 2 { // mock reset returns 2 default items
		t.Errorf("expected 2 items after reset, got %d", len(items))
	}
}

func TestReset_RepoError(t *testing.T) {
	repo := newMockRepo()
	repo.resetErr = fmt.Errorf("db error")
	svc := NewService(repo)

	err := svc.Reset(context.Background(), 1)
	if err == nil {
		t.Fatal("Reset() expected error when repo fails")
	}
}

// --- service Add at exactly max capacity boundary ---

func TestAdd_AtExactlyMaxMinusOne(t *testing.T) {
	repo := newMockRepo()
	symbols := make([]string, maxWatchlistSize-1)
	for i := 0; i < maxWatchlistSize-1; i++ {
		symbols[i] = fmt.Sprintf("SYM%d/USDT", i)
	}
	repo.seed(1, symbols...)
	svc := NewService(repo)

	// should succeed - one slot left
	err := svc.Add(context.Background(), 1, "LAST/USDT")
	if err != nil {
		t.Fatalf("Add() at max-1 should succeed: %v", err)
	}

	// now at max, next add should fail
	err = svc.Add(context.Background(), 1, "OVERFLOW/USDT")
	if err == nil {
		t.Fatal("Add() at max should fail")
	}
}
