// Package source parses webhook payloads from monitoring systems into
// []alert.Alert. Each parser is a pure function so it is easy to unit test.
package source

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/rakaprad/alert-copilot/internal/alert"
)

// alertmanagerPayload is the subset of the Alertmanager webhook (version 4)
// that we use. See https://prometheus.io/docs/alerting/latest/configuration/#webhook_config
type alertmanagerPayload struct {
	Version     string `json:"version"`
	Status      string `json:"status"`
	ExternalURL string `json:"externalURL"`
	Alerts      []struct {
		Status       string            `json:"status"`
		Labels       map[string]string `json:"labels"`
		Annotations  map[string]string `json:"annotations"`
		StartsAt     time.Time         `json:"startsAt"`
		GeneratorURL string            `json:"generatorURL"`
		Fingerprint  string            `json:"fingerprint"`
	} `json:"alerts"`
}

// ParseAlertmanager parses a Prometheus Alertmanager webhook body. A single
// webhook can carry several grouped alerts; each becomes one alert.Alert.
func ParseAlertmanager(r io.Reader) ([]alert.Alert, error) {
	var p alertmanagerPayload
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return nil, fmt.Errorf("decode alertmanager payload: %w", err)
	}
	if len(p.Alerts) == 0 {
		return nil, errors.New("alertmanager payload has no alerts")
	}

	out := make([]alert.Alert, 0, len(p.Alerts))
	for _, a := range p.Alerts {
		labels := make(map[string]string, len(a.Labels))
		for k, v := range a.Labels {
			switch k {
			case "alertname", "severity", "instance":
				// promoted to dedicated fields below
			default:
				labels[k] = v
			}
		}
		status := alert.StatusFiring
		if a.Status == "resolved" {
			status = alert.StatusResolved
		}
		target := a.Labels["instance"]
		if target == "" {
			target = firstNonEmpty(a.Labels["pod"], a.Labels["service"], a.Labels["job"])
		}
		out = append(out, alert.Alert{
			Source:      "alertmanager",
			Name:        firstNonEmpty(a.Labels["alertname"], "unnamed"),
			Status:      status,
			Severity:    a.Labels["severity"],
			Target:      target,
			Summary:     a.Annotations["summary"],
			Description: firstNonEmpty(a.Annotations["description"], a.Annotations["message"]),
			Value:       a.Annotations["value"],
			Labels:      labels,
			StartsAt:    a.StartsAt,
			URL:         a.GeneratorURL,
			ID:          a.Fingerprint,
		})
	}
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
