// Package ai turns an alert into a short, structured triage using Claude.
// Everything else in the service depends only on the Analyzer interface, so
// tests use fakes and the service works fine with no analyzer at all.
package ai

import (
	"context"

	"github.com/rakaprad/alert-copilot/internal/alert"
)

// Triage is the structured result we ask Claude for.
type Triage struct {
	Summary     string   `json:"summary"`      // one sentence, plain language
	LikelyCause string   `json:"likely_cause"` // best hypothesis
	Severity    string   `json:"severity"`     // critical | high | medium | low | info
	Checks      []string `json:"checks"`       // first checks / commands to run (max 3)
	Noise       bool     `json:"noise"`        // likely flapping / non-actionable?
	NoiseReason string   `json:"noise_reason"` // why it might be noise ("" if not)
}

// Analyzer produces a Triage for a firing alert.
type Analyzer interface {
	Analyze(ctx context.Context, a alert.Alert) (*Triage, error)
}
