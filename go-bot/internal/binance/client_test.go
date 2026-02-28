package binance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSign(t *testing.T) {
	// known hmac-sha256 test vector
	queryString := "timestamp=1234567890"
	secret := "test-secret"

	sig := sign(queryString, secret)

	if sig == "" {
		t.Fatal("sign() returned empty string")
	}

	// same input should produce same signature
	sig2 := sign(queryString, secret)
	if sig != sig2 {
		t.Error("sign() is not deterministic")
	}

	// different secret should produce different signature
	sig3 := sign(queryString, "other-secret")
	if sig == sig3 {
		t.Error("different secrets produced same signature")
	}

	// different query should produce different signature
	sig4 := sign("timestamp=9999999999", secret)
	if sig == sig4 {
		t.Error("different query strings produced same signature")
	}
}

func TestAPIPermissions_ToJSON(t *testing.T) {
	perms := &APIPermissions{
		Spot:     true,
		Futures:  false,
		Withdraw: true,
	}

	result := perms.ToJSON()

	if !result["spot"] {
		t.Error("expected spot=true")
	}
	if result["futures"] {
		t.Error("expected futures=false")
	}
	if !result["withdraw"] {
		t.Error("expected withdraw=true")
	}
}

func TestValidateKeys_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify api key header is present
		apiKey := r.Header.Get("X-MBX-APIKEY")
		if apiKey == "" {
			t.Error("missing X-MBX-APIKEY header")
		}

		// verify query params
		if r.URL.Query().Get("timestamp") == "" {
			t.Error("missing timestamp query param")
		}
		if r.URL.Query().Get("signature") == "" {
			t.Error("missing signature query param")
		}

		resp := accountResponse{
			CanTrade:    true,
			CanWithdraw: false,
			CanDeposit:  true,
			AccountType: "SPOT",
			Permissions: []string{"SPOT"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, true)
	perms, err := client.ValidateKeys(context.Background(), "test-key", "test-secret")
	if err != nil {
		t.Fatalf("ValidateKeys() error: %v", err)
	}

	if !perms.Spot {
		t.Error("expected spot=true")
	}
	if perms.Futures {
		t.Error("expected futures=false")
	}
	if perms.Withdraw {
		t.Error("expected withdraw=false")
	}
}

func TestValidateKeys_WithFutures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := accountResponse{
			CanTrade:    true,
			CanWithdraw: false,
			Permissions: []string{"SPOT", "FUTURES"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, true)
	perms, err := client.ValidateKeys(context.Background(), "test-key", "test-secret")
	if err != nil {
		t.Fatalf("ValidateKeys() error: %v", err)
	}

	if !perms.Spot {
		t.Error("expected spot=true")
	}
	if !perms.Futures {
		t.Error("expected futures=true")
	}
}

func TestValidateKeys_WithWithdraw(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := accountResponse{
			CanTrade:    true,
			CanWithdraw: true,
			Permissions: []string{"SPOT"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, true)
	perms, err := client.ValidateKeys(context.Background(), "test-key", "test-secret")
	if err != nil {
		t.Fatalf("ValidateKeys() error: %v", err)
	}

	if !perms.Withdraw {
		t.Error("expected withdraw=true")
	}
}

func TestValidateKeys_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(apiError{Code: -2015, Message: "Invalid API-key, IP, or permissions for action."})
	}))
	defer server.Close()

	client := NewClient(server.URL, true)
	_, err := client.ValidateKeys(context.Background(), "bad-key", "bad-secret")
	if err == nil {
		t.Fatal("ValidateKeys() expected error for invalid keys")
	}
}

func TestValidateKeys_ServerDown(t *testing.T) {
	client := NewClient("http://localhost:1", true) // nothing running on port 1
	_, err := client.ValidateKeys(context.Background(), "key", "secret")
	if err == nil {
		t.Fatal("ValidateKeys() expected error when server is down")
	}
}

func TestValidateKeys_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, true)
	_, err := client.ValidateKeys(context.Background(), "key", "secret")
	if err == nil {
		t.Fatal("ValidateKeys() expected error for invalid json response")
	}
}

func TestValidateKeys_NonJSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, true)
	_, err := client.ValidateKeys(context.Background(), "key", "secret")
	if err == nil {
		t.Fatal("ValidateKeys() expected error for 500 response")
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient("https://api.binance.com", false)
	if c == nil {
		t.Fatal("NewClient() returned nil")
	}
	if c.baseURL != "https://api.binance.com" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "https://api.binance.com")
	}
	if c.testnet {
		t.Error("testnet should be false")
	}
}
