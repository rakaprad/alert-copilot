package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TELEGRAM_CHAT_ID", "-100")
	t.Setenv("ANTHROPIC_API_KEY", "")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Model != "claude-opus-5-5" || c.MaxTokens != 2048 || c.AICooldown != 30*time.Minute || c.ListenAddr != ":8080" {
		t.Errorf("unexpected defaults: %+v", c)
	}
}

func TestLoadValidation(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("SLACK_WEBHOOK_URL", "")
	t.Setenv("AI_TIMEOUT", "soon")
	if _, err := Load(); err == nil {
		t.Fatal("expected errors for missing notifier and bad duration")
	}
}
