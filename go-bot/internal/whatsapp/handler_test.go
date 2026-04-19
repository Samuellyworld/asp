package whatsapp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleVerification(t *testing.T) {
	bot := NewBot("phone123", "token")
	handler := NewCommandHandler(bot, "my-verify-token")

	req := httptest.NewRequest("GET",
		"/webhook?hub.mode=subscribe&hub.verify_token=my-verify-token&hub.challenge=test-challenge", nil)
	w := httptest.NewRecorder()

	handler.HandleVerification(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "test-challenge" {
		t.Fatalf("expected challenge echo, got %q", w.Body.String())
	}
}

func TestHandleVerificationBadToken(t *testing.T) {
	bot := NewBot("phone123", "token")
	handler := NewCommandHandler(bot, "my-verify-token")

	req := httptest.NewRequest("GET",
		"/webhook?hub.mode=subscribe&hub.verify_token=wrong-token&hub.challenge=test", nil)
	w := httptest.NewRecorder()

	handler.HandleVerification(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestHandleWebhookReturns200(t *testing.T) {
	// capture messages instead of sending to real API
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	bot := NewBot("phone123", "token")
	bot.apiBase = srv.URL
	handler := NewCommandHandler(bot, "verify")

	webhook := Webhook{
		Object: "whatsapp_business_account",
		Entry: []WebhookEntry{{
			Changes: []WebhookChange{{
				Value: WebhookValue{
					Messages: []IncomingMessage{{
						From: "1234567890",
						Type: "text",
						Text: MessageText{Body: "/help"},
					}},
				},
			}},
		}},
	}

	body, _ := json.Marshal(webhook)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCustomCommandRegistration(t *testing.T) {
	bot := NewBot("phone123", "token")
	handler := NewCommandHandler(bot, "verify")

	called := false
	handler.RegisterCommand("ping", func(_ string, _ []string) string {
		called = true
		return "pong"
	})

	if _, ok := handler.commands["ping"]; !ok {
		t.Fatal("expected ping command to be registered")
	}

	result := handler.commands["ping"]("", nil)
	if !called || result != "pong" {
		t.Fatal("command not executed correctly")
	}
}
