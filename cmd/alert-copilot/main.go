// Command alert-copilot receives monitoring webhooks, asks Claude for a short
// triage, and posts the enriched alert to Telegram and/or Slack.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rakaprad/alert-copilot/internal/ai"
	"github.com/rakaprad/alert-copilot/internal/config"
	"github.com/rakaprad/alert-copilot/internal/notify"
	"github.com/rakaprad/alert-copilot/internal/server"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		level = slog.LevelInfo
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	var analyzer ai.Analyzer
	if cfg.AnthropicAPIKey != "" {
		claude := ai.NewClaude(ai.ClaudeConfig{
			APIKey:    cfg.AnthropicAPIKey,
			Model:     cfg.Model,
			MaxTokens: cfg.MaxTokens,
			Effort:    cfg.Effort,
			Timeout:   cfg.AITimeout,
		})
		analyzer = ai.NewCooldown(claude, cfg.AICooldown)
		log.Info("ai triage enabled", "model", cfg.Model, "max_tokens", cfg.MaxTokens, "effort", cfg.Effort, "cooldown", cfg.AICooldown.String())
	} else {
		log.Warn("ANTHROPIC_API_KEY not set: alerts will be forwarded without AI triage")
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	var notifiers []notify.Notifier
	if cfg.TelegramToken != "" {
		notifiers = append(notifiers, &notify.Telegram{
			BaseURL: cfg.TelegramAPIURL, Token: cfg.TelegramToken,
			ChatID: cfg.TelegramChatID, ThreadID: cfg.TelegramThreadID, Client: httpClient,
		})
	}
	if cfg.SlackWebhookURL != "" {
		notifiers = append(notifiers, &notify.Slack{WebhookURL: cfg.SlackWebhookURL, Client: httpClient})
	}
	if cfg.WebhookToken == "" {
		log.Warn("WEBHOOK_TOKEN not set: webhooks are unauthenticated")
	}

	srv := server.New(server.Options{
		Analyzer: analyzer, Notifiers: notifiers, Token: cfg.WebhookToken,
		Workers: cfg.Workers, Logger: log,
	})
	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.ListenAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown", "err", err)
	}
	srv.Close() // deliver alerts that were already accepted
	log.Info("bye")
	return nil
}
