package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/database"
)

// newTestServer creates a Server with nil repos (sufficient for auth/validation tests).
func newTestServer(apiKey string) *Server {
	return &Server{apiKey: apiKey}
}

func serve(srv *Server, method, url string, headers map[string]string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	req := httptest.NewRequest(method, url, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	// recover from panics caused by nil repos (handlers pass auth/validation then hit nil pool)
	func() {
		defer func() { recover() }()
		mux.ServeHTTP(rr, req)
	}()
	return rr
}

// ==================== auth middleware ====================

func TestAuth_MissingKey(t *testing.T) {
	rr := serve(newTestServer("secret123"), http.MethodGet, "/api/positions?user_id=1", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
	var body map[string]string
	json.NewDecoder(rr.Body).Decode(&body)
	if body["error"] == "" {
		t.Error("expected error message")
	}
}

func TestAuth_ValidKey_Header(t *testing.T) {
	rr := serve(newTestServer("secret123"), http.MethodGet, "/api/positions?user_id=1",
		map[string]string{"X-API-Key": "secret123"})
	// passes auth → panics on nil repo → recovered as 500, but NOT 401
	if rr.Code == http.StatusUnauthorized {
		t.Error("should pass auth with valid header key")
	}
}

func TestAuth_ValidKey_QueryParam(t *testing.T) {
	rr := serve(newTestServer("secret123"), http.MethodGet,
		"/api/positions?user_id=1&api_key=secret123", nil)
	if rr.Code == http.StatusUnauthorized {
		t.Error("should pass auth with query param key")
	}
}

func TestAuth_WrongKey(t *testing.T) {
	rr := serve(newTestServer("secret123"), http.MethodGet, "/api/positions?user_id=1",
		map[string]string{"X-API-Key": "wrong"})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAuth_NoKey_ConfiguredEmpty(t *testing.T) {
	// empty apiKey disables auth
	rr := serve(newTestServer(""), http.MethodGet, "/api/positions?user_id=1", nil)
	if rr.Code == http.StatusUnauthorized {
		t.Error("should not require auth when api key is empty")
	}
}

func TestMethodNotAllowed_POST(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodPost, "/api/positions?user_id=1", nil)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestMethodNotAllowed_PUT(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodPut, "/api/trades?user_id=1", nil)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestMethodNotAllowed_DELETE(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodDelete, "/api/candles?symbol=BTC", nil)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

// ==================== parameter validation ====================

func TestPositions_MissingUserID(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodGet, "/api/positions", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestPositions_InvalidStatus(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodGet, "/api/positions?user_id=1&status=INVALID", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestPositions_ValidStatuses(t *testing.T) {
	for _, status := range []string{"OPEN", "CLOSED", "LIQUIDATED", "open", "closed"} {
		rr := serve(newTestServer(""), http.MethodGet, "/api/positions?user_id=1&status="+status, nil)
		if rr.Code == http.StatusBadRequest {
			t.Errorf("status=%s should be valid, got 400", status)
		}
	}
}

func TestTrades_MissingUserID(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodGet, "/api/trades", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestDecisions_MissingUserID(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodGet, "/api/decisions", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestDecisions_MissingSymbol(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodGet, "/api/decisions?user_id=1", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestDailyStats_MissingUserID(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodGet, "/api/stats/daily", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestSummary_MissingUserID(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodGet, "/api/stats/summary", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestCandles_MissingSymbol(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodGet, "/api/candles", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestPositionByID_InvalidID(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodGet, "/api/positions/abc", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestPositionByID_NegativeID(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodGet, "/api/positions/-1", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestPositionByID_ZeroID(t *testing.T) {
	rr := serve(newTestServer(""), http.MethodGet, "/api/positions/0", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// ==================== helper functions ====================

func TestParseDateRangeParams_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	from, to := parseDateRangeParams(req, 7)

	if to.Before(time.Now().Truncate(24 * time.Hour)) {
		t.Error("default 'to' should be today or later")
	}
	expected := time.Now().AddDate(0, 0, -7)
	if from.After(expected.Add(time.Hour)) {
		t.Error("default 'from' should be ~7 days ago")
	}
}

func TestParseDateRangeParams_Custom(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?from=2024-06-01&to=2024-12-31", nil)
	from, to := parseDateRangeParams(req, 7)

	if from.Format("2006-01-02") != "2024-06-01" {
		t.Errorf("from = %s, want 2024-06-01", from.Format("2006-01-02"))
	}
	if to.Format("2006-01-02") != "2024-12-31" {
		t.Errorf("to = %s, want 2024-12-31", to.Format("2006-01-02"))
	}
}

func TestParseDateRangeParams_InvalidDates(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?from=bad&to=bad", nil)
	from, to := parseDateRangeParams(req, 30)

	// should fall back to defaults
	expected := time.Now().AddDate(0, 0, -30)
	if from.After(expected.Add(time.Hour)) {
		t.Error("invalid from should fall back to default")
	}
	if to.Before(time.Now().Truncate(24 * time.Hour)) {
		t.Error("invalid to should fall back to default")
	}
}

func TestIntParam_Default(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	if intParam(req, "limit", 50) != 50 {
		t.Error("should return default")
	}
}

func TestIntParam_Provided(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?limit=25", nil)
	if intParam(req, "limit", 50) != 25 {
		t.Errorf("got %d, want 25", intParam(req, "limit", 50))
	}
}

func TestIntParam_Invalid(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?limit=abc", nil)
	if intParam(req, "limit", 50) != 50 {
		t.Error("should return default for invalid input")
	}
}

func TestIntParam_Negative(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?limit=-1", nil)
	if intParam(req, "limit", 50) != 50 {
		t.Error("should return default for negative input")
	}
}

func TestIntParam_Zero(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?limit=0", nil)
	if intParam(req, "limit", 50) != 50 {
		t.Error("should return default for zero")
	}
}

func TestRequiredIntParam_Valid(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?user_id=42", nil)
	v, err := requiredIntParam(req, "user_id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 42 {
		t.Errorf("got %d, want 42", v)
	}
}

func TestRequiredIntParam_Missing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, err := requiredIntParam(req, "user_id")
	if err == nil {
		t.Error("should error when param missing")
	}
}

func TestRequiredIntParam_NonNumeric(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?user_id=abc", nil)
	_, err := requiredIntParam(req, "user_id")
	if err == nil {
		t.Error("should error for non-numeric")
	}
}

// ==================== response helpers ====================

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, map[string]string{"status": "ok"})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %s", ct)
	}
	var body map[string]string
	json.NewDecoder(rr.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Error("unexpected body")
	}
}

func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusBadRequest, "bad input")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	var body map[string]string
	json.NewDecoder(rr.Body).Decode(&body)
	if body["error"] != "bad input" {
		t.Errorf("error = %s", body["error"])
	}
}

// ==================== model converters ====================

func TestPositionToAPI_Full(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	closed := now.Add(time.Hour)
	p := &database.PersistedPosition{
		ID: 1, InternalID: "pt_1", UserID: 5, Symbol: "BTC/USDT",
		Side: "LONG", PositionType: "FUTURES", Status: "CLOSED",
		EntryPrice: 50000, ClosePrice: 52000, Quantity: 0.1,
		PositionSize: 5000, Leverage: 10, IsPaper: true,
		RealizedPnL: 200, CloseReason: "take_profit", Platform: "binance",
		StopLoss: 48000, TakeProfit: 55000, Margin: 500,
		OpenedAt: now, ClosedAt: &closed,
	}

	result := positionToAPI(p)
	if result["id"] != 1 {
		t.Errorf("id = %v, want 1", result["id"])
	}
	if result["symbol"] != "BTC/USDT" {
		t.Errorf("symbol = %v", result["symbol"])
	}
	if result["position_type"] != "FUTURES" {
		t.Errorf("position_type = %v", result["position_type"])
	}
	if result["close_reason"] != "take_profit" {
		t.Errorf("close_reason = %v", result["close_reason"])
	}
	if result["stop_loss"] != 48000.0 {
		t.Errorf("stop_loss = %v", result["stop_loss"])
	}
	if result["take_profit"] != 55000.0 {
		t.Errorf("take_profit = %v", result["take_profit"])
	}
	if result["margin"] != 500.0 {
		t.Errorf("margin = %v", result["margin"])
	}
	if _, ok := result["closed_at"]; !ok {
		t.Error("closed_at should be present")
	}
	if result["opened_at"] != now.Format(time.RFC3339) {
		t.Errorf("opened_at = %v", result["opened_at"])
	}
}

func TestPositionToAPI_OmitsZeroFields(t *testing.T) {
	p := &database.PersistedPosition{
		ID: 1, Symbol: "ETH/USDT", Side: "LONG", Status: "OPEN",
		EntryPrice: 3000, Quantity: 1, IsPaper: true,
		OpenedAt: time.Now(),
	}

	result := positionToAPI(p)
	for _, field := range []string{"stop_loss", "take_profit", "close_price", "close_reason",
		"closed_at", "mark_price", "liquidation_price", "margin", "unrealized_pnl",
		"realized_pnl", "funding_paid", "action"} {
		if _, ok := result[field]; ok {
			t.Errorf("zero %s should be omitted", field)
		}
	}
}

func TestPositionsToAPI_Slice(t *testing.T) {
	positions := []*database.PersistedPosition{
		{ID: 1, Symbol: "BTC/USDT", Side: "LONG", Status: "OPEN", OpenedAt: time.Now()},
		{ID: 2, Symbol: "ETH/USDT", Side: "SHORT", Status: "CLOSED", OpenedAt: time.Now()},
	}
	result := positionsToAPI(positions)
	if len(result) != 2 {
		t.Errorf("len = %d, want 2", len(result))
	}
	if result[0]["symbol"] != "BTC/USDT" {
		t.Errorf("first symbol = %v", result[0]["symbol"])
	}
	if result[1]["symbol"] != "ETH/USDT" {
		t.Errorf("second symbol = %v", result[1]["symbol"])
	}
}

func TestTradesToAPI(t *testing.T) {
	posID := 42
	trades := []*database.TradeRecord{
		{
			ID: 1, UserID: 5, Symbol: "BTC/USDT", Side: "BUY",
			TradeType: "FUTURES_LONG", Quantity: 0.1, Price: 50000,
			Fee: 5.0, FeeCurrency: "USDT", IsPaper: false,
			PositionID: &posID, ExchangeOrderID: "binance_123",
			ExecutedAt: time.Now(),
		},
		{
			ID: 2, UserID: 5, Symbol: "ETH/USDT", Side: "SELL",
			TradeType: "SPOT", Quantity: 1, Price: 3000,
			IsPaper: true, ExecutedAt: time.Now(),
		},
	}

	result := tradesToAPI(trades)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}

	// first trade: has position_id, exchange_order_id, fee_currency
	if result[0]["position_id"] != 42 {
		t.Errorf("position_id = %v", result[0]["position_id"])
	}
	if result[0]["exchange_order_id"] != "binance_123" {
		t.Errorf("exchange_order_id = %v", result[0]["exchange_order_id"])
	}
	if result[0]["fee_currency"] != "USDT" {
		t.Errorf("fee_currency = %v", result[0]["fee_currency"])
	}

	// second trade: no optional fields
	if _, ok := result[1]["position_id"]; ok {
		t.Error("nil position_id should be omitted")
	}
	if _, ok := result[1]["exchange_order_id"]; ok {
		t.Error("empty exchange_order_id should be omitted")
	}
	if _, ok := result[1]["fee_currency"]; ok {
		t.Error("empty fee_currency should be omitted")
	}
}

func TestDecisionsToAPI(t *testing.T) {
	approved := true
	decisions := []*database.AIDecisionRecord{
		{
			ID: 1, UserID: 5, Symbol: "BTC/USDT", Decision: "BUY",
			Confidence: 85, Timeframe: "4h", EntryPrice: 50000,
			StopLoss: 48000, TakeProfit: 55000, PositionSizeUSD: 5000,
			RiskRewardRatio: 2.5, Reasoning: "strong bullish",
			PromptTokens: 1000, CompletionTokens: 500, LatencyMs: 1200,
			WasApproved: &approved, WasExecuted: true,
			IndicatorsData: map[string]interface{}{"rsi": 65},
			CreatedAt: time.Now(),
		},
		{
			ID: 2, UserID: 5, Symbol: "ETH/USDT", Decision: "HOLD",
			Confidence: 40, WasExecuted: false, CreatedAt: time.Now(),
		},
	}

	result := decisionsToAPI(decisions)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}

	// first: full fields
	if result[0]["confidence"] != 85 {
		t.Errorf("confidence = %v", result[0]["confidence"])
	}
	if result[0]["reasoning"] != "strong bullish" {
		t.Errorf("reasoning = %v", result[0]["reasoning"])
	}
	if result[0]["was_approved"] != true {
		t.Errorf("was_approved = %v", result[0]["was_approved"])
	}
	if result[0]["prompt_tokens"] != 1000 {
		t.Errorf("prompt_tokens = %v", result[0]["prompt_tokens"])
	}

	// second: sparse — should omit zero fields
	for _, field := range []string{"timeframe", "entry_price", "stop_loss", "take_profit",
		"reasoning", "prompt_tokens", "completion_tokens", "latency_ms", "was_approved"} {
		if _, ok := result[1][field]; ok {
			t.Errorf("zero %s should be omitted", field)
		}
	}
}

func TestDailyStatsToAPI(t *testing.T) {
	date := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	stats := []*database.DailyStatsRecord{
		{
			ID: 1, UserID: 5, Date: date,
			TotalTrades: 10, WinningTrades: 7, LosingTrades: 3,
			RealizedPnL: 500.50, FeesPaid: 25.0, FundingPaid: 5.0,
			AIDecisionsMade: 15, AIDecisionsApproved: 8, NotificationsSent: 20,
		},
	}

	result := dailyStatsToAPI(stats)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}

	if result[0]["date"] != "2024-06-15" {
		t.Errorf("date = %v", result[0]["date"])
	}
	if result[0]["total_trades"] != 10 {
		t.Errorf("total_trades = %v", result[0]["total_trades"])
	}
	if result[0]["winning_trades"] != 7 {
		t.Errorf("winning_trades = %v", result[0]["winning_trades"])
	}
	if result[0]["realized_pnl"] != 500.50 {
		t.Errorf("realized_pnl = %v", result[0]["realized_pnl"])
	}
	if result[0]["fees_paid"] != 25.0 {
		t.Errorf("fees_paid = %v", result[0]["fees_paid"])
	}
	if result[0]["ai_decisions_made"] != 15 {
		t.Errorf("ai_decisions_made = %v", result[0]["ai_decisions_made"])
	}
}
