package ai

import (
	"context"
	"sync"
	"time"

	"github.com/rakaprad/alert-copilot/internal/alert"
)

// Cooldown wraps an Analyzer and reuses a previous triage for the same alert
// (by fingerprint) for a TTL. Alertmanager and CloudMonitor re-send firing
// alerts periodically; without this every repeat would cost an API call.
// Failed analyses are not cached, so the next repeat tries again.
type Cooldown struct {
	inner Analyzer
	ttl   time.Duration
	now   func() time.Time

	mu    sync.Mutex
	cache map[string]cooldownEntry
}

type cooldownEntry struct {
	triage *Triage
	at     time.Time
}

// NewCooldown returns an Analyzer that calls inner at most once per alert
// fingerprint per ttl.
func NewCooldown(inner Analyzer, ttl time.Duration) *Cooldown {
	return &Cooldown{inner: inner, ttl: ttl, now: time.Now, cache: map[string]cooldownEntry{}}
}

// Analyze returns the cached triage if it is fresh, otherwise calls inner.
func (c *Cooldown) Analyze(ctx context.Context, a alert.Alert) (*Triage, error) {
	key := a.Fingerprint()
	if t, ok := c.Cached(key); ok {
		return t, nil
	}
	t, err := c.inner.Analyze(ctx, a)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for k, e := range c.cache { // opportunistic cleanup keeps memory bounded
		if now.Sub(e.at) >= c.ttl {
			delete(c.cache, k)
		}
	}
	c.cache[key] = cooldownEntry{triage: t, at: now}
	return t, nil
}

// Cached reports whether a fresh triage exists for the fingerprint.
func (c *Cooldown) Cached(key string) (*Triage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || c.now().Sub(e.at) >= c.ttl {
		return nil, false
	}
	return e.triage, true
}
