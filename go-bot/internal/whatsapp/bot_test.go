package whatsapp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestNewBot(t *testing.T) {
	bot := NewBot("12345", "token123")
	if bot.phoneNumberID != "12345" {
		t.Errorf("expected phone number ID 12345, got %s", bot.phoneNumberID)
	}
	if bot.accessToken != "token123" {
		t.Errorf("expected token token123, got %s", bot.accessToken)
	}
	if bot.apiBase != defaultAPIBase {
		t.Errorf("expected default API base, got %s", bot.apiBase)
	}
}

func TestSendMessage_Success(t *testing.T) {
	var mu sync.Mutex
	var receivedPayload messageRequest
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		receivedAuth = r.Header.Get("Authorization")

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"messages":[{"id":"wamid.xxx"}]}`))
	}))
	defer server.Close()

	bot := NewBot("12345", "test-token")
	bot.apiBase = server.URL

	err := bot.SendMessage("+1234567890", "Hello from trading bot!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if receivedAuth != "Bearer test-token" {
		t.Errorf("expected Bearer test-token, got %s", receivedAuth)
	}
	if receivedPayload.MessagingProduct != "whatsapp" {
		t.Errorf("expected messaging_product=whatsapp, got %s", receivedPayload.MessagingProduct)
	}
	if receivedPayload.To != "+1234567890" {
		t.Errorf("expected to=+1234567890, got %s", receivedPayload.To)
	}
	if receivedPayload.Type != "text" {
		t.Errorf("expected type=text, got %s", receivedPayload.Type)
	}
	if receivedPayload.Text.Body != "Hello from trading bot!" {
		t.Errorf("expected text body 'Hello from trading bot!', got %s", receivedPayload.Text.Body)
	}
}

func TestSendMessage_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid OAuth access token"}}`))
	}))
	defer server.Close()

	bot := NewBot("12345", "bad-token")
	bot.apiBase = server.URL

	err := bot.SendMessage("+1234567890", "test")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestSendMessage_NetworkError(t *testing.T) {
	bot := NewBot("12345", "token")
	bot.apiBase = "http://localhost:1" // nothing listening

	err := bot.SendMessage("+1234567890", "test")
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

func TestSendTemplate_Success(t *testing.T) {
	var mu sync.Mutex
	var receivedPayload templateRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"messages":[{"id":"wamid.xxx"}]}`))
	}))
	defer server.Close()

	bot := NewBot("12345", "token")
	bot.apiBase = server.URL

	err := bot.SendTemplate("+1234567890", "trading_alert", "en_US")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if receivedPayload.MessagingProduct != "whatsapp" {
		t.Errorf("expected messaging_product=whatsapp, got %s", receivedPayload.MessagingProduct)
	}
	if receivedPayload.Type != "template" {
		t.Errorf("expected type=template, got %s", receivedPayload.Type)
	}
	if receivedPayload.Template.Name != "trading_alert" {
		t.Errorf("expected template name trading_alert, got %s", receivedPayload.Template.Name)
	}
	if receivedPayload.Template.Language.Code != "en_US" {
		t.Errorf("expected language en_US, got %s", receivedPayload.Template.Language.Code)
	}
}

func TestSendTemplate_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Template not found"}}`))
	}))
	defer server.Close()

	bot := NewBot("12345", "token")
	bot.apiBase = server.URL

	err := bot.SendTemplate("+1234567890", "nonexistent", "en_US")
	if err == nil {
		t.Fatal("expected error for bad template")
	}
}

func TestSendMessage_ContentType(t *testing.T) {
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"messages":[{"id":"wamid.xxx"}]}`))
	}))
	defer server.Close()

	bot := NewBot("12345", "token")
	bot.apiBase = server.URL

	bot.SendMessage("+1234567890", "test")

	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", receivedContentType)
	}
}

func TestSendMessage_EndpointFormat(t *testing.T) {
	var receivedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"messages":[{"id":"wamid.xxx"}]}`))
	}))
	defer server.Close()

	bot := NewBot("99887766", "token")
	bot.apiBase = server.URL

	bot.SendMessage("+1234567890", "test")

	if receivedPath != "/99887766/messages" {
		t.Errorf("expected path /99887766/messages, got %s", receivedPath)
	}
}

func TestWebhookParsing(t *testing.T) {
	raw := `{
		"object": "whatsapp_business_account",
		"entry": [{
			"id": "123",
			"changes": [{
				"value": {
					"messaging_product": "whatsapp",
					"messages": [{
						"from": "+1234567890",
						"id": "wamid.abc",
						"type": "text",
						"text": {"body": "/start"}
					}],
					"contacts": [{
						"wa_id": "+1234567890",
						"profile": {"name": "John"}
					}]
				},
				"field": "messages"
			}]
		}]
	}`

	var webhook Webhook
	if err := json.Unmarshal([]byte(raw), &webhook); err != nil {
		t.Fatalf("failed to parse webhook: %v", err)
	}

	if webhook.Object != "whatsapp_business_account" {
		t.Errorf("expected object whatsapp_business_account, got %s", webhook.Object)
	}
	if len(webhook.Entry) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(webhook.Entry))
	}
	if len(webhook.Entry[0].Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(webhook.Entry[0].Changes))
	}

	msg := webhook.Entry[0].Changes[0].Value.Messages[0]
	if msg.From != "+1234567890" {
		t.Errorf("expected from +1234567890, got %s", msg.From)
	}
	if msg.Text.Body != "/start" {
		t.Errorf("expected text /start, got %s", msg.Text.Body)
	}

	contact := webhook.Entry[0].Changes[0].Value.Contacts[0]
	if contact.Profile.Name != "John" {
		t.Errorf("expected contact name John, got %s", contact.Profile.Name)
	}
}
