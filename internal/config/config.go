// Package config loads settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime settings. See .env.example for documentation.
type Config struct {
	ListenAddr   string
	WebhookToken string
	LogLevel     string
	Workers      int

	AnthropicAPIKey string // empty = AI disabled, alerts forwarded raw
	Model           string
	MaxTokens       int64
	Effort          string
	AITimeout       time.Duration
	AICooldown      time.Duration

	TelegramAPIURL   string
	TelegramToken    string
	TelegramChatID   string
	TelegramThreadID int64

	SlackWebhookURL string
}

// Load reads the environment and validates it.
func Load() (Config, error) {
	c := Config{
		ListenAddr:      env("LISTEN_ADDR", ":8080"),
		WebhookToken:    os.Getenv("WEBHOOK_TOKEN"),
		LogLevel:        env("LOG_LEVEL", "info"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		Model:           env("AI_MODEL", "claude-opus-5-5"),
		Effort:          env("AI_EFFORT", "low"),
		TelegramAPIURL:  env("TELEGRAM_API_URL", "https://api.telegram.org"),
		TelegramToken:   os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:  os.Getenv("TELEGRAM_CHAT_ID"),
		SlackWebhookURL: os.Getenv("SLACK_WEBHOOK_URL"),
	}

	var errs []error
	c.Workers = int(intEnv("WORKERS", 4, &errs))
	c.MaxTokens = intEnv("AI_MAX_TOKENS", 2048, &errs)
	c.TelegramThreadID = intEnv("TELEGRAM_THREAD_ID", 0, &errs)
	c.AITimeout = durEnv("AI_TIMEOUT", 30*time.Second, &errs)
	c.AICooldown = durEnv("AI_COOLDOWN", 30*time.Minute, &errs)

	switch c.Effort {
	case "low", "medium", "high", "xhigh", "max":
	default:
		errs = append(errs, fmt.Errorf("AI_EFFORT must be low|medium|high|xhigh|max, got %q", c.Effort))
	}
	if (c.TelegramToken == "") != (c.TelegramChatID == "") {
		errs = append(errs, errors.New("set both TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID, or neither"))
	}
	if c.TelegramToken == "" && c.SlackWebhookURL == "" {
		errs = append(errs, errors.New("configure at least one notifier: TELEGRAM_BOT_TOKEN+TELEGRAM_CHAT_ID or SLACK_WEBHOOK_URL"))
	}
	return c, errors.Join(errs...)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intEnv(key string, def int64, errs *[]error) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return n
}

func durEnv(key string, def time.Duration, errs *[]error) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return d
}
