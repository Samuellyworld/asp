// discord gateway websocket client for receiving events
package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// event handler processes dispatched gateway events
type EventHandler interface {
	HandleInteraction(ctx context.Context, interaction *Interaction)
}

// Gateway manages the websocket connection to discord
type Gateway struct {
	token     string
	bot       *Bot
	handler   EventHandler
	conn      *websocket.Conn
	sequence  *int
	sessionID string
	mu        sync.Mutex
	done      chan struct{}
}

func NewGateway(token string, bot *Bot, handler EventHandler) *Gateway {
	return &Gateway{
		token:   token,
		bot:     bot,
		handler: handler,
		done:    make(chan struct{}),
	}
}

// connects to the gateway and starts listening for events
func (g *Gateway) Run(ctx context.Context) error {
	gatewayURL, err := g.bot.GetGatewayURL()
	if err != nil {
		return fmt.Errorf("failed to get gateway url: %w", err)
	}

	wsURL := gatewayURL + "?v=10&encoding=json"
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to gateway: %w", err)
	}
	g.conn = conn
	defer g.conn.Close()

	// read hello
	hello, err := g.readHello()
	if err != nil {
		return fmt.Errorf("failed to read hello: %w", err)
	}

	// send identify
	if err := g.sendIdentify(); err != nil {
		return fmt.Errorf("failed to identify: %w", err)
	}

	// start heartbeat
	go g.heartbeat(ctx, time.Duration(hello.HeartbeatInterval)*time.Millisecond)

	// listen for events
	return g.listen(ctx)
}

// stops the gateway connection
func (g *Gateway) Close() {
	close(g.done)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conn != nil {
		g.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		g.conn.Close()
	}
}

func (g *Gateway) readHello() (*GatewayHello, error) {
	var payload GatewayPayload
	if err := g.conn.ReadJSON(&payload); err != nil {
		return nil, fmt.Errorf("failed to read hello payload: %w", err)
	}
	if payload.Op != OpHello {
		return nil, fmt.Errorf("expected op %d (hello), got %d", OpHello, payload.Op)
	}

	var hello GatewayHello
	if err := json.Unmarshal(payload.D, &hello); err != nil {
		return nil, fmt.Errorf("failed to unmarshal hello: %w", err)
	}
	return &hello, nil
}

func (g *Gateway) sendIdentify() error {
	identify := GatewayIdentify{
		Token:   g.token,
		Intents: IntentGuilds | IntentGuildMessages | IntentDirectMessages | IntentMessageContent,
		Properties: GatewayIdentifyData{
			OS:      "linux",
			Browser: "go-bot",
			Device:  "go-bot",
		},
	}

	data, err := json.Marshal(identify)
	if err != nil {
		return err
	}

	payload := GatewayPayload{Op: OpIdentify, D: data}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.conn.WriteJSON(payload)
}

func (g *Gateway) heartbeat(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-g.done:
			return
		case <-ticker.C:
			g.mu.Lock()
			var seqData json.RawMessage
			if g.sequence != nil {
				seqData, _ = json.Marshal(*g.sequence)
			} else {
				seqData = json.RawMessage("null")
			}
			err := g.conn.WriteJSON(GatewayPayload{Op: OpHeartbeat, D: seqData})
			g.mu.Unlock()
			if err != nil {
				log.Printf("discord gateway: heartbeat error: %v", err)
				return
			}
		}
	}
}

func (g *Gateway) listen(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-g.done:
			return nil
		default:
		}

		var payload GatewayPayload
		if err := g.conn.ReadJSON(&payload); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return fmt.Errorf("gateway read error: %w", err)
		}

		if payload.S != nil {
			g.sequence = payload.S
		}

		switch payload.Op {
		case OpDispatch:
			g.handleDispatch(ctx, payload)
		case OpHeartbeatACK:
			// heartbeat acknowledged
		case OpReconnect:
			log.Println("discord gateway: reconnect requested")
			return fmt.Errorf("reconnect requested")
		case OpInvalidSess:
			log.Println("discord gateway: invalid session")
			return fmt.Errorf("invalid session")
		}
	}
}

func (g *Gateway) handleDispatch(ctx context.Context, payload GatewayPayload) {
	switch payload.T {
	case "READY":
		var ready GatewayReady
		if err := json.Unmarshal(payload.D, &ready); err != nil {
			log.Printf("discord gateway: failed to unmarshal ready: %v", err)
			return
		}
		g.sessionID = ready.SessionID
		if ready.User != nil {
			log.Printf("discord gateway: connected as %s#%s", ready.User.Username, ready.User.Discriminator)
		}

	case "INTERACTION_CREATE":
		var interaction Interaction
		if err := json.Unmarshal(payload.D, &interaction); err != nil {
			log.Printf("discord gateway: failed to unmarshal interaction: %v", err)
			return
		}
		go g.handler.HandleInteraction(ctx, &interaction)
	}
}
