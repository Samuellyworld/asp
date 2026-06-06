package telegram

import (
	"strings"
	"testing"
	"unicode/utf8"
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

func TestSplitTelegramMessage(t *testing.T) {
	text := strings.Repeat("a", maxTelegramMessageLen+10)
	parts := splitTelegramMessage(text, maxTelegramMessageLen)

	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	for i, part := range parts {
		if len(part) > maxTelegramMessageLen {
			t.Fatalf("part %d length = %d, want <= %d", i, len(part), maxTelegramMessageLen)
		}
	}
	if strings.Join(parts, "") != text {
		t.Fatal("split parts did not preserve message text")
	}
}

func TestSplitTelegramMessagePreservesUTF8(t *testing.T) {
	text := strings.Repeat("✅", maxTelegramMessageLen/3+10)
	parts := splitTelegramMessage(text, maxTelegramMessageLen)

	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	for i, part := range parts {
		if !utf8.ValidString(part) {
			t.Fatalf("part %d is invalid utf-8", i)
		}
		if len(part) > maxTelegramMessageLen {
			t.Fatalf("part %d length = %d, want <= %d", i, len(part), maxTelegramMessageLen)
		}
	}
	if strings.Join(parts, "") != text {
		t.Fatal("split parts did not preserve utf-8 message text")
	}
}

func TestTruncateTelegramMessage(t *testing.T) {
	text := strings.Repeat("a", maxTelegramMessageLen+10)
	got := truncateTelegramMessage(text, maxTelegramMessageLen)

	if len(got) > maxTelegramMessageLen {
		t.Fatalf("length = %d, want <= %d", len(got), maxTelegramMessageLen)
	}
	if !strings.HasSuffix(got, "[truncated]") {
		t.Fatalf("expected truncation marker, got %q", got[len(got)-20:])
	}
}

func TestTruncateTelegramMessagePreservesUTF8(t *testing.T) {
	text := strings.Repeat("🚀", maxTelegramMessageLen/4+10)
	got := truncateTelegramMessage(text, maxTelegramMessageLen)

	if !utf8.ValidString(got) {
		t.Fatal("truncated message is invalid utf-8")
	}
	if len(got) > maxTelegramMessageLen {
		t.Fatalf("length = %d, want <= %d", len(got), maxTelegramMessageLen)
	}
	if !strings.HasSuffix(got, "[truncated]") {
		t.Fatalf("expected truncation marker, got %q", got[len(got)-20:])
	}
}
