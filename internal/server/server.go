// Package server wires HTTP webhooks to the analyze-and-notify pipeline.
package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rakaprad/alert-copilot/internal/ai"
	"github.com/rakaprad/alert-copilot/internal/alert"
	"github.com/rakaprad/alert-copilot/internal/notify"
	"github.com/rakaprad/alert-copilot/internal/source"
)

const maxBodyBytes = 1 << 20 // 1 MiB is plenty for any alert webhook

// Server receives webhooks, enriches alerts and forwards them.
//
// Handlers only parse and enqueue, then return 202, so slow AI calls never
// make the monitoring system time out and retry. A fixed pool of workers
// drains the queue.
type Server struct {
	analyzer  ai.Analyzer // nil = AI disabled
	notifiers []notify.Notifier
	token     string
	log       *slog.Logger

	queue chan alert.Alert
	wg    sync.WaitGroup
}

// Options configures a Server.
type Options struct {
	Analyzer  ai.Analyzer
	Notifiers []notify.Notifier
	Token     string // optional shared secret for webhooks
	Workers   int
	QueueSize int
	Logger    *slog.Logger
}

// New creates a Server and starts its workers.
func New(o Options) *Server {
	if o.Workers <= 0 {
		o.Workers = 4
	}
	if o.QueueSize <= 0 {
		o.QueueSize = 100
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	s := &Server{
		analyzer:  o.Analyzer,
		notifiers: o.Notifiers,
		token:     o.Token,
		log:       o.Logger,
		queue:     make(chan alert.Alert, o.QueueSize),
	}
	for range o.Workers {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for a := range s.queue {
				s.Process(context.Background(), a)
			}
		}()
	}
	return s
}

// Close stops accepting work and waits for queued alerts to be delivered.
// Call it after http.Server.Shutdown so no handler can enqueue anymore.
func (s *Server) Close() { close(s.queue); s.wg.Wait() }

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.Handle("POST /webhook/alertmanager", s.webhook(func(r *http.Request) ([]alert.Alert, error) {
		return source.ParseAlertmanager(r.Body)
	}))
	mux.Handle("POST /webhook/alicloud", s.webhook(func(r *http.Request) ([]alert.Alert, error) {
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		return source.ParseAliCloud(r.PostForm)
	}))
	mux.Handle("POST /webhook/generic", s.webhook(func(r *http.Request) ([]alert.Alert, error) {
		return source.ParseGeneric(r.Body)
	}))
	return mux
}

func (s *Server) webhook(parse func(*http.Request) ([]alert.Alert, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		alerts, err := parse(r)
		if err != nil {
			s.log.Warn("rejected webhook", "path", r.URL.Path, "err", err)
			http.Error(w, "bad payload: "+err.Error(), http.StatusBadRequest)
			return
		}
		for _, a := range alerts {
			select {
			case s.queue <- a:
			default:
				// Queue full: ask the sender to retry rather than drop silently.
				s.log.Error("queue full, rejecting alert", "alert", a.Name)
				http.Error(w, "busy", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "accepted\n")
	})
}

// authorized checks the optional shared secret. It accepts
// "Authorization: Bearer <token>" (Alertmanager http_config), an
// "X-Webhook-Token" header, or a "?token=" query parameter (for senders like
// CloudMonitor that can only be configured with a URL).
func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	got := r.Header.Get("X-Webhook-Token")
	if v, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		got = v
	}
	if got == "" {
		got = r.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

// Process enriches one alert and sends it to every notifier. AI is strictly
// best-effort: on any failure the raw alert is still delivered.
func (s *Server) Process(ctx context.Context, a alert.Alert) {
	log := s.log.With("source", a.Source, "alert", a.Name, "status", a.Status, "fingerprint", a.Fingerprint())
	msg := notify.Message{Alert: a}

	switch {
	case a.Status == alert.StatusResolved:
		// Nothing to triage on recovery; keep it cheap and fast.
	case s.analyzer == nil:
		msg.Note = "AI triage disabled"
	default:
		start := time.Now()
		t, err := s.analyzer.Analyze(ctx, a)
		if err != nil {
			log.Warn("ai triage failed, forwarding raw alert", "err", err, "elapsed", time.Since(start).Round(time.Millisecond).String())
			msg.Note = "AI triage unavailable"
			if errors.Is(err, context.DeadlineExceeded) {
				msg.Note = "AI triage timed out"
			}
		} else {
			log.Info("ai triage done", "severity", t.Severity, "noise", t.Noise, "elapsed", time.Since(start).Round(time.Millisecond).String())
			msg.Triage = t
		}
	}

	for _, n := range s.notifiers {
		nctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := n.Notify(nctx, msg); err != nil {
			log.Error("notify failed", "notifier", n.Name(), "err", err)
		} else {
			log.Info("notified", "notifier", n.Name(), "ai", msg.Triage != nil)
		}
		cancel()
	}
}
