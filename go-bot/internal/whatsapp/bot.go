// whatsapp cloud api client for sending messages via Meta's Business API
package whatsapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const defaultAPIBase = "https://graph.facebook.com/v21.0"

// Bot wraps the WhatsApp Cloud API
type Bot struct {
	phoneNumberID string
	accessToken   string
	apiBase       string
	client        *http.Client
}

func NewBot(phoneNumberID, accessToken string) *Bot {
	return &Bot{
		phoneNumberID: phoneNumberID,
		accessToken:   accessToken,
		apiBase:       defaultAPIBase,
		client:        &http.Client{},
	}
}

// sends a text message to a WhatsApp recipient (phone number or JID)
func (b *Bot) SendMessage(recipientID string, text string) error {
	endpoint := fmt.Sprintf("%s/%s/messages", b.apiBase, b.phoneNumberID)

	payload := messageRequest{
		MessagingProduct: "whatsapp",
		To:               recipientID,
		Type:             "text",
		Text:             textBody{Body: text},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.accessToken)

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send whatsapp message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("whatsapp api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// sends a template message (required for initiating conversations)
func (b *Bot) SendTemplate(recipientID string, templateName string, languageCode string) error {
	endpoint := fmt.Sprintf("%s/%s/messages", b.apiBase, b.phoneNumberID)

	payload := templateRequest{
		MessagingProduct: "whatsapp",
		To:               recipientID,
		Type:             "template",
		Template: templateBody{
			Name:     templateName,
			Language: templateLanguage{Code: languageCode},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal template message: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.accessToken)

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send whatsapp template: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("whatsapp api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// request types for WhatsApp Cloud API

type messageRequest struct {
	MessagingProduct string   `json:"messaging_product"`
	To               string   `json:"to"`
	Type             string   `json:"type"`
	Text             textBody `json:"text"`
}

type textBody struct {
	Body string `json:"body"`
}

type templateRequest struct {
	MessagingProduct string       `json:"messaging_product"`
	To               string       `json:"to"`
	Type             string       `json:"type"`
	Template         templateBody `json:"template"`
}

type templateBody struct {
	Name     string           `json:"name"`
	Language templateLanguage `json:"language"`
}

type templateLanguage struct {
	Code string `json:"code"`
}

// Webhook represents an incoming WhatsApp webhook notification
type Webhook struct {
	Object string         `json:"object"`
	Entry  []WebhookEntry `json:"entry"`
}

type WebhookEntry struct {
	ID      string          `json:"id"`
	Changes []WebhookChange `json:"changes"`
}

type WebhookChange struct {
	Value WebhookValue `json:"value"`
	Field string       `json:"field"`
}

type WebhookValue struct {
	MessagingProduct string           `json:"messaging_product"`
	Messages         []IncomingMessage `json:"messages"`
	Contacts         []WebhookContact `json:"contacts"`
}

type IncomingMessage struct {
	From string      `json:"from"`
	ID   string      `json:"id"`
	Type string      `json:"type"`
	Text MessageText `json:"text"`
}

type MessageText struct {
	Body string `json:"body"`
}

type WebhookContact struct {
	WaID    string         `json:"wa_id"`
	Profile ContactProfile `json:"profile"`
}

type ContactProfile struct {
	Name string `json:"name"`
}
