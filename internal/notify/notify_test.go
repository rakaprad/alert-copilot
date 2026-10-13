package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rakaprad/alert-copilot/internal/ai"
	"github.com/rakaprad/alert-copilot/internal/alert"
)

func sample() Message {
	return Message{
		Alert: alert.Alert{
			Source: "alertmanager", Name: "HighCPU <script>", Status: alert.StatusFiring,
			Severity: "warning", Target: "web-01", Value: "94%",
			Summary: `CPU > 90% & "rising"`, URL: "http://prom/graph?a=1&b=2",
		},
		Triage: &ai.Triage{
			Summary: "CPU saturated", LikelyCause: "traffic spike <b>", Severity: "high",
			Checks: []string{"kubectl top pod -n payments | sort -k2 > /tmp/x"},
		},
	}
}

func TestFormatTelegramEscapesHTML(t *testing.T) {
	out := FormatTelegram(sample())
	for _, bad := range []string{"<script>", "traffic spike <b>", "& \"rising\"", "| sort -k2 > /tmp"} {
		if strings.Contains(out, bad) {
			t.Errorf("unescaped %q in output:\n%s", bad, out)
		}
	}
	for _, want := range []string{"HighCPU &lt;script&gt;", "CPU &gt; 90% &amp; &#34;rising&#34;", "<code>kubectl top pod -n payments | sort -k2 &gt; /tmp/x</code>", "severity <b>high</b>", `href="http://prom/graph?a=1&amp;b=2"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestFormatTelegramFallbackNote(t *testing.T) {
	m := sample()
	m.Triage = nil
	m.Note = "AI triage unavailable"
	out := FormatTelegram(m)
	if strings.Contains(out, "AI triage</b>") || !strings.Contains(out, "<i>AI triage unavailable</i>") {
		t.Errorf("unexpected fallback output:\n%s", out)
	}
}

func TestFormatTelegramTruncatesLongFields(t *testing.T) {
	m := sample()
	m.Alert.Description = strings.Repeat("x", 10000)
	if out := FormatTelegram(m); len([]rune(out)) > 4096 {
		t.Errorf("message exceeds Telegram limit: %d", len([]rune(out)))
	}
}

func TestFormatSlackEscapesMentions(t *testing.T) {
	m := sample()
	m.Alert.Summary = "<!channel> wake up"
	out := FormatSlack(m)
	if strings.Contains(out, "<!channel>") || !strings.Contains(out, "&lt;!channel&gt;") {
		t.Errorf("slack mention not escaped:\n%s", out)
	}
}

func TestTelegramNotifySendsThreadAndParseMode(t *testing.T) {
	var got map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	tg := &Telegram{BaseURL: srv.URL, Token: "123:abc", ChatID: "-1001", ThreadID: 42, Client: srv.Client()}
	if err := tg.Notify(context.Background(), sample()); err != nil {
		t.Fatal(err)
	}
	if path != "/bot123:abc/sendMessage" {
		t.Errorf("path = %s", path)
	}
	if got["parse_mode"] != "HTML" || got["chat_id"] != "-1001" || got["message_thread_id"] != float64(42) {
		t.Errorf("unexpected body: %v", got)
	}
}

func TestTelegramErrorDoesNotLeakToken(t *testing.T) {
	tg := &Telegram{BaseURL: "http://127.0.0.1:1", Token: "SECRET-TOKEN", ChatID: "1", Client: http.DefaultClient}
	err := tg.Notify(context.Background(), sample())
	if err == nil || strings.Contains(err.Error(), "SECRET-TOKEN") {
		t.Fatalf("expected error without token, got %v", err)
	}
}
