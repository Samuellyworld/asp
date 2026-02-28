package telegram

import (
	"testing"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCommand string
		wantArgs    string
	}{
		{
			name:        "simple command",
			input:       "/start",
			wantCommand: "start",
			wantArgs:    "",
		},
		{
			name:        "command with args",
			input:       "/watchadd BTCUSDT",
			wantCommand: "watchadd",
			wantArgs:    "BTCUSDT",
		},
		{
			name:        "command with multiple args",
			input:       "/set confidence 70",
			wantCommand: "set",
			wantArgs:    "confidence 70",
		},
		{
			name:        "command with bot mention",
			input:       "/start@mybot",
			wantCommand: "start",
			wantArgs:    "",
		},
		{
			name:        "command with bot mention and args",
			input:       "/watchadd@mybot ETHUSDT",
			wantCommand: "watchadd",
			wantArgs:    "ETHUSDT",
		},
		{
			name:        "not a command",
			input:       "hello world",
			wantCommand: "",
			wantArgs:    "hello world",
		},
		{
			name:        "empty string",
			input:       "",
			wantCommand: "",
			wantArgs:    "",
		},
		{
			name:        "just a slash",
			input:       "/",
			wantCommand: "",
			wantArgs:    "",
		},
		{
			name:        "command with leading spaces",
			input:       "  /help",
			wantCommand: "help",
			wantArgs:    "",
		},
		{
			name:        "command with trailing spaces in args",
			input:       "/watchadd   BTCUSDT  ",
			wantCommand: "watchadd",
			wantArgs:    "BTCUSDT",
		},
		{
			name:        "help command",
			input:       "/help",
			wantCommand: "help",
			wantArgs:    "",
		},
		{
			name:        "cancel command",
			input:       "/cancel",
			wantCommand: "cancel",
			wantArgs:    "",
		},
		{
			name:        "setup command",
			input:       "/setup",
			wantCommand: "setup",
			wantArgs:    "",
		},
		{
			name:        "status command",
			input:       "/status",
			wantCommand: "status",
			wantArgs:    "",
		},
		{
			name:        "settings command",
			input:       "/settings",
			wantCommand: "settings",
			wantArgs:    "",
		},
		{
			name:        "watchlist alias",
			input:       "/wl",
			wantCommand: "wl",
			wantArgs:    "",
		},
		{
			name:        "watchadd alias",
			input:       "/wa SOLUSDT",
			wantCommand: "wa",
			wantArgs:    "SOLUSDT",
		},
		{
			name:        "watchremove alias",
			input:       "/wr BTCUSDT",
			wantCommand: "wr",
			wantArgs:    "BTCUSDT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCmd, gotArgs := ParseCommand(tt.input)
			if gotCmd != tt.wantCommand {
				t.Errorf("ParseCommand(%q) command = %q, want %q", tt.input, gotCmd, tt.wantCommand)
			}
			if gotArgs != tt.wantArgs {
				t.Errorf("ParseCommand(%q) args = %q, want %q", tt.input, gotArgs, tt.wantArgs)
			}
		})
	}
}
