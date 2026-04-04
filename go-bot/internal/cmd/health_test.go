package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthCheck_Liveness(t *testing.T) {
	hs := newHealthServer(nil, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", hs.handleHealth)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var status healthStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if status.Status != "ok" {
		t.Fatalf("expected status 'ok', got %q", status.Status)
	}
}

func TestHealthCheck_Readiness_NoDeps(t *testing.T) {
	hs := newHealthServer(nil, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/health/ready", hs.handleReady)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHealthCheck_WithBinancePing(t *testing.T) {
	// mock binance server
	mockBinance := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/ping" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockBinance.Close()

	hs := newHealthServer(nil, nil)
	hs.SetBinanceURL(mockBinance.URL)

	mux := http.NewServeMux()
	mux.HandleFunc("/health/ready", hs.handleReady)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var status healthStatus
	json.NewDecoder(w.Body).Decode(&status)

	if status.Services["binance"] != "up" {
		t.Fatalf("expected binance 'up', got %q", status.Services["binance"])
	}
}

func TestHealthCheck_WithMLService(t *testing.T) {
	mockML := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockML.Close()

	hs := newHealthServer(nil, nil)
	hs.SetMLURL(mockML.URL)

	mux := http.NewServeMux()
	mux.HandleFunc("/health/ready", hs.handleReady)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var status healthStatus
	json.NewDecoder(w.Body).Decode(&status)

	if status.Services["ml_service"] != "up" {
		t.Fatalf("expected ml_service 'up', got %q", status.Services["ml_service"])
	}
}

func TestHealthCheck_BinanceDown(t *testing.T) {
	hs := newHealthServer(nil, nil)
	hs.SetBinanceURL("http://localhost:1") // nothing listening

	mux := http.NewServeMux()
	mux.HandleFunc("/health/ready", hs.handleReady)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var status healthStatus
	json.NewDecoder(w.Body).Decode(&status)

	if status.Services["binance"] != "down" {
		t.Fatalf("expected binance 'down', got %q", status.Services["binance"])
	}
}

func TestHttpPing_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := httpPing(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestHttpPing_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	err := httpPing(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestHttpPing_Unreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := httpPing(ctx, "http://localhost:1") // nothing listening
	if err == nil {
		t.Fatal("expected error for unreachable host")
	}
}
