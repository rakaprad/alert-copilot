package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
)

// Telegram sends messages through the Bot API using HTML parse mode.
type Telegram struct {
	BaseURL  string // https://api.telegram.org (overridable for tests)
	Token    string
	ChatID   string // numeric id (-100...) or @channelname
	ThreadID int64  // forum topic id; 0 = main chat
	Client   *http.Client
}

func (t *Telegram) Name() string { return "telegram" }

// Notify posts the formatted message to sendMessage.
func (t *Telegram) Notify(ctx context.Context, m Message) error {
	body := map[string]any{
		"chat_id":                  t.ChatID,
		"text":                     FormatTelegram(m),
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	if t.ThreadID != 0 {
		body["message_thread_id"] = t.ThreadID
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := strings.TrimRight(t.BaseURL, "/") + "/bot" + t.Token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.Client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: %w", stripURL(err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("telegram returned %d: %s", resp.StatusCode, msg)
	}
	return nil
}

// FormatTelegram renders a Message as Telegram HTML. Every dynamic value is
// escaped: alert content and AI output are both untrusted text.
func FormatTelegram(m Message) string {
	a := m.Alert
	e := func(s string, n int) string { return html.EscapeString(truncate(s, n)) }

	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b> · %s\n", e(statusLine(a), 60), e(a.Name, 200))
	meta := []string{"source: " + a.Source}
	if a.Target != "" {
		meta = append(meta, "target: "+a.Target)
	}
	if a.Value != "" {
		meta = append(meta, "value: "+a.Value)
	}
	if r := a.Labels["regionName"]; r != "" {
		meta = append(meta, "region: "+r)
	}
	fmt.Fprintf(&b, "<i>%s</i>\n", e(strings.Join(meta, " · "), 300))
	if a.Summary != "" {
		fmt.Fprintf(&b, "%s\n", e(a.Summary, 400))
	}
	if a.Description != "" && a.Description != a.Summary {
		fmt.Fprintf(&b, "%s\n", e(a.Description, 800))
	}

	if t := m.Triage; t != nil {
		noise := "no"
		if t.Noise {
			noise = "likely"
		}
		fmt.Fprintf(&b, "\n<b>🤖 AI triage</b> · severity <b>%s</b> · noise: %s\n", e(t.Severity, 20), noise)
		fmt.Fprintf(&b, "%s\n", e(t.Summary, 400))
		fmt.Fprintf(&b, "<b>Likely cause:</b> %s\n", e(t.LikelyCause, 500))
		if len(t.Checks) > 0 {
			b.WriteString("<b>First checks:</b>\n")
			for i, c := range t.Checks {
				fmt.Fprintf(&b, "%d. <code>%s</code>\n", i+1, e(c, 300))
			}
		}
		if t.Noise && t.NoiseReason != "" {
			fmt.Fprintf(&b, "<i>Noise: %s</i>\n", e(t.NoiseReason, 300))
		}
	} else if m.Note != "" {
		fmt.Fprintf(&b, "\n<i>%s</i>\n", e(m.Note, 200))
	}

	if a.URL != "" {
		fmt.Fprintf(&b, "\n<a href=\"%s\">source</a>", html.EscapeString(a.URL))
	}
	return strings.TrimRight(b.String(), "\n")
}
