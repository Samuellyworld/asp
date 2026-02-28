// discord api types and constants
package discord

import "encoding/json"

// gateway opcodes
const (
	OpDispatch     = 0
	OpHeartbeat    = 1
	OpIdentify     = 2
	OpResume       = 6
	OpReconnect    = 7
	OpInvalidSess  = 9
	OpHello        = 10
	OpHeartbeatACK = 11
)

// interaction types
const (
	InteractionPing      = 1
	InteractionCommand   = 2
	InteractionComponent = 3
)

// interaction response types
const (
	ResponsePong           = 1
	ResponseMessage        = 4
	ResponseDeferredMsg    = 5
	ResponseDeferredUpdate = 6
	ResponseUpdateMessage  = 7
)

// component types
const (
	ComponentActionRow = 1
	ComponentButton    = 2
)

// button styles
const (
	ButtonPrimary   = 1
	ButtonSecondary = 2
	ButtonSuccess   = 3
	ButtonDanger    = 4
	ButtonLink      = 5
)

// command option types
const (
	OptionString  = 3
	OptionInteger = 4
)

// embed colors
const (
	ColorGreen = 0x2ECC71
	ColorRed   = 0xE74C3C
	ColorBlue  = 0x3498DB
	ColorGold  = 0xF1C40F
)

// gateway intents
const (
	IntentGuilds         = 1 << 0
	IntentGuildMessages  = 1 << 9
	IntentDirectMessages = 1 << 12
	IntentMessageContent = 1 << 15
)

// ephemeral message flag (only sender can see)
const FlagEphemeral = 64

// gateway payload sent and received over websocket
type GatewayPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
	S  *int            `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

// hello event payload (op 10)
type GatewayHello struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

// identify payload (op 2)
type GatewayIdentify struct {
	Token      string              `json:"token"`
	Intents    int                 `json:"intents"`
	Properties GatewayIdentifyData `json:"properties"`
}

// identify properties
type GatewayIdentifyData struct {
	OS      string `json:"os"`
	Browser string `json:"browser"`
	Device  string `json:"device"`
}

// ready event
type GatewayReady struct {
	SessionID string       `json:"session_id"`
	User      *DiscordUser `json:"user"`
}

// discord user object
type DiscordUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	Bot           bool   `json:"bot"`
}

// guild member object
type GuildMember struct {
	User *DiscordUser `json:"user"`
	Nick string       `json:"nick,omitempty"`
}

// message create event
type MessageCreate struct {
	ID        string       `json:"id"`
	ChannelID string       `json:"channel_id"`
	GuildID   string       `json:"guild_id,omitempty"`
	Author    *DiscordUser `json:"author"`
	Content   string       `json:"content"`
}

// interaction received from gateway
type Interaction struct {
	ID            string           `json:"id"`
	ApplicationID string           `json:"application_id"`
	Type          int              `json:"type"`
	Data          *InteractionData `json:"data,omitempty"`
	GuildID       string           `json:"guild_id,omitempty"`
	ChannelID     string           `json:"channel_id"`
	Member        *GuildMember     `json:"member,omitempty"`
	User          *DiscordUser     `json:"user,omitempty"`
	Token         string           `json:"token"`
	Message       *DiscordMessage  `json:"message,omitempty"`
}

// interaction data with command name, options, or component id
type InteractionData struct {
	ID       string                     `json:"id"`
	Name     string                     `json:"name"`
	Type     int                        `json:"type"`
	Options  []ApplicationCommandOption `json:"options,omitempty"`
	CustomID string                     `json:"custom_id,omitempty"`
}

// resolved option value from a slash command
type ApplicationCommandOption struct {
	Name  string      `json:"name"`
	Type  int         `json:"type"`
	Value interface{} `json:"value,omitempty"`
}

// slash command definition for registration
type ApplicationCommand struct {
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Options     []ApplicationCommandOptionDef `json:"options,omitempty"`
	Type        int                          `json:"type,omitempty"`
}

// option definition for slash command registration
type ApplicationCommandOptionDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        int    `json:"type"`
	Required    bool   `json:"required"`
}

// discord message object
type DiscordMessage struct {
	ID        string  `json:"id"`
	ChannelID string  `json:"channel_id"`
	Content   string  `json:"content"`
	Embeds    []Embed `json:"embeds,omitempty"`
}

// embed for rich message formatting
type Embed struct {
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Footer      *EmbedFooter `json:"footer,omitempty"`
}

// embed field displayed inline or stacked
type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// footer text in an embed
type EmbedFooter struct {
	Text string `json:"text"`
}

// message component (action row or button)
type Component struct {
	Type       int         `json:"type"`
	Components []Component `json:"components,omitempty"`
	Style      int         `json:"style,omitempty"`
	Label      string      `json:"label,omitempty"`
	CustomID   string      `json:"custom_id,omitempty"`
	Disabled   bool        `json:"disabled,omitempty"`
}

// response sent back for an interaction
type InteractionResponse struct {
	Type int                      `json:"type"`
	Data *InteractionCallbackData `json:"data,omitempty"`
}

// data payload for an interaction response
type InteractionCallbackData struct {
	Content    string      `json:"content,omitempty"`
	Embeds     []Embed     `json:"embeds,omitempty"`
	Components []Component `json:"components,omitempty"`
	Flags      int         `json:"flags,omitempty"`
}

// gateway url response
type GatewayURLResponse struct {
	URL string `json:"url"`
}
