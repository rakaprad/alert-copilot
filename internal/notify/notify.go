// Package notify formats alerts (plus optional AI triage) and delivers them
// to chat channels.
package notify

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/rakaprad/alert-copilot/internal/ai"
	"github.com/rakaprad/alert-copilot/internal/alert"
)

// Message is what gets delivered: the alert plus optional AI triage.
type Message struct {
	Alert  alert.Alert
	Triage *ai.Triage // nil when AI is disabled, failed, or the alert is resolved
	Note   string     // e.g. "AI triage unavailable"
}

// Notifier delivers a Message somewhere.
type Notifier interface {
	Name() string
	Notify(ctx context.Context, m Message) error
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func statusLine(a alert.Alert) string {
	if a.Status == alert.StatusResolved {
		return "✅ RESOLVED"
	}
	if a.Severity != "" {
		return fmt.Sprintf("🔴 FIRING · %s", a.Severity)
	}
	return "🔴 FIRING"
}

// stripURL drops the request URL from transport errors. Both the Telegram
// bot token and the Slack webhook URL are secrets embedded in the URL, and
// *url.Error would otherwise print them into our logs.
func stripURL(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return fmt.Errorf("%s request failed: %w", uerr.Op, uerr.Err)
	}
	return err
}
