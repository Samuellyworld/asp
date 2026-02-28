// discord rest api client for sending messages and managing slash commands
package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const apiBaseURL = "https://discord.com/api/v10"

// Bot wraps the discord rest api
type Bot struct {
	token         string
	applicationID string
	client        *http.Client
}

func NewBot(token, applicationID string) *Bot {
	return &Bot{
		token:         token,
		applicationID: applicationID,
		client:        &http.Client{},
	}
}

// sends a plain text message to a channel
func (b *Bot) SendMessage(channelID string, content string) error {
	return b.sendJSON("POST", fmt.Sprintf("/channels/%s/messages", channelID), map[string]interface{}{
		"content": content,
	})
}

// sends a message with embeds and components
func (b *Bot) SendEmbed(channelID string, content string, embeds []Embed, components []Component) error {
	payload := map[string]interface{}{
		"content": content,
	}
	if len(embeds) > 0 {
		payload["embeds"] = embeds
	}
	if len(components) > 0 {
		payload["components"] = components
	}
	return b.sendJSON("POST", fmt.Sprintf("/channels/%s/messages", channelID), payload)
}

// responds to an interaction immediately
func (b *Bot) RespondInteraction(interactionID, interactionToken string, resp *InteractionResponse) error {
	url := fmt.Sprintf("%s/interactions/%s/%s/callback", apiBaseURL, interactionID, interactionToken)

	body, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal interaction response: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to respond to interaction: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("discord api error (status %d): %s", httpResp.StatusCode, string(respBody))
	}
	return nil
}

// edits the original interaction response
func (b *Bot) EditInteractionResponse(interactionToken string, content string, embeds []Embed, components []Component) error {
	payload := map[string]interface{}{}
	if content != "" {
		payload["content"] = content
	}
	if embeds != nil {
		payload["embeds"] = embeds
	}
	if components != nil {
		payload["components"] = components
	}
	return b.sendJSON("PATCH", fmt.Sprintf("/webhooks/%s/%s/messages/@original", b.applicationID, interactionToken), payload)
}

// registers slash commands globally for the application
func (b *Bot) RegisterCommands(commands []ApplicationCommand) error {
	return b.sendJSON("PUT", fmt.Sprintf("/applications/%s/commands", b.applicationID), commands)
}

// returns the gateway websocket url
func (b *Bot) GetGatewayURL() (string, error) {
	req, err := http.NewRequest("GET", apiBaseURL+"/gateway/bot", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bot "+b.token)

	resp, err := b.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get gateway url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("discord api error (status %d): %s", resp.StatusCode, string(body))
	}

	var gw GatewayURLResponse
	if err := json.NewDecoder(resp.Body).Decode(&gw); err != nil {
		return "", fmt.Errorf("failed to decode gateway response: %w", err)
	}
	return gw.URL, nil
}

func (b *Bot) sendJSON(method, path string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	url := apiBaseURL + path
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bot "+b.token)

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("discord api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord api error (status %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}
