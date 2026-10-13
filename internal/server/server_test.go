package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/rakaprad/alert-copilot/internal/ai"
	"github.com/rakaprad/alert-copilot/internal/alert"
	"github.com/rakaprad/alert-copilot/internal/notify"
)

type fakeAnalyzer struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeAnalyzer) Analyze(context.Context, alert.Alert) (*ai.Triage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &ai.Triage{Summary: "triaged", Severity: "high"}, nil
}

type fakeNotifier struct {
	mu   sync.Mutex
	msgs []notify.Message
}

func (f *fakeNotifier) Name() string { return "fake" }
func (f *fakeNotifier) Notify(_ context.Context, m notify.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, m)
	return nil
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func firing() alert.Alert {
	return alert.Alert{Source: "generic", Name: "DiskFull", Status: alert.StatusFiring}
}

func TestProcessFallsBackWhenAnalyzerFails(t *testing.T) {
	n := &fakeNotifier{}
	s := New(Options{Analyzer: &fakeAnalyzer{err: errors.New("api down")}, Notifiers: []notify.Notifier{n}, Logger: quiet})
	defer s.Close()

	s.Process(context.Background(), firing())
	if len(n.msgs) != 1 {
		t.Fatalf("raw alert must still be delivered, got %d messages", len(n.msgs))
	}
	if n.msgs[0].Triage != nil || n.msgs[0].Note != "AI triage unavailable" {
		t.Errorf("unexpected message: %+v", n.msgs[0])
	}
}

func TestProcessTimeoutNote(t *testing.T) {
	n := &fakeNotifier{}
	s := New(Options{Analyzer: &fakeAnalyzer{err: context.DeadlineExceeded}, Notifiers: []notify.Notifier{n}, Logger: quiet})
	defer s.Close()
	s.Process(context.Background(), firing())
	if n.msgs[0].Note != "AI triage timed out" {
		t.Errorf("note = %q", n.msgs[0].Note)
	}
}

func TestProcessWithoutAnalyzer(t *testing.T) {
	n := &fakeNotifier{}
	s := New(Options{Notifiers: []notify.Notifier{n}, Logger: quiet})
	defer s.Close()
	s.Process(context.Background(), firing())
	if len(n.msgs) != 1 || n.msgs[0].Note != "AI triage disabled" {
		t.Fatalf("unexpected: %+v", n.msgs)
	}
}

func TestProcessSkipsAIForResolved(t *testing.T) {
	an, n := &fakeAnalyzer{}, &fakeNotifier{}
	s := New(Options{Analyzer: an, Notifiers: []notify.Notifier{n}, Logger: quiet})
	defer s.Close()

	a := firing()
	a.Status = alert.StatusResolved
	s.Process(context.Background(), a)
	if an.calls != 0 {
		t.Errorf("resolved alerts must not call the analyzer")
	}
	if len(n.msgs) != 1 || n.msgs[0].Note != "" {
		t.Errorf("unexpected: %+v", n.msgs)
	}
}

func TestProcessAttachesTriage(t *testing.T) {
	n := &fakeNotifier{}
	s := New(Options{Analyzer: &fakeAnalyzer{}, Notifiers: []notify.Notifier{n}, Logger: quiet})
	defer s.Close()
	s.Process(context.Background(), firing())
	if n.msgs[0].Triage == nil || n.msgs[0].Triage.Summary != "triaged" {
		t.Errorf("triage missing: %+v", n.msgs[0])
	}
}

func TestTokenAuth(t *testing.T) {
	n := &fakeNotifier{}
	s := New(Options{Notifiers: []notify.Notifier{n}, Token: "s3cret", Logger: quiet})
	h := s.Handler()
	body := `{"name":"X"}`

	cases := []struct {
		name   string
		mutate func(*http.Request)
		want   int
	}{
		{"no token", func(*http.Request) {}, http.StatusUnauthorized},
		{"wrong token", func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }, http.StatusUnauthorized},
		{"bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer s3cret") }, http.StatusAccepted},
		{"header", func(r *http.Request) { r.Header.Set("X-Webhook-Token", "s3cret") }, http.StatusAccepted},
		{"query", func(r *http.Request) { r.URL.RawQuery = "token=s3cret" }, http.StatusAccepted},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhook/generic", strings.NewReader(body))
			c.mutate(req)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("got %d, want %d", rec.Code, c.want)
			}
		})
	}
	s.Close()
	if len(n.msgs) != 3 {
		t.Errorf("want 3 delivered alerts, got %d", len(n.msgs))
	}
}

func TestHandlersEndToEnd(t *testing.T) {
	n := &fakeNotifier{}
	s := New(Options{Notifiers: []notify.Notifier{n}, Logger: quiet})
	h := s.Handler()

	post := func(path, ctype, file string) int {
		raw, err := os.ReadFile("../../testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(strings.TrimSpace(string(raw))))
		req.Header.Set("Content-Type", ctype)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := post("/webhook/alertmanager", "application/json", "alertmanager.json"); c != http.StatusAccepted {
		t.Errorf("alertmanager: %d", c)
	}
	if c := post("/webhook/alicloud", "application/x-www-form-urlencoded", "alicloud.form"); c != http.StatusAccepted {
		t.Errorf("alicloud: %d", c)
	}
	if c := post("/webhook/generic", "application/json", "generic.json"); c != http.StatusAccepted {
		t.Errorf("generic: %d", c)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/generic", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad payload: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("healthz: %d", rec.Code)
	}

	s.Close()             // drains the queue
	if len(n.msgs) != 4 { // 2 alertmanager + 1 alicloud + 1 generic
		t.Errorf("want 4 delivered alerts, got %d", len(n.msgs))
	}
}
