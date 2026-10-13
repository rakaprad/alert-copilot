// Package alert defines the source-agnostic Alert that every webhook is
// normalized into before analysis and notification.
package alert

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

// Status is the lifecycle state of an alert.
type Status string

const (
	StatusFiring   Status = "firing"
	StatusResolved Status = "resolved"
)

// Alert is the common shape all sources are normalized into.
type Alert struct {
	Source      string            `json:"source"`                // alertmanager, alicloud, generic
	Name        string            `json:"name"`                  // e.g. HighCPUUsage
	Status      Status            `json:"status"`                // firing or resolved
	Severity    string            `json:"severity,omitempty"`    // as reported by the source
	Target      string            `json:"target,omitempty"`      // instance, pod, host, resource...
	Summary     string            `json:"summary,omitempty"`     // one-line human summary
	Description string            `json:"description,omitempty"` // longer text, if any
	Value       string            `json:"value,omitempty"`       // current metric value
	Labels      map[string]string `json:"labels,omitempty"`      // extra key/value context
	StartsAt    time.Time         `json:"starts_at,omitempty"`
	URL         string            `json:"url,omitempty"` // link back to the source (generator/console)
	ID          string            `json:"-"`             // source-provided fingerprint, if any
}

// Fingerprint returns a stable key identifying "the same alert" across
// repeated notifications. It is used for the AI cooldown cache.
func (a Alert) Fingerprint() string {
	if a.ID != "" {
		return a.Source + ":" + a.ID
	}
	keys := make([]string, 0, len(a.Labels))
	for k := range a.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(a.Source + "\x00" + a.Name + "\x00" + a.Target)
	for _, k := range keys {
		b.WriteString("\x00" + k + "=" + a.Labels[k])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return a.Source + ":" + hex.EncodeToString(sum[:8])
}
