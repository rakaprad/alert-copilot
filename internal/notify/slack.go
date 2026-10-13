package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Slack posts to an incoming webhook using mrkdwn text.
type Slack struct {
	WebhookURL string
	Client     *http.Client
}

func (s *Slack) Name() string { return "slack" }

func (s *Slack) Notify(ctx context.Context, m Message) error {
	payload, err := json.Marshal(map[string]string{"text": FormatSlack(m)})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.Client.Do(req)
	if err != nil {
		return fmt.Errorf("slack: %w", stripURL(err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned %d", resp.StatusCode)
	}
	return nil
}

// slackEscape escapes the three characters Slack mrkdwn treats as control
// characters, so alert text cannot inject mentions like <!channel>.
func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// FormatSlack renders a Message as Slack mrkdwn.
func FormatSlack(m Message) string {
	a := m.Alert
	e := func(s string, n int) string { return slackEscape(truncate(s, n)) }
	var b strings.Builder
	fmt.Fprintf(&b, "*%s* · %s\n", e(statusLine(a), 60), e(a.Name, 200))
	fmt.Fprintf(&b, "_source: %s", e(a.Source, 50))
	if a.Target != "" {
		fmt.Fprintf(&b, " · target: %s", e(a.Target, 200))
	}
	if a.Value != "" {
		fmt.Fprintf(&b, " · value: %s", e(a.Value, 50))
	}
	b.WriteString("_\n")
	if a.Summary != "" {
		fmt.Fprintf(&b, "%s\n", e(a.Summary, 400))
	}
	if t := m.Triage; t != nil {
		fmt.Fprintf(&b, "\n*AI triage* · severity *%s* · noise: %t\n%s\n*Likely cause:* %s\n",
			e(t.Severity, 20), t.Noise, e(t.Summary, 400), e(t.LikelyCause, 500))
		for i, c := range t.Checks {
			fmt.Fprintf(&b, "%d. `%s`\n", i+1, e(strings.ReplaceAll(c, "`", "'"), 300))
		}
	} else if m.Note != "" {
		fmt.Fprintf(&b, "\n_%s_\n", e(m.Note, 200))
	}
	return strings.TrimRight(b.String(), "\n")
}
