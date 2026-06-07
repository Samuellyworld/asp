// telegram bot message types and helpers
package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	apiBase               = "https://api.telegram.org/bot"
	maxTelegramMessageLen = 4096
)

// update represents an incoming telegram update
type Update struct {
	UpdateID      int            `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

// message represents a telegram message
type Message struct {
	MessageID int    `json:"message_id"`
	From      *From  `json:"from"`
	Chat      *Chat  `json:"chat"`
	Text      string `json:"text"`
}

// from represents the sender of a message
type From struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// chat represents a telegram chat
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// callback query from an inline keyboard button press
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *From    `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

// inline keyboard button with callback data or url
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// inline keyboard markup containing rows of buttons
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// bot wraps the telegram bot api
type Bot struct {
	token  string
	client *http.Client
}

func NewBot(token string) *Bot {
	return &Bot{
		token:  token,
		client: &http.Client{},
	}
}

// sendMessage sends a text message to a chat.
// Tries Markdown first; falls back to plain text if Telegram can't parse entities.
func (b *Bot) SendMessage(chatID int64, text string) error {
	parts := splitTelegramMessage(text, maxTelegramMessageLen)
	for _, part := range parts {
		if err := b.sendSingleMessage(chatID, part); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bot) sendSingleMessage(chatID int64, text string) error {
	endpoint := fmt.Sprintf("%s%s/sendMessage", apiBase, b.token)

	data := url.Values{}
	data.Set("chat_id", strconv.FormatInt(chatID, 10))
	data.Set("text", text)
	data.Set("parse_mode", "Markdown")

	resp, err := b.client.PostForm(endpoint, data)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(body), "can't parse entities") {
			// retry without Markdown
			data.Del("parse_mode")
			resp2, err2 := b.client.PostForm(endpoint, data)
			if err2 != nil {
				return fmt.Errorf("failed to send plain message: %w", err2)
			}
			defer resp2.Body.Close()
			if resp2.StatusCode != http.StatusOK {
				body2, _ := io.ReadAll(resp2.Body)
				return fmt.Errorf("telegram api error (status %d): %s", resp2.StatusCode, string(body2))
			}
			return nil
		}
		return fmt.Errorf("telegram api error (status %d): %s", resp.StatusCode, string(body))
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram api error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// sends a message with an inline keyboard attached
func (b *Bot) SendMessageWithKeyboard(chatID int64, text string, keyboard *InlineKeyboardMarkup) error {
	text = truncateTelegramMessage(text, maxTelegramMessageLen)
	endpoint := fmt.Sprintf("%s%s/sendMessage", apiBase, b.token)

	payload := map[string]interface{}{
		"chat_id":      chatID,
		"text":         text,
		"parse_mode":   "Markdown",
		"reply_markup": keyboard,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := b.client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send message with keyboard: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		respBody, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(respBody), "can't parse entities") {
			delete(payload, "parse_mode")
			body, err = json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("failed to marshal plain keyboard payload: %w", err)
			}
			resp2, err2 := b.client.Post(endpoint, "application/json", bytes.NewReader(body))
			if err2 != nil {
				return fmt.Errorf("failed to send plain message with keyboard: %w", err2)
			}
			defer resp2.Body.Close()
			if resp2.StatusCode != http.StatusOK {
				respBody2, _ := io.ReadAll(resp2.Body)
				return fmt.Errorf("telegram api error (status %d): %s", resp2.StatusCode, string(respBody2))
			}
			return nil
		}
		return fmt.Errorf("telegram api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func truncateTelegramMessage(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	const suffix = "\n\n[truncated]"
	if maxLen <= len(suffix) {
		return text[:utf8SafeCut(text, maxLen)]
	}
	return text[:utf8SafeCut(text, maxLen-len(suffix))] + suffix
}

func splitTelegramMessage(text string, maxLen int) []string {
	if text == "" {
		return []string{""}
	}

	var parts []string
	remaining := text
	for len(remaining) > maxLen {
		cut := strings.LastIndex(remaining[:maxLen], "\n")
		if cut <= 0 {
			cut = maxLen
		}
		cut = utf8SafeCut(remaining, cut)
		parts = append(parts, remaining[:cut])
		remaining = strings.TrimLeft(remaining[cut:], "\n")
	}
	parts = append(parts, remaining)
	return parts
}

func utf8SafeCut(text string, cut int) int {
	if cut >= len(text) {
		return len(text)
	}
	if cut <= 0 {
		return 0
	}
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return cut
}

// edits the text (and optionally keyboard) of an existing message
func (b *Bot) EditMessageText(chatID int64, messageID int, text string, keyboard *InlineKeyboardMarkup) error {
	endpoint := fmt.Sprintf("%s%s/editMessageText", apiBase, b.token)

	payload := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
		"parse_mode": "Markdown",
	}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := b.client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to edit message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// answers a callback query to dismiss the loading indicator
func (b *Bot) AnswerCallbackQuery(queryID string, text string) error {
	endpoint := fmt.Sprintf("%s%s/answerCallbackQuery", apiBase, b.token)

	payload := map[string]interface{}{
		"callback_query_id": queryID,
	}
	if text != "" {
		payload["text"] = text
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := b.client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to answer callback query: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// deleteMessage deletes a message from a chat
func (b *Bot) DeleteMessage(chatID int64, messageID int) error {
	endpoint := fmt.Sprintf("%s%s/deleteMessage", apiBase, b.token)

	data := url.Values{}
	data.Set("chat_id", strconv.FormatInt(chatID, 10))
	data.Set("message_id", strconv.Itoa(messageID))

	resp, err := b.client.PostForm(endpoint, data)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// getUpdates polls for new updates using long polling
func (b *Bot) GetUpdates(offset int, timeout int) ([]Update, error) {
	endpoint := fmt.Sprintf("%s%s/getUpdates?offset=%d&timeout=%d", apiBase, b.token, offset, timeout)

	resp, err := b.client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get updates: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse updates: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("telegram api returned ok=false")
	}

	return result.Result, nil
}

// parseCommand extracts the command name from a message text (e.g. "/start" -> "start")
func ParseCommand(text string) (command string, args string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", text
	}

	parts := strings.SplitN(text, " ", 2)
	command = strings.TrimPrefix(parts[0], "/")
	// strip @botname suffix
	if idx := strings.Index(command, "@"); idx > 0 {
		command = command[:idx]
	}

	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	return command, args
}
